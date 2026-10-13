package service_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/service"
)

// TestSignupBrowserFixture is a source-test-only server with explicit synthetic
// lifecycle admission. It does not add a development switch to the shipped CLI.
func TestSignupBrowserFixture(t *testing.T) {
	configPath := os.Getenv("SERENITY_SIGNUP_FIXTURE_CONFIG")
	if configPath == "" {
		t.Skip("opt-in local signup browser fixture")
	}
	cfg, err := service.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := net.SplitHostPort(cfg.Bind)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		t.Fatal("browser fixture requires a loopback bind")
	}
	if cfg.PublicOrigin != "http://"+cfg.Bind {
		t.Fatal("browser fixture requires matching loopback origin")
	}
	svc, err := service.NewWithDependencies(context.Background(), cfg, true, os.Stdout, testLifecycleDependencies(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	}()
	server := &http.Server{Addr: cfg.Bind, Handler: svc.Handler, ReadHeaderTimeout: 5 * time.Second}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-signals.Done():
	case err := <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("signup browser fixture exceeded bounded lifetime")
	}
}
