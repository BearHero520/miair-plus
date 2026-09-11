package server

import (
	"errors"
	"github.com/BearHero520/miair-plus/internal/config"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (a *API) alarmAPI(w http.ResponseWriter, r *http.Request, path string) {
	e := a.Manager.alarms
	var err error
	switch {
	case path == "alarms/upload" && r.Method == "POST":
		a.uploadAlarm(w, r)
		return
	case path == "alarms/import-nas" && r.Method == "POST":
		var p struct {
			Path string `json:"path"`
		}
		if !decode(w, r, &p) {
			return
		}
		uid, userErr := gatewayUID(r)
		if userErr != nil {
			fail(w, 403, userErr)
			return
		}
		if !allowedNASAudio(p.Path) {
			fail(w, 400, errors.New("请选择 NAS 存储空间中的音频文件"))
			return
		}
		resolved, pathErr := filepath.EvalSymlinks(p.Path)
		if pathErr != nil || !allowedNASAudio(resolved) {
			fail(w, 400, errors.New("所选文件路径不可访问"))
			return
		}
		if aclErr := checkFnosRead(r.Context(), fnosHTTP, uid, p.Path); aclErr != nil {
			fail(w, 403, aclErr)
			return
		}
		if resolved != p.Path {
			if aclErr := checkFnosRead(r.Context(), fnosHTTP, uid, resolved); aclErr != nil {
				fail(w, 403, aclErr)
				return
			}
		}
		var sound string
		sound, err = e.ImportNAS(resolved)
		if err == nil {
			jsonOut(w, map[string]string{"sound": sound})
			return
		}
	case path == "alarms/calendar" && r.Method == "POST":
		var p struct {
			Year int `json:"year"`
		}
		if !decode(w, r, &p) {
			return
		}
		if p.Year == 0 {
			p.Year = time.Now().Year()
		}
		err = e.calendar.Refresh(r.Context(), p.Year)
	case path == "alarms" && r.Method == "GET":
		jsonOut(w, e.List())
		return
	case path == "alarms/files" && r.Method == "GET":
		var result any
		result, err = e.Files(r.URL.Query().Get("path"))
		if err == nil {
			jsonOut(w, result)
			return
		}
	case path == "alarms" && r.Method == "POST":
		var p config.Alarm
		if !decode(w, r, &p) {
			return
		}
		err = e.Save(r.Context(), p)
	case strings.HasPrefix(path, "alarms/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(path, "alarms/"), "/")
		switch {
		case r.Method == "DELETE" && action == "":
			err = e.Delete(id)
		case r.Method == "POST" && (action == "stop" || action == "snooze"):
			err = e.Stop(r.Context(), id, action == "snooze")
		case r.Method == "POST" && action == "test":
			err = errors.New("闹钟不存在")
			for _, alarm := range a.Store.Snapshot().Alarms {
				if alarm.ID == id {
					err = e.Start(a.ctx, alarm)
					break
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, 400, err)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}
