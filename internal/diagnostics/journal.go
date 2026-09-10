// Package diagnostics records bounded, redacted operational events.
package diagnostics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const Capacity = 1000
const fileLimit = 2 << 20

type Entry struct {
	ID      uint64            `json:"id"`
	Time    time.Time         `json:"time"`
	Level   string            `json:"level"`
	Module  string            `json:"module"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}
type Journal struct {
	mu      sync.Mutex
	entries []Entry
	next    uint64
	path    string
	failure string
}

var secret = regexp.MustCompile(`(?i)(pass_?token|service_?token|ssecurity|cookie|authorization|password|token|secret)["']?\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
var urls = regexp.MustCompile(`https?://[^\s"'<>]+`)

func Redact(s string) string {
	s = secret.ReplaceAllString(s, "$1=[已隐藏]")
	s = urls.ReplaceAllStringFunc(s, func(raw string) string {
		u, e := url.Parse(raw)
		if e != nil {
			return "[地址已隐藏]"
		}
		return u.Scheme + "://" + u.Host + "/[地址已隐藏]"
	})
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	if len(s) > 4096 {
		s = string([]rune(s)[:min(len([]rune(s)), 1024)]) + "…"
	}
	return s
}
func Event(level, module, message string, details map[string]string) {
	b, _ := json.Marshal(Entry{Time: time.Now(), Level: level, Module: module, Message: message, Details: details})
	log.Printf("MIAIR_EVENT %s", b)
}
func New(dir string) *Journal {
	j := &Journal{entries: []Entry{}, path: filepath.Join(dir, "events.jsonl")}
	if e := os.MkdirAll(dir, 0700); e != nil {
		j.failure = e.Error()
		return j
	}
	for _, p := range []string{j.path + ".1", j.path} {
		f, e := os.Open(p)
		if e != nil {
			continue
		}
		scan := bufio.NewScanner(f)
		scan.Buffer(make([]byte, 4096), 32768)
		for scan.Scan() {
			var v Entry
			if json.Unmarshal(scan.Bytes(), &v) == nil {
				j.clean(&v)
				j.entries = append(j.entries, v)
				if v.ID > j.next {
					j.next = v.ID
				}
				j.trim()
			}
		}
		f.Close()
	}
	return j
}
func (j *Journal) trim() {
	if len(j.entries) > Capacity {
		j.entries = append([]Entry(nil), j.entries[len(j.entries)-Capacity:]...)
	}
}
func (j *Journal) clean(e *Entry) {
	e.Message = Redact(e.Message)
	e.Module = Redact(e.Module)
	if e.Level != "error" && e.Level != "warn" {
		e.Level = "info"
	}
	if len(e.Details) > 16 {
		e.Details = nil
	}
	for k, v := range e.Details {
		if secret.MatchString(k + "=x") {
			e.Details[k] = "[已隐藏]"
		} else {
			e.Details[k] = Redact(v)
		}
	}
}
func (j *Journal) Write(b []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		e := Entry{Time: time.Now(), Level: "info", Module: "系统", Message: line}
		if _, raw, ok := strings.Cut(line, "MIAIR_EVENT "); ok {
			var parsed Entry
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				e = parsed
			}
		}
		if e.Message == line {
			if len(line) > 20 && line[4] == '/' && line[10] == ' ' {
				e.Message = line[20:]
			}
			switch {
			case strings.Contains(line, "FFmpeg"):
				e.Module = "FFmpeg"
			case strings.Contains(line, "AirPlay 2"):
				e.Module = "AirPlay 2"
			case strings.Contains(line, "AirPlay"):
				e.Module = "AirPlay"
			case strings.Contains(line, "DLNA"):
				e.Module = "DLNA"
			}
			lower := strings.ToLower(line)
			if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(line, "失败") || strings.Contains(line, "中断") {
				e.Level = "error"
			}
		}
		j.next++
		e.ID = j.next
		j.clean(&e)
		j.entries = append(j.entries, e)
		j.trim()
		data, _ := json.Marshal(e)
		data = append(data, '\n')
		if err := j.persist(data); err != nil {
			if j.failure != Redact(err.Error()) {
				fmt.Fprintf(os.Stderr, "日志写入失败: %s\n", Redact(err.Error()))
			}
			j.failure = Redact(err.Error())
		} else {
			j.failure = ""
		}
	}
	return len(b), nil
}
func (j *Journal) persist(b []byte) error {
	if st, e := os.Stat(j.path); e == nil && st.Size()+int64(len(b)) > fileLimit {
		if e = os.Remove(j.path + ".1"); e != nil && !os.IsNotExist(e) {
			return e
		}
		if e = os.Rename(j.path, j.path+".1"); e != nil {
			return e
		}
	}
	f, e := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func (j *Journal) Snapshot() ([]Entry, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := append([]Entry{}, j.entries...)
	for i := range out {
		if out[i].Details != nil {
			d := map[string]string{}
			for k, v := range out[i].Details {
				d[k] = v
			}
			out[i].Details = d
		}
	}
	return out, j.failure
}
func Lines(entries []Entry) []string {
	out := []string{}
	for _, e := range entries {
		d := ""
		if len(e.Details) > 0 {
			b, _ := json.Marshal(e.Details)
			d = " " + string(b)
		}
		out = append(out, fmt.Sprintf("%s [%s] [%s] %s%s", e.Time.Format(time.RFC3339), e.Level, e.Module, e.Message, d))
	}
	return out
}
