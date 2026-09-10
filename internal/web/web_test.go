package web

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedAssetsIncludeUnderscoreModules(t *testing.T) {
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
		t.Skip("build frontend before asset integration test")
	}
}
