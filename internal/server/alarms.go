package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/BearHero520/miair-plus/internal/calendar"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/diagnostics"
	"github.com/BearHero520/miair-plus/internal/dlna"
)

type alarmRun struct {
	Alarm                    config.Alarm
	Token, URI, State, Error string
	Started                  time.Time
	Cancel                   context.CancelFunc
	Renderer                 *dlna.Renderer
}
type AlarmEngine struct {
	mu        sync.Mutex
	store     *config.Store
	manager   *Manager
	runs      map[string]*alarmRun
	importing chan struct{}
	root      string
	calendar  *calendar.Calendar
}

func newAlarms(s *config.Store, m *Manager) *AlarmEngine {
	root := os.Getenv("MIAIR_RINGTONE_DIR")
	if root == "" {
		root = "/var/apps/miair-plus/share/ringtones"
	}
	return &AlarmEngine{store: s, manager: m, runs: map[string]*alarmRun{}, importing: make(chan struct{}, 1), root: root, calendar: calendar.New(s.Directory())}
}
func alarmDue(a config.Alarm, now time.Time, c *calendar.Calendar) (string, bool) {
	if !a.Enabled {
		return "", false
	}
	if a.SnoozeAt > 0 && now.Unix()-a.SnoozeAt < 60 {
		return fmt.Sprint("snooze:", a.SnoozeAt), now.Unix() >= a.SnoozeAt && now.Unix()-a.SnoozeAt < 60
	}
	loc, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return "", false
	}
	local := now.In(loc)
	key := local.Format("2006-01-02 15:04")
	match := a.Days&(1<<uint(local.Weekday())) != 0
	if a.Rule != "" && a.Rule != "weekly" {
		match, _ = c.Match(a.Rule, local)
	}
	return key, match && local.Format("15:04") == a.Time && key != a.LastFire
}
func (e *AlarmEngine) Run(ctx context.Context) {
	go e.refreshCalendars(ctx)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	defer func() {
		e.mu.Lock()
		ids := []string{}
		for id, r := range e.runs {
			if r.State == "ringing" || r.State == "starting" {
				ids = append(ids, id)
			}
		}
		e.mu.Unlock()
		for _, id := range ids {
			stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = e.Stop(stop, id, false)
			cancel()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			for _, a := range e.store.Snapshot().Alarms {
				key, due := alarmDue(a, now, e.calendar)
				if !due {
					continue
				}
				claimed := false
				err := e.store.Update(func(s *config.State) error {
					for i := range s.Alarms {
						b := &s.Alarms[i]
						if b.ID == a.ID {
							k, ok := alarmDue(*b, now, e.calendar)
							if ok && k == key {
								b.LastFire = key
								b.SnoozeAt = 0
								a = *b
								claimed = true
							}
							break
						}
					}
					return nil
				})
				if err != nil {
					diagnostics.Event("error", "闹钟", "无法保存触发记录，已跳过响铃", nil)
					continue
				}
				if claimed {
					if err = e.Start(ctx, a); err != nil {
						e.mu.Lock()
						if current := e.runs[a.ID]; current == nil || current.State == "ended" || current.State == "failed" {
							e.runs[a.ID] = &alarmRun{Alarm: a, State: "failed", Error: err.Error()}
						}
						e.mu.Unlock()
						diagnostics.Event("warn", "闹钟", err.Error(), map[string]string{"闹钟": a.Name})
					}
				}
			}
		}
	}
}

func (e *AlarmEngine) refreshCalendars(ctx context.Context) {
	tick := time.NewTicker(24 * time.Hour)
	defer tick.Stop()
	for {
		// Preserve offline data if the network or next year's announcement is unavailable.
		for _, year := range []int{time.Now().Year(), time.Now().Year() + 1} {
			if ctx.Err() != nil {
				return
			}
			_ = e.calendar.Refresh(ctx, year)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (e *AlarmEngine) Start(ctx context.Context, a config.Alarm) error {
	e.mu.Lock()
	found := false
	for _, current := range e.store.Snapshot().Alarms {
		if current.ID == a.ID {
			a = current
			found = true
			break
		}
	}
	if !found {
		e.mu.Unlock()
		return errors.New("闹钟不存在")
	}
	if !validAudioID(a.Audio) {
		e.mu.Unlock()
		return errors.New("铃声未准备完成")
	}
	if st, err := os.Stat(filepath.Join(e.store.Directory(), "alarm-audio", a.Audio+".mp3")); err != nil || st.Size() == 0 {
		e.mu.Unlock()
		return errors.New("铃声缓存缺失，请重新选择铃声并保存")
	}
	for _, r := range e.runs {
		if (r.State == "ringing" || r.State == "starting" || r.State == "stopping") && r.Alarm.Speaker == a.Speaker {
			e.mu.Unlock()
			return errors.New("该音箱已有闹钟正在响铃")
		}
	}
	renderer := e.manager.Lookup(dlna.UUID(a.Speaker))
	if renderer == nil {
		e.mu.Unlock()
		return errors.New("目标音箱未启用或投送服务未就绪")
	}
	token := config.Random(24)
	host, port, _, _, _ := e.manager.Info()
	if host == "" {
		e.mu.Unlock()
		return errors.New("NAS 地址未就绪")
	}
	runCtx, cancel := context.WithCancel(ctx)
	run := &alarmRun{Alarm: a, Token: token, URI: fmt.Sprintf("http://%s:%d/alarm-audio/%s", host, port, token), State: "starting", Started: time.Now(), Cancel: cancel, Renderer: renderer}
	e.runs[a.ID] = run
	e.mu.Unlock()
	go e.play(runCtx, run)
	return nil
}
func (e *AlarmEngine) play(ctx context.Context, r *alarmRun) {
	err := r.Renderer.StartAlarm(ctx, r.URI, r.Alarm.Volume)
	e.mu.Lock()
	if r.State != "starting" {
		e.mu.Unlock()
		return
	}
	if err != nil {
		r.State = "failed"
		r.Error = err.Error()
		r.Cancel()
		r.Token = ""
		e.mu.Unlock()
		diagnostics.Event("error", "闹钟", "响铃失败", map[string]string{"闹钟": r.Alarm.Name, "原因": err.Error()})
		return
	}
	r.State = "ringing"
	r.Started = time.Now()
	e.mu.Unlock()
	diagnostics.Event("info", "闹钟", "开始响铃", map[string]string{"闹钟": r.Alarm.Name, "音箱": r.Renderer.Config().DisplayName()})
	deadline := time.NewTimer(time.Duration(r.Alarm.Minutes) * time.Minute)
	defer deadline.Stop()
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			stop, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_ = e.Stop(stop, r.Alarm.ID, false)
			cancel()
			return
		case <-poll.C:
			s := r.Renderer.Snapshot()
			if s.URI != r.URI || time.Since(r.Started) > 12*time.Second && (s.State == "STOPPED" || s.State == "PAUSED_PLAYBACK") {
				e.mu.Lock()
				if e.runs[r.Alarm.ID] == r && r.State == "ringing" {
					r.State = "ended"
					r.Token = ""
					r.Cancel()
				}
				e.mu.Unlock()
				diagnostics.Event("info", "闹钟", "响铃已结束，不再自动重播", map[string]string{"闹钟": r.Alarm.Name})
				return
			}
		}
	}
}
func (e *AlarmEngine) Stop(ctx context.Context, id string, snooze bool) error {
	e.mu.Lock()
	r := e.runs[id]
	if r == nil || (r.State != "ringing" && r.State != "starting" && r.State != "stop_failed") {
		e.mu.Unlock()
		return errors.New("闹钟当前没有响铃")
	}
	r.State = "stopping"
	r.Token = ""
	r.Cancel()
	e.mu.Unlock()
	err := r.Renderer.StopURI(ctx, r.URI)
	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil {
		r.State = "stop_failed"
		r.Error = "停止命令未确认，请说“小爱同学，停止播放”或按音箱暂停键；音频最长为设定时长"
		diagnostics.Event("error", "闹钟", "停止响铃失败", map[string]string{"原因": err.Error()})
		return errors.New(r.Error)
	}
	r.State = "ended"
	r.Error = ""
	err = e.store.Update(func(s *config.State) error {
		for i := range s.Alarms {
			if s.Alarms[i].ID == id {
				s.Alarms[i].SnoozeAt = 0
				if snooze {
					s.Alarms[i].SnoozeAt = time.Now().Add(5 * time.Minute).Unix()
					s.Alarms[i].Enabled = true
				}
				break
			}
		}
		return nil
	})
	diagnostics.Event("info", "闹钟", "已停止响铃", map[string]string{"闹钟": r.Alarm.Name, "稍后提醒": fmt.Sprint(snooze)})
	return err
}
func (e *AlarmEngine) List() any {
	e.mu.Lock()
	defer e.mu.Unlock()
	list := []map[string]any{}
	for _, a := range e.store.Snapshot().Alarms {
		state, detail := "idle", ""
		if r := e.runs[a.ID]; r != nil {
			state, detail = r.State, r.Error
		}
		list = append(list, map[string]any{"alarm": a, "state": state, "error": detail, "next": e.next(a), "calendar_ready": e.calendarReady(a)})
	}
	return map[string]any{"alarms": list, "ringtone_directory": e.root, "server_time": time.Now().Format(time.RFC3339), "calendar": e.calendar.Info()}
}
func (e *AlarmEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/alarm-audio/")
	e.mu.Lock()
	file := ""
	for _, run := range e.runs {
		if token != "" && run.Token == token && (run.State == "starting" || run.State == "ringing") && time.Since(run.Started) < time.Duration(run.Alarm.Minutes+1)*time.Minute {
			file = run.Alarm.Audio
			break
		}
	}
	e.mu.Unlock()
	if !validAudioID(file) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(e.store.Directory(), "alarm-audio", file+".mp3"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "alarm.mp3", st.ModTime(), f)
}
func validAudioID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

var alarmFormats = map[string]string{".mp3": "mp3", ".wav": "wav", ".flac": "flac", ".ogg": "ogg", ".aac": "aac"}

func (e *AlarmEngine) Files(path string) (any, error) {
	root, err := os.OpenRoot(e.root)
	if err != nil {
		return nil, errors.New("铃声目录不可访问，请在飞牛文件管理中查看 miair-plus/ringtones")
	}
	defer root.Close()
	if path == "" {
		path = "."
	}
	dir, err := root.Open(path)
	if err != nil {
		return nil, errors.New("目录不存在或不在铃声目录中")
	}
	defer dir.Close()
	entries, err := dir.ReadDir(2001)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > 2000 {
		return nil, errors.New("单个目录超过 2000 项，请分子目录整理")
	}
	files := []map[string]any{}
	for _, v := range entries {
		if strings.HasPrefix(v.Name(), ".") || v.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !v.IsDir() && alarmFormats[strings.ToLower(filepath.Ext(v.Name()))] == "" {
			continue
		}
		files = append(files, map[string]any{"name": v.Name(), "path": filepath.ToSlash(filepath.Join(path, v.Name())), "directory": v.IsDir()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i]["name"].(string) < files[j]["name"].(string) })
	return map[string]any{"files": files, "path": path}, nil
}
func validateAlarm(a config.Alarm) error {
	parsed, err := time.Parse("15:04", a.Time)
	if err != nil || parsed.Format("15:04") != a.Time {
		return errors.New("请输入 HH:mm 时间")
	}
	if _, err = time.LoadLocation(a.Timezone); err != nil {
		return errors.New("时区无效")
	}
	if a.Rule != "" && a.Rule != "weekly" && a.Rule != "workday" && a.Rule != "restday" && a.Rule != "holiday" {
		return errors.New("闹钟重复规则无效")
	}
	if a.Rule != "" && a.Rule != "weekly" && a.Timezone != "Asia/Shanghai" {
		return errors.New("法定节假日规则请使用北京时间 Asia/Shanghai")
	}
	if strings.TrimSpace(a.Name) == "" || len(a.Name) > 100 || a.Days < 1 || a.Days > 127 || a.Volume < 1 || a.Volume > 100 || a.Minutes < 1 || a.Minutes > 10 {
		return errors.New("请填写名称、重复日期、1–100 音量和 1–10 分钟最长时长")
	}
	return nil
}
func (e *AlarmEngine) Save(ctx context.Context, a config.Alarm) error {
	if err := validateAlarm(a); err != nil {
		return err
	}
	if sp, ok := e.store.Snapshot().Speakers[a.Speaker]; !ok || !sp.Enabled {
		return errors.New("请选择已启用的音箱")
	}
	select {
	case e.importing <- struct{}{}:
		defer func() { <-e.importing }()
	default:
		return errors.New("正在处理铃声，请稍后再试")
	}
	e.mu.Lock()
	if r := e.runs[a.ID]; r != nil && (r.State == "ringing" || r.State == "starting" || r.State == "stopping" || r.State == "stop_failed") {
		e.mu.Unlock()
		return errors.New("请先停止闹钟再编辑")
	}
	e.mu.Unlock()
	old := config.Alarm{}
	for _, v := range e.store.Snapshot().Alarms {
		if v.ID == a.ID {
			old = v
			break
		}
	}
	if a.ID != "" && old.ID == "" {
		return errors.New("闹钟不存在")
	}
	if a.ID == "" {
		a.ID = config.Random(16)
		if len(e.store.Snapshot().Alarms) >= 32 {
			return errors.New("最多可保存 32 个闹钟")
		}
	}
	a.Audio = old.Audio
	a.LastFire = old.LastFire
	a.SnoozeAt = 0
	st, cacheErr := os.Stat(filepath.Join(e.store.Directory(), "alarm-audio", a.Audio+".mp3"))
	if old.Sound != a.Sound || old.Minutes != a.Minutes || !validAudioID(a.Audio) || cacheErr != nil || st.Size() == 0 {
		audio, err := e.importSound(ctx, a.Sound, a.Minutes)
		if err != nil {
			return err
		}
		a.Audio = audio
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.runs[a.ID]; r != nil && (r.State == "ringing" || r.State == "starting" || r.State == "stopping" || r.State == "stop_failed") {
		if a.Audio != old.Audio {
			os.Remove(filepath.Join(e.store.Directory(), "alarm-audio", a.Audio+".mp3"))
		}
		return errors.New("闹钟已开始响铃，请先停止再保存")
	}
	err := e.store.Update(func(s *config.State) error {
		for i := range s.Alarms {
			if s.Alarms[i].ID == a.ID {
				a.LastFire = s.Alarms[i].LastFire
				s.Alarms[i] = a
				return nil
			}
		}
		s.Alarms = append(s.Alarms, a)
		return nil
	})
	if err != nil && a.Audio != old.Audio {
		os.Remove(filepath.Join(e.store.Directory(), "alarm-audio", a.Audio+".mp3"))
	}
	if err == nil && old.Audio != a.Audio && validAudioID(old.Audio) {
		os.Remove(filepath.Join(e.store.Directory(), "alarm-audio", old.Audio+".mp3"))
	}
	return err
}
func (e *AlarmEngine) importSound(ctx context.Context, path string, minutes int) (string, error) {
	format := alarmFormats[strings.ToLower(filepath.Ext(path))]
	if format == "" {
		return "", errors.New("支持 MP3、WAV、FLAC、OGG、AAC 铃声")
	}
	root, err := os.OpenRoot(e.root)
	if err != nil {
		return "", errors.New("NAS 铃声目录不可访问")
	}
	defer root.Close()
	input, err := root.Open(path)
	if err != nil {
		return "", errors.New("无法读取所选 NAS 音频")
	}
	defer input.Close()
	st, err := input.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 100<<20 || st.Size() == 0 {
		return "", errors.New("请选择 100 MB 以内的音频文件")
	}
	ffmpeg := findFFmpeg(e.store.Snapshot().FFmpeg)
	if ffmpeg == "" {
		return "", errors.New("FFmpeg 不可用")
	}
	dir := filepath.Join(e.store.Directory(), "alarm-audio")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	id := config.Random(16)
	out := filepath.Join(dir, id+".mp3")
	operation, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	command := exec.CommandContext(operation, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "pipe", "-f", format, "-i", "pipe:0", "-t", fmt.Sprint(minutes*60), "-vn", "-map", "0:a:0", "-ac", "2", "-ar", "44100", "-c:a", "libmp3lame", "-b:a", "128k", "-f", "mp3", out)
	command.Stdin = io.LimitReader(input, 100<<20)
	if err = command.Run(); err != nil {
		os.Remove(out)
		return "", errors.New("铃声转码失败或超时，请选择有效音频文件")
	}
	if err = os.Chmod(out, 0600); err != nil {
		os.Remove(out)
		return "", err
	}
	return id, nil
}
func (e *AlarmEngine) Delete(id string) error {
	select {
	case e.importing <- struct{}{}:
		defer func() { <-e.importing }()
	default:
		return errors.New("正在保存，请稍后重试")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.runs[id]; r != nil && (r.State == "ringing" || r.State == "starting" || r.State == "stopping" || r.State == "stop_failed") {
		return errors.New("请先停止闹钟")
	}
	audio := ""
	err := e.store.Update(func(s *config.State) error {
		for i, a := range s.Alarms {
			if a.ID == id {
				audio = a.Audio
				s.Alarms = append(s.Alarms[:i], s.Alarms[i+1:]...)
				return nil
			}
		}
		return errors.New("闹钟不存在")
	})
	if err == nil {
		delete(e.runs, id)
		if validAudioID(audio) {
			os.Remove(filepath.Join(e.store.Directory(), "alarm-audio", audio+".mp3"))
		}
	}
	return err
}

func (e *AlarmEngine) calendarReady(a config.Alarm) bool {
	if a.Rule == "" || a.Rule == "weekly" {
		return true
	}
	loc, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return false
	}
	_, ok := e.calendar.Match(a.Rule, time.Now().In(loc))
	return ok
}
func (e *AlarmEngine) next(a config.Alarm) []string {
	result := []string{}
	if !a.Enabled {
		return result
	}
	loc, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return result
	}
	now := time.Now().In(loc)
	if a.SnoozeAt > now.Unix() {
		result = append(result, time.Unix(a.SnoozeAt, 0).In(loc).Format("2006-01-02 15:04"))
		return result
	}
	for i := 0; i < 370 && len(result) < 3; i++ {
		day := now.AddDate(0, 0, i)
		at, err := time.ParseInLocation("2006-01-02 15:04", day.Format("2006-01-02")+" "+a.Time, loc)
		if err != nil || !at.After(now) {
			continue
		}
		if _, ok := alarmDue(a, at, e.calendar); ok {
			result = append(result, at.Format("2006-01-02 15:04"))
		}
	}
	return result
}
