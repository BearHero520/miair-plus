package server

import (
	"fmt"
	"github.com/BearHero520/miair-plus/internal/airplay2"
	"github.com/BearHero520/miair-plus/internal/diagnostics"
	"path/filepath"
	"runtime"
	"time"
)

type Check struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

func (m *Manager) FFmpegInfo() (string, string) {
	m.mu.RLock()
	path := m.ffmpeg
	m.mu.RUnlock()
	source := "missing"
	if path != "" {
		source = "system"
		if dir := airplay2.RuntimeDir(); dir != "" && filepath.Clean(path) == filepath.Clean(filepath.Join(dir, "bin/ffmpeg")) {
			source = "bundled"
		}
		if m.store.Snapshot().FFmpeg != "" {
			source = "custom"
		}
	}
	return path, source
}
func (a *API) diagnosticReport() map[string]any {
	s := a.Store.Snapshot()
	host, port, count, running, codec := a.Manager.Info()
	ap2, ap2Error := a.Manager.AirPlay2Info()
	_, source := a.Manager.FFmpegInfo()
	checks := []Check{{"DLNA", "ok", fmt.Sprintf("%s:%d · %d 台音箱已启用", host, port, count), "/devices"}, {"小米账号", "ok", "已保存登录凭据；实际连接结果见投送记录", "/account"}, {"FFmpeg", "ok", "已检测到音频组件", "/settings"}, {"AirPlay 2", "off", "未开启", "/settings"}}
	if !running {
		checks[0] = Check{"DLNA", "warn", "服务未就绪，请检查 NAS 地址与端口占用", "/settings"}
	} else if count == 0 {
		checks[0].State = "warn"
		checks[0].Message = "服务已启动，尚未启用音箱"
	}
	if s.Xiaomi.ServiceToken == "" {
		checks[1].State = "warn"
		checks[1].Message = "请先连接小米账号"
	}
	if !codec {
		checks[2].State = "error"
		checks[2].Message = "未找到音频组件，请检查安装包或自定义路径"
	} else if source == "bundled" {
		checks[2].Message = "内置 FFmpeg 已就绪"
	} else {
		checks[2].Message = "当前使用系统或自定义 FFmpeg"
	}
	if s.AirPlay && s.AirPlay2 {
		checks[3].State = "warn"
		checks[3].Message = "等待接收组件启动"
		if ap2 {
			checks[3].State = "ok"
			checks[3].Message = "接收组件已就绪"
		}
		if ap2Error != "" {
			checks[3].State = "error"
			if airPlay2Target(s) == "" {
				checks[3].State = "warn"
			}
			checks[3].Message = diagnostics.Redact(ap2Error)
		}
	}
	entries, storage := a.journal.Snapshot()
	return map[string]any{"version": Version, "arch": runtime.GOARCH, "generated_at": time.Now(), "checks": checks, "entries": entries, "storage_error": storage, "capacity": diagnostics.Capacity}
}
