package web

import (
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedAssetsIncludeUnderscoreModules(t *testing.T) {
	if _, err := os.Stat("dist/index.html"); err != nil {
		t.Skip("build frontend first")
	}
	err := filepath.WalkDir("dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, err = assets.ReadFile(filepath.ToSlash(path)); err != nil {
			t.Errorf("built asset missing from executable: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	_ = fs.WalkDir(assets, "dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(path, "_plugin-vue_export-helper") {
			found = true
			w := httptest.NewRecorder()
			Handler().ServeHTTP(w, httptest.NewRequest("GET", "/"+strings.TrimPrefix(path, "dist/"), nil))
			if w.Code != 200 {
				t.Fatal(path, w.Code)
			}
		}
		return nil
	})
	if !found {
		t.Fatal("Vue helper module missing from embedded frontend")
	}
}

func TestHTMLIsNotCached(t *testing.T) {
	if _, err := assets.ReadFile("dist/index.html"); err != nil {
		t.Skip("build frontend first")
	}
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest("GET", "/setup", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Header())
	}
}
