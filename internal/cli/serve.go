package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/direction"
	coredisposition "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/events"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/secrets"
	"github.com/sirerun/serenity/internal/server"
	serverdirection "github.com/sirerun/serenity/internal/server/direction"
	serverdisposition "github.com/sirerun/serenity/internal/server/disposition"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/server/memory"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "serve", Short: "Serve MCP over stdio or authenticated Streamable HTTP", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) (runErr error) {
		stdio, err := cmd.Flags().GetBool("stdio")
		if err != nil {
			return err
		}
		httpMode, err := cmd.Flags().GetBool("http")
		if err != nil {
			return err
		}
		if !stdio && !httpMode {
			return fmt.Errorf("choose --stdio or --http to serve MCP")
		}
		profile, hasProfile, err := resolveCredentialProfile(cmd)
		if err != nil {
			return err
		}
		if stdio {
			// stdio has no bearer-token authentication at all (RFC-BRAIN-AUTH-02:
			// "profile+stdio must reject rather than imply HTTP authentication
			// applies to stdio") -- a profile flag here would silently do nothing,
			// which is worse than refusing.
			if hasProfile {
				return fmt.Errorf("--%s has no effect with --stdio: stdio has no bearer-token authentication to select a credential for", credentialProfileFlagName)
			}
			return runServeStdio(cmd)
		}
		return runServeHTTP(cmd, profile, hasProfile)
	}}
	cmd.Flags().Bool("stdio", false, "read and write newline-delimited MCP JSON-RPC")
	cmd.Flags().Bool("http", false, "serve authenticated MCP Streamable HTTP at /mcp (RFC 0001 section 14)")
	addCredentialProfileFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive("stdio", "http")
	return cmd
}

// runServeStdio is `serenity serve --stdio` (T4.1/T4.5): the original
// newline-delimited JSON-RPC transport over the process's own stdin/
// stdout, unchanged by T4.21.
func runServeStdio(cmd *cobra.Command) (runErr error) {
	tools, closeDeps, _, _, err := memoryTools(flagRoot, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if closeDeps != nil {
		defer func() { runErr = errors.Join(runErr, closeDeps()) }()
	}
	mcpServer, err := mcp.New(Version, tools)
	if err != nil {
		return err
	}
	input := cmd.InOrStdin()
	output := cmd.OutOrStdout()
	if file, ok := input.(*os.File); ok {
		prepared, cleanup, err := pollableMCPFile(file)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, cleanup()) }()
		input = prepared
	}
	if file, ok := output.(*os.File); ok {
		prepared, cleanup, err := pollableMCPFile(file)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, cleanup()) }()
		output = prepared
	}
	closer, ok := input.(io.ReadCloser)
	if !ok {
		// In-memory readers used by embedded callers are finite. Arbitrary blocking
		// readers must expose Close so cancellation can unblock them.
		if _, finite := input.(interface{ Len() int }); !finite {
			return fmt.Errorf("MCP input must be closeable or an in-memory reader")
		}
		closer = io.NopCloser(input)
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return mcpServer.Serve(ctx, closer, output)
}

// runServeHTTP is `serenity serve --http` (T4.21): the authenticated MCP
// Streamable HTTP transport at /mcp. It reuses internal/server's existing
// loopback-by-default listener with bearer auth and optional mTLS (RFC
// 0001 section 14) wholesale -- no bespoke auth or listener path -- and
// the exact same memoryTools registry construction --stdio uses, so a
// client sees the same five MEMORY_VERBS tools over either transport.
//
// profile/hasProfile come from --credential-profile (RFC-BRAIN-AUTH-02):
// absent (hasProfile=false) is the exact legacy path, byte-for-byte; a
// given profile is validated by resolveCredentialProfile before this is
// ever called, so a malformed name never reaches here, but an
// unprovisioned valid one still must fail closed below rather than fall
// back to the legacy shared token.
func runServeHTTP(cmd *cobra.Command, profile string, hasProfile bool) (runErr error) {
	stderr := cmd.ErrOrStderr()
	tools, closeDeps, eng, q, err := memoryTools(flagRoot, stderr)
	if err != nil {
		return err
	}
	if closeDeps != nil {
		defer func() { runErr = errors.Join(runErr, closeDeps()) }()
	}
	mcpServer, err := mcp.New(Version, tools)
	if err != nil {
		return err
	}

	var tokenSource func() (string, error)
	if hasProfile {
		if _, err := secrets.ProfileDaemonToken(profile); err != nil {
			return fmt.Errorf("serve --http --%s %s: token missing -- run `serenity connect --%s %s --provision-token` first: %w", credentialProfileFlagName, profile, credentialProfileFlagName, profile, err)
		}
		tokenSource = func() (string, error) { return secrets.ProfileDaemonToken(profile) }
	} else {
		if _, err := secrets.DaemonToken(); err != nil {
			return fmt.Errorf("serve --http: daemon auth token missing -- run `serenity init` first: %w", err)
		}
		tokenSource = secrets.DaemonToken
	}

	cfg := server.FromBrainConfig(loadServerConfig(flagRoot))
	cfg.TokenSource = tokenSource
	srv := server.New(cfg)
	httpHandler := mcp.NewHTTPHandler(mcpServer)
	srv.Handle("/mcp", httpHandler)
	if eng != nil && q != nil {
		dispositionStore := coredisposition.NewStore(eng)
		serverdirection.New(direction.NewStore(flagRoot, q), dispositionStore, flagRoot).Register(srv)
		serverdisposition.New(dispositionStore, events.NewStore(eng)).Register(srv)
	}
	if err := srv.Listen(); err != nil {
		return fmt.Errorf("serve --http: %w", err)
	}
	// Report the bound endpoint without secrets (acc: "reports its bound
	// endpoint without secrets") -- the address only, never the bearer
	// token, which stays in the OS keychain.
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "serenity MCP HTTP listening on http://%s/mcp\n", srv.Addr())

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveErr := srv.Serve(ctx)
	// Cancel and join every in-flight tool call before the deferred
	// closeDeps above drains the writer queue and closes the index (acc:
	// "process shutdown closes the listener and joins workers before
	// draining the shared writer queue and closing the index").
	httpHandler.Close()
	if serveErr != nil && !errors.Is(serveErr, context.Canceled) && !errors.Is(serveErr, context.DeadlineExceeded) {
		return serveErr
	}
	return nil
}

// loadServerConfig loads serenity.yml's server: section for --http's
// listener config. Mirrors memoryTools' own "not a brain repo" tolerance
// (T4.1's bare-transport-smoke-test posture): outside a brain repo, or
// with no server: section, --http still starts, on the transport's own
// secure loopback-port-zero default.
func loadServerConfig(root string) config.Server {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return config.Server{}
	}
	return cfg.Server
}

// memoryTools builds MEMORY_VERBS v1's five tool registrations
// (internal/server/memory, T4.5) over root, the exact gap this task fills
// in what was previously an unconditional mcp.New(Version, nil).
//
// root not naming a brain repo (config.Load fails) is not a serve-time
// error: unlike ask/capture/compact, whose entire purpose is a brain
// operation, serve's own accepted scope (T4.1) is the daemon/transport
// core with no CLI-level dependency on a live brain -- protocol
// negotiation, ping, and shutdown must keep working even when --stdio is
// invoked outside a brain repo (a real posture for a bare transport
// smoke-test). This falls back to zero tools, the same shape mcp.New(Version,
// nil) already had before this task, noting the fallback on stderr (stdout
// is reserved for JSON-RPC frames) rather than silently changing nothing
// about serve's prior behavior. Once root does name a brain repo, opening
// its derived index for real (providers.OpenIndex) is no longer optional:
// a failure there is a genuine infra problem and is returned as a hard
// error, the same posture internal/cli/ask.go's own runAsk takes.
func memoryTools(root string, stderr io.Writer) ([]mcp.Tool, func() error, *index.SQLite, *writer.Queue, error) {
	_, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "serve: %s is not a brain repo -- serving MCP transport with no MEMORY_VERBS tools\n", root)
		return nil, nil, nil, nil, nil
	}

	owner, err := writer.AcquireBrain(root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// Re-read under ownership: config may have changed while recognizing the brain.
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return nil, nil, nil, nil, errors.Join(err, owner.Close())
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return nil, nil, nil, nil, errors.Join(fmt.Errorf("serve: open index: %w", err), owner.Close())
	}
	// The writer queue is this daemon's own owned resource (memory-compat-
	// mapping.md coordinator refinement #2: "stdio serve must close its
	// owned queue") -- closed alongside the index on shutdown, never left
	// running past Serve's own return.
	q := writer.NewQueue(nil)
	closeDeps := func() error {
		q.Close()
		_, flushErr := writer.Flush(q, root)
		return errors.Join(flushErr, eng.Close(), owner.Close())
	}

	ledger := &providers.IndexSpendLedger{Eng: eng}

	var embedder embed.Embedder
	if er, ok, note := providers.BuildEmbeddingRouter(cfg, ledger); ok {
		embedder = &embed.RouterEmbedder{Router: er, Pin: cfg.Models.Embedding}
	} else {
		_, _ = fmt.Fprintf(stderr, "serve: %s -- recall/synthesize widen to full-text/lexical matching only\n", note)
	}

	var composer compose.Completer
	var composerNote string
	if cr, ok, note := providers.BuildComposerRouter(cfg, ledger); ok {
		composer = cr
	} else {
		composerNote = note
	}

	deps := memory.Deps{
		Root:                    root,
		Config:                  cfg,
		Index:                   eng,
		Embedder:                embedder,
		Composer:                composer,
		ComposerModelVersion:    cfg.Models.Composer,
		ComposerUnavailableNote: composerNote,
		Queue:                   q,
		Sources:                 store.NewSourceStore(root),
		Fence:                   store.NewFenceWriter(root),
		Shard:                   store.NewShardStore(root),
	}
	handlers := memory.New(deps)
	return append(handlers.Tools(), handlers.ExtensionTools()...), closeDeps, eng, q, nil
}

// Inherited stdin/stdout may be blocking descriptors outside Go's runtime poller.
// Closing those files cannot interrupt an active syscall. A nonblocking duplicate
// wrapped by NewFile joins the poller, making Close interrupt concurrent pipe I/O.
// Duplicates share status flags, so cleanup restores the original flags after
// Serve has joined all I/O workers. Original pipes/sockets stay open; other
// files pass through and are owned by Serve.
func pollableMCPFile(file *os.File) (*os.File, func() error, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("inspect MCP stream: %w", err)
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return file, func() error { return nil }, nil
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, nil, fmt.Errorf("access MCP stream: %w", err)
	}
	var flags, fd int
	var operationErr error
	if err := raw.Control(func(original uintptr) {
		flags, operationErr = unix.FcntlInt(original, unix.F_GETFL, 0)
		if operationErr == nil {
			fd, operationErr = unix.FcntlInt(original, unix.F_DUPFD_CLOEXEC, 0)
		}
	}); err != nil {
		return nil, nil, fmt.Errorf("access MCP descriptor: %w", err)
	}
	if operationErr != nil {
		return nil, nil, fmt.Errorf("duplicate MCP stream: %w", operationErr)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, nil, fmt.Errorf("prepare MCP stream: %w", err)
	}
	prepared := os.NewFile(uintptr(fd), file.Name())
	cleanup := func() error {
		closeErr := prepared.Close()
		if errors.Is(closeErr, os.ErrClosed) {
			closeErr = nil
		}
		var restoreErr error
		controlErr := raw.Control(func(original uintptr) {
			_, restoreErr = unix.FcntlInt(original, unix.F_SETFL, flags)
		})
		return errors.Join(closeErr, controlErr, restoreErr)
	}
	return prepared, cleanup, nil
}
