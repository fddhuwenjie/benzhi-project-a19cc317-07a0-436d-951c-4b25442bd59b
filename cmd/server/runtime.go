package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cave-microclimate-clearance/internal/application"
	"cave-microclimate-clearance/internal/evidence"
	"cave-microclimate-clearance/internal/httpapi"
	"cave-microclimate-clearance/internal/store"
)

func buildServer(dataDir string) (*http.Server, error) {
	repo, err := store.NewFileStore(dataDir)
	if err != nil {
		return nil, err
	}
	app := application.NewService(repo)
	ev := evidence.NewService(repo)
	api := httpapi.New(app, ev)
	return &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10}, nil
}

func serveNormal(c config) error {
	srv, err := buildServer(c.DataDir)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return err
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	log.Printf("洞穴微气候放行服务监听 %s", c.Addr)
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-sigCtx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return err
		}
		err = <-serveErr
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
