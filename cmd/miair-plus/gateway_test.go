package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGatewaySocketLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fnOS socket permissions require Linux")
	}
	path := filepath.Join(t.TempDir(), "app.sock")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenGateway(path); err == nil {
		t.Fatal("ordinary file replaced")
	}
	os.Remove(path)
	first, err := listenGateway(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := listenGateway(path); err == nil {
		t.Fatal("live socket replaced")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0666 {
		t.Fatal("gateway cannot access socket", err)
	}
	first.Close()
	second, err := listenGateway(path)
	if err != nil {
		t.Fatal("restart failed", err)
	}
	second.Close()
}
