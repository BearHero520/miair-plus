package media

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestRangeForwarding(t *testing.T) {
	p := NewProxy("http://nas", "key")
	url, _ := p.URL("https://example.com/song")
	p.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") != "bytes=3-5" {
			t.Fatal("lost range")
		}
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": []string{"bytes 3-5/10"}, "Content-Length": []string{"3"}}, Body: io.NopCloser(strings.NewReader("abc"))}, nil
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", url, nil)
	r.Header.Set("Range", "bytes=3-5")
	p.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "abc" || w.Header().Get("Content-Range") == "" {
		t.Fatal(w)
	}
}
func TestHeadFallback(t *testing.T) {
	p := NewProxy("http://nas", "key")
	url, _ := p.URL("https://example.com/song")
	count := 0
	p.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		count++
		code := 200
		if r.Method == "HEAD" {
			code = 405
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Length": []string{"3"}}, Body: io.NopCloser(strings.NewReader("abc"))}, nil
	})
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("HEAD", url, nil))
	if w.Code != 200 || count != 2 || w.Body.Len() != 0 {
		t.Fatal(w.Code, count, w.Body.String())
	}
}
func TestProxyBoundAndPrivateAddressPolicy(t *testing.T) {
	for _, u := range []string{"file:///etc/passwd", "http://127.0.0.1/a", "http://169.254.169.254/a", "http://[::1]/a", "https://user:pass@example.com/a"} {
		if ValidateURL(u) == nil {
			t.Fatal(u)
		}
	}
	if ValidateURL("http://192.168.1.30/music") != nil {
		t.Fatal("LAN audio should be allowed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if conn, err := safeDial(ctx, "tcp", "localhost:9"); err == nil {
		conn.Close()
		t.Fatal("DNS loopback allowed")
	}
	p := NewProxy("http://nas", "key")
	p.entries["expired"] = Entry{Expires: time.Now().Add(-time.Second)}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("GET", "/media/expired", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestLiveMemoryIsBoundedAndClose(t *testing.T) {
	l := NewLive(65536, "audio/mpeg")
	data := []byte(strings.Repeat("a", 1000000))
	if n, err := l.Write(data); n != len(data) || err != nil {
		t.Fatal(n, err)
	}
	if len(l.data) != 65536 || l.Size() != int64(len(data)) {
		t.Fatal("buffer grew")
	}
	l.Close()
	if _, err := l.Write([]byte("x")); err == nil {
		t.Fatal("write after close")
	}
}
