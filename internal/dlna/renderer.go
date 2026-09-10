package dlna

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/diagnostics"
	"github.com/BearHero520/miair-plus/internal/media"
	"strings"
	"sync"
	"time"
)

type Controller interface {
	Play(context.Context, config.Speaker, string) error
	Operation(context.Context, config.Speaker, string) error
	Volume(context.Context, config.Speaker, int) error
	Status(context.Context, config.Speaker) (map[string]any, error)
}
type Snapshot struct {
	DID           string     `json:"did"`
	Name          string     `json:"name"`
	DLNAName      string     `json:"dlna_name"`
	Hardware      string     `json:"hardware"`
	Enabled       bool       `json:"enabled"`
	Compatibility bool       `json:"compatibility_mode"`
	UDN           string     `json:"udn"`
	State         string     `json:"transport_state"`
	URI           string     `json:"current_uri"`
	AirPlay       bool       `json:"airplay_active"`
	Client        string     `json:"airplay_client"`
	Volume        int        `json:"volume"`
	Mute          bool       `json:"mute"`
	Position      float64    `json:"position"`
	Duration      float64    `json:"duration"`
	Metadata      string     `json:"metadata"`
	NextURI       string     `json:"next_uri"`
	NameSync      string     `json:"name_sync"`
	Error         string     `json:"last_error"`
	NowPlaying    NowPlaying `json:"now_playing"`
}
type NowPlaying struct {
	Playing bool   `json:"playing"`
	Source  string `json:"source"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Client  string `json:"client"`
}
type Renderer struct {
	closeFn                                                           context.CancelFunc
	mu                                                                sync.Mutex
	opMu                                                              sync.Mutex
	cfg                                                               config.Speaker
	state, uri, metadata, nextURI, nextMeta, title, artist, lastError string
	volume                                                            int
	mute                                                              bool
	position, duration                                                float64
	started                                                           time.Time
	generation                                                        uint64
	cancel                                                            context.CancelFunc
	root                                                              context.Context
	controller                                                        Controller
	proxy                                                             *media.Proxy
	Transcode                                                         func(string, float64) (string, error)
	Notify                                                            func()
	airplay                                                           bool
	airClient                                                         string
}

func UUID(did string) string {
	h := sha256.Sum256([]byte("miair-plus:" + did))
	s := hex.EncodeToString(h[:16])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
func NewRenderer(ctx context.Context, s config.Speaker, c Controller, p *media.Proxy, volume int) *Renderer {
	ctx, cancel := context.WithCancel(ctx)
	return &Renderer{cfg: s, root: ctx, closeFn: cancel, controller: c, proxy: p, volume: volume, state: "STOPPED"}
}
func (r *Renderer) Config() config.Speaker  { r.mu.Lock(); defer r.mu.Unlock(); return r.cfg }
func (r *Renderer) Update(s config.Speaker) { r.mu.Lock(); r.cfg = s; r.mu.Unlock(); r.changed() }
func (r *Renderer) changed() {
	if r.Notify != nil {
		r.Notify()
	}
}
func (r *Renderer) positionLocked() float64 {
	p := r.position
	if r.state == "PLAYING" && !r.started.IsZero() {
		p += time.Since(r.started).Seconds()
	}
	if r.duration > 0 && p > r.duration {
		p = r.duration
	}
	return p
}
func (r *Renderer) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	source := "dlna"
	if r.airplay {
		source = "airplay"
	}
	return Snapshot{DID: r.cfg.DID, Name: r.cfg.Name, DLNAName: r.cfg.DisplayName(), Hardware: r.cfg.Hardware, Enabled: r.cfg.Enabled, Compatibility: r.cfg.Compatibility, UDN: UUID(r.cfg.DID), State: r.state, URI: r.uri, AirPlay: r.airplay, Client: r.airClient, Volume: r.volume, Mute: r.mute, Position: r.positionLocked(), Duration: r.duration, Metadata: r.metadata, NextURI: r.nextURI, NameSync: "synced", Error: r.lastError, NowPlaying: NowPlaying{Playing: r.state == "PLAYING", Source: source, Title: r.title, Artist: r.artist, Client: r.airClient}}
}
func (r *Renderer) invalidateLocked() {
	r.generation++
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}
func (r *Renderer) SetURI(uri, metadata string) error {
	if err := media.ValidateURL(uri); err != nil {
		return err
	}
	r.mu.Lock()
	r.invalidateLocked()
	r.uri = uri
	r.metadata = metadata
	r.position = 0
	r.started = time.Time{}
	r.state = "STOPPED"
	r.airplay = false
	r.lastError = ""
	r.title, r.artist, r.duration = parseMetadata(metadata)
	r.mu.Unlock()
	r.changed()
	return nil
}
func (r *Renderer) SetNext(uri, metadata string) error {
	if uri != "" {
		if err := media.ValidateURL(uri); err != nil {
			return err
		}
	}
	r.mu.Lock()
	r.nextURI = uri
	r.nextMeta = metadata
	r.mu.Unlock()
	r.changed()
	return nil
}
func parseMetadata(raw string) (title, artist string, duration float64) {
	d := xml.NewDecoder(strings.NewReader(raw))
	for {
		token, err := d.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "title":
			_ = d.DecodeElement(&title, &start)
		case "artist", "creator":
			if artist == "" {
				_ = d.DecodeElement(&artist, &start)
			}
		case "res":
			for _, a := range start.Attr {
				if a.Name.Local == "duration" {
					duration, _ = media.ParseTime(a.Value)
				}
			}
		}
	}
	return
}
func (r *Renderer) command(ctx context.Context, fn func(context.Context, config.Speaker) error, success func()) error {
	r.mu.Lock()
	generation := r.generation
	r.mu.Unlock()
	return r.commandAt(ctx, generation, fn, success)
}
func (r *Renderer) commandAt(ctx context.Context, generation uint64, fn func(context.Context, config.Speaker) error, success func()) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	r.mu.Lock()
	if generation != r.generation || r.root.Err() != nil {
		r.mu.Unlock()
		return errors.New("播放请求已被更新的操作替代")
	}
	operation, cancel := context.WithTimeout(ctx, 15*time.Second)
	r.cancel = cancel
	cfg := r.cfg
	r.mu.Unlock()
	defer cancel()
	started := time.Now()
	err := fn(operation, cfg)
	r.mu.Lock()
	if generation != r.generation || r.root.Err() != nil {
		r.mu.Unlock()
		return errors.New("播放请求已取消")
	}
	r.cancel = nil
	if err != nil {
		r.lastError = err.Error()
	} else {
		r.lastError = ""
		success()
	}
	state, air := r.state, r.airplay
	r.mu.Unlock()
	module := "DLNA"
	if air {
		module = "AirPlay"
	}
	detail := map[string]string{"音箱": cfg.DisplayName(), "状态": state, "耗时": fmt.Sprintf("%d ms", time.Since(started).Milliseconds())}
	level, message := "info", "音箱操作完成"
	if err != nil {
		level, message = "error", "音箱操作失败"
		detail["原因"] = err.Error()
	}
	diagnostics.Event(level, module, message, detail)
	r.changed()
	return err
}
func (r *Renderer) Play(ctx context.Context) error {
	r.mu.Lock()
	generation := r.generation
	r.mu.Unlock()
	return r.playAt(ctx, generation)
}
func (r *Renderer) playAt(ctx context.Context, generation uint64) error {
	r.mu.Lock()
	if generation != r.generation || r.root.Err() != nil {
		r.mu.Unlock()
		return context.Canceled
	}
	uri, state := r.uri, r.state
	r.mu.Unlock()
	if uri == "" {
		return errors.New("还没有音频地址")
	}
	if state == "PLAYING" {
		return nil
	}
	if state == "PAUSED_PLAYBACK" {
		return r.commandAt(ctx, generation, func(ctx context.Context, s config.Speaker) error { return r.controller.Operation(ctx, s, "play") }, func() { r.state = "PLAYING"; r.started = time.Now() })
	}
	target, err := r.proxy.URL(uri)
	if err != nil {
		return err
	}
	return r.commandAt(ctx, generation, func(ctx context.Context, s config.Speaker) error {
		r.mu.Lock()
		already := r.state == "PLAYING"
		volume := r.volume
		r.mu.Unlock()
		if already {
			return nil
		}
		if err := r.controller.Play(ctx, s, target); err != nil {
			return err
		}
		_ = r.controller.Volume(ctx, s, volume)
		return nil
	}, func() {
		if r.state != "PLAYING" {
			r.started = time.Now()
		}
		r.state = "PLAYING"
	})
}
func (r *Renderer) Pause(ctx context.Context) error {
	r.mu.Lock()
	r.invalidateLocked()
	generation := r.generation
	r.mu.Unlock()
	return r.commandAt(ctx, generation, func(ctx context.Context, s config.Speaker) error { return r.controller.Operation(ctx, s, "pause") }, func() { r.position = r.positionLocked(); r.state = "PAUSED_PLAYBACK"; r.started = time.Time{} })
}
func (r *Renderer) Stop(ctx context.Context) error {
	return r.StopURI(ctx, "")
}

// StopURI prevents a delayed alarm timeout from stopping a newer cast.
func (r *Renderer) StopURI(ctx context.Context, expected string) error {
	r.mu.Lock()
	if expected != "" && r.uri != expected {
		r.mu.Unlock()
		return nil
	}
	r.invalidateLocked()
	generation := r.generation
	r.mu.Unlock()
	return r.commandAt(ctx, generation, func(ctx context.Context, s config.Speaker) error { return r.controller.Operation(ctx, s, "stop") }, func() { r.state = "STOPPED"; r.position = 0; r.started = time.Time{}; r.airplay = false })
}

// StartAlarm reserves the URI and generation before any cloud operation.
func (r *Renderer) StartAlarm(ctx context.Context, uri string, volume int) error {
	if volume < 1 || volume > 100 {
		return errors.New("无效闹钟音量")
	}
	if err := r.SetURI(uri, ""); err != nil {
		return err
	}
	r.mu.Lock()
	generation := r.generation
	r.nextURI, r.nextMeta = "", ""
	r.volume = volume
	r.title = "闹钟"
	r.mu.Unlock()
	target, err := r.proxy.URL(uri)
	if err != nil {
		return err
	}
	return r.commandAt(ctx, generation, func(ctx context.Context, s config.Speaker) error {
		if err := r.controller.Volume(ctx, s, volume); err != nil {
			return err
		}
		return r.controller.Play(ctx, s, target)
	}, func() { r.state = "PLAYING"; r.started = time.Now() })
}
func (r *Renderer) Seek(ctx context.Context, seconds float64) error {
	if seconds < 0 || seconds > 86400 {
		return errors.New("无效进度")
	}
	r.mu.Lock()
	uri, duration := r.uri, r.duration
	paused := r.state == "PAUSED_PLAYBACK"
	r.invalidateLocked()
	generation := r.generation
	r.mu.Unlock()
	if uri == "" || r.Transcode == nil || seconds > duration && duration > 0 {
		return errors.New("当前音频不支持此进度")
	}
	target, err := r.Transcode(uri, seconds)
	if err != nil {
		return err
	}
	return r.commandAt(ctx, generation, func(ctx context.Context, s config.Speaker) error {
		if err := r.controller.Play(ctx, s, target); err != nil {
			return err
		}
		if paused {
			return r.controller.Operation(ctx, s, "pause")
		}
		return nil
	}, func() {
		r.position = seconds
		if paused {
			r.state = "PAUSED_PLAYBACK"
			r.started = time.Time{}
		} else {
			r.state = "PLAYING"
			r.started = time.Now()
		}
	})
}
func (r *Renderer) SetVolume(ctx context.Context, volume int) error {
	if volume < 0 || volume > 100 {
		return errors.New("音量超出范围")
	}
	return r.command(ctx, func(ctx context.Context, s config.Speaker) error { return r.controller.Volume(ctx, s, volume) }, func() { r.volume = volume; r.mute = false })
}
func (r *Renderer) SetMute(ctx context.Context, mute bool) error {
	r.mu.Lock()
	volume := r.volume
	r.mu.Unlock()
	target := volume
	if mute {
		target = 0
	}
	return r.command(ctx, func(ctx context.Context, s config.Speaker) error { return r.controller.Volume(ctx, s, target) }, func() { r.mute = mute })
}
func (r *Renderer) Next(ctx context.Context) error {
	r.mu.Lock()
	uri, metadata := r.nextURI, r.nextMeta
	r.nextURI = ""
	r.nextMeta = ""
	r.mu.Unlock()
	if uri == "" {
		return errors.New("没有下一首")
	}
	if err := r.SetURI(uri, metadata); err != nil {
		return err
	}
	return r.Play(ctx)
}
func (r *Renderer) StartAirPlay(ctx context.Context, stream, client string) error {
	r.mu.Lock()
	r.invalidateLocked()
	generation := r.generation
	r.mu.Unlock()
	return r.commandAt(ctx, generation, func(ctx context.Context, s config.Speaker) error { return r.controller.Play(ctx, s, stream) }, func() {
		r.airplay = true
		r.airClient = client
		r.state = "PLAYING"
		r.started = time.Now()
		r.position = 0
		r.uri = ""
		r.title = "AirPlay 音频"
		r.artist = ""
		r.duration = 0
	})
}
func (r *Renderer) EndAirPlay(ctx context.Context) {
	r.mu.Lock()
	active := r.airplay
	r.mu.Unlock()
	if active {
		_ = r.Stop(ctx)
	}
}
func (r *Renderer) Metadata(title, artist string) {
	r.mu.Lock()
	r.title = title
	r.artist = artist
	r.mu.Unlock()
	r.changed()
}
func (r *Renderer) Close() { r.closeFn(); r.mu.Lock(); r.invalidateLocked(); r.mu.Unlock() }

func (r *Renderer) AutoPlay() {
	r.mu.Lock()
	generation := r.generation
	r.mu.Unlock()
	go func() { _ = r.playAt(r.root, generation) }()
}
