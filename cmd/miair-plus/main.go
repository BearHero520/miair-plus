package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/server"
	"github.com/BearHero520/miair-plus/internal/xiaomi"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	data := flag.String("data", "data", "private data directory")
	listen := flag.String("listen", ":8310", "management listen address")
	noDiscovery := flag.Bool("no-discovery", false, "disable multicast advertising for development")
	version := flag.Bool("version", false, "print version")
	flag.Parse()
	if *version {
		fmt.Println("MiAir Plus " + server.Version)
		return nil
	}
	store, err := config.Open(*data)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := xiaomi.New(store)
	manager := server.NewManager(ctx, store, client, *noDiscovery)
	api := server.NewAPI(ctx, store, manager, client)
	log.SetOutput(io.MultiWriter(os.Stderr, api))
	go manager.Run()
	go client.Maintain(ctx)
	httpServer := &http.Server{Addr: *listen, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768}
	done := make(chan error, 1)
	go func() {
		log.Printf("MiAir Plus %s · Go · %s", server.Version, *listen)
		done <- httpServer.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
	case err = <-done:
		if err != http.ErrServerClosed {
			cancel()
			manager.Close()
			return err
		}
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	_ = httpServer.Shutdown(shutdown)
	manager.Close()
	return nil
}
