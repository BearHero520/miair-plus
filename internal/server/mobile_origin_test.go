package server

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMobileGatewayOrigin(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	for _, origin := range []string{"null", "https://remote.example:8443"} {
		for _, tc := range []struct {
			name, path, method, token, client, content string
			gateway                                    bool
			status                                     int
		}{
			{"authenticated", "settings", "GET", token, "", "", true, 200},
			{"login", "login", "POST", "", "miair-plus", "application/json", true, 200},
			{"status", "login/status", "GET", "", "miair-plus", "", true, 200},
			{"form", "login", "POST", "", "", "application/x-www-form-urlencoded", true, 403},
			{"preflight", "login", "OPTIONS", "", "", "", true, 403},
			{"invalid-token", "settings", "GET", "wrong", "miair-plus", "", true, 403},
			{"spoofed-direct", "settings", "GET", token, "miair-plus", "", false, 403},
		} {
			t.Run(origin+tc.name, func(t *testing.T) {
				r := httptest.NewRequest(tc.method, "/app/miair-plus/api/v1/"+tc.path, strings.NewReader(`{"username":"admin","password":"test-password-123"}`))
				r.Host = "127.0.0.1"
				r.Header.Set("Origin", origin)
				r.Header.Set("X-Trim-Userid", "1000")
				r.Header.Set("X-Miair-Authorization", "Bearer "+tc.token)
				r.Header.Set("X-Miair-Client", tc.client)
				r.Header.Set("Content-Type", tc.content)
				w := httptest.NewRecorder()
				h := a.Handler()
				if tc.gateway {
					h = a.GatewayHandler()
				}
				h.ServeHTTP(w, r)
				if w.Code != tc.status {
					t.Fatalf("%d want %d: %s", w.Code, tc.status, w.Body.String())
				}
				if w.Header().Get("Access-Control-Allow-Origin") != "" {
					t.Fatal("API must not grant cross-origin access")
				}
			})
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Trim-Userid", "1000")
		a.GatewayHandler().ServeHTTP(w, r)
	}))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/app/miair-plus/api/v1/ws?miair_token="+token, http.Header{"Origin": []string{"null"}})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
