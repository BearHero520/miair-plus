package server

import (
	"context"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/media"
	"net/http"
	"os/exec"
	"time"
)

// Seek uses the signed proxy so FFmpeg cannot access arbitrary local protocols.
// Jobs expire with their reader; total transcoding concurrency is bounded.
var transcodeSlots = make(chan struct{}, 4)

func (m *Manager) seekURL(uri string, seconds float64) (string, error) {
	m.mu.RLock()
	ffmpeg, proxy := m.ffmpeg, m.proxy
	m.mu.RUnlock()
	if ffmpeg == "" {
		return "", fmt.Errorf("拖动进度需要 FFmpeg")
	}
	source, err := proxy.URL(uri)
	if err != nil {
		return "", err
	}
	select {
	case transcodeSlots <- struct{}{}:
	default:
		return "", fmt.Errorf("转码任务繁忙")
	}
	ctx, cancel := context.WithTimeout(m.ctx, 4*time.Hour)
	live := media.NewLive(4<<20, "audio/mpeg")
	url, remove := m.publish(live)
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "http,tcp", "-re", "-ss", fmt.Sprintf("%.3f", seconds), "-i", source, "-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-f", "mp3", "-flush_packets", "1", "pipe:1")
	cmd.Stdout = live
	if err = cmd.Start(); err != nil {
		cancel()
		remove()
		<-transcodeSlots
		return "", err
	}
	go func() {
		defer cancel()
		defer func() { <-transcodeSlots }()
		_ = cmd.Wait()
		live.Close()
		select {
		case <-m.ctx.Done():
		case <-time.After(time.Minute):
		}
		remove()
	}()
	return url, nil
}

var _ http.Handler = (*media.Live)(nil)
