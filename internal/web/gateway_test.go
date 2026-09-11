package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayRouting(t *testing.T) {
	for _, path := range []string{"/api/v1/status", "/api/v1/ws"} {
		handler := GatewayHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path || r.URL.RawQuery != "token=test" || r.Host != "nas.example.com" {
				t.Fatalf("incorrect forwarded request: %s %s", r.Host, r.URL)
			}
			if r.Header.Get("Upgrade") != "websocket" {
				t.Fatal("upgrade header lost")
			}
			w.WriteHeader(204)
		}))
		r := httptest.NewRequest("GET", "https://nas.example.com"+GatewayPrefix+path+"?token=test", nil)
		r.Header.Set("Upgrade", "websocket")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
}

func TestGatewayHTMLAndDirectAccess(t *testing.T) {
	for _, path := range []string{"/", "/setup", "/index.html", GatewayPrefix + "/", GatewayPrefix + "/alarms", GatewayPrefix + "/nas-file-callback?state=test"} {
		w := httptest.NewRecorder()
		GatewayHandler(Handler()).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		base := "/"
		if strings.HasPrefix(path, GatewayPrefix+"/") {
			base = GatewayPrefix + "/"
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `<base href="`+base+`"`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: status=%d, wrong base or cache header", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	GatewayHandler(Handler()).ServeHTTP(w, httptest.NewRequest("GET", GatewayPrefix+"?state=test", nil))
	if w.Code != 307 || w.Header().Get("Location") != GatewayPrefix+"/?state=test" {
		t.Fatal(w.Code, w.Header())
	}
	w = httptest.NewRecorder()
	GatewayHandler(Handler()).ServeHTTP(w, httptest.NewRequest("GET", GatewayPrefix+"/assets/missing.js", nil))
	if w.Code != 404 {
		t.Fatal("missing module must not return HTML", w.Code)
	}
}
