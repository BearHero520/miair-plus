package config

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicConfigAndConcurrentUpdates(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Update(func(st *State) error { st.AuthRevision++; return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	before := s.Snapshot()
	if before.AuthRevision != 20 {
		t.Fatal(before.AuthRevision)
	}
	_ = s.Update(func(st *State) error { st.Secret = "bad"; return errors.New("abort") })
	if s.Snapshot().Secret != before.Secret {
		t.Fatal("failed update leaked")
	}
	again, err := Open(dir)
	if err != nil || again.Snapshot().AuthRevision != 20 {
		t.Fatal(err)
	}
	copy := s.Snapshot()
	copy.Speakers["bad"] = Speaker{}
	if len(s.Snapshot().Speakers) != 0 {
		t.Fatal("snapshot aliases state")
	}
}
func TestCorruptConfigNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "miair-plus.json")
	_ = os.WriteFile(path, []byte("broken"), 0600)
	if _, err := Open(dir); err == nil {
		t.Fatal("invalid config accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "broken" {
		t.Fatal("corrupt original overwritten")
	}
}
