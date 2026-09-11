package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/BearHero520/miair-plus/internal/config"
)

// Backups deliberately contain portable preferences, never account credentials.
type configBackup struct {
	Format   string         `json:"format"`
	Schema   int            `json:"schema_version"`
	Version  string         `json:"app_version"`
	Exported string         `json:"exported_at"`
	Settings *settingsPatch `json:"settings"`
	Alarms   []config.Alarm `json:"alarms"`
}

func (a *API) exportConfig(w http.ResponseWriter, r *http.Request) {
	s := a.Store.Snapshot()
	p := &settingsPatch{Hostname: &s.Hostname, Port: &s.DLNAPort, AutoPlay: &s.AutoPlay,
		Recover: &s.AutoRecover, Volume: &s.DefaultVolume, AirPlay: &s.AirPlay,
		AirPlay2: &s.AirPlay2, AirPlay2Port: &s.AirPlay2Port, AirPlay2Target: &s.AirPlay2Target, FFmpeg: &s.FFmpeg,
		AudioID: &s.AudioID, AutoCheckUpdate: &s.AutoCheckUpdate, Speakers: map[string]speakerPatch{}}
	for did, sp := range s.Speakers {
		p.Speakers[did] = speakerPatch{Enabled: &sp.Enabled, Name: &sp.DLNAName, Compatibility: &sp.Compatibility}
	}
	alarms := append([]config.Alarm{}, s.Alarms...)
	for i := range alarms {
		alarms[i].LastFire = ""
		alarms[i].SnoozeAt = 0
	}
	w.Header().Set("Content-Disposition", "attachment; filename=miair-plus-config.json")
	jsonOut(w, configBackup{"miair-plus", 1, Version, time.Now().UTC().Format(time.RFC3339), p, alarms})
}

func (a *API) importConfig(w http.ResponseWriter, r *http.Request) {
	var backup configBackup
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&backup); err != nil {
		fail(w, 400, errors.New("配置文件格式错误、含不支持的字段或超过 1 MB"))
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || backup.Format != "miair-plus" || backup.Schema != 1 || backup.Settings == nil || backup.Alarms == nil {
		fail(w, 400, errors.New("请选择 MiAir Plus 导出的版本 1 配置文件"))
		return
	}
	p := backup.Settings
	if p.Hostname == nil || p.Port == nil || p.AutoPlay == nil || p.Recover == nil || p.Volume == nil || p.AirPlay == nil || p.AirPlay2 == nil || p.AirPlay2Target == nil || p.FFmpeg == nil || p.AudioID == nil || p.AutoCheckUpdate == nil || p.Speakers == nil || len(p.Speakers) > 256 || len(backup.Alarms) > 32 {
		fail(w, 400, errors.New("配置内容不完整或超过数量限制"))
		return
	}
	ids := map[string]bool{}
	for i := range backup.Alarms {
		alarm := &backup.Alarms[i]
		if err := validateAlarm(*alarm); err != nil {
			fail(w, 400, fmt.Errorf("闹钟 %d：%w", i+1, err))
			return
		}
		if !validAudioID(alarm.ID) || ids[alarm.ID] || (alarm.Audio != "" && !validAudioID(alarm.Audio)) || !filepath.IsLocal(alarm.Sound) || strings.Contains(alarm.Sound, "\\") || len(alarm.Sound) > 1024 || len(alarm.Speaker) > 256 {
			fail(w, 400, errors.New("闹钟标识、音箱或铃声路径无效"))
			return
		}
		ids[alarm.ID] = true
		alarm.Enabled, alarm.SnoozeAt, alarm.LastFire = false, 0, ""
	}
	// Serialize against ringtone preparation and alarm starts before replacing schedules.
	e := a.Manager.alarms
	select {
	case e.importing <- struct{}{}:
		defer func() { <-e.importing }()
	default:
		fail(w, 409, errors.New("正在处理铃声，请稍后导入"))
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, run := range e.runs {
		if run.State == "ringing" || run.State == "starting" || run.State == "stopping" || run.State == "stop_failed" {
			fail(w, 409, errors.New("请先停止正在运行的闹钟再导入配置"))
			return
		}
	}
	skipped := 0
	err := a.Store.Update(func(s *config.State) error {
		for did := range p.Speakers {
			if _, ok := s.Speakers[did]; !ok {
				delete(p.Speakers, did)
				skipped++
			}
		}
		if _, ok := s.Speakers[*p.AirPlay2Target]; *p.AirPlay2Target != "" && !ok {
			*p.AirPlay2Target = ""
		}
		if err := p.apply(s); err != nil {
			return err
		}
		s.Alarms = backup.Alarms
		return nil
	})
	if err != nil {
		fail(w, 400, err)
		return
	}
	e.runs = map[string]*alarmRun{}
	a.Manager.Schedule()
	jsonOut(w, map[string]any{"ok": true, "alarms": len(backup.Alarms), "skipped_speakers": skipped,
		"message": "配置已导入，闹钟保持关闭。请检查音箱并重新选择铃声、保存后启用。"})
}
