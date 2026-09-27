package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sirerun/serenity/internal/writer"

	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
	"github.com/sirerun/serenity/internal/hosted/plans"
	"github.com/sirerun/serenity/internal/hosted/service"
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
	var path string
	serve := &cobra.Command{Use: "serve", Short: "Serve hosted signup, dashboard and authenticated memory", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) (runErr error) {
		cfg, err := service.Load(path)
		if err != nil {
			return err
		}
		if cfg.BuildIdentity == "" {
			cfg.BuildIdentity = Version
		}
		if err = cfg.Validate(os.Getenv("SERENITY_HOSTED_DEV") == "1"); err != nil {
			return err
		}
		if err = os.MkdirAll(cfg.DataDir, 0700); err != nil {
			return err
		}
		owner, err := writer.AcquireBrain(cfg.DataDir)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, owner.Close()) }()
		s, err := service.New(cfg, os.Getenv("SERENITY_HOSTED_DEV") == "1", cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, s.Close()) }()
		socket := filepath.Join(cfg.DataDir, ".hosted-admin.sock")
		if err = os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		adminListener, err := net.Listen("unix", socket)
		if err != nil {
			return err
		}
		if err = os.Chmod(socket, 0600); err != nil {
			_ = adminListener.Close()
			return err
		}
		adminServer := &http.Server{Handler: s.AdminHandler(), ReadHeaderTimeout: 5 * time.Second}
		adminDone := make(chan error, 1)
		go func() { adminDone <- adminServer.Serve(adminListener) }()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			e := adminServer.Shutdown(shutdownCtx)
			if e != nil {
				e = errors.Join(e, adminServer.Close())
			}
			serveErr := <-adminDone
			if !errors.Is(serveErr, http.ErrServerClosed) {
				e = errors.Join(e, serveErr)
			}
			runErr = errors.Join(runErr, e)
		}()
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		server := &http.Server{Addr: cfg.Bind, Handler: s.Handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		done := make(chan error, 1)
		go func() { done <- server.ListenAndServe() }()
		select {
		case err = <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err = server.Shutdown(shutdownCtx)
			if err != nil {
				err = errors.Join(err, server.Close())
			}
			serveErr := <-done
			if !errors.Is(serveErr, http.ErrServerClosed) {
				err = errors.Join(err, serveErr)
			}
			return err
		}
	}}
	serve.Flags().StringVar(&path, "config", "", "hosted JSON configuration file")
	if err := serve.MarkFlagRequired("config"); err != nil {
		panic(err)
	}
	cmd.AddCommand(serve)
	for _, action := range []string{"backup", "restore"} {
		var dataDir, snapshot, journalBucket, journalRegion, journalDir string
		var journalGeneration int64
		child := &cobra.Command{Use: action, Short: action + " a hosted control snapshot and brain bundles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			if action == "backup" {
				var journal contracts.DeletionJournal
				_, socketErr := os.Stat(filepath.Join(dataDir, ".hosted-admin.sock"))
				if errors.Is(socketErr, os.ErrNotExist) {
					switch {
					case journalDir != "":
						var err error
						journal, err = deletion.NewFilesystemJournal(journalDir, "", journalGeneration, nil)
						if err != nil {
							return err
						}
					case journalBucket != "" && journalRegion != "":
						var err error
						journal, err = deletion.NewAWSJournal(cmd.Context(), journalBucket, journalRegion, "", journalGeneration)
						if err != nil {
							return err
						}
					default:
						return errors.New("offline hosted backup requires --journal-dir or both --journal-bucket and --journal-region")
					}
				} else if socketErr != nil {
					return socketErr
				}
				return backup.Request(cmd.Context(), dataDir, snapshot, Version, journal)
			}
			return backup.Restore(cmd.Context(), snapshot, dataDir)
		}}
		child.Flags().StringVar(&dataDir, "data-dir", "", "hosted data directory")
		child.Flags().StringVar(&snapshot, "snapshot", "", "local snapshot directory")
		if action == "backup" {
			child.Flags().StringVar(&journalBucket, "journal-bucket", "", "production deletion journal S3 bucket (offline backup only)")
			child.Flags().StringVar(&journalRegion, "journal-region", "", "AWS region for the deletion journal bucket")
			child.Flags().StringVar(&journalDir, "journal-dir", "", "explicit local filesystem deletion journal (development only)")
			child.Flags().Int64Var(&journalGeneration, "journal-generation", 1, "active deletion journal generation")
		}
		if err := child.MarkFlagRequired("data-dir"); err != nil {
			panic(err)
		}
		if err := child.MarkFlagRequired("snapshot"); err != nil {
			panic(err)
		}
		cmd.AddCommand(child)
	}
	return cmd
}
