package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/diagnostics"
	"github.com/BearHero520/miair-plus/internal/web"
	"github.com/BearHero520/miair-plus/internal/xiaomi"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const Version = "2.0.0-alpha.6"

type API struct {
	Store    *config.Store
	Manager  *Manager
	Client   *xiaomi.Client
	QR       *xiaomi.QRManager
	ctx      context.Context
	started  time.Time
	mu       sync.Mutex
	attempts map[string][]time.Time
	journal  *diagnostics.Journal
	logins   []map[string]any
	level    string
	wsSlots  chan struct{}
}

func NewAPI(ctx context.Context, s *config.Store, m *Manager, c *xiaomi.Client) *API {
	return &API{Store: s, Manager: m, Client: c, QR: xiaomi.NewQR(ctx, c), ctx: ctx, started: time.Now(), attempts: map[string][]time.Time{}, journal: diagnostics.New(s.Directory()), logins: []map[string]any{}, level: "info", wsSlots: make(chan struct{}, 16)}
}
func (a *API) Write(b []byte) (int, error) { return a.journal.Write(b) }
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"detail": err.Error(), "error": err.Error(), "success": false})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := d.Decode(v); err != nil {
		fail(w, 400, errors.New("请求格式错误"))
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		fail(w, 400, errors.New("请求包含多余内容"))
		return false
	}
	return true
}
func (a *API) token() string {
	s := a.Store.Snapshot()
	payload := fmt.Sprintf("%d:%d", time.Now().Add(24*time.Hour).Unix(), s.AuthRevision)
	h := hmac.New(sha256.New, []byte(s.Secret))
	h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (a *API) valid(token string) bool {
	s := a.Store.Snapshot()
	if s.PasswordHash == "" {
		return false
	}
	p, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	b, e := base64.RawURLEncoding.DecodeString(p)
	signed, e2 := base64.RawURLEncoding.DecodeString(sig)
	if e != nil || e2 != nil {
		return false
	}
	h := hmac.New(sha256.New, []byte(s.Secret))
	h.Write(b)
	if !hmac.Equal(h.Sum(nil), signed) {
		return false
	}
	expiry, revision, ok := strings.Cut(string(b), ":")
	n, e := strconv.ParseInt(expiry, 10, 64)
	return ok && e == nil && n > time.Now().Unix() && revision == strconv.Itoa(s.AuthRevision)
}
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/", a.api)
	mux.Handle("/", web.Handler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(u.Host, r.Host) {
				fail(w, 403, errors.New("跨站请求被拒绝"))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *API) allowed(r *http.Request) bool {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, attempts := range a.attempts {
		kept := attempts[:0]
		for _, t := range attempts {
			if now.Sub(t) < time.Minute {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(a.attempts, k)
		} else {
			a.attempts[k] = kept
		}
	}
	if len(a.attempts) >= 1024 || len(a.attempts[ip]) >= 10 {
		return false
	}
	a.attempts[ip] = append(a.attempts[ip], now)
	return true
}
func (a *API) login(w http.ResponseWriter, r *http.Request, setup bool) {
	if !a.allowed(r) {
		fail(w, 429, errors.New("尝试过于频繁，请一分钟后重试"))
		return
	}
	var req struct{ Username, Password string }
	if !decode(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Password) < 8 || len(req.Password) > 72 || len(req.Username) < 1 || len(req.Username) > 64 {
		fail(w, 400, errors.New("用户名需 1–64 字节，密码需 8–72 字节"))
		return
	}
	s := a.Store.Snapshot()
	if setup {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err == nil {
			err = a.Store.Update(func(s *config.State) error {
				if s.PasswordHash != "" {
					return errors.New("管理员已初始化")
				}
				s.Username = req.Username
				s.PasswordHash = string(hash)
				s.AuthRevision++
				return nil
			})
		}
		if err != nil {
			fail(w, 409, err)
			return
		}
	} else {
		valid := s.Username == req.Username && bcrypt.CompareHashAndPassword([]byte(s.PasswordHash), []byte(req.Password)) == nil
		a.mu.Lock()
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		a.logins = append([]map[string]any{{"username": req.Username, "ip": ip, "success": valid, "time": time.Now().Format(time.RFC3339)}}, a.logins...)
		if len(a.logins) > 100 {
			a.logins = a.logins[:100]
		}
		a.mu.Unlock()
		if !valid {
			fail(w, 401, errors.New("账号或密码错误"))
			return
		}
	}
	jsonOut(w, map[string]string{"access_token": a.token(), "token_type": "bearer"})
}
func (a *API) api(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if path == "login/status" && r.Method == "GET" {
		jsonOut(w, map[string]bool{"initialized": a.Store.Snapshot().PasswordHash != ""})
		return
	}
	if path == "login/setup" && r.Method == "POST" {
		a.login(w, r, true)
		return
	}
	if path == "login" && r.Method == "POST" {
		a.login(w, r, false)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if path == "ws" {
		token = r.URL.Query().Get("token")
	}
	if !a.valid(token) {
		fail(w, 401, errors.New("请重新登录"))
		return
	}
	if strings.HasPrefix(path, "alarms") {
		a.alarmAPI(w, r, path)
		return
	}
	s := a.Store.Snapshot()
	switch r.Method + " " + path {
	case "GET me":
		jsonOut(w, map[string]string{"username": s.Username})
	case "POST login/password":
		var p struct {
			Old string `json:"old_password"`
			New string `json:"new_password"`
		}
		if !decode(w, r, &p) {
			return
		}
		if len(p.New) < 8 || len(p.New) > 72 || bcrypt.CompareHashAndPassword([]byte(s.PasswordHash), []byte(p.Old)) != nil {
			fail(w, 400, errors.New("原密码错误或新密码长度不符合要求"))
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(p.New), bcrypt.DefaultCost)
		err := a.Store.Update(func(st *config.State) error {
			if st.PasswordHash != s.PasswordHash {
				return errors.New("密码已更改，请重新登录")
			}
			st.PasswordHash = string(hash)
			st.AuthRevision++
			return nil
		})
		if err != nil {
			fail(w, 400, err)
			return
		}
		jsonOut(w, map[string]bool{"ok": true})
	case "GET login/logs":
		a.mu.Lock()
		logs := append([]map[string]any{}, a.logins...)
		a.mu.Unlock()
		jsonOut(w, map[string]any{"logs": logs})
	case "GET status":
		jsonOut(w, a.status())
	case "GET speakers":
		jsonOut(w, a.Manager.Snapshots())
	case "GET settings":
		jsonOut(w, a.settings())
	case "POST settings":
		a.saveSettings(w, r)
	case "GET devices":
		a.devices(w, r)
	case "GET account/status":
		status := "offline"
		if s.Xiaomi.ServiceToken != "" {
			status = "healthy"
		}
		jsonOut(w, map[string]any{"user_id": s.Xiaomi.UserID, "logged_in": s.Xiaomi.ServiceToken != "", "status": status, "service_token_remaining_hours": nil, "has_password_fallback": false, "token_refresh_running": s.Xiaomi.PassToken != "", "has_account": s.Xiaomi.UserID != ""})
	case "DELETE account":
		a.QR.CancelAll()
		err := a.Client.Logout()
		if err != nil {
			fail(w, 500, err)
			return
		}
		a.Manager.Schedule()
		jsonOut(w, map[string]any{"ok": true, "message": "账号已移除"})
	case "POST account/login", "POST account/token":
		var p struct {
			Username, Password string
			UserID             string `json:"user_id"`
			PassToken          string `json:"pass_token"`
		}
		if !decode(w, r, &p) {
			return
		}
		cookie := ""
		if path == "account/token" {
			cookie = "userId=" + p.UserID + "; passToken=" + p.PassToken
		}
		if err := a.Client.Login(r.Context(), p.Username, p.Password, cookie); err != nil {
			diagnostics.Event("error", "小米账号", "账号连接失败", map[string]string{"原因": err.Error()})
			jsonOut(w, map[string]any{"success": false, "state": "failed", "error": err.Error(), "message": err.Error()})
			return
		}
		a.Manager.Schedule()
		jsonOut(w, map[string]any{"success": true, "state": "success", "message": "小米账号已连接"})
		diagnostics.Event("info", "小米账号", "账号已连接", nil)
	case "POST account/qrcode":
		qr, err := a.QR.Start(r.Context())
		if err != nil {
			diagnostics.Event("error", "小米账号", "登录二维码获取失败", map[string]string{"原因": err.Error()})
			fail(w, 502, err)
			return
		}
		jsonOut(w, map[string]any{"success": true, "session_id": qr.ID, "qrcode_url": qr.Image, "login_url": qr.LoginURL})
	case "GET account/qrcode/poll":
		qr, ok := a.QR.Get(r.URL.Query().Get("session_id"))
		if !ok {
			fail(w, 404, errors.New("扫码会话不存在"))
			return
		}
		if qr.State == "confirmed" {
			a.Manager.Schedule()
		}
		jsonOut(w, map[string]any{"success": true, "state": qr.State, "message": qr.Message})
	case "GET logs":
		entries, failure := a.journal.Snapshot()
		jsonOut(w, map[string]any{"entries": entries, "lines": diagnostics.Lines(entries), "capacity": diagnostics.Capacity, "storage_error": failure})
	case "GET diagnostics", "GET diagnostics/download":
		result := a.diagnosticReport()
		if path == "diagnostics/download" {
			w.Header().Set("Content-Disposition", "attachment; filename=miair-plus-diagnostics.json")
		}
		jsonOut(w, result)
	case "GET logs/download":
		entries, _ := a.journal.Snapshot()
		w.Header().Set("Content-Disposition", "attachment; filename=miair-plus.log")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, strings.Join(diagnostics.Lines(entries), "\n"))
	case "GET logs/level":
		jsonOut(w, map[string]string{"level": "info"})
	case "POST system/restart_services":
		a.Manager.Schedule()
		jsonOut(w, map[string]any{"ok": true, "message": "已安排服务检查"})
	case "GET system/check_update":
		jsonOut(w, map[string]any{"current": Version, "latest": nil, "update_available": false, "release_url": "https://github.com/BearHero520/miair-plus/releases", "error": "请到 GitHub Releases 查看预览版更新"})
	case "GET ws":
		a.websocket(w, r, token)
	default:
		if r.Method == "POST" && strings.HasPrefix(path, "speakers/") && strings.HasSuffix(path, "/rename") {
			did := strings.TrimSuffix(strings.TrimPrefix(path, "speakers/"), "/rename")
			var p struct {
				Name string `json:"dlna_name"`
			}
			if !decode(w, r, &p) {
				return
			}
			p.Name = strings.TrimSpace(p.Name)
			if !validName(p.Name) {
				fail(w, 400, errors.New("投送名称需 1–40 个字符且不能包含控制字符"))
				return
			}
			err := a.Store.Update(func(st *config.State) error {
				sp, ok := st.Speakers[did]
				if !ok {
					return errors.New("音箱不存在")
				}
				sp.DLNAName = p.Name
				st.Speakers[did] = sp
				return nil
			})
			if err != nil {
				fail(w, 400, err)
				return
			}
			a.Manager.Schedule()
			jsonOut(w, map[string]any{"ok": true, "dlna_name": p.Name, "name_sync": "pending", "message": "已保存，后台更新发现名称"})
			return
		}
		http.NotFound(w, r)
	}
}
func validName(s string) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) < 1 || utf8.RuneCountInString(s) > 40 {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func (a *API) status() map[string]any {
	host, port, count, running, codec := a.Manager.Info()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	s := a.Store.Snapshot()
	return map[string]any{"version": Version, "engine_version": runtime.Version(), "go_version": runtime.Version(), "arch": runtime.GOARCH, "dlna_running": running, "renderers_count": count, "hostname": host, "dlna_port": port, "has_account": s.Xiaomi.UserID != "", "logged_in": s.Xiaomi.ServiceToken != "", "uptime_seconds": time.Since(a.started).Seconds(), "memory_mb": float64(memory.Sys) / (1 << 20), "memory_source": "go_runtime", "airplay_enabled": s.AirPlay, "airplay2_enabled": s.AirPlay2, "airplay2_target": s.AirPlay2Target, "ffmpeg_available": codec}
}
func (a *API) settings() map[string]any {
	ap2Running, ap2Error := a.Manager.AirPlay2Info()
	ffmpegResolved, ffmpegSource := a.Manager.FFmpegInfo()
	s := a.Store.Snapshot()
	_, _, n, running, codec := a.Manager.Info()
	return map[string]any{"version": Version, "engine_version": runtime.Version(), "hostname": s.Hostname, "dlna_port": s.DLNAPort, "auto_play_on_set_uri": s.AutoPlay, "auto_restart": s.AutoRecover, "default_volume": s.DefaultVolume, "default_audio_id": s.AudioID, "airplay_enabled": s.AirPlay, "airplay2_enabled": s.AirPlay2, "airplay2_target": s.AirPlay2Target, "ffmpeg_path": s.FFmpeg, "ffmpeg_resolved": ffmpegResolved, "ffmpeg_source": ffmpegSource, "ffmpeg_available": codec, "has_account": s.Xiaomi.UserID != "", "speakers": s.Speakers, "airplay2_running": ap2Running, "airplay2_error": ap2Error, "mi_did": "", "cookie": "", "dlna_running": running, "renderers_count": n, "need_use_play_music_api": []string{}}
}

type settingsPatch struct {
	Hostname       *string `json:"hostname"`
	Port           *int    `json:"dlna_port"`
	AutoPlay       *bool   `json:"auto_play_on_set_uri"`
	Recover        *bool   `json:"auto_restart"`
	Volume         *int    `json:"default_volume"`
	AirPlay        *bool   `json:"airplay_enabled"`
	AirPlay2       *bool   `json:"airplay2_enabled"`
	AirPlay2Target *string `json:"airplay2_target"`
	FFmpeg         *string `json:"ffmpeg_path"`
	AudioID        *string `json:"default_audio_id"`
	Speakers       map[string]struct {
		Enabled       *bool   `json:"enabled"`
		Name          *string `json:"dlna_name"`
		Compatibility *bool   `json:"compatibility_mode"`
	} `json:"speakers"`
}

func (a *API) saveSettings(w http.ResponseWriter, r *http.Request) {
	var p settingsPatch
	if !decode(w, r, &p) {
		return
	}
	err := a.Store.Update(func(s *config.State) error {
		if p.Hostname != nil {
			if *p.Hostname != "" {
				if _, err := LANHost(*p.Hostname); err != nil {
					return err
				}
			}
			s.Hostname = *p.Hostname
		}
		if p.Port != nil {
			if *p.Port < 1024 || *p.Port > 65535 {
				return errors.New("端口范围为 1024–65535")
			}
			s.DLNAPort = *p.Port
		}
		if p.Volume != nil {
			if *p.Volume < 0 || *p.Volume > 100 {
				return errors.New("音量范围为 0–100")
			}
			s.DefaultVolume = *p.Volume
		}
		if p.AutoPlay != nil {
			s.AutoPlay = *p.AutoPlay
		}
		if p.Recover != nil {
			s.AutoRecover = *p.Recover
		}
		if p.AirPlay != nil {
			s.AirPlay = *p.AirPlay
		}
		if p.AirPlay2 != nil {
			s.AirPlay2 = *p.AirPlay2
		}
		if p.AirPlay2Target != nil {
			if *p.AirPlay2Target != "" {
				if _, ok := s.Speakers[*p.AirPlay2Target]; !ok {
					return errors.New("AirPlay 2 目标音箱不存在")
				}
			}
			s.AirPlay2Target = *p.AirPlay2Target
		}
		if p.FFmpeg != nil {
			s.FFmpeg = *p.FFmpeg
		}
		if p.AudioID != nil {
			if len(*p.AudioID) > 64 {
				return errors.New("audioID 过长")
			}
			s.AudioID = *p.AudioID
		}
		for did, change := range p.Speakers {
			sp, ok := s.Speakers[did]
			if !ok {
				return errors.New("请先从小米账号刷新设备列表")
			}
			if change.Enabled != nil {
				sp.Enabled = *change.Enabled
			}
			if change.Compatibility != nil {
				sp.Compatibility = *change.Compatibility
			}
			if change.Name != nil {
				if !validName(*change.Name) {
					return errors.New("名称格式错误")
				}
				sp.DLNAName = *change.Name
			}
			s.Speakers[did] = sp
		}
		return nil
	})
	if err != nil {
		fail(w, 400, err)
		return
	}
	a.Manager.Schedule()
	diagnostics.Event("info", "设置", "设置已保存，后台应用中", nil)
	jsonOut(w, map[string]any{"ok": true, "message": "已保存，服务将在后台更新"})
}
func (a *API) devices(w http.ResponseWriter, r *http.Request) {
	devices, err := a.Client.Devices(r.Context())
	if err != nil {
		jsonOut(w, map[string]any{"devices": []any{}, "error": err.Error()})
		diagnostics.Event("error", "小米账号", "读取音箱列表失败", map[string]string{"原因": err.Error()})
		return
	}
	err = a.Store.Update(func(s *config.State) error {
		for _, sp := range devices {
			if old, ok := s.Speakers[sp.DID]; ok {
				sp.DLNAName = old.DLNAName
				sp.Enabled = old.Enabled
				sp.Compatibility = old.Compatibility
			}
			s.Speakers[sp.DID] = sp
		}
		return nil
	})
	if err != nil {
		fail(w, 500, err)
		return
	}
	out := []map[string]string{}
	for _, d := range devices {
		out = append(out, map[string]string{"miotDID": d.DID, "name": d.Name, "hardware": d.Hardware})
	}
	a.Manager.Schedule()
	diagnostics.Event("info", "小米账号", "音箱列表已刷新", map[string]string{"数量": fmt.Sprint(len(out))})
	jsonOut(w, map[string]any{"devices": out})
}
func (a *API) websocket(w http.ResponseWriter, r *http.Request, token string) {
	select {
	case a.wsSlots <- struct{}{}:
		defer func() { <-a.wsSlots }()
	default:
		fail(w, 503, errors.New("状态连接过多"))
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, e := url.Parse(origin)
		return e == nil && strings.EqualFold(u.Host, r.Host)
	}}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1024)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if !a.valid(token) {
			conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4401, "session expired"), time.Now().Add(time.Second))
			return
		}
		_, _, n, running, _ := a.Manager.Info()
		conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		if err = conn.WriteJSON(map[string]any{"type": "status", "dlna_running": running, "renderers_count": n, "speakers": a.Manager.Snapshots()}); err != nil {
			return
		}
		select {
		case <-a.ctx.Done():
			return
		case <-done:
			return
		case <-tick.C:
		}
	}
}

var _ = log.Print
