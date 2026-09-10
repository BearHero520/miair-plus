package server

import (
	"encoding/json"
	"github.com/BearHero520/miair-plus/internal/config"
	"strings"
	"testing"
)

func TestDiagnosticsRequiresAuthAndOmitsCredentials(t *testing.T) {
	a := testAPI(t)
	if w := callAPI(a, "GET", "diagnostics/download", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	token := setupToken(t, a)
	_ = a.Store.Update(func(s *config.State) error {
		s.Xiaomi.PassToken = "private-account-token"
		s.Xiaomi.ServiceToken = "private-service-token"
		return nil
	})
	a.Write([]byte("failure https://example.com/audio?token=private-url-token cookie=private-cookie"))
	w := callAPI(a, "GET", "diagnostics/download", "", token)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "miair-plus-diagnostics.json") {
		t.Fatal(w.Code, w.Header())
	}
	if strings.Contains(w.Body.String(), "private-") {
		t.Fatal("diagnostics exposed credentials")
	}
	var report struct {
		Checks  []Check `json:"checks"`
		Entries []any   `json:"entries"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &report); e != nil || len(report.Checks) != 4 || len(report.Entries) != 1 {
		t.Fatal(e, w.Body.String())
	}
}
