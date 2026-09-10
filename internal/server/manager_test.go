package server

import (
	"context"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/xiaomi"
	"net"
	"sync"
	"testing"
)

func TestManagerConcurrentReadsAndApply(t *testing.T) {
	s, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	_ = s.Update(func(st *config.State) error {
		st.Hostname = "127.0.0.1"
		st.DLNAPort = port
		st.AirPlay = false
		st.Xiaomi.UserID = "fixture"
		st.Speakers["test"] = config.Speaker{DID: "test", Name: "Test", Enabled: true}
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(ctx, s, xiaomi.New(s), true)
	defer m.Close()
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				m.Info()
				m.Snapshots()
			}
		}
	}()
	for i := 0; i < 5; i++ {
		if err = m.Apply(); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Update(func(st *config.State) error {
		sp := st.Speakers["test"]
		sp.Enabled = false
		st.Speakers["test"] = sp
		return nil
	})
	if err = m.Apply(); err != nil {
		t.Fatal(err)
	}
	close(done)
	wg.Wait()
}
