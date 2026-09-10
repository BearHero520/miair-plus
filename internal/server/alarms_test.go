package server

import (
	"context"
	"github.com/BearHero520/miair-plus/internal/calendar"
	"github.com/BearHero520/miair-plus/internal/config"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAlarmDueRestartSnoozeAndTimezone(t *testing.T) {
	c := calendar.New(t.TempDir())
	a := config.Alarm{Enabled: true, Time: "07:00", Timezone: "Asia/Shanghai", Rule: "workday", Days: 62}
	at := time.Date(2026, 9, 19, 23, 0, 5, 0, time.UTC) // Sunday makeup workday, 07:00 China.
	key, due := alarmDue(a, at, c)
	if !due {
		t.Fatal("makeup Sunday did not fire")
	}
	a.LastFire = key
	if _, due = alarmDue(a, at.Add(20*time.Second), c); due {
		t.Fatal("duplicate after persisted claim")
	}
	a.SnoozeAt = at.Add(5 * time.Minute).Unix()
	if _, due = alarmDue(a, at, c); due {
		t.Fatal("early snooze")
	}
	if _, due = alarmDue(a, at.Add(5*time.Minute), c); !due {
		t.Fatal("snooze not due")
	}
	a.SnoozeAt = at.Add(-2 * time.Hour).Unix()
	a.LastFire = ""
	if _, due = alarmDue(a, at, c); !due {
		t.Fatal("missed snooze disabled ordinary schedule")
	}
	a.Rule = "holiday"
	if _, due = alarmDue(a, at, c); due {
		t.Fatal("makeup day treated as holiday")
	}
	a.Rule = "workday"
	if _, due = alarmDue(a, time.Date(2027, 1, 4, 23, 0, 0, 0, time.UTC), c); due {
		t.Fatal("unknown calendar fired")
	}
	a.Enabled = false
	if _, due = alarmDue(a, at, c); due {
		t.Fatal("disabled alarm fired")
	}
}
func TestAlarmFileConfinement(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "sounds")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(dir, "private.mp3"), []byte("private"), 0600)
	os.WriteFile(filepath.Join(root, "ok.mp3"), []byte("mp3"), 0600)
	s, _ := config.Open(t.TempDir())
	e := newAlarms(s, nil)
	e.root = root
	if _, err := e.Files(".."); err == nil {
		t.Fatal("directory escape")
	}
	if _, err := e.importSound(context.Background(), "../private.mp3", 1); err == nil {
		t.Fatal("file escape")
	}
	if err := os.Symlink(filepath.Join(dir, "private.mp3"), filepath.Join(root, "escape.mp3")); err == nil {
		if _, err = e.importSound(context.Background(), "escape.mp3", 1); err == nil {
			t.Fatal("symlink escape")
		}
	}
	request := httptest.NewRequest("GET", "http://nas/alarm-audio/unknown", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, request)
	if w.Code != 404 {
		t.Fatal("unsigned audio served")
	}
}
func TestAlarmRealTranscodeAndRevokedURL(t *testing.T) {
	ff := os.Getenv("TEST_FFMPEG")
	if ff == "" {
		ff, _ = exec.LookPath("ffmpeg")
	}
	if ff == "" {
		t.Skip("FFmpeg unavailable")
	}
	root := t.TempDir()
	source := filepath.Join(root, "wake.wav")
	if b, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.4", source).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, b)
	}
	s, _ := config.Open(t.TempDir())
	s.Update(func(st *config.State) error { st.FFmpeg = ff; return nil })
	e := newAlarms(s, nil)
	e.root = root
	id, err := e.importSound(context.Background(), "wake.wav", 1)
	if err != nil {
		t.Fatal(err)
	}
	e.runs["a"] = &alarmRun{Alarm: config.Alarm{Audio: id, Minutes: 1}, Token: "test-token", State: "ringing", Started: time.Now()}
	req := httptest.NewRequest("GET", "http://nas/alarm-audio/test-token", nil)
	req.Header.Set("Range", "bytes=0-31")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.Len() != 32 || !strings.Contains(w.Header().Get("Content-Type"), "audio/mpeg") {
		t.Fatalf("range: %d %d", w.Code, w.Body.Len())
	}
	e.runs["a"].Token = ""
	w = httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal("revoked token still works")
	}
}
