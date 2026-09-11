package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

type gatewayRequestKey struct{}

// Remote/mobile fnOS can rewrite Host or use an opaque Origin. For that
// transport, require the gateway identity plus explicit application auth.
// Login uses a custom header and JSON, so a cross-site form cannot submit it;
// API responses deliberately do not grant cross-origin access.
func (a *API) allowedOrigin(r *http.Request) bool {
	if allowedOrigin(r) {
		return true
	}
	if _, err := gatewayUID(r); err != nil {
		return false
	}
	if token := strings.TrimPrefix(r.Header.Get("X-Miair-Authorization"), "Bearer "); token != "" && a.valid(token) {
		return true
	}
	if strings.HasSuffix(r.URL.Path, "/api/v1/ws") && a.valid(r.URL.Query().Get("miair_token")) {
		return true
	}
	if r.Header.Get("X-Miair-Client") != "miair-plus" {
		return false
	}
	path := strings.TrimPrefix(r.URL.Path, "/app/miair-plus")
	if path == "/api/v1/login/status" && r.Method == "GET" {
		return true
	}
	return (path == "/api/v1/login" || path == "/api/v1/login/setup") && r.Method == "POST" && strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]), "application/json")
}

// Only the dedicated Unix listener may mark requests as forwarded by fnOS.
// Never infer trust from an HTTP header or from the public URL prefix.
func (a *API) GatewayHandler() http.Handler {
	handler := a.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), gatewayRequestKey{}, true)))
	})
}

func allowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	gateway, _ := r.Context().Value(gatewayRequestKey{}).(bool)
	host := &url.URL{Host: r.Host}
	// fnOS nginx uses `proxy_set_header Host $host`: the external port is lost.
	// Allow that specific mismatch only on the gateway socket and same hostname.
	return gateway && host.Port() == "" && strings.EqualFold(u.Hostname(), host.Hostname())
}
