package airplay

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// SDP describes the classic RAOP audio stream. FairPlay and AirPlay 2 are
// deliberately not advertised: they require a different authenticated protocol.
type SDP struct {
	Codec          string
	Rate, Channels int
	ALAC           []uint32
	Key, IV        []byte
}

func ParseSDP(body string) (SDP, error) {
	s := SDP{Rate: 44100, Channels: 2}
	var encrypted, iv string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r", ""), "\n") {
		if strings.HasPrefix(line, "a=rtpmap:") {
			p := strings.Fields(line)
			if len(p) == 2 {
				v := strings.Split(p[1], "/")
				s.Codec = v[0]
				if len(v) > 1 {
					s.Rate, _ = strconv.Atoi(v[1])
				}
				if len(v) > 2 {
					s.Channels, _ = strconv.Atoi(v[2])
				}
			}
		}
		if strings.HasPrefix(line, "a=fmtp:") {
			p := strings.Fields(line)
			for _, v := range p[1:] {
				n, err := strconv.ParseUint(v, 10, 32)
				if err != nil {
					return s, errors.New("invalid ALAC configuration")
				}
				s.ALAC = append(s.ALAC, uint32(n))
			}
		}
		if strings.HasPrefix(line, "a=rsaaeskey:") {
			encrypted = strings.TrimPrefix(line, "a=rsaaeskey:")
		}
		if strings.HasPrefix(line, "a=aesiv:") {
			iv = strings.TrimPrefix(line, "a=aesiv:")
		}
		if strings.HasPrefix(line, "a=fpaeskey:") {
			return s, errors.New("FairPlay encrypted streams are not supported")
		}
	}
	if s.Codec == "AppleLossless" {
		if len(s.ALAC) != 11 {
			return s, errors.New("ALAC requires eleven format parameters")
		}
		s.Rate = int(s.ALAC[10])
		s.Channels = int(s.ALAC[6])
		if s.ALAC[0] == 0 || s.ALAC[0] > 4096 || s.ALAC[2] != 16 && s.ALAC[2] != 24 {
			return s, errors.New("unsupported ALAC frame format")
		}
	} else if s.Codec != "L16" {
		return s, errors.New("supported RAOP codecs: ALAC and L16")
	}
	if s.Rate < 8000 || s.Rate > 96000 || s.Channels < 1 || s.Channels > 2 {
		return s, errors.New("unsupported sample format")
	}
	if encrypted != "" {
		var err error
		s.Key, err = decryptKey(encrypted)
		if err != nil {
			return s, err
		}
		s.IV, err = decode64(iv)
		if err != nil || len(s.IV) != 16 || len(s.Key) != 16 {
			return s, errors.New("invalid AES parameters")
		}
	}
	return s, nil
}
func be(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func box(name string, parts ...[]byte) []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 4))
	b.WriteString(name)
	for _, p := range parts {
		b.Write(p)
	}
	out := b.Bytes()
	binary.BigEndian.PutUint32(out, uint32(len(out)))
	return out
}
func full(name string, flags uint32, parts ...[]byte) []byte {
	return box(name, append([][]byte{be(flags)}, parts...)...)
}
func zeros(n int) []byte { return make([]byte, n) }

var matrix = []byte{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x40, 0, 0, 0}

// Fragmented MP4 gives FFmpeg a streaming ALAC container without a seekable file.
func (s SDP) initMP4() []byte {
	a := s.ALAC
	cookie := append(be(a[0]), byte(a[1]), byte(a[2]), byte(a[3]), byte(a[4]), byte(a[5]), byte(a[6]), byte(a[7]>>8), byte(a[7]))
	cookie = append(cookie, be(a[8])...)
	cookie = append(cookie, be(a[9])...)
	cookie = append(cookie, be(a[10])...)
	sample := append(zeros(6), 0, 1)
	sample = append(sample, zeros(8)...)
	sample = append(sample, 0, byte(s.Channels), 0, byte(a[2]), 0, 0, 0, 0)
	sample = append(sample, be(uint32(s.Rate)<<16)...)
	stbl := box("stbl", full("stsd", 0, be(1), box("alac", sample, full("alac", 0, cookie))), full("stts", 0, be(0)), full("stsc", 0, be(0)), full("stsz", 0, be(0), be(0)), full("stco", 0, be(0)))
	minf := box("minf", full("smhd", 0, zeros(4)), box("dinf", full("dref", 0, be(1), full("url ", 1))), stbl)
	mdia := box("mdia", full("mdhd", 0, zeros(8), be(uint32(s.Rate)), be(0), []byte{0x55, 0xc4, 0, 0}), full("hdlr", 0, be(0), []byte("soun"), zeros(12), []byte("MiAir audio\x00")), minf)
	tkhd := full("tkhd", 7, zeros(8), be(1), zeros(16), []byte{0, 0, 0, 0, 1, 0, 0, 0}, matrix, zeros(8))
	mvhd := full("mvhd", 0, zeros(8), be(uint32(s.Rate)), be(0), be(0x10000), []byte{1, 0}, zeros(10), matrix, zeros(24), be(2))
	moov := box("moov", mvhd, box("trak", tkhd, mdia), box("mvex", full("trex", 0, be(1), be(1), be(a[0]), be(0), be(0))))
	return append(box("ftyp", []byte("isom"), be(0x200), []byte("isomiso6mp41")), moov...)
}
func fragment(seq uint32, timestamp uint64, duration uint32, payload []byte) []byte {
	tfdt := make([]byte, 8)
	binary.BigEndian.PutUint64(tfdt, timestamp)
	makeMoof := func(offset uint32) []byte {
		return box("moof", full("mfhd", 0, be(seq)), box("traf", full("tfhd", 0x20000, be(1)), full("tfdt", 0x1000000, tfdt), full("trun", 0x301, be(1), be(offset), be(duration), be(uint32(len(payload))))))
	}
	moof := makeMoof(0)
	moof = makeMoof(uint32(len(moof) + 8))
	return append(moof, box("mdat", payload)...)
}

type Decoder struct {
	input     io.WriteCloser
	cancel    context.CancelFunc
	done      chan struct{}
	mu        sync.Mutex
	seq       uint32
	timestamp uint64
	sdp       SDP
	err       error
}

func NewDecoder(ctx context.Context, ffmpeg string, s SDP, output io.Writer) (*Decoder, error) {
	if ffmpeg == "" {
		return nil, errors.New("AirPlay requires FFmpeg")
	}
	ctx, cancel := context.WithCancel(ctx)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-probesize", "32768", "-analyzeduration", "0"}
	if s.Codec == "L16" {
		args = append(args, "-f", "s16be", "-ar", strconv.Itoa(s.Rate), "-ac", strconv.Itoa(s.Channels))
	} else {
		args = append(args, "-f", "mp4")
	}
	args = append(args, "-i", "pipe:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-write_xing", "0", "-flush_packets", "1", "-f", "mp3", "pipe:1")
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdout = output
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	d := &Decoder{input: input, cancel: cancel, done: make(chan struct{}), sdp: s}
	go func() { err := cmd.Wait(); d.mu.Lock(); d.err = err; d.mu.Unlock(); close(d.done) }()
	if s.Codec == "AppleLossless" {
		if _, err = input.Write(s.initMP4()); err != nil {
			d.Close()
			return nil, err
		}
	}
	return d, nil
}
func (d *Decoder) WritePacket(payload []byte) error {
	if d.sdp.Codec == "AppleLossless" {
		d.seq++
		p := fragment(d.seq, d.timestamp, d.sdp.ALAC[0], payload)
		d.timestamp += uint64(d.sdp.ALAC[0])
		_, err := d.input.Write(p)
		return err
	}
	_, err := d.input.Write(payload)
	return err
}
func (d *Decoder) Close() { d.cancel(); d.input.Close(); <-d.done }
