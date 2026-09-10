package server

import (
	"context"
	"encoding/json"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/xiaomi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testAPI(t *testing.T) *API {
	t.Helper()
	s, e := config.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := xiaomi.New(s)
	return NewAPI(ctx, s, NewManager(ctx, s, c, true), c)
}
func callAPI(a *API, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/v1/"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func setupToken(t *testing.T, a *API) string {
	t.Helper()
	w := callAPI(a, "POST", "login/setup", `{"username":"admin","password":"test-password-123"}`, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	return response["access_token"]
}
func TestAuthSetupAndRevocation(t *testing.T) {
	a := testAPI(t)
	if w := callAPI(a, "GET", "settings", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	token := setupToken(t, a)
	if w := callAPI(a, "POST", "login/setup", `{"username":"other","password":"test-password-123"}`, ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := callAPI(a, "GET", "settings", "", token); w.Code != 200 || strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "service_token") {
		t.Fatal(w.Body.String())
	}
	w := callAPI(a, "POST", "login/password", `{"old_password":"test-password-123","new_password":"updated-password-123"}`, token)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if a.valid(token) {
		t.Fatal("old token survives password change")
	}
}
func TestRenameReturnsBeforeNetworkWorker(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	_ = a.Store.Update(func(s *config.State) error {
		s.Speakers["did"] = config.Speaker{DID: "did", Name: "客厅", Enabled: true}
		return nil
	})
	start := time.Now()
	w := callAPI(a, "POST", "speakers/did/rename", `{"dlna_name":"卧室"}`, token)
	if w.Code != 200 || time.Since(start) > 500*time.Millisecond {
		t.Fatal(w.Code, w.Body.String(), time.Since(start))
	}
	if a.Store.Snapshot().Speakers["did"].DLNAName != "卧室" {
		t.Fatal("not persisted")
	}
	if !strings.Contains(w.Body.String(), "pending") {
		t.Fatal(w.Body.String())
	}
}
func TestOriginAndBounds(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	r := httptest.NewRequest("POST", "http://nas/api/v1/settings", strings.NewReader(`{}`))
	r.Header.Set("Origin", "http://evil.example")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal(w.Code)
	}
	if w = callAPI(a, "POST", "settings", `{"dlna_port":-1}`, token); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
