package server

import (
	"context"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/airplay"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/dlna"
	"github.com/BearHero520/miair-plus/internal/media"
	"github.com/BearHero520/miair-plus/internal/xiaomi"
	"log"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"sync"
	"time"
)

type Manager struct {
	mu          sync.RWMutex
	applyMu     sync.Mutex
	ctx         context.Context
	store       *config.Store
	client      *xiaomi.Client
	renderers   map[string]*dlna.Renderer
	air         map[string]*airplay.Server
	live        map[string]*media.Live
	names       map[string]string
	errors      map[string]string
	proxy       *media.Proxy
	events      *dlna.Events
	discovery   *dlna.Discovery
	http        *http.Server
	host        string
	port        int
	ffmpeg      string
	noDiscovery bool
	wake        chan struct{}
	closed      bool
}

func NewManager(ctx context.Context, store *config.Store, client *xiaomi.Client, noDiscovery bool) *Manager {
	return &Manager{ctx: ctx, store: store, client: client, noDiscovery: noDiscovery, renderers: map[string]*dlna.Renderer{}, air: map[string]*airplay.Server{}, live: map[string]*media.Live{}, names: map[string]string{}, errors: map[string]string{}, wake: make(chan struct{}, 1), events: dlna.NewEvents(ctx)}
}
func LANHost(configured string) (string, error) {
	if configured != "" {
		ip := net.ParseIP(configured)
		if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() {
			return "", fmt.Errorf("请输入有效的 NAS IPv4 地址")
		}
		return ip.String(), nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			if ip != nil && ip.To4() != nil && ip.IsPrivate() {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("未找到局域网 IPv4 地址，请在设置中填写")
}
func findFFmpeg(path string) string {
	if path != "" {
		if p, e := exec.LookPath(path); e == nil {
			return p
		}
		return ""
	}
	for _, candidate := range []string{"ffmpeg", "/usr/bin/ffmpeg", "/usr/trim/bin/ffmpeg", "/var/apps/ffmpeg/target/bin/ffmpeg"} {
		if p, e := exec.LookPath(candidate); e == nil {
			return p
		}
	}
	return ""
}
func (m *Manager) Schedule() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) Run() {
	m.Schedule()
	retry := time.NewTicker(30 * time.Second)
	defer retry.Stop()
	for {
		select {
		case <-m.ctx.Done():
			m.Close()
			return
		case <-m.wake:
			if err := m.Apply(); err != nil {
				log.Printf("服务配置生效失败: %v", err)
			}
		case <-retry.C:
			if m.store.Snapshot().AutoRecover {
				if err := m.Apply(); err != nil {
					log.Printf("服务恢复重试: %v", err)
				}
			}
		}
	}
}
func (m *Manager) publish(live *media.Live) (string, func()) {
	id := config.Random(24)
	m.mu.Lock()
	m.live[id] = live
	base := fmt.Sprintf("http://%s:%d", m.host, m.port)
	m.mu.Unlock()
	return base + "/live/" + id, func() { m.mu.Lock(); delete(m.live, id); m.mu.Unlock() }
}
func (m *Manager) Apply() error {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()
	if m.closed {
		return context.Canceled
	}
	state := m.store.Snapshot()
	host, err := LANHost(state.Hostname)
	if err != nil {
		return err
	}
	ffmpeg := findFFmpeg(state.FFmpeg)
	if m.host != host || m.port != state.DLNAPort {
		m.stop()
		listener, err := net.Listen("tcp4", net.JoinHostPort(host, fmt.Sprint(state.DLNAPort)))
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.host = host
		m.port = state.DLNAPort
		m.proxy = media.NewProxy(fmt.Sprintf("http://%s:%d", host, state.DLNAPort), state.Secret)
		m.mu.Unlock()
		mux := http.NewServeMux()
		mux.Handle("/media/", m.proxy)
		mux.HandleFunc("GET /live/{id}", func(w http.ResponseWriter, r *http.Request) {
			m.mu.RLock()
			live := m.live[r.PathValue("id")]
			m.mu.RUnlock()
			if live == nil {
				http.NotFound(w, r)
				return
			}
			live.ServeHTTP(w, r)
		})
		mux.Handle("/dlna/", &dlna.HTTPHandler{Lookup: m.Lookup, Events: m.events, AutoPlay: func() bool { return m.store.Snapshot().AutoPlay }})
		m.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32768, BaseContext: func(net.Listener) context.Context { return m.ctx }}
		httpServer := m.http
		go func() {
			if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
				log.Printf("DLNA HTTP: %v", err)
			}
		}()
	}
	m.mu.Lock()
	m.ffmpeg = ffmpeg
	m.mu.Unlock()
	// Protocol registration may take seconds; API writes return before this worker.
	for did, r := range m.renderers {
		sp, exists := state.Speakers[did]
		if !exists || !sp.Enabled || state.Xiaomi.UserID == "" {
			if a := m.air[did]; a != nil {
				a.Close()
				delete(m.air, did)
			}
			r.Close()
			m.mu.Lock()
			delete(m.renderers, did)
			m.mu.Unlock()
		}
	}
	for did, sp := range state.Speakers {
		if !sp.Enabled || state.Xiaomi.UserID == "" {
			continue
		}
		r := m.Lookup(dlna.UUID(did))
		if r == nil {
			r = dlna.NewRenderer(m.ctx, sp, m.client, m.proxy, state.DefaultVolume)
			r.Notify = func() { m.events.Changed(r) }
			r.Transcode = m.seekURL
			go r.Poll(m.ctx)
			m.mu.Lock()
			m.renderers[did] = r
			m.mu.Unlock()
		} else {
			r.Update(sp)
		}
		nameError := ""
		if state.AirPlay && ffmpeg != "" {
			if a := m.air[did]; a != nil {
				if err = a.Rename(sp.DisplayName()); err != nil {
					nameError = err.Error()
				}
			} else {
				a, e := airplay.Start(m.ctx, host, sp.DisplayName(), did, ffmpeg, r, m.publish, !m.noDiscovery)
				if e != nil {
					nameError = e.Error()
				} else {
					m.air[did] = a
				}
			}
		} else if a := m.air[did]; a != nil {
			a.Close()
			delete(m.air, did)
		}
		m.mu.Lock()
		m.names[did] = sp.DisplayName()
		m.errors[did] = nameError
		m.mu.Unlock()
	}
	if !m.noDiscovery {
		if m.discovery == nil {
			d := dlna.NewDiscovery(host, state.DLNAPort)
			d.Set(m.ids())
			if err = d.Start(m.ctx); err != nil {
				return err
			}
			m.discovery = d
		} else {
			m.discovery.Set(m.ids())
			m.discovery.Announce()
		}
	}
	return nil
}
func (m *Manager) ids() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := []string{}
	for did := range m.renderers {
		ids = append(ids, dlna.UUID(did))
	}
	return ids
}
func (m *Manager) Lookup(id string) *dlna.Renderer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for did, r := range m.renderers {
		if dlna.UUID(did) == id {
			return r
		}
	}
	return nil
}
func (m *Manager) Snapshots() []dlna.Snapshot {
	state := m.store.Snapshot()
	out := []dlna.Snapshot{}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for did, sp := range state.Speakers {
		var s dlna.Snapshot
		if r := m.renderers[did]; r != nil {
			s = r.Snapshot()
		} else {
			s = dlna.Snapshot{DID: did, Name: sp.Name, DLNAName: sp.DisplayName(), Hardware: sp.Hardware, Enabled: sp.Enabled, Compatibility: sp.Compatibility, State: "STOPPED", UDN: dlna.UUID(did)}
		}
		s.DLNAName = sp.DisplayName()
		s.NameSync = "synced"
		if m.names[did] != sp.DisplayName() && sp.Enabled {
			s.NameSync = "pending"
		}
		if m.errors[did] != "" {
			s.NameSync = "failed"
			s.Error = m.errors[did]
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DID < out[j].DID })
	return out
}
func (m *Manager) Info() (host string, port, count int, running, codec bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.host, m.port, len(m.renderers), m.host != "", m.ffmpeg != ""
}
func (m *Manager) stop() {
	for did, a := range m.air {
		a.Close()
		delete(m.air, did)
	}
	if m.discovery != nil {
		m.discovery.Stop()
		m.discovery = nil
	}
	if m.http != nil {
		m.http.Close()
		m.http = nil
	}
	m.mu.Lock()
	for _, r := range m.renderers {
		r.Close()
	}
	m.renderers = map[string]*dlna.Renderer{}
	for _, l := range m.live {
		l.Close()
	}
	m.live = map[string]*media.Live{}
	m.host = ""
	m.port = 0
	m.mu.Unlock()
}
func (m *Manager) Close() { m.applyMu.Lock(); defer m.applyMu.Unlock(); m.closed = true; m.stop() }
