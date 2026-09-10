package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadRequiresAuthAndKeepsFileInsideLibrary(t *testing.T) {
	a := testAPI(t)
	a.Manager.alarms.root = t.TempDir()
	token := setupToken(t, a)
	for _, auth := range []string{"", token} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("file", "../../wake.wav")
		part.Write([]byte("audio fixture"))
		writer.Close()
		req := httptest.NewRequest("POST", "/api/v1/alarms/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+auth)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if auth == "" {
			if w.Code != 401 {
				t.Fatal(w.Code)
			}
			continue
		}
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result map[string]string
		json.Unmarshal(w.Body.Bytes(), &result)
		if filepath.Base(result["sound"]) != result["sound"] {
			t.Fatal("path escaped")
		}
		b, err := os.ReadFile(filepath.Join(a.Manager.alarms.root, result["sound"]))
		if err != nil || string(b) != "audio fixture" {
			t.Fatal(err, string(b))
		}
	}
}
func TestNASAudioRejectsSystemPaths(t *testing.T) {
	for _, path := range []string{"/etc/passwd", "/vol2/@appdata/miair-plus/private.mp3", "/vol2/1000/../@appdata/a.mp3", "/vol2/1000/.private/song.mp3", "/vol2/1000/song.json"} {
		if allowedNASAudio(path) {
			t.Fatal("accepted", path)
		}
	}
	if !allowedNASAudio("/vol2/1000/音乐/铃声.mp3") {
		t.Fatal("ordinary NAS audio rejected")
	}
}
