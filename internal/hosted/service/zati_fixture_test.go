package service_test

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/service"
)

// TestZatiHostedAssemblyProcess is an opt-in process fixture. It runs actual
// hosted config, OAuth, HTTP handlers and durable storage with the established
// synthetic lifecycle genesis. It is not a managed CLI release/broker proof.
func TestZatiHostedAssemblyProcess(t *testing.T) {
	if os.Getenv("SERENITY_ASSEMBLY_FIXTURE") != "1" {
		t.Skip("external assembly fixture not requested")
	}
	if len(os.Args) < 2 {
		t.Fatal("fixture config is required")
	}
	path := os.Args[len(os.Args)-1]
	if !filepath.IsAbs(path) {
		t.Fatal("fixture config must be absolute")
	}
	cfg, err := service.Load(path)
	if err != nil {
		t.Fatal("fixture config invalid")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	svc, err := service.NewWithDependencies(ctx, cfg, true, os.Stdout, testLifecycleDependencies(t))
	if err != nil {
		t.Fatalf("fixture assembly failed: %v", err)
	}
	server := &http.Server{Addr: cfg.Bind, Handler: svc.Handler, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if err != http.ErrServerClosed {
			t.Fatalf("fixture HTTP server: %v", err)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			t.Errorf("fixture shutdown: %v", err)
		}
		if err := <-done; err != http.ErrServerClosed {
			t.Errorf("fixture HTTP shutdown: %v", err)
		}
	}
	if err := svc.Close(); err != nil {
		t.Errorf("fixture storage close: %v", err)
	}
}
