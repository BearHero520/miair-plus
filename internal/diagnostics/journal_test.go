package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRedactionPersistenceAndSnapshotIsolation(t *testing.T) {
	dir := t.TempDir()
	j := New(dir)
	raw, _ := json.Marshal(Entry{Level: "error", Module: "DLNA", Message: "fetch https://user:private@example.com/song?token=private passToken=private", Details: map[string]string{"password": "private", "原因": "Cookie=private", "音箱": "客厅"}})
	j.Write(append([]byte("MIAIR_EVENT "), raw...))
	b, e := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "private") {
		t.Fatal("credential leaked", string(b))
	}
	restored := New(dir)
	entries, problem := restored.Snapshot()
	if len(entries) != 1 || problem != "" || entries[0].Details["音箱"] != "客厅" {
		t.Fatal(entries, problem)
	}
	entries[0].Details["音箱"] = "changed"
	again, _ := restored.Snapshot()
	if again[0].Details["音箱"] != "客厅" {
		t.Fatal("snapshot aliases journal")
	}
	restored.Write([]byte("next"))
	again, _ = restored.Snapshot()
	if again[1].ID <= again[0].ID {
		t.Fatal("IDs reset on restart")
	}
}
func TestRotationAndConcurrentBound(t *testing.T) {
	dir := t.TempDir()
	j := New(dir)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 260; n++ {
				j.Write([]byte("operation"))
			}
		}()
	}
	wg.Wait()
	entries, err := j.Snapshot()
	if len(entries) != Capacity || err != "" {
		t.Fatal(len(entries), err)
	}
	if e := os.WriteFile(j.path, []byte(strings.Repeat("x", fileLimit)), 0600); e != nil {
		t.Fatal(e)
	}
	j.Write([]byte("after rotation"))
	st, e := os.Stat(j.path)
	if e != nil || st.Size() > fileLimit {
		t.Fatal(st, e)
	}
	if _, e = os.Stat(j.path + ".1"); e != nil {
		t.Fatal(e)
	}
}
