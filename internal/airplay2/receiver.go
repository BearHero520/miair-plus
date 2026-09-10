// Package airplay2 supervises native Shairport Sync and NQPTP. Protocol parsing,
// authentication and clock recovery stay in the upstream receiver.
package airplay2

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BearHero520/miair-plus/internal/airplay"
	"github.com/BearHero520/miair-plus/internal/media"
)

type Options struct {
	Host, Name, Directory, Runtime, FFmpeg string
	Target                                 airplay.Target
	Publish                                func(*media.Live) (string, func())
}

type Receiver struct {
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	failure string
}

func RuntimeDir() string { return os.Getenv("MIAIR_AUDIO_DIR") }

func interfaceName(host string) (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, _ := net.ParseCIDR(addr.String())
			if ip != nil && ip.String() == host {
				return iface.Name, nil
			}
		}
	}
	return "", errors.New("AirPlay 2 找不到所选 NAS 地址对应的网卡")
}

// strconv.Quote produces libconfig-compatible strings for validated UTF-8 names.
func configuration(name, iface string, port int) string {
	return fmt.Sprintf(`general = { name = %s; interface = %s; port = 7000; output_backend = "stdout"; };
stdout = { output_rate = 44100; output_format = "S16_BE"; output_channels = 2; };
metadata = { enabled = "yes"; include_cover_art = "no"; socket_address = "127.0.0.1"; socket_port = %d; socket_msglength = 4096; };
sessioncontrol = { active_state_timeout = 10.0; };
`, strconv.Quote(name), strconv.Quote(iface), port)
}

func (r *Receiver) fail(err error) {
	r.mu.Lock()
	if r.failure == "" {
		r.failure = err.Error()
		log.Printf("AirPlay 2: %v", err)
	}
	r.mu.Unlock()
	r.cancel()
}
func (r *Receiver) Error() string { r.mu.Lock(); defer r.mu.Unlock(); return r.failure }
func (r *Receiver) Close()        { r.cancel(); <-r.done }
func (r *Receiver) Alive() bool {
	select {
	case <-r.done:
		return false
	default:
		return r.Error() == ""
	}
}

func Start(parent context.Context, o Options) (*Receiver, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("AirPlay 2 原生接收组件仅支持 Linux NAS")
	}
	if o.Runtime == "" || o.FFmpeg == "" {
		return nil, errors.New("缺少 AirPlay 2 原生组件，请安装包含音频组件的通用 FPK")
	}
	for _, bin := range []string{"shairport-sync", "nqptp"} {
		if _, err := exec.LookPath(filepath.Join(o.Runtime, "bin", bin)); err != nil {
			return nil, fmt.Errorf("缺少 %s: %w", bin, err)
		}
	}
	if _, err := os.Stat("/run/dbus/system_bus_socket"); err != nil {
		return nil, errors.New("AirPlay 2 需要 NAS 系统 D-Bus 和 Avahi 服务；DLNA 不受影响")
	}
	iface, err := interfaceName(o.Host)
	if err != nil {
		return nil, err
	}
	// Never connect to, kill, or claim another receiver already using our RTSP port.
	probe, err := net.Listen("tcp4", net.JoinHostPort(o.Host, "7000"))
	if err != nil {
		return nil, fmt.Errorf("AirPlay 2 TCP 7000 已占用: %w", err)
	}
	probe.Close()
	metadata, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(o.Directory, 0700); err != nil {
		metadata.Close()
		return nil, err
	}
	configPath := filepath.Join(o.Directory, "shairport-sync.conf")
	if err = os.WriteFile(configPath, []byte(configuration(o.Name, iface, metadata.LocalAddr().(*net.UDPAddr).Port)), 0600); err != nil {
		metadata.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	r := &Receiver{cancel: cancel, done: make(chan struct{})}
	var wg sync.WaitGroup
	// NQPTP alone receives CAP_NET_BIND_SERVICE during package startup. The Go
	// service and Shairport Sync retain the unprivileged package account.
	start := func(cmd *exec.Cmd) error {
		cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
		cmd.WaitDelay = 3 * time.Second
		stderr, e := cmd.StderrPipe()
		if e != nil {
			return e
		}
		if e = cmd.Start(); e != nil {
			stderr.Close()
			return e
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				if len(line) > 2048 {
					line = line[:2048]
				}
				log.Printf("AirPlay 2 %s: %s", filepath.Base(cmd.Path), line)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := cmd.Wait()
			if ctx.Err() == nil {
				r.fail(fmt.Errorf("%s 已退出 (%v)，请检查组件日志及 UDP 319/320、Avahi 服务", filepath.Base(cmd.Path), e))
			}
		}()
		return nil
	}
	abort := func(e error) (*Receiver, error) { cancel(); metadata.Close(); wg.Wait(); return nil, e }
	clock := exec.CommandContext(ctx, filepath.Join(o.Runtime, "bin/nqptp"))
	if err = start(clock); err != nil {
		return abort(err)
	}
	select {
	case <-ctx.Done():
		return abort(errors.New(r.Error()))
	case <-time.After(600 * time.Millisecond):
	}
	receiver := exec.CommandContext(ctx, filepath.Join(o.Runtime, "bin/shairport-sync"), "-c", configPath)
	pcm, err := receiver.StdoutPipe()
	if err != nil {
		return abort(err)
	}
	if err = start(receiver); err != nil {
		pcm.Close()
		return abort(err)
	}
	wg.Add(1)
	go func() { defer wg.Done(); r.bridge(ctx, o, pcm, metadata) }()
	go func() { <-ctx.Done(); metadata.Close(); pcm.Close() }()
	go func() { wg.Wait(); cancel(); close(r.done) }()
	// Do not report readiness on exec success alone. Detect early native crashes.
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.Close()
			return nil, fmt.Errorf("AirPlay 2 启动失败: %s", r.Error())
		case <-timer.C:
			r.fail(errors.New("AirPlay 2 未在 TCP 7000 就绪"))
			r.Close()
			return nil, errors.New(r.Error())
		case <-ticker.C:
			c, e := net.DialTimeout("tcp4", net.JoinHostPort(o.Host, "7000"), 100*time.Millisecond)
			if e == nil {
				c.Close()
				return r, nil
			}
		}
	}
}

// The two producer queues are bounded. A failed decoder cancels the receiver,
// allowing the manager's recovery loop to rebuild the entire audio pipeline.
func (r *Receiver) bridge(ctx context.Context, o Options, pcm io.Reader, metadata *net.UDPConn) {
	chunks := make(chan []byte, 4)
	events := make(chan string, 16)
	var producers sync.WaitGroup
	producers.Add(2)
	go func() {
		defer producers.Done()
		b := make([]byte, 8192)
		for {
			n, e := pcm.Read(b)
			if n > 0 {
				p := append([]byte(nil), b[:n]...)
				select {
				case chunks <- p:
				case <-ctx.Done():
					return
				}
			}
			if e != nil {
				if ctx.Err() == nil {
					r.fail(fmt.Errorf("PCM 输出中断: %w", e))
				}
				return
			}
		}
	}()
	go func() {
		defer producers.Done()
		b := make([]byte, 4096)
		for {
			n, _, e := metadata.ReadFromUDP(b)
			if e != nil {
				return
			}
			if n < 8 {
				continue
			}
			event := string(b[:n])
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() {
		r.cancel()
		metadata.Close()
		if c, ok := pcm.(io.Closer); ok {
			c.Close()
		}
		producers.Wait()
	}()
	var decoder *airplay.Decoder
	var live *media.Live
	var stop context.CancelFunc
	var unpublish func()
	var playDone chan struct{}
	var lastPCM time.Time
	ended := false
	end := func() {
		if decoder == nil {
			return
		}
		stop()
		decoder.Close()
		live.Close()
		unpublish()
		<-playDone
		endCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		o.Target.EndAirPlay(endCtx)
		cancel()
		decoder = nil
	}
	defer end()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			if e[:8] == "ssncpend" {
				ended = true
				end()
			}
			if e[:8] == "ssncpbeg" {
				ended = false
			}
			if len(e) <= 2056 && decoder != nil && strings.HasPrefix(e, "core") {
				// Artist/title packets may arrive independently; renderer owns merging.
				if e[4:8] == "minm" {
					o.Target.Metadata(e[8:], "")
				}
			}
		case <-tick.C:
			if decoder != nil && time.Since(lastPCM) > 12*time.Second {
				end()
			}
		case b := <-chunks:
			if ended {
				continue
			}
			lastPCM = time.Now()
			if decoder == nil {
				audioCtx, cancel := context.WithCancel(ctx)
				stop = cancel
				live = media.NewLive(512*1024, "audio/mpeg")
				url, remove := o.Publish(live)
				unpublish = remove
				ready := make(chan struct{})
				d, e := airplay.NewDecoder(audioCtx, o.FFmpeg, airplay.SDP{Codec: "L16", Rate: 44100, Channels: 2}, &audioReady{Writer: live, ready: ready})
				if e != nil {
					cancel()
					live.Close()
					remove()
					r.fail(e)
					return
				}
				decoder = d
				playDone = make(chan struct{})
				done := playDone
				go func() {
					defer close(done)
					select {
					case <-ready:
					case <-audioCtx.Done():
						return
					}
					if e := o.Target.StartAirPlay(audioCtx, url, "AirPlay 2"); e != nil && audioCtx.Err() == nil {
						r.fail(fmt.Errorf("小米音箱启动播放失败: %w", e))
					}
				}()
			}
			if e := decoder.WritePacket(b); e != nil {
				if ctx.Err() == nil {
					r.fail(fmt.Errorf("音频转码中断: %w", e))
				}
				return
			}
		}
	}
}

// Cloud playback begins only once the decoder has emitted actual MP3 bytes.
type audioReady struct {
	io.Writer
	ready chan struct{}
	once  sync.Once
}

func (w *audioReady) Write(p []byte) (int, error) {
	n, e := w.Writer.Write(p)
	if n > 0 {
		w.once.Do(func() { close(w.ready) })
	}
	return n, e
}
