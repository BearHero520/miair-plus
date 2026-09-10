// Package media provides a signed, bounded streaming proxy. Audio is never
// buffered as an entire file in RAM; HTTP ranges are forwarded unchanged.
package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	URL     string
	Expires time.Time
}
type Proxy struct {
	mu      sync.Mutex
	entries map[string]Entry
	secret  []byte
	Base    string
	Client  *http.Client
	slots   chan struct{}
}

func NewProxy(base, secret string) *Proxy {
	transport := &http.Transport{DialContext: safeDial, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 60 * time.Second, MaxIdleConns: 32, MaxIdleConnsPerHost: 8, DisableCompression: true}
	return &Proxy{entries: map[string]Entry{}, secret: []byte(secret), Base: base, Client: &http.Client{Transport: transport, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		return ValidateURL(r.URL.String())
	}}, slots: make(chan struct{}, 16)}
}
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return errors.New("需要 HTTP/HTTPS 音频地址")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast()) {
		return errors.New("不接受本机或链路本地音频地址")
	}
	return nil
}
func (p *Proxy) URL(raw string) (string, error) {
	if err := ValidateURL(raw); err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, p.secret)
	h.Write([]byte(raw))
	id := hex.EncodeToString(h.Sum(nil))[:40]
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for k, e := range p.entries {
		if e.Expires.Before(now) {
			delete(p.entries, k)
		}
	}
	if len(p.entries) >= 256 {
		if _, ok := p.entries[id]; !ok {
			return "", errors.New("音频代理队列已满，请稍后重试")
		}
	}
	p.entries[id] = Entry{URL: raw, Expires: now.Add(24 * time.Hour)}
	return p.Base + "/media/" + id, nil
}
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	p.mu.Lock()
	e, ok := p.entries[strings.TrimPrefix(r.URL.Path, "/media/")]
	p.mu.Unlock()
	if !ok || time.Now().After(e.Expires) {
		http.Error(w, "expired audio URL", 404)
		return
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	default:
		http.Error(w, "too many streams", 503)
		return
	}
	// Total request lifetime is bounded but allows long music/podcast playback.
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Hour)
	defer cancel()
	var resp *http.Response
	method := r.Method
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, method, e.URL, nil)
		req.Header.Set("User-Agent", "MiAirPlus/2.0")
		for _, k := range []string{"Range", "If-Range"} {
			if v := r.Header.Get(k); v != "" {
				req.Header.Set(k, v)
			}
		}
		resp, err = p.Client.Do(req)
		if err == nil && method == "HEAD" && (resp.StatusCode == 405 || resp.StatusCode == 501) {
			resp.Body.Close()
			method = "GET"
			continue
		}
		if err == nil && resp.StatusCode < 500 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err != nil {
		http.Error(w, "audio source unavailable", 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 206 && resp.StatusCode != 416 {
		http.Error(w, "audio source rejected request", resp.StatusCode)
		return
	}
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000")
	w.WriteHeader(resp.StatusCode)
	if r.Method == "HEAD" {
		return
	}
	_, _ = io.CopyBuffer(w, resp.Body, make([]byte, 32*1024))
}

// Live is a fixed-size ring. Slow consumers are disconnected instead of growing
// memory or blocking the audio decoder. Absolute offsets survive wraparound.
type Live struct {
	mu      sync.Mutex
	data    []byte
	end     int64
	changed chan struct{}
	closed  bool
	mime    string
}

func NewLive(bytes int, mime string) *Live {
	if bytes < 65536 {
		bytes = 65536
	}
	return &Live{data: make([]byte, bytes), changed: make(chan struct{}), mime: mime}
}
func (l *Live) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, io.ErrClosedPipe
	}
	n := len(p)
	start := 0
	if n > len(l.data) {
		start = n - len(l.data)
	}
	for i := start; i < n; i++ {
		l.data[(l.end+int64(i))%int64(len(l.data))] = p[i]
	}
	l.end += int64(n)
	close(l.changed)
	l.changed = make(chan struct{})
	return n, nil
}
func (l *Live) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.closed = true
		close(l.changed)
	}
}
func (l *Live) Size() int64 { l.mu.Lock(); defer l.mu.Unlock(); return l.end }
func (l *Live) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	w.Header().Set("Content-Type", l.mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("transferMode.dlna.org", "Streaming")
	if r.Method == "HEAD" {
		w.WriteHeader(200)
		return
	}
	cursor := int64(0)
	if header := r.Header.Get("Range"); header != "" && header != "bytes=0-" {
		http.Error(w, "live audio is not seekable", 416)
		return
	}
	rc := http.NewResponseController(w)
	w.WriteHeader(200)
	for {
		l.mu.Lock()
		oldest := l.end - int64(len(l.data))
		if oldest < 0 {
			oldest = 0
		}
		if cursor == 0 && oldest > 0 {
			cursor = l.end - 32768
			if cursor < oldest {
				cursor = oldest
			}
		}
		if cursor < oldest {
			l.mu.Unlock()
			return
		}
		count := l.end - cursor
		if count > 32768 {
			count = 32768
		}
		chunk := make([]byte, int(count))
		for i := range chunk {
			chunk[i] = l.data[(cursor+int64(i))%int64(len(l.data))]
		}
		cursor += count
		changed, closed := l.changed, l.closed
		l.mu.Unlock()
		if len(chunk) > 0 {
			_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := w.Write(chunk); err != nil {
				return
			}
			_ = rc.Flush()
			continue
		}
		if closed {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		case <-time.After(30 * time.Second):
			return
		}
	}
}
func FormatTime(seconds float64) string {
	n := int(seconds)
	if n < 0 {
		n = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", n/3600, n/60%60, n%60)
}
func ParseTime(s string) (float64, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, errors.New("invalid time")
	}
	h, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	sec, e3 := strconv.ParseFloat(parts[2], 64)
	if e1 != nil || e2 != nil || e3 != nil || h < 0 || m < 0 || m > 59 || sec < 0 || sec >= 60 {
		return 0, errors.New("invalid time")
	}
	return float64(h*3600+m*60) + sec, nil
}

func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	for _, ip := range ips {
		if ip.IP.IsLoopback() || ip.IP.IsUnspecified() || ip.IP.IsLinkLocalUnicast() || ip.IP.IsMulticast() {
			return nil, errors.New("audio source resolves to a prohibited address")
		}
	}
	for _, ip := range ips {
		conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if e == nil {
			return conn, nil
		}
		err = e
	}
	if err == nil {
		err = errors.New("audio host has no addresses")
	}
	return nil, err
}
