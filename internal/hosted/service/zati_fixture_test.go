package service_test

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
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
	development := true
	if os.Getenv("SERENITY_OAUTH_OFFICIAL_FIXTURE") == "1" {
		host, _, bindErr := net.SplitHostPort(cfg.Bind)
		embedding, embeddingErr := url.Parse(cfg.EmbeddingBaseURL)
		if cfg.PublicOrigin != "https://serenity.sire.run" || bindErr != nil || host != "127.0.0.1" || embeddingErr != nil || embedding.Scheme != "https" || embedding.Hostname() != "127.0.0.1" || embedding.Port() == "" {
			t.Fatal("official issuer fixture requires pinned issuer and loopback dependencies")
		}
		development = false
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	var svc *service.Service
	if development {
		svc, err = service.NewWithDependencies(ctx, cfg, true, os.Stdout, testLifecycleDependencies(t))
	} else {
		if err = cfg.Validate(false); err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Join(cfg.DataDir, "brains"), 0700); err != nil {
			t.Fatal(err)
		}
		var db *store.Store
		db, err = store.Open(filepath.Join(cfg.DataDir, "control.db"))
		if err != nil {
			t.Fatal(err)
		}
		svc, err = service.AssembleWithDependencies(ctx, cfg, false, db, &identity.DevSender{Writer: os.Stdout}, embedding{}, testLifecycleDependencies(t))
		if err != nil {
			_ = db.Close()
		}
	}
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
