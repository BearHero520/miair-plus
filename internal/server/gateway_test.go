package server

import (
	"github.com/gorilla/websocket"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayAuthenticationAndWebSocket(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	for _, tc := range []struct {
		token, origin string
		status        int
	}{{"", "https://nas.example.com", 401}, {token, "https://nas.example.com", 200}, {token, "https://evil.example", 403}} {
		r := httptest.NewRequest("GET", "https://nas.example.com/app/miair-plus/api/v1/settings", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		// Gateway metadata never replaces the application's own authentication.
		r.Header.Set("X-Trim-Isadmin", "true")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(w.Code, tc.status, w.Body.String())
		}
		if w.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
			t.Fatal("same-origin fnOS iframe must be allowed")
		}
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/app/miair-plus/api/v1/ws?token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
}
