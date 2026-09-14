package server

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BearHero520/miair-plus/internal/config"
)

func TestAlarmPlaybackValidation(t *testing.T) {
	base := config.Alarm{Name: "Wake", Time: "07:00", Timezone: "Asia/Shanghai", Days: 127, Volume: 30, Minutes: 1}
	for _, mode := range []string{"", "timed", "full", "repeat", "invalid"} {
		for _, count := range []int{0, 1, 2, 20, 21} {
			a := base
			a.Playback, a.Repeats = mode, count
			valid := mode != "invalid" && (mode != "repeat" || count >= 2 && count <= 20)
			if (validateAlarm(a) == nil) != valid {
				t.Fatalf("mode=%s count=%d", mode, count)
			}
		}
	}
	base.Playback, base.Minutes = "full", 0
	if err := validateAlarm(base); err != nil {
		t.Fatal("full track should not need minutes:", err)
	}
	base.Playback = "timed"
	if validateAlarm(base) == nil {
		t.Fatal("timed mode accepted zero minutes")
	}
}

func TestAlarmPlaybackCacheAndDuration(t *testing.T) {
	ff := os.Getenv("TEST_FFMPEG")
	if ff == "" {
		ff, _ = exec.LookPath("ffmpeg")
	}
	if ff == "" {
		t.Skip("FFmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Update(func(st *config.State) error {
		st.FFmpeg = ff
		st.Speakers["test"] = config.Speaker{DID: "test", Enabled: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	e := newAlarms(s, nil)
	e.root = t.TempDir()
	for name, duration := range map[string]string{"long.wav": "62", "short.wav": "0.5"} {
		if b, err := exec.CommandContext(ctx, ff, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration="+duration, filepath.Join(e.root, name)).CombinedOutput(); err != nil {
			t.Fatalf("fixture: %v %s", err, b)
		}
	}
	probe := filepath.Join(filepath.Dir(ff), "ffprobe"+filepath.Ext(ff))
	duration := func(a config.Alarm) float64 {
		b, err := exec.CommandContext(ctx, probe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", filepath.Join(s.Directory(), "alarm-audio", a.Audio+".mp3")).Output()
		if err != nil {
			t.Fatal(err)
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	a := config.Alarm{Name: "Wake", Time: "07:00", Timezone: "Asia/Shanghai", Days: 127, Volume: 30, Minutes: 1, Sound: "long.wav", Speaker: "test"}
	if err := e.Save(ctx, a); err != nil {
		t.Fatal(err)
	}
	a = s.Snapshot().Alarms[0]
	oldAudio := a.Audio
	if d := duration(a); d < 60 || d > 60.2 {
		t.Fatalf("legacy cutoff: %f", d)
	}
	a.Playback = "full"
	if err := e.Save(ctx, a); err != nil {
		t.Fatal(err)
	}
	a = s.Snapshot().Alarms[0]
	if a.Audio == oldAudio {
		t.Fatal("reused truncated cache")
	}
	if d := duration(a); d < 62 || d > 62.2 {
		t.Fatalf("full track truncated: %f", d)
	}
	if _, err := os.Stat(filepath.Join(s.Directory(), "alarm-audio", oldAudio+".mp3")); !os.IsNotExist(err) {
		t.Fatal("old cache not removed")
	}
	a.Playback, a.Repeats, a.Sound = "repeat", 3, "short.wav"
	if err := e.Save(ctx, a); err != nil {
		t.Fatal(err)
	}
	a = s.Snapshot().Alarms[0]
	if d := duration(a); d < 1.5 || d > 1.7 {
		t.Fatalf("expected three complete plays: %f", d)
	}
	reopened, err := config.Open(s.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().Alarms[0]; got.Playback != "repeat" || got.Repeats != 3 {
		t.Fatal("playback settings not persisted")
	}
	// Full/repeat URLs must not inherit the old minutes-based expiry.
	e.runs[a.ID] = &alarmRun{Alarm: a, Token: "test-token", State: "ringing", Started: time.Now().Add(-3 * time.Minute)}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", "/alarm-audio/test-token", nil))
	if w.Code != 200 || w.Body.Len() == 0 {
		t.Fatal("complete track URL expired", w.Code)
	}
	e.runs[a.ID].Token = ""
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", "/alarm-audio/test-token", nil))
	if w.Code != 404 {
		t.Fatal("stopped loop still served")
	}
	files, _ := filepath.Glob(filepath.Join(s.Directory(), "alarm-audio", ".source-*"))
	if len(files) != 0 {
		t.Fatal("temporary sources leaked")
	}
}
