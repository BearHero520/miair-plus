package dlna

import (
	"context"
	"encoding/json"
	"github.com/BearHero520/miair-plus/internal/diagnostics"
	"strconv"
	"time"
)

// One poll per renderer, bounded globally, never converts a network error into
// STOPPED. A generation check prevents old cloud status undoing a new command.
var pollSlots = make(chan struct{}, 4)

func (r *Renderer) Poll(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	var lastFailure time.Time
	failed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.root.Done():
			return
		case <-tick.C:
		}
		r.mu.Lock()
		generation, cfg := r.generation, r.cfg
		active := r.state != "STOPPED" && r.cancel == nil
		r.mu.Unlock()
		if !active {
			continue
		}
		select {
		case pollSlots <- struct{}{}:
		default:
			continue
		}
		operation, cancel := context.WithTimeout(ctx, 4*time.Second)
		info, err := r.controller.Status(operation, cfg)
		cancel()
		<-pollSlots
		if err != nil {
			if !failed || time.Since(lastFailure) > time.Minute {
				diagnostics.Event("warn", "小米账号", "音箱状态读取失败", map[string]string{"音箱": cfg.DisplayName(), "原因": err.Error(), "建议": "检查网络或重新连接小米账号"})
				lastFailure = time.Now()
			}
			failed = true
			continue
		}
		if failed {
			diagnostics.Event("info", "小米账号", "音箱状态连接恢复", map[string]string{"音箱": cfg.DisplayName()})
			failed = false
		}
		parse := func(v any) (int, bool) {
			switch n := v.(type) {
			case json.Number:
				i, e := strconv.Atoi(string(n))
				return i, e == nil
			case float64:
				return int(n), true
			case int:
				return n, true
			}
			return 0, false
		}
		status, valid := parse(info["status"])
		if !valid || status < 0 || status > 2 {
			continue
		}
		state := []string{"STOPPED", "PLAYING", "PAUSED_PLAYBACK"}[status]
		r.mu.Lock()
		if generation != r.generation || r.cancel != nil || r.state == "PLAYING" && time.Since(r.started) < 10*time.Second {
			r.mu.Unlock()
			continue
		}
		wasPlaying := r.state == "PLAYING"
		atEnd := r.duration > 0 && r.positionLocked() >= r.duration-2
		advance := wasPlaying && state == "STOPPED" && atEnd && r.nextURI != "" && !r.airplay
		if state != r.state {
			r.position = r.positionLocked()
			r.state = state
			r.started = time.Time{}
			if state == "PLAYING" {
				r.started = time.Now()
			}
		}
		if v, ok := parse(info["volume"]); ok && v >= 0 && v <= 100 && !r.mute {
			r.volume = v
		}
		r.mu.Unlock()
		r.changed()
		if advance {
			_ = r.Next(ctx)
		}
	}
}
