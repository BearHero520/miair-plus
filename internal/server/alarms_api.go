package server

import (
	"errors"
	"github.com/BearHero520/miair-plus/internal/config"
	"net/http"
	"strings"
	"time"
)

func (a *API) alarmAPI(w http.ResponseWriter, r *http.Request, path string) {
	e := a.Manager.alarms
	var err error
	switch {
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
