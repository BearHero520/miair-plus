package web

import (
	"context"
	"net/http"
	"strings"
)

const GatewayPrefix = "/app/miair-plus"

type basePathKey struct{}

// fnOS preserves the public prefix when forwarding HTTP and WebSocket requests.
func GatewayHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == GatewayPrefix {
			target := GatewayPrefix + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			return
		}
		if strings.HasPrefix(r.URL.Path, GatewayPrefix+"/") {
			r = r.Clone(context.WithValue(r.Context(), basePathKey{}, GatewayPrefix+"/"))
			r.URL.Path = strings.TrimPrefix(r.URL.Path, GatewayPrefix)
			if r.URL.RawPath != "" {
				r.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, GatewayPrefix)
			}
		}
		next.ServeHTTP(w, r)
	})
}
