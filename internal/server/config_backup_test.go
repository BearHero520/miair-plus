package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/BearHero520/miair-plus/internal/config"
)

func TestConfigBackupRoundTripAndValidation(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	if w := callAPI(a, "GET", "settings/export", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := callAPI(a, "POST", "settings/import", "{}", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	_ = a.Store.Update(func(s *config.State) error {
		s.Xiaomi.PassToken = "private-account-token"
		s.Speakers["speaker"] = config.Speaker{DID: "speaker", Name: "Speaker", Enabled: true}
		s.Alarms = []config.Alarm{{ID: config.Random(16), Name: "Wake", Time: "07:00", Timezone: "Asia/Shanghai", Rule: "weekly", Days: 127, Speaker: "speaker", Sound: "wake.mp3", Volume: 40, Minutes: 1, Enabled: true}}
		return nil
	})
	w := callAPI(a, "GET", "settings/export", "", token)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, secret := range []string{"password_hash", "secret", "pass_token", "private-account-token", "auth_revision"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("backup leaks", secret)
		}
	}
	var backup configBackup
	if err := json.Unmarshal(w.Body.Bytes(), &backup); err != nil {
		t.Fatal(err)
	}
	saved := a.Store.Snapshot()
	port := 9411
	backup.Settings.Port = &port
	data, _ := json.Marshal(backup)
	w = callAPI(a, "POST", "settings/import", string(data), token)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	after := a.Store.Snapshot()
	if after.DLNAPort != port || len(after.Alarms) != 1 || after.Alarms[0].Enabled || !reflect.DeepEqual(after.Xiaomi, saved.Xiaomi) || after.Secret != saved.Secret || after.PasswordHash != saved.PasswordHash || !a.valid(token) {
		t.Fatal("round trip changed protected data or failed to restore")
	}
	invalid := []string{`{}`, string(data) + `{}`, strings.Replace(string(data), `"schema_version":1`, `"schema_version":9`, 1), strings.Replace(string(data), `"dlna_port":9411`, `"dlna_port":-1`, 1), strings.Replace(string(data), `"wake.mp3"`, `"../private.mp3"`, 1), strings.Replace(string(data), `"settings":{`, `"settings":{"password_hash":"injected",`, 1)}
	for _, body := range invalid {
		w = callAPI(a, "POST", "settings/import", body, token)
		if w.Code != 400 || !reflect.DeepEqual(after, a.Store.Snapshot()) {
			t.Fatal("invalid import mutated state", w.Code, w.Body.String())
		}
	}
	a.Manager.alarms.runs[after.Alarms[0].ID] = &alarmRun{State: "ringing"}
	if w = callAPI(a, "POST", "settings/import", string(data), token); w.Code != 409 {
		t.Fatal("active alarm import not rejected", w.Body.String())
	}
}

func TestImportSkipsUnknownSpeakerAndPersistsUpdatePreference(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	w := callAPI(a, "GET", "settings/export", "", token)
	var backup configBackup
	_ = json.Unmarshal(w.Body.Bytes(), &backup)
	enabled := false
	name := "Room"
	unknown := "other-device"
	backup.Settings.AutoCheckUpdate = &enabled
	backup.Settings.Speakers[unknown] = speakerPatch{Name: &name}
	backup.Settings.AirPlay2Target = &unknown
	data, _ := json.Marshal(backup)
	w = callAPI(a, "POST", "settings/import", string(data), token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"skipped_speakers":1`) {
		t.Fatal(w.Body.String())
	}
	reopened, err := config.Open(a.Store.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().AutoCheckUpdate || reopened.Snapshot().AirPlay2Target != "" || len(reopened.Snapshot().Speakers) != 0 {
		t.Fatal("import did not persist")
	}
}
