package airplay

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"time"
)

type Packet struct {
	Seq     uint16
	Payload []byte
}

func ParseRTP(b []byte, key, iv []byte) (Packet, error) {
	if len(b) < 12 || b[0]>>6 != 2 {
		return Packet{}, errors.New("invalid RTP")
	}
	offset := 12 + int(b[0]&15)*4
	if offset > len(b) {
		return Packet{}, errors.New("truncated CSRC")
	}
	if b[0]&16 != 0 {
		if offset+4 > len(b) {
			return Packet{}, errors.New("truncated extension")
		}
		offset += 4 + int(binary.BigEndian.Uint16(b[offset+2:]))*4
	}
	end := len(b)
	if b[0]&32 != 0 {
		pad := int(b[end-1])
		if pad == 0 {
			return Packet{}, errors.New("invalid padding")
		}
		end -= pad
	}
	if offset >= end {
		return Packet{}, errors.New("empty RTP")
	}
	payload := append([]byte(nil), b[offset:end]...)
	if len(key) > 0 {
		block, err := aes.NewCipher(key)
		if err != nil || len(iv) != aes.BlockSize {
			return Packet{}, errors.New("invalid AES")
		}
		n := len(payload) / aes.BlockSize * aes.BlockSize
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(payload[:n], payload[:n])
	}
	return Packet{binary.BigEndian.Uint16(b[2:4]), payload}, nil
}

// Jitter is used by one session goroutine. Sequence arithmetic handles wrap;
// bounded reordering plus retransmission requests avoids unbounded latency.
type Jitter struct {
	packets   map[uint16]Packet
	next      uint16
	started   bool
	gap       time.Time
	requested time.Time
}

func (j *Jitter) Reset() { *j = Jitter{packets: map[uint16]Packet{}} }
func (j *Jitter) Push(p Packet) {
	if j.packets == nil {
		j.Reset()
	}
	if !j.started {
		j.next = p.Seq
		j.started = true
	}
	distance := int16(p.Seq - j.next)
	if distance < 0 {
		return
	}
	if distance > 512 {
		j.Reset()
		j.started = true
		j.next = p.Seq
	}
	j.packets[p.Seq] = p
}
func (j *Jitter) Drain(now time.Time, write func([]byte) error, resend func(uint16, uint16)) error {
	for j.started {
		if p, ok := j.packets[j.next]; ok {
			delete(j.packets, j.next)
			j.next++
			j.gap = time.Time{}
			if err := write(p.Payload); err != nil {
				return err
			}
			continue
		}
		if len(j.packets) == 0 {
			j.gap = time.Time{}
			return nil
		}
		if j.gap.IsZero() {
			j.gap = now
		}
		if now.Sub(j.requested) > 25*time.Millisecond {
			resend(j.next, 1)
			j.requested = now
		}
		if now.Sub(j.gap) < 120*time.Millisecond {
			return nil
		}
		j.next++
		j.gap = now
	}
	return nil
}
