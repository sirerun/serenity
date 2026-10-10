package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/sirerun/serenity/internal/hosted/admintransport"
	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/plans"
	"github.com/sirerun/serenity/internal/hosted/runtimebroker"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/writer"
	"github.com/spf13/cobra"
)

func newHostedCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "hosted", Short: "Operate the managed Serenity service"}
	cmd.AddCommand(&cobra.Command{Use: "plans", Short: "Print the version 1 hosted pricing and limits", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		data, err := plans.JSON()
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}})
	var configPath, socket string
	serve := &cobra.Command{Use: "serve", Short: "Serve hosted signup, dashboard and authenticated memory", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return runHosted(ctx, configPath, socket)
	}}
	serve.Flags().StringVar(&configPath, "config", "", "hosted JSON configuration file")
	serve.Flags().StringVar(&socket, "broker-socket", "", "root-owned managed process broker socket")
	mustRequired(serve, "config")
	cmd.AddCommand(serve)
	var brokerConfig string
	broker := &cobra.Command{Use: "broker", Short: "Run the privileged hosted authority broker", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		cfg, err := runtimebroker.LoadConfig(ctx, brokerConfig)
		if err != nil {
			return err
		}
		return runtimebroker.Serve(ctx, cfg)
	}}
	broker.Flags().StringVar(&brokerConfig, "config", "", "root-only broker configuration file")
	mustRequired(broker, "config")
	cmd.AddCommand(broker)
	var backupDir, snapshot string
	backupCmd := &cobra.Command{Use: "backup", Short: "Request a witnessed backup from the running service", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if _, err := os.Stat(filepath.Join(backupDir, ".hosted-admin.sock")); err != nil {
			return fmt.Errorf("%w: live backup coordinator required: %v", service.ErrStartupUnavailable, err)
		}
		return backup.Request(cmd.Context(), backupDir, snapshot, "", nil)
	}}
	backupCmd.Flags().StringVar(&backupDir, "data-dir", "", "hosted data directory")
	backupCmd.Flags().StringVar(&snapshot, "snapshot", "", "local snapshot directory")
	mustRequired(backupCmd, "data-dir", "snapshot")
	cmd.AddCommand(backupCmd)
	var restoreDir, restoreSnapshot, operatorConfig string
	restore := &cobra.Command{Use: "restore", Short: "Verify a witnessed snapshot and restore frozen accounts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if operatorConfig == "" {
			return fmt.Errorf("%w: explicit operator configuration required", service.ErrStartupUnavailable)
		}
		client, closeClient, err := runtimebroker.OpenOperator(cmd.Context(), operatorConfig)
		if err != nil {
			return err
		}
		defer closeClient()
		return backup.RestoreAuthorized(cmd.Context(), restoreSnapshot, restoreDir, client)
	}}
	restore.Flags().StringVar(&restoreDir, "data-dir", "", "new destination for frozen restored data")
	restore.Flags().StringVar(&restoreSnapshot, "snapshot", "", "witnessed local snapshot directory")
	restore.Flags().StringVar(&operatorConfig, "operator-config", "", "root-only explicit operator profile configuration")
	mustRequired(restore, "data-dir", "snapshot")
	cmd.AddCommand(restore)
	cmd.AddCommand(newHostedAuthorityCmd())
	return cmd
}
func mustRequired(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}
}

func runHosted(ctx context.Context, configPath, socket string) (resultErr error) {
	if socket == "" {
		return fmt.Errorf("%w: root broker socket required", service.ErrStartupUnavailable)
	}
	cfg, err := service.Load(configPath)
	if err != nil {
		return err
	}
	if err = cfg.Validate(false); err != nil {
		return err
	}
	if err = os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return err
	}
	lock, err := writer.AcquireBrain(cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, lock.Close()) }()
	admission, err := runtimebroker.OpenSession(ctx, socket, cfg.DataDir)
	if err != nil {
		return err
	}
	var s *service.Service
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if s != nil {
			resultErr = errors.Join(resultErr, s.CloseContext(cleanup))
		} else {
			resultErr = errors.Join(resultErr, admission.Close(cleanup))
		}
	}()
	deps := service.LifecycleDependencies{Journal: admission.Journal(), BuildSHA: admission.BuildSHA(), Startup: admission, JournalReader: admission.Reader(), StartupLease: admission, AuthorizedBackup: func(ctx context.Context, destination string) error {
		return backup.CreateAuthorized(ctx, cfg.DataDir, destination, admission.BuildSHA(), admission.Authority(), admission.Lease())
	}}
	s, err = service.NewWithDependencies(ctx, cfg, false, os.Stderr, deps)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Bind)
	if err != nil {
		return err
	}
	admin, err := admintransport.Listen(ctx, filepath.Join(cfg.DataDir, ".hosted-admin.sock"))
	if err != nil {
		return errors.Join(err, listener.Close())
	}
	requests, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	publicTracker := &hostedRequests{next: s.Handler}
	adminTracker := &hostedRequests{next: s.AdminHandler()}
	public := &http.Server{Handler: publicTracker, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 32 * 1024, BaseContext: func(net.Listener) context.Context { return requests }}
	private, err := admin.HTTPServer(adminTracker)
	if err != nil {
		return errors.Join(err, listener.Close(), admin.Close())
	}
	private.BaseContext = func(net.Listener) context.Context { return requests }
	lifetime, cancelLifetime := context.WithCancel(ctx)
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		select {
		case <-admission.Done():
			cancelLifetime()
		case <-lifetime.Done():
		}
	}()
	serveErr := serveHosted(lifetime, cancelRequests, public, private, listener, admin, publicTracker, adminTracker)
	cancelLifetime()
	<-monitorDone
	return errors.Join(serveErr, admission.Err())
}

// hostedRequests fences new handler admission and joins even handlers whose
// connections were forcibly closed. Server.Close alone does not join handlers.
type hostedRequests struct {
	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
	next    http.Handler
}

func (h *hostedRequests) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		http.Error(w, "Service shutting down", http.StatusServiceUnavailable)
		return
	}
	h.wg.Add(1)
	h.mu.Unlock()
	defer h.wg.Done()
	h.next.ServeHTTP(w, r)
}
func (h *hostedRequests) stop() { h.mu.Lock(); h.stopped = true; h.mu.Unlock() }
func serveHosted(ctx context.Context, cancelRequests context.CancelFunc, public, private *http.Server, listener, admin net.Listener, a, b *hostedRequests) error {
	results := make(chan error, 2)
	go func() { results <- public.Serve(listener) }()
	go func() { results <- private.Serve(admin) }()
	var first error
	received := 0
	select {
	case <-ctx.Done():
	case first = <-results:
		received = 1
	}
	a.stop()
	b.stop()
	cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	shutdown := make(chan error, 2)
	go func() { shutdown <- public.Shutdown(cleanup) }()
	go func() { shutdown <- private.Shutdown(cleanup) }()
	errA, errB := <-shutdown, <-shutdown
	cancelRequests()
	closeA, closeB := public.Close(), private.Close()
	a.wg.Wait()
	b.wg.Wait()
	for received < 2 {
		err := <-results
		if !errors.Is(err, http.ErrServerClosed) {
			first = errors.Join(first, err)
		}
		received++
	}
	if errors.Is(first, http.ErrServerClosed) {
		first = nil
	}
	if errors.Is(closeA, net.ErrClosed) {
		closeA = nil
	}
	if errors.Is(closeB, net.ErrClosed) {
		closeB = nil
	}
	return errors.Join(first, errA, errB, closeA, closeB)
}
