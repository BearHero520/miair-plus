// Package config owns bounded, private, atomically persisted application state.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Speaker struct {
	DID           string `json:"did"`
	DeviceID      string `json:"device_id"`
	Name          string `json:"name"`
	DLNAName      string `json:"dlna_name"`
	Hardware      string `json:"hardware"`
	Enabled       bool   `json:"enabled"`
	Compatibility bool   `json:"compatibility_mode"`
}

func (s Speaker) DisplayName() string {
	if s.DLNAName != "" {
		return s.DLNAName
	}
	return s.Name
}

type Credentials struct {
	UserID       string `json:"user_id"`
	PassToken    string `json:"pass_token"`
	ServiceToken string `json:"service_token"`
	Security     string `json:"ssecurity"`
	DeviceID     string `json:"device_id"`
	IssuedAt     int64  `json:"issued_at"`
}
type State struct {
	AutoCheckUpdate bool               `json:"auto_check_update"`
	Alarms          []Alarm            `json:"alarms,omitempty"`
	Version         int                `json:"schema_version"`
	Username        string             `json:"username"`
	PasswordHash    string             `json:"password_hash"`
	Secret          string             `json:"secret"`
	AuthRevision    int                `json:"auth_revision"`
	Hostname        string             `json:"hostname"`
	DLNAPort        int                `json:"dlna_port"`
	AirPlay         bool               `json:"airplay_enabled"`
	AirPlay2        bool               `json:"airplay2_enabled"`
	AirPlay2Port    int                `json:"airplay2_port"`
	AirPlay2Target  string             `json:"airplay2_target"`
	AutoPlay        bool               `json:"auto_play_on_set_uri"`
	AutoRecover     bool               `json:"auto_restart"`
	DefaultVolume   int                `json:"default_volume"`
	CacheMB         int                `json:"cache_mb"`
	FFmpeg          string             `json:"ffmpeg_path"`
	AudioID         string             `json:"default_audio_id"`
	Xiaomi          Credentials        `json:"xiaomi"`
	Speakers        map[string]Speaker `json:"speakers"`
}
type Store struct {
	mu    sync.RWMutex
	path  string
	state State
}

func Random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "miair-plus.json"), state: State{AutoCheckUpdate: true, Version: 1, Secret: Random(32), DLNAPort: 8311, AirPlay2Port: 7000, AirPlay: true, AutoPlay: true, AutoRecover: true, DefaultVolume: 40, CacheMB: 32, AudioID: "1582971365183456177", Speakers: map[string]Speaker{}}}
	b, err := os.ReadFile(s.path)
	if err == nil {
		if err = json.Unmarshal(b, &s.state); err != nil {
			return nil, errors.New("配置文件损坏，请从备份恢复；程序未覆盖原文件")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &fields) == nil {
			if _, exists := fields["auto_check_update"]; !exists {
				s.state.AutoCheckUpdate = true
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if s.state.Speakers == nil {
		s.state.Speakers = map[string]Speaker{}
	}
	if s.state.Secret == "" {
		s.state.Secret = Random(32)
	}
	if s.state.Xiaomi.DeviceID == "" {
		s.state.Xiaomi.DeviceID = Random(16)
	}
	if err = s.Update(func(*State) error { return nil }); err != nil {
		return nil, err
	}
	return s, nil
}
func clone(s State) State {
	result := s
	result.Alarms = append([]Alarm(nil), s.Alarms...)
	result.Speakers = make(map[string]Speaker, len(s.Speakers))
	for k, v := range s.Speakers {
		result.Speakers[k] = v
	}
	return result
}
func (s *Store) Snapshot() State   { s.mu.RLock(); defer s.mu.RUnlock(); return clone(s.state) }
func (s *Store) Directory() string { return filepath.Dir(s.path) }
func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.state)
	if err := fn(&next); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".config-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.state = next
	return nil
}
