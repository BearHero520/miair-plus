package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFnosACLAndToken(t *testing.T) {
	t.Setenv("TRIM_API_TOKEN", "first")
	expectedToken := "first"
	response := `{"code":0,"data":[{"path":"/vol1/1000/music.mp3","readable":true}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+expectedToken {
			t.Error("token not read from current environment")
		}
		var body struct {
			Req     string
			AppName string
			Data    struct {
				UID  int
				Path string
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/trimapp" || body.Req != "trim.file.checkUserACL" || body.AppName != "miair-plus" || body.Data.UID != 1000 {
			t.Error("incorrect platform request")
		}
		w.Write([]byte(response))
	}))
	defer srv.Close()
	client := &http.Client{Transport: rewriteTransport{srv.URL, http.DefaultTransport}}
	if err := checkFnosRead(context.Background(), client, 1000, "/vol1/1000/music.mp3"); err != nil {
		t.Fatal(err)
	}
	expectedToken = "rotated"
	t.Setenv("TRIM_API_TOKEN", expectedToken)
	for _, body := range []string{
		`{"code":0,"data":[{"path":"/vol1/1000/other.mp3","readable":true}]}`,
		`{"code":0,"data":[{"path":"/vol1/1000/music.mp3","readable":false}]}`,
		`{"code":0,"data":[]}`, `{"code":403,"data":[]}`, `{"data":[]}`, `invalid`,
	} {
		response = body
		if err := checkFnosRead(context.Background(), client, 1000, "/vol1/1000/music.mp3"); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

type rewriteTransport struct {
	target string
	base   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	u, _ := http.NewRequest(r.Method, t.target+r.URL.Path, r.Body)
	u.Header = r.Header.Clone()
	return t.base.RoundTrip(u.WithContext(r.Context()))
}

func TestGatewayIdentityCannotBeSpoofedOnPort(t *testing.T) {
	r := httptest.NewRequest("POST", "/app/miair-plus/api/v1/alarms/import-nas", nil)
	r.Header.Set("X-Trim-Userid", "1000")
	if _, err := gatewayUID(r); err == nil {
		t.Fatal("trusted direct client identity")
	}
	r = r.WithContext(context.WithValue(r.Context(), gatewayRequestKey{}, true))
	if uid, err := gatewayUID(r); err != nil || uid != 1000 {
		t.Fatal(uid, err)
	}
	r.Header.Set("X-Trim-Userid", "invalid")
	if _, err := gatewayUID(r); err == nil {
		t.Fatal("accepted invalid user")
	}
}
