package dlna

import (
	"context"
	"errors"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/media"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeController struct {
	mu      sync.Mutex
	calls   []string
	started chan struct{}
	fail    bool
}

func (f *fakeController) Play(ctx context.Context, s config.Speaker, u string) error {
	f.mu.Lock()
	f.calls = append(f.calls, "play")
	started := f.started
	fail := f.fail
	f.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
	if fail {
		return errors.New("cloud failure")
	}
	return nil
}
func (f *fakeController) Operation(ctx context.Context, s config.Speaker, a string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, a)
	return nil
}
func (f *fakeController) Volume(context.Context, config.Speaker, int) error { return nil }
func (f *fakeController) Status(context.Context, config.Speaker) (map[string]any, error) {
	return nil, nil
}
func rendererForTest(f *fakeController) *Renderer {
	return NewRenderer(context.Background(), config.Speaker{DID: "123", Name: "客厅", Enabled: true}, f, media.NewProxy("http://192.168.1.2:8311", "test"), 40)
}
func TestStopCancelsSlowPlay(t *testing.T) {
	f := &fakeController{started: make(chan struct{}, 1)}
	r := rendererForTest(f)
	_ = r.SetURI("https://example.com/song.mp3", "")
	done := make(chan error, 1)
	go func() { done <- r.Play(context.Background()) }()
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("play did not begin")
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("stale play succeeded")
	}
	if s := r.Snapshot(); s.State != "STOPPED" {
		t.Fatal(s.State)
	}
}
func TestOldAutoPlayCannotRestartAfterStop(t *testing.T) {
	r := rendererForTest(&fakeController{})
	_ = r.SetURI("https://example.com/old", "")
	r.mu.Lock()
	g := r.generation
	r.mu.Unlock()
	_ = r.Stop(context.Background())
	if err := r.playAt(context.Background(), g); err == nil {
		t.Fatal("old request restarted music")
	}
}
func TestFailureDoesNotPretendPlaying(t *testing.T) {
	r := rendererForTest(&fakeController{fail: true})
	_ = r.SetURI("https://example.com/song", "")
	if r.Play(context.Background()) == nil {
		t.Fatal("expected failure")
	}
	s := r.Snapshot()
	if s.State != "STOPPED" || s.Error == "" {
		t.Fatal(s)
	}
}
func TestSOAPAndEscaping(t *testing.T) {
	r := rendererForTest(&fakeController{})
	h := HTTPHandler{Lookup: func(string) *Renderer { return r }, Events: NewEvents(context.Background())}
	body := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><CurrentURI>https://example.com/a?x=1&amp;y=2</CurrentURI><CurrentURIMetaData/></u:SetAVTransportURI></s:Body></s:Envelope>`
	req := httptest.NewRequest("POST", "/dlna/id/AVTransport/control", strings.NewReader(body))
	req.Header.Set("SOAPAction", `"urn:schemas-upnp-org:service:AVTransport:1#SetAVTransportURI"`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || r.Snapshot().URI != "https://example.com/a?x=1&y=2" {
		t.Fatal(w.Code, w.Body.String())
	}
	r.Update(config.Speaker{DID: "123", Name: `客厅 <&>`})
	if !strings.Contains(Description(r), "客厅 &lt;&amp;&gt;") {
		t.Fatal(Description(r))
	}
}
func TestSearchMXAndMalformed(t *testing.T) {
	st, mx, ok := ParseSearch("M-SEARCH * HTTP/1.1\r\nMAN: \"ssdp:discover\"\r\nST: ssdp:all\r\nMX: 999\r\n\r\n")
	if !ok || st != "ssdp:all" || mx != 3 {
		t.Fatal(st, mx, ok)
	}
	if _, _, ok = ParseSearch(strings.Repeat("x", 4097)); ok {
		t.Fatal("accepted oversized datagram")
	}
}
func TestRenameDoesNotWaitForCloud(t *testing.T) {
	f := &fakeController{started: make(chan struct{}, 1)}
	r := rendererForTest(f)
	_ = r.SetURI("https://example.com/a", "")
	done := make(chan struct{})
	go func() { _ = r.Play(context.Background()); close(done) }()
	<-f.started
	start := time.Now()
	r.Update(config.Speaker{DID: "123", Name: "卧室"})
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("rename blocked on cloud")
	}
	r.Close()
	<-done
}
