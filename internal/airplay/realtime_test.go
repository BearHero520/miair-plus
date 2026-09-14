package airplay

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BearHero520/miair-plus/internal/media"
)

// Unlike the bulk-input test, this sends 352-sample ALAC packets at the phone's
// real rate, including highly compressed silence (the smallest probe input).
func TestRealtimeALACStartup(t *testing.T) {
	ff := os.Getenv("TEST_FFMPEG")
	if ff == "" {
		ff, _ = exec.LookPath("ffmpeg")
	}
	if ff == "" {
		t.Skip("FFmpeg required")
	}
	for _, gain := range []string{"1", "0"} {
		t.Run("gain="+gain, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			input := filepath.Join(t.TempDir(), "short.m4a")
			b, err := exec.CommandContext(ctx, ff, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=44100", "-af", "atrim=end_sample=352,volume="+gain, "-ac", "2", "-sample_fmt", "s16p", "-c:a", "alac", input).CombinedOutput()
			if err != nil {
				t.Fatalf("fixture: %v %s", err, b)
			}
			probe := filepath.Join(filepath.Dir(ff), "ffprobe"+filepath.Ext(ff))
			b, err = exec.CommandContext(ctx, probe, "-v", "error", "-show_packets", "-show_data", "-show_entries", "packet=data,duration", "-of", "json", input).Output()
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Packets []struct {
					Data     string
					Duration int
				}
			}
			if err := json.Unmarshal(b, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Packets) != 1 || result.Packets[0].Duration != 352 {
				t.Fatal("fixture must contain one 352-sample ALAC frame")
			}
			var frame []byte
			for _, line := range strings.Split(result.Packets[0].Data, "\n") {
				_, data, ok := strings.Cut(line, ": ")
				if !ok {
					continue
				}
				hexpart, _, _ := strings.Cut(data, "  ")
				part, err := hex.DecodeString(strings.ReplaceAll(hexpart, " ", ""))
				if err != nil {
					t.Fatal(err)
				}
				frame = append(frame, part...)
			}
			live := media.NewLive(65536, "audio/mpeg")
			s := SDP{Codec: "AppleLossless", Rate: 44100, Channels: 2, ALAC: []uint32{352, 0, 16, 40, 10, 14, 2, 255, 0, 0, 44100}}
			d, err := NewDecoder(ctx, ff, s, live)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			start := time.Now()
			tick := time.NewTicker(time.Second * 352 / 44100)
			defer tick.Stop()
			for live.Size() < 4096 {
				select {
				case <-ctx.Done():
					t.Fatal("decoder blocked", ctx.Err())
				case <-tick.C:
					if time.Since(start) >= 4*time.Second {
						t.Fatalf("not ready within startup budget, packet=%d output=%d", len(frame), live.Size())
					}
					if err := d.WritePacket(frame); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Logf("packet=%d bytes, MP3 ready after %s", len(frame), time.Since(start))
		})
	}
}
