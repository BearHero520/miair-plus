package airplay2

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BearHero520/miair-plus/internal/media"
)

type testTarget struct{ starts, ends atomic.Int32 }

func (t *testTarget) StartAirPlay(context.Context, string, string) error { t.starts.Add(1); return nil }
func (t *testTarget) EndAirPlay(context.Context)                         { t.ends.Add(1) }
func (*testTarget) SetVolume(context.Context, int) error                 { return nil }
func (*testTarget) Metadata(string, string)                              {}

func TestConfigurationEscapesNames(t *testing.T) {
	s := configuration("客厅\"; port=9; //", "eth0", 12345)
	if !strings.Contains(s, `name = "客厅\"; port=9; //";`) || !strings.Contains(s, "socket_port = 12345") {
		t.Fatal(s)
	}
}

func TestBridgeClosesSessionAndProducers(t *testing.T) {
	ff := os.Getenv("TEST_FFMPEG")
	if ff == "" {
		ff, _ = exec.LookPath("ffmpeg")
	}
	if ff == "" {
		t.Skip("FFmpeg unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Receiver{cancel: cancel, done: make(chan struct{})}
	reader, writer := io.Pipe()
	defer writer.Close()
	udp, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	target := &testTarget{}
	var removed atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.bridge(ctx, Options{FFmpeg: ff, Target: target, Publish: func(*media.Live) (string, func()) { return "http://test/live", func() { removed.Add(1) } }}, reader, udp)
	}()
	go func() { b := make([]byte, 176400); _, _ = writer.Write(b) }()
	deadline := time.After(5 * time.Second)
	for target.starts.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("audio did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	c, e := net.DialUDP("udp4", nil, udp.LocalAddr().(*net.UDPAddr))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, _ = c.Write([]byte("ssncpend"))
	deadline = time.After(5 * time.Second)
	for target.ends.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("metadata did not end stream")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("bridge leaked a producer")
	}
	if target.starts.Load() != 1 || target.ends.Load() != 1 || removed.Load() != 1 {
		t.Fatalf("starts=%d ends=%d removed=%d", target.starts.Load(), target.ends.Load(), removed.Load())
	}
}

// Run inside the Debian build container as the package-like unprivileged user.
// This checks real native startup, UTF-8 config, Avahi and clock permissions.
func TestNativeReceiver(t *testing.T) {
	dir := os.Getenv("TEST_AIRPLAY2_RUNTIME")
	if dir == "" {
		t.Skip("native runtime integration is opt-in")
	}
	conn, e := net.Dial("udp4", "192.0.2.1:9")
	if e != nil {
		t.Fatal(e)
	}
	host := conn.LocalAddr().(*net.UDPAddr).IP.String()
	conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, e := Start(ctx, Options{Host: host, Name: "MiAir 测试 \"音箱\"", Directory: t.TempDir(), Runtime: dir, FFmpeg: filepath.Join(dir, "bin/ffmpeg"), Target: &testTarget{}, Publish: func(*media.Live) (string, func()) { return "http://test/live", func() {} }})
	if e != nil {
		t.Fatal(e)
	}
	if !r.Alive() {
		t.Fatal(r.Error())
	}
	c, e := net.DialTimeout("tcp4", net.JoinHostPort(host, "7000"), time.Second)
	if e != nil {
		r.Close()
		t.Fatal(e)
	}
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, e = io.WriteString(c, "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	if e != nil {
		t.Fatal(e)
	}
	b := make([]byte, 4096)
	n, e := c.Read(b)
	c.Close()
	r.Close()
	if e != nil || !strings.Contains(string(b[:n]), "200 OK") {
		t.Fatalf("RTSP response: %q (%v)", b[:n], e)
	}
}
