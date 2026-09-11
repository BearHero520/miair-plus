package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:dist
var assets embed.FS

func Handler() http.Handler {
	sub, _ := fs.Sub(assets, "dist")
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			if strings.Contains(path, ".") {
				http.NotFound(w, r)
				return
			}
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		// Embedded assets contain no user data. WebViews may use an opaque Origin.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			body, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "Frontend is unavailable", 503)
				return
			}
			if base, ok := r.Context().Value(basePathKey{}).(string); ok {
				body = bytes.ReplaceAll(body, []byte(`<base href="/"`), []byte(`<base href="`+base+`"`))
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(body))
			return
		}
		files.ServeHTTP(w, r)
	})
}
