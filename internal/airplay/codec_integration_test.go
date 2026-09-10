package airplay

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestALACDecodeIntegration(t *testing.T) {
	ffmpeg := os.Getenv("TEST_FFMPEG")
	if ffmpeg == "" {
		var err error
		ffmpeg, err = exec.LookPath("ffmpeg")
		if err != nil {
			t.Skip("FFmpeg not installed")
		}
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe"+filepath.Ext(ffmpeg))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	input := filepath.Join(t.TempDir(), "input.m4a")
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=44100:duration=0.5", "-ac", "2", "-c:a", "alac", "-y", input)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("encode: %v %s", err, b)
	}
	b, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_packets", "-show_data", "-show_entries", "packet=data", "-of", "json", input).Output()
	if err != nil {
		t.Fatal(err)
	}
	var packets struct{ Packets []struct{ Data string } }
	if err = json.Unmarshal(b, &packets); err != nil {
		t.Fatal(err)
	}
	s := SDP{Codec: "AppleLossless", Rate: 44100, Channels: 2, ALAC: []uint32{4096, 0, 16, 40, 10, 14, 2, 0, 0, 0, 44100}}
	var mp4 bytes.Buffer
	mp4.Write(s.initMP4())
	var timestamp uint64
	for i, p := range packets.Packets {
		var raw []byte
		for _, line := range strings.Split(p.Data, "\n") {
			_, data, ok := strings.Cut(line, ": ")
			if !ok {
				continue
			}
			hexpart, _, _ := strings.Cut(data, "  ")
			decoded, err := hex.DecodeString(strings.ReplaceAll(hexpart, " ", ""))
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, decoded...)
		}
		mp4.Write(fragment(uint32(i+1), timestamp, 4096, raw))
		timestamp += 4096
	}
	decode := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "mp4", "-i", "pipe:0", "-f", "s16le", "pipe:1")
	decode.Stdin = &mp4
	var stderr bytes.Buffer
	decode.Stderr = &stderr
	pcm, err := decode.Output()
	if err != nil {
		t.Fatalf("Go-fragmented ALAC decode: %v %s", err, stderr.String())
	}
	if len(pcm) < 44100*2*2/3 {
		t.Fatalf("decoded too little audio: %d", len(pcm))
	}
	allZero := true
	for _, v := range pcm {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("decoded silence")
	}
}
