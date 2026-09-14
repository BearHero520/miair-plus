package airplay

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/media"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testTarget struct {
	started     chan string
	volumeDelay time.Duration
	lastVolume  atomic.Int32
}

func (t *testTarget) StartAirPlay(ctx context.Context, url, client string) error {
	select {
	case t.started <- url:
	default:
	}
	return nil
}
func (t *testTarget) EndAirPlay(context.Context) {}
func (t *testTarget) SetVolume(ctx context.Context, volume int) error {
	select {
	case <-time.After(t.volumeDelay):
		t.lastVolume.Store(int32(volume))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (t *testTarget) Metadata(string, string) {}
func TestRTSPAudioSession(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delay time.Duration
		udp   bool
	}{{"tcp", 0, false}, {"tcp-slow-cloud", 4 * time.Second, false}, {"udp-slow-cloud", 4 * time.Second, true}} {
		t.Run(tc.name, func(t *testing.T) { testRTSPAudioSession(t, tc.delay, tc.udp) })
	}
}
func testRTSPAudioSession(t *testing.T, volumeDelay time.Duration, useUDP bool) {
	ffmpeg := os.Getenv("TEST_FFMPEG")
	if ffmpeg == "" {
		ffmpeg, _ = exec.LookPath("ffmpeg")
	}
	if ffmpeg == "" {
		t.Skip("FFmpeg required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	target := &testTarget{started: make(chan string, 1), volumeDelay: volumeDelay}
	s, err := Start(ctx, "127.0.0.1", "Test speaker", "test", ffmpeg, target, func(l *media.Live) (string, func()) {
		httpServer := httptest.NewServer(l)
		return httpServer.URL + "/live/test", httpServer.Close
	}, false)
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
	send := func(method, headers, body string) string {
		t.Helper()
		seq++
		fmt.Fprintf(conn, "%s rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: %d\r\n%sContent-Length: %d\r\n\r\n%s", method, seq, headers, len(body), body)
		line, e := reader.ReadString('\n')
		if e != nil || !strings.HasPrefix(line, "RTSP/1.0 200") {
			t.Fatalf("%s: %s %v", method, line, e)
		}
		var response strings.Builder
		for {
			line, e = reader.ReadString('\n')
			if e != nil {
				t.Fatal(e)
			}
			if line == "\r\n" {
				break
			}
			response.WriteString(line)
		}
		return response.String()
	}
	sdp := "v=0\r\na=rtpmap:96 L16/44100/2\r\n"
	key, iv := []byte("0123456789abcdef"), []byte("fedcba9876543210")
	if useUDP {
		encrypted, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, &privateKey().PublicKey, key, nil)
		if err != nil {
			t.Fatal(err)
		}
		sdp += "a=rsaaeskey:" + base64.RawStdEncoding.EncodeToString(encrypted) + "\r\na=aesiv:" + base64.RawStdEncoding.EncodeToString(iv) + "\r\n"
	}
	send("ANNOUNCE", "Content-Type: application/sdp\r\n", sdp)
	var udp *net.UDPConn
	if useUDP {
		control, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer control.Close()
		response := send("SETUP", fmt.Sprintf("Transport: RTP/AVP/UDP;unicast;control_port=%d\r\n", control.LocalAddr().(*net.UDPAddr).Port), "")
		_, tail, ok := strings.Cut(response, "server_port=")
		if !ok {
			t.Fatal("no server port", response)
		}
		portText, _, _ := strings.Cut(tail, ";")
		port, err := strconv.Atoi(portText)
		if err != nil {
			t.Fatal(err)
		}
		udp, err = net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			t.Fatal(err)
		}
		defer udp.Close()
	} else {
		send("SETUP", "Transport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n", "")
	}
	send("RECORD", "", "")
	if volumeDelay > 0 {
		// Phone volume updates arrive before audio. Cloud latency must not stall
		// the RTSP reader until the audio readiness timeout cancels the session.
		started := time.Now()
		send("SET_PARAMETER", "Content-Type: text/parameters\r\n", "volume: -15.000000\r\n")
		send("SET_PARAMETER", "Content-Type: text/parameters\r\n", "volume: -12.000000\r\n")
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Errorf("RTSP volume acknowledgement blocked on cloud for %s", elapsed)
		}
	}
	for i := 0; i < 200; i++ {
		packet := make([]byte, 12+352*4)
		packet[0] = 0x80
		packet[1] = 96
		binary.BigEndian.PutUint16(packet[2:], uint16(i))
		for j := 12; j < len(packet); j += 2 {
			binary.BigEndian.PutUint16(packet[j:], uint16(j*17+i))
		}
		header := []byte{'$', 0, byte(len(packet) >> 8), byte(len(packet))}
		if useUDP {
			block, cipherErr := aes.NewCipher(key)
			if cipherErr != nil {
				t.Fatal(cipherErr)
			}
			payload := packet[12:]
			n := len(payload) / aes.BlockSize * aes.BlockSize
			cipher.NewCBCEncrypter(block, iv).CryptBlocks(payload[:n], payload[:n])
			_, err = udp.Write(packet)
			time.Sleep(time.Second * 352 / 44100)
		} else {
			_, err = conn.Write(append(header, packet...))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	select {
	case url := <-target.started:
		if volumeDelay > 0 && target.lastVolume.Load() != 60 {
			t.Fatal("latest initial phone volume not applied", target.lastVolume.Load())
		}
		if !strings.Contains(url, "/live/") {
			t.Fatal(url)
		}
		client := &http.Client{Timeout: 3 * time.Second}
		request, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		request.Header.Set("Range", "bytes=0-1")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		mp3 := make([]byte, 8192)
		_, err = io.ReadFull(response.Body, mp3)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			t.Fatalf("speaker stream: status=%d error=%v", response.StatusCode, err)
		}
		decode := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "mp3", "-i", "pipe:0", "-f", "s16le", "pipe:1")
		decode.Stdin = bytes.NewReader(mp3)
		pcm, err := decode.Output()
		if err != nil || len(pcm) < 4096 {
			t.Fatalf("HTTP audio cannot decode: %v bytes=%d", err, len(pcm))
		}
		audible := false
		for _, b := range pcm {
			if b != 0 {
				audible = true
				break
			}
		}
		if !audible {
			t.Fatal("HTTP stream decoded to silence")
		}
		t.Logf("HTTP MP3=%d bytes, decoded nonzero PCM=%d bytes", len(mp3), len(pcm))
	case <-ctx.Done():
		t.Fatal("RAOP never initiated speaker playback")
	}
	send("TEARDOWN", "", "")
	_, _ = io.Copy(io.Discard, reader)
}
