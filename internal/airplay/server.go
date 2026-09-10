package airplay

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/media"
	"github.com/grandcat/zeroconf"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Target interface {
	StartAirPlay(context.Context, string, string) error
	EndAirPlay(context.Context)
	SetVolume(context.Context, int) error
	Metadata(string, string)
}
type Server struct {
	mu                 sync.Mutex
	host, name, ffmpeg string
	mac                []byte
	listener           net.Listener
	service            *zeroconf.Server
	ctx                context.Context
	cancel             context.CancelFunc
	target             Target
	publish            func(*media.Live) (string, func())
	active             *session
	wg                 sync.WaitGroup
	announce           bool
}

func Start(ctx context.Context, host, name, id, ffmpeg string, target Target, publish func(*media.Live) (string, func()), announce bool) (*Server, error) {
	if ffmpeg == "" {
		return nil, errors.New("FFmpeg is unavailable")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(host, "0"))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	hash := sha256.Sum256([]byte(id))
	mac := append([]byte(nil), hash[:6]...)
	mac[0] = (mac[0] | 2) & 0xfe
	s := &Server{host: host, name: name, ffmpeg: ffmpeg, mac: mac, listener: listener, ctx: ctx, cancel: cancel, target: target, publish: publish, announce: announce}
	if announce {
		if err = s.advertise(name); err != nil {
			listener.Close()
			cancel()
			return nil, err
		}
	}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}
func (s *Server) advertise(name string) error {
	var ifaces []net.Interface
	all, _ := net.Interfaces()
	for _, iface := range all {
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			if ip != nil && ip.String() == s.host {
				ifaces = append(ifaces, iface)
				break
			}
		}
	}
	if len(ifaces) == 0 {
		return errors.New("AirPlay LAN interface unavailable")
	}
	service, err := zeroconf.RegisterProxy(strings.ToUpper(hex.EncodeToString(s.mac))+"@"+name, "_raop._tcp", "local.", s.listener.Addr().(*net.TCPAddr).Port, "miair-"+hex.EncodeToString(s.mac)+".local.", []string{s.host}, []string{"txtvers=1", "ch=2", "cn=0,1", "et=0,1", "md=0,1,2", "pw=false", "sr=44100", "ss=16", "tp=UDP,TCP", "vn=3", "vs=105.1", "am=MiAirPlus"}, ifaces)
	if err != nil {
		return err
	}
	s.service = service
	return nil
}
func (s *Server) Rename(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == s.name {
		return nil
	}
	if s.service != nil {
		s.service.Shutdown()
		s.service = nil
	}
	if s.announce {
		if err := s.advertise(name); err != nil {
			return err
		}
	}
	s.name = name
	return nil
}
func (s *Server) Port() int { return s.listener.Addr().(*net.TCPAddr).Port }
func (s *Server) Close() {
	s.cancel()
	s.listener.Close()
	s.mu.Lock()
	if s.active != nil {
		s.active.cancel()
		s.active.conn.Close()
	}
	if s.service != nil {
		s.service.Shutdown()
		s.service = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
}
func (s *Server) accept() {
	defer s.wg.Done()
	slots := make(chan struct{}, 4)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		select {
		case slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { <-slots }()
			ctx, cancel := context.WithCancel(s.ctx)
			ss := &session{server: s, conn: conn, ctx: ctx, cancel: cancel, packets: make(chan []byte, 512), flush: make(chan struct{}, 1)}
			ss.run()
		}()
	}
}

type session struct {
	server                *Server
	conn                  net.Conn
	ctx                   context.Context
	cancel                context.CancelFunc
	sdp                   SDP
	announced, recorded   bool
	data, control, timing *net.UDPConn
	remote                *net.UDPAddr
	decoder               *Decoder
	live                  *media.Live
	unpublish             func()
	stream                string
	packets               chan []byte
	flush                 chan struct{}
	wg                    sync.WaitGroup
}

func (ss *session) run() {
	defer func() {
		ss.cancel()
		ss.conn.Close()
		for _, c := range []*net.UDPConn{ss.data, ss.control, ss.timing} {
			if c != nil {
				c.Close()
			}
		}
		if ss.decoder != nil {
			ss.decoder.Close()
		}
		ss.wg.Wait()
		if ss.live != nil {
			ss.live.Close()
		}
		if ss.unpublish != nil {
			ss.unpublish()
		}
		ss.server.mu.Lock()
		active := ss.server.active == ss
		if active {
			ss.server.active = nil
		}
		ss.server.mu.Unlock()
		if active && ss.recorded {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ss.server.target.EndAirPlay(ctx)
		}
	}()
	go func() { <-ss.ctx.Done(); ss.conn.Close() }()
	reader := bufio.NewReaderSize(ss.conn, 65536)
	for {
		ss.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		first, err := reader.Peek(1)
		if err != nil {
			return
		}
		if first[0] == '$' {
			header := make([]byte, 4)
			if _, err = io.ReadFull(reader, header); err != nil {
				return
			}
			n := int(binary.BigEndian.Uint16(header[2:]))
			if n > 16384 {
				return
			}
			body := make([]byte, n)
			if _, err = io.ReadFull(reader, body); err != nil {
				return
			}
			if header[1] == 0 {
				ss.enqueue(body)
			}
			continue
		}
		line, err := reader.ReadString('\n')
		if err != nil || len(line) > 4096 {
			return
		}
		parts := strings.Fields(line)
		if len(parts) != 3 || parts[2] != "RTSP/1.0" {
			return
		}
		headers := textproto.MIMEHeader{}
		total := 0
		for {
			line, err = reader.ReadString('\n')
			total += len(line)
			if err != nil || total > 32768 {
				return
			}
			if strings.TrimSpace(line) == "" {
				break
			}
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				return
			}
			headers.Add(k, strings.TrimSpace(v))
		}
		length := 0
		if v := headers.Get("Content-Length"); v != "" {
			length, err = strconv.Atoi(v)
			if err != nil || length < 0 || length > 1<<20 {
				return
			}
		}
		body := make([]byte, length)
		if _, err = io.ReadFull(reader, body); err != nil {
			return
		}
		code, response, payload := ss.handle(parts[0], headers, body)
		response["CSeq"] = headers.Get("CSeq")
		response["Audio-Jack-Status"] = "connected; type=analog"
		if v := headers.Get("Apple-Challenge"); v != "" {
			ip, _, _ := net.SplitHostPort(ss.conn.LocalAddr().String())
			if answer, err := challenge(v, net.ParseIP(ip), ss.server.mac); err == nil {
				response["Apple-Response"] = answer
			} else {
				code = 400
			}
		}
		response["Content-Length"] = strconv.Itoa(len(payload))
		var out strings.Builder
		fmt.Fprintf(&out, "RTSP/1.0 %d %s\r\n", code, map[int]string{200: "OK", 400: "Bad Request", 455: "Method Not Valid in This State", 461: "Unsupported Transport", 500: "Internal Server Error", 501: "Not Implemented", 503: "Service Unavailable"}[code])
		for k, v := range response {
			if !strings.ContainsAny(v, "\r\n") {
				fmt.Fprintf(&out, "%s: %s\r\n", k, v)
			}
		}
		out.WriteString("\r\n")
		out.Write(payload)
		ss.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err = io.WriteString(ss.conn, out.String()); err != nil {
			return
		}
		if parts[0] == "TEARDOWN" {
			return
		}
	}
}
func (ss *session) handle(method string, h textproto.MIMEHeader, body []byte) (int, map[string]string, []byte) {
	result := map[string]string{}
	switch method {
	case "OPTIONS":
		result["Public"] = "OPTIONS, ANNOUNCE, SETUP, RECORD, FLUSH, TEARDOWN, GET_PARAMETER, SET_PARAMETER"
	case "ANNOUNCE":
		if ss.recorded {
			return 455, result, nil
		}
		sdp, err := ParseSDP(string(body))
		if err != nil {
			return 400, result, []byte(err.Error())
		}
		ss.sdp = sdp
		ss.announced = true
	case "SETUP":
		if !ss.announced || ss.live != nil {
			return 455, result, nil
		}
		transport := h.Get("Transport")
		if !strings.HasPrefix(transport, "RTP/AVP/UDP") && !strings.HasPrefix(transport, "RTP/AVP/TCP") {
			return 461, result, nil
		}
		ss.server.mu.Lock()
		if ss.server.active != nil && ss.server.active != ss {
			ss.server.mu.Unlock()
			return 503, result, nil
		}
		ss.server.active = ss
		ss.server.mu.Unlock()
		ss.live = media.NewLive(2<<20, "audio/mpeg")
		ss.stream, ss.unpublish = ss.server.publish(ss.live)
		decoder, err := NewDecoder(ss.ctx, ss.server.ffmpeg, ss.sdp, ss.live)
		if err != nil {
			return 500, result, nil
		}
		ss.decoder = decoder
		if strings.HasPrefix(transport, "RTP/AVP/TCP") {
			result["Transport"] = "RTP/AVP/TCP;unicast;interleaved=0-1;mode=record"
		} else {
			if err = ss.udp(transport); err != nil {
				return 500, result, nil
			}
			result["Transport"] = fmt.Sprintf("RTP/AVP/UDP;unicast;mode=record;server_port=%d;control_port=%d;timing_port=%d", ss.data.LocalAddr().(*net.UDPAddr).Port, ss.control.LocalAddr().(*net.UDPAddr).Port, ss.timing.LocalAddr().(*net.UDPAddr).Port)
		}
		ss.wg.Add(1)
		go ss.audio()
		result["Session"] = "1"
	case "RECORD":
		if ss.decoder == nil {
			return 455, result, nil
		}
		if !ss.recorded {
			ss.recorded = true
			ss.wg.Add(1)
			go func() {
				defer ss.wg.Done()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				tick := time.NewTicker(20 * time.Millisecond)
				defer tick.Stop()
				for ss.live.Size() < 4096 {
					select {
					case <-ss.ctx.Done():
						return
					case <-timer.C:
						ss.cancel()
						return
					case <-tick.C:
					}
				}
				if err := ss.server.target.StartAirPlay(ss.ctx, ss.stream, h.Get("User-Agent")); err != nil {
					ss.cancel()
				}
			}()
		}
		result["Audio-Latency"] = "11025"
	case "FLUSH":
		select {
		case ss.flush <- struct{}{}:
		default:
		}
	case "TEARDOWN":
	case "GET_PARAMETER":
		if strings.Contains(string(body), "volume") {
			result["Content-Type"] = "text/parameters"
			return 200, result, []byte("volume: 0.000000\r\n")
		}
	case "SET_PARAMETER":
		if strings.HasPrefix(h.Get("Content-Type"), "text/parameters") {
			for _, line := range strings.Split(string(body), "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(line), "volume:"); ok {
					db, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
					if err != nil {
						return 400, result, nil
					}
					volume := int((db + 30) * 100 / 30)
					if volume < 0 {
						volume = 0
					}
					if volume > 100 {
						volume = 100
					}
					ctx, cancel := context.WithTimeout(ss.ctx, 5*time.Second)
					err = ss.server.target.SetVolume(ctx, volume)
					cancel()
					if err != nil {
						return 500, result, nil
					}
				}
			}
		} else if h.Get("Content-Type") == "application/x-dmap-tagged" {
			title, artist := metadata(body, 0)
			ss.server.target.Metadata(title, artist)
		}
	default:
		return 501, result, nil
	}
	return 200, result, nil
}
func (ss *session) enqueue(p []byte) {
	select {
	case ss.packets <- p:
	default:
	}
}
func (ss *session) udp(transport string) error {
	peer, _, _ := net.SplitHostPort(ss.conn.RemoteAddr().String())
	controlPort := 0
	for _, part := range strings.Split(transport, ";") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && k == "control_port" {
			controlPort, _ = strconv.Atoi(v)
		}
	}
	if controlPort < 1 || controlPort > 65535 {
		return errors.New("invalid control port")
	}
	ss.remote = &net.UDPAddr{IP: net.ParseIP(peer), Port: controlPort}
	for i, dest := range []**net.UDPConn{&ss.data, &ss.control, &ss.timing} {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(ss.server.host)})
		if err != nil {
			return err
		}
		*dest = c
		_ = c.SetReadBuffer(1 << 20)
		ss.wg.Add(1)
		go func(kind int, conn *net.UDPConn) {
			defer ss.wg.Done()
			buf := make([]byte, 16384)
			for {
				n, addr, err := conn.ReadFromUDP(buf)
				if err != nil {
					return
				}
				if !addr.IP.Equal(ss.remote.IP) {
					continue
				}
				if kind == 2 {
					if n >= 32 && buf[1]&127 == 82 {
						out := make([]byte, 32)
						out[0] = 0x80
						out[1] = 0xd3
						copy(out[2:4], buf[2:4])
						copy(out[8:16], buf[24:32])
						stamp := ntp(time.Now())
						copy(out[16:24], stamp)
						copy(out[24:32], stamp)
						conn.WriteToUDP(out, addr)
					}
					continue
				}
				p := buf[:n]
				if kind == 1 {
					if n < 4 || buf[1]&127 != 86 {
						continue
					}
					p = p[4:]
				}
				ss.enqueue(append([]byte(nil), p...))
			}
		}(i, c)
	}
	return nil
}
func (ss *session) audio() {
	defer ss.wg.Done()
	var jitter Jitter
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	resend := func(seq, count uint16) {
		if ss.control == nil {
			return
		}
		b := []byte{0x80, 0xd5, 0, 1, byte(seq >> 8), byte(seq), byte(count >> 8), byte(count)}
		ss.control.WriteToUDP(b, ss.remote)
	}
	for {
		select {
		case <-ss.ctx.Done():
			return
		case <-ss.flush:
			jitter.Reset()
			for len(ss.packets) > 0 {
				<-ss.packets
			}
		case p := <-ss.packets:
			packet, err := ParseRTP(p, ss.sdp.Key, ss.sdp.IV)
			if err == nil {
				jitter.Push(packet)
			}
		case <-tick.C:
		}
		if err := jitter.Drain(time.Now(), ss.decoder.WritePacket, resend); err != nil {
			ss.cancel()
			return
		}
	}
}
func ntp(t time.Time) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, uint32(t.Unix()+2208988800))
	binary.BigEndian.PutUint32(b[4:], uint32(uint64(t.Nanosecond())<<32/1000000000))
	return b
}
func metadata(b []byte, depth int) (title, artist string) {
	if depth > 8 {
		return
	}
	for len(b) >= 8 {
		tag := string(b[:4])
		n := int(binary.BigEndian.Uint32(b[4:8]))
		b = b[8:]
		if n > len(b) {
			return
		}
		v := b[:n]
		b = b[n:]
		switch tag {
		case "minm":
			title = string(v)
		case "asar":
			artist = string(v)
		case "mlit", "mdcl":
			t, a := metadata(v, depth+1)
			if t != "" {
				title = t
			}
			if a != "" {
				artist = a
			}
		}
	}
	return
}
