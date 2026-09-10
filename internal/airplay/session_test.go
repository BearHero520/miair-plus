package airplay

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/media"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type testTarget struct{ started chan string }

func (t *testTarget) StartAirPlay(ctx context.Context, url, client string) error {
	select {
	case t.started <- url:
	default:
	}
	return nil
}
func (t *testTarget) EndAirPlay(context.Context)           {}
func (t *testTarget) SetVolume(context.Context, int) error { return nil }
func (t *testTarget) Metadata(string, string)              {}
func TestRTSPAudioSession(t *testing.T) {
	ffmpeg := os.Getenv("TEST_FFMPEG")
	if ffmpeg == "" {
		ffmpeg, _ = exec.LookPath("ffmpeg")
	}
	if ffmpeg == "" {
		t.Skip("FFmpeg required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	target := &testTarget{started: make(chan string, 1)}
	s, err := Start(ctx, "127.0.0.1", "Test speaker", "test", ffmpeg, target, func(l *media.Live) (string, func()) { return "http://127.0.0.1/live/test", func() {} }, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port()), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(12 * time.Second))
	reader := bufio.NewReader(conn)
	seq := 0
	send := func(method, headers, body string) {
		t.Helper()
		seq++
		fmt.Fprintf(conn, "%s rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: %d\r\n%sContent-Length: %d\r\n\r\n%s", method, seq, headers, len(body), body)
		line, e := reader.ReadString('\n')
		if e != nil || !strings.HasPrefix(line, "RTSP/1.0 200") {
			t.Fatalf("%s: %s %v", method, line, e)
		}
		for {
			line, e = reader.ReadString('\n')
			if e != nil {
				t.Fatal(e)
			}
			if line == "\r\n" {
				break
			}
		}
	}
	send("ANNOUNCE", "Content-Type: application/sdp\r\n", "v=0\r\na=rtpmap:96 L16/44100/2\r\n")
	send("SETUP", "Transport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n", "")
	send("RECORD", "", "")
	for i := 0; i < 200; i++ {
		packet := make([]byte, 12+352*4)
		packet[0] = 0x80
		packet[1] = 96
		binary.BigEndian.PutUint16(packet[2:], uint16(i))
		for j := 12; j < len(packet); j += 2 {
			binary.BigEndian.PutUint16(packet[j:], uint16(j*17+i))
		}
		header := []byte{'$', 0, byte(len(packet) >> 8), byte(len(packet))}
		if _, err = conn.Write(append(header, packet...)); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case url := <-target.started:
		if !strings.Contains(url, "/live/") {
			t.Fatal(url)
		}
	case <-ctx.Done():
		t.Fatal("RAOP never initiated speaker playback")
	}
	send("TEARDOWN", "", "")
	_, _ = io.Copy(io.Discard, reader)
}
