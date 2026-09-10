package dlna

import (
	"context"
	"fmt"
	"golang.org/x/net/ipv4"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Discovery struct {
	mu      sync.RWMutex
	targets map[string]string
	conn    *net.UDPConn
	packet  *ipv4.PacketConn
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	slots   chan struct{}
	host    string
	port    int
}

func NewDiscovery(host string, port int) *Discovery {
	return &Discovery{targets: map[string]string{}, slots: make(chan struct{}, 64), host: host, port: port}
}
func (d *Discovery) Set(ids []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.targets = map[string]string{}
	for _, id := range ids {
		d.targets[id] = fmt.Sprintf("http://%s:%d/dlna/%s/description.xml", d.host, d.port, id)
	}
}
func (d *Discovery) Start(parent context.Context) error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	var selected *net.Interface
	for _, iface := range interfaces {
		addresses, _ := iface.Addrs()
		for _, addr := range addresses {
			ip, _, _ := net.ParseCIDR(addr.String())
			if ip != nil && ip.String() == d.host {
				copy := iface
				selected = &copy
				break
			}
		}
	}
	if selected == nil {
		return fmt.Errorf("SSDP interface for %s unavailable", d.host)
	}
	conn, err := net.ListenMulticastUDP("udp4", selected, &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 1900})
	if err != nil {
		return err
	}
	pc := ipv4.NewPacketConn(conn)
	_ = pc.SetMulticastInterface(selected)
	_ = conn.SetReadBuffer(1 << 20)
	_ = pc.SetMulticastTTL(2)
	d.conn = conn
	d.packet = pc
	ctx, cancel := context.WithCancel(parent)
	d.cancel = cancel
	d.wg.Add(2)
	go d.read(ctx)
	go d.announceLoop(ctx)
	return nil
}
func types(id string) []string {
	return []string{"upnp:rootdevice", "uuid:" + id, "urn:schemas-upnp-org:device:MediaRenderer:1", "urn:schemas-upnp-org:service:AVTransport:1", "urn:schemas-upnp-org:service:RenderingControl:1", "urn:schemas-upnp-org:service:ConnectionManager:1"}
}
func usn(id, kind string) string {
	if kind == "uuid:"+id {
		return kind
	}
	return "uuid:" + id + "::" + kind
}
func (d *Discovery) packets(kind string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []string
	for id, location := range d.targets {
		for _, t := range types(id) {
			out = append(out, fmt.Sprintf("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nCACHE-CONTROL: max-age=1800\r\nLOCATION: %s\r\nNT: %s\r\nNTS: ssdp:%s\r\nSERVER: Linux/3 UPnP/1.0 MiAirPlus/2.0\r\nUSN: %s\r\n\r\n", location, t, kind, usn(id, t)))
		}
	}
	return out
}
func (d *Discovery) Announce() {
	if d.conn == nil {
		return
	}
	for _, p := range d.packets("alive") {
		_, _ = d.conn.WriteToUDP([]byte(p), &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 1900})
	}
}
func (d *Discovery) announceLoop(ctx context.Context) {
	defer d.wg.Done()
	d.Announce()
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.Announce()
		}
	}
}
func ParseSearch(raw string) (string, int, bool) {
	if len(raw) > 4096 || !strings.HasPrefix(raw, "M-SEARCH * HTTP/1.1\r\n") {
		return "", 0, false
	}
	headers := map[string]string{}
	for _, line := range strings.Split(raw, "\r\n")[1:] {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			headers[strings.ToUpper(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	if !strings.EqualFold(strings.Trim(headers["MAN"], "\""), "ssdp:discover") || headers["ST"] == "" {
		return "", 0, false
	}
	mx, _ := strconv.Atoi(headers["MX"])
	if mx < 0 {
		mx = 0
	}
	if mx > 3 {
		mx = 3
	}
	return headers["ST"], mx, true
}
func (d *Discovery) read(ctx context.Context) {
	defer d.wg.Done()
	buffer := make([]byte, 4097)
	for {
		n, addr, err := d.conn.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		st, mx, ok := ParseSearch(string(buffer[:n]))
		if !ok || addr.IP.IsMulticast() {
			continue
		}
		select {
		case d.slots <- struct{}{}:
		default:
			continue
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			defer func() { <-d.slots }()
			timer := time.NewTimer(time.Duration(rand.Intn(mx*1000+1)) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			d.mu.RLock()
			defer d.mu.RUnlock()
			for id, location := range d.targets {
				for _, kind := range types(id) {
					if st != "ssdp:all" && st != kind {
						continue
					}
					p := fmt.Sprintf("HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nEXT:\r\nLOCATION: %s\r\nSERVER: Linux/3 UPnP/1.0 MiAirPlus/2.0\r\nST: %s\r\nUSN: %s\r\n\r\n", location, kind, usn(id, kind))
					_, _ = d.conn.WriteToUDP([]byte(p), addr)
				}
			}
		}()
	}
}
func (d *Discovery) Stop() {
	if d.cancel == nil {
		return
	}
	d.cancel()
	for _, p := range d.packets("byebye") {
		_, _ = d.conn.WriteToUDP([]byte(p), &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 1900})
	}
	_ = d.conn.Close()
	d.wg.Wait()
}
