package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestGatewayAppTokenDoesNotOccupySystemToken(t *testing.T) {
	a := testAPI(t)
	token := setupToken(t, a)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fnOS interprets standard auth fields before forwarding to the app.
		if r.Header.Get("Authorization") != "" || r.URL.Query().Get("token") != "" {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("invalid token"))
			return
		}
		a.GatewayHandler().ServeHTTP(w, r)
	}))
	defer proxy.Close()
	for _, appToken := range []string{token, "wrong", ""} {
		r, _ := http.NewRequest("GET", proxy.URL+"/app/miair-plus/api/v1/settings", nil)
		r.Header.Set("X-Miair-Authorization", "Bearer "+appToken)
		resp, err := proxy.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		want := 401
		if appToken == token {
			want = 200
		}
		if resp.StatusCode != want || !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("status %d, want %d", resp.StatusCode, want)
		}
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+"/app/miair-plus/api/v1/ws?miair_token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
