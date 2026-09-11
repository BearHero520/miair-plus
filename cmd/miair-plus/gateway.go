package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

func listenGateway(path string) (net.Listener, error) {
	// Refuse to remove ordinary files, and never unlink a live service socket.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("gateway path is not a socket: %s", path)
		}
		conn, dialErr := net.DialTimeout("unix", path, time.Second)
		if dialErr == nil {
			conn.Close()
			return nil, fmt.Errorf("gateway socket already in use: %s", path)
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("cannot verify stale gateway socket: %w", dialErr)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen gateway: %w", err)
	}
	// fnOS gateway runs as a different system account. Business APIs retain token auth.
	if err := os.Chmod(path, 0666); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}
