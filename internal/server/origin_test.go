package server

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestWebViewAssetsDoNotRequireAPIOrigin(t *testing.T) {
	a := testAPI(t)
	page := httptest.NewRecorder()
	a.GatewayHandler().ServeHTTP(page, httptest.NewRequest("GET", "/app/miair-plus/", nil))
	assets := regexp.MustCompile(`(?:src|href)="\./(assets/[^" ]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(assets) < 2 {
		t.Fatal("entry JS/CSS absent")
	}
	for _, origin := range []string{"null", "https://remote.example"} {
		for _, asset := range assets {
			r := httptest.NewRequest("GET", "/app/miair-plus/"+asset[1], nil)
			r.Host = "127.0.0.1"
			r.Header.Set("Origin", origin)
			w := httptest.NewRecorder()
			a.GatewayHandler().ServeHTTP(w, r)
			if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Fatalf("asset failed: %s %d", asset[1], w.Code)
			}
		}
	}
	r := httptest.NewRequest("POST", "/app/miair-plus/api/v1/login", strings.NewReader(`{}`))
	r.Header.Set("Origin", "null")
	w := httptest.NewRecorder()
	a.GatewayHandler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("login origin protection lost", w.Code)
	}
}

func TestFnosStrippedHostPort(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	for _, tc := range []struct {
		name, host, origin, token string
		gateway                   bool
		status                    int
	}{
		{"gateway-ip", "192.168.50.200", "http://192.168.50.200:5000", token, true, 200},
		{"gateway-domain", "nas.example.com", "https://nas.example.com:8443", token, true, 200},
		{"gateway-needs-token", "nas.example.com", "https://nas.example.com:8443", "", true, 401},
		{"gateway-cross-host", "nas.example.com", "https://evil.example:8443", token, true, 403},
		{"gateway-null-origin", "nas.example.com", "null", token, true, 403},
		{"gateway-explicit-wrong-port", "nas.example.com:5000", "https://nas.example.com:8443", token, true, 403},
		{"direct-matching", "192.168.50.200:8310", "http://192.168.50.200:8310", token, false, 200},
		{"direct-wrong-port", "192.168.50.200:8310", "http://192.168.50.200:5000", token, false, 403},
		{"spoofed-gateway-headers", "nas.example.com", "https://nas.example.com:8443", token, false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+tc.host+"/app/miair-plus/api/v1/settings", nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("X-Trim-Isadmin", "true")
			r.Header.Set("X-Forwarded-Host", strings.TrimPrefix(tc.origin, "https://"))
			w := httptest.NewRecorder()
			h := a.Handler()
			if tc.gateway {
				h = a.GatewayHandler()
			}
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
	// This reproduces fnOS: browser Origin includes its port; upstream Host does not.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "nas.example.com"
		a.GatewayHandler().ServeHTTP(w, r)
	}))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/app/miair-plus/api/v1/ws?token="+token, http.Header{"Origin": []string{"https://nas.example.com:8443"}})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
