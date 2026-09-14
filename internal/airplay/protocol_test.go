package airplay

import (
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"net"
	"net/textproto"
	"testing"
	"time"
)

func TestChallenge(t *testing.T) {
	mac := []byte{2, 3, 4, 5, 6, 7}
	input := make([]byte, 16)
	s, err := challenge(base64.StdEncoding.EncodeToString(input), net.ParseIP("192.168.1.4"), mac)
	if err != nil {
		t.Fatal(err)
	}
	signed, _ := decode64(s)
	want := append(input, 192, 168, 1, 4)
	want = append(want, mac...)
	want = append(want, make([]byte, 32-len(want))...)
	if err = rsa.VerifyPKCS1v15(&privateKey().PublicKey, crypto.Hash(0), want, signed); err != nil {
		t.Fatal(err)
	}
}
func TestJitterReorderWrapAndLoss(t *testing.T) {
	var j Jitter
	now := time.Now()
	out := []byte{}
	write := func(b []byte) error { out = append(out, b...); return nil }
	resends := 0
	resend := func(uint16, uint16) { resends++ }
	j.Push(Packet{65535, []byte{1}})
	j.Push(Packet{1, []byte{3}})
	_ = j.Drain(now, write, resend)
	j.Push(Packet{0, []byte{2}})
	_ = j.Drain(now, write, resend)
	if string(out) != string([]byte{1, 2, 3}) || resends != 1 {
		t.Fatal(out, resends)
	}
	j.Push(Packet{3, []byte{5}})
	_ = j.Drain(now, write, resend)
	_ = j.Drain(now.Add(130*time.Millisecond), write, resend)
	if out[len(out)-1] != 5 {
		t.Fatal("gap never recovered", out)
	}
}
func TestMalformedRTPAndSDP(t *testing.T) {
	for _, p := range [][]byte{nil, {0x80}, make([]byte, 12), {0xb0, 96, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 200}} {
		if _, err := ParseRTP(p, nil, nil); err == nil {
			t.Fatal("malformed RTP accepted")
		}
	}
	s, err := ParseSDP("a=rtpmap:96 AppleLossless\r\na=fmtp:96 352 0 16 40 10 14 2 255 0 0 44100\r\n")
	if err != nil || s.Rate != 44100 {
		t.Fatal(s, err)
	}
	if _, err = ParseSDP("a=rtpmap:96 AppleLossless\na=fpaeskey:xxx"); err == nil {
		t.Fatal("unsupported crypto accepted")
	}
}
func TestNTPFraction(t *testing.T) {
	b := ntp(time.Unix(0, 500000000))
	if binary.BigEndian.Uint32(b) != 2208988800 || binary.BigEndian.Uint32(b[4:]) != 1<<31 {
		t.Fatal(b)
	}
}

func TestVolumeMinimumIsNotMute(t *testing.T) {
	ss := &session{volume: make(chan int, 1)}
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", "text/parameters")
	for _, tc := range []struct {
		body string
		want int
	}{{"-144", 0}, {"-30", 1}, {"-29.9", 1}, {"-15", 50}, {"0", 100}} {
		code, _, _ := ss.handle("SET_PARAMETER", h, []byte("volume: "+tc.body+"\r\n"))
		if code != 200 {
			t.Fatal(code)
		}
		if got := <-ss.volume; got != tc.want {
			t.Fatalf("%s dB: %d, want %d", tc.body, got, tc.want)
		}
		_, _, body := ss.handle("GET_PARAMETER", h, []byte("volume"))
		if len(body) == 0 {
			t.Fatal("missing volume response")
		}
	}
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		if code, _, _ := ss.handle("SET_PARAMETER", h, []byte("volume: "+value)); code != 400 {
			t.Fatal("invalid volume accepted", value)
		}
	}
}
func FuzzRTP(f *testing.F) {
	f.Add([]byte{0x80, 96, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ParseRTP(b, nil, nil) })
}
