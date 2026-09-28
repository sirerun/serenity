package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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
// Streamable HTTP transport at /mcp plus the DISPOSITION and DIRECTION
// handlers, all live on the one bearer-authenticated listener. Every
// caller of those routes is therefore an agent holding the credential:
// DISPOSITION records `agent:<credential id>` as the actor and refuses to
// accept a precept_draft or decompose item, which only `serenity inbox`
// may accept (ADR 022). It reuses internal/server's existing
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
	// credentialID names the one bearer credential this listener accepts;
	// DISPOSITION records `agent:<credentialID>` as the actor of every
	// dispose it serves (ADR 022), never an actor the caller claims.
	credentialID := serverdisposition.DefaultCredentialID
	if hasProfile {
		credentialID = "profile:" + profile
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

	serverConfig, err := loadServerConfig(flagRoot)
	if err != nil {
		return err
	}
	if err := refuseNonLoopbackBind(serverConfig, stderr); err != nil {
		return err
	}
	cfg := server.FromBrainConfig(serverConfig)
	cfg.TokenSource = tokenSource
	srv := server.New(cfg)
	httpHandler := mcp.NewHTTPHandlerWithConfig(mcpServer, mcp.HTTPConfig{MaxInFlightCalls: serverConfig.MaxInFlightCalls})
	srv.Handle("/mcp", httpHandler)
	if eng != nil && q != nil {
		dispositionStore := coredisposition.NewStore(eng)
		directionOpts, err := checkPlanOptions(flagRoot, eng, stderr)
		if err != nil {
			return err
		}
		serverdirection.New(direction.NewStore(flagRoot, q), dispositionStore, flagRoot, directionOpts...).Register(srv)
		serverdisposition.New(dispositionStore, events.NewStore(eng), serverdisposition.WithCredentialID(credentialID)).Register(srv)
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

// checkPlanOptions wires DIRECTION check_plan's free-text classification
// router from root's serenity.yml. Classification is a local-cheap task
// class (RFC 0001 section 9) and serenity.yml has no separate
// classification pin, so it runs on the brain's pinned local-cheap chat
// model, models.extraction, and asserts that pin on every call. With no
// usable pin the router stays nil and free-text plans report unverified,
// noted once on stderr; structured actions are matched either way.
func checkPlanOptions(root string, eng *index.SQLite, stderr io.Writer) ([]serverdirection.Option, error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return nil, fmt.Errorf("serve --http: load config for check_plan: %w", err)
	}
	rtr, ok, note := providers.BuildExtractionRouter(cfg, &providers.IndexSpendLedger{Eng: eng})
	if !ok {
		_, _ = fmt.Fprintf(stderr, "serve: check_plan has no classification model (%s) -- free-text plans report unverified\n", note)
		return nil, nil
	}
	return []serverdirection.Option{serverdirection.WithRouter(rtr), serverdirection.WithModelVersion(cfg.Models.Extraction)}, nil
}

// loadServerConfig loads serenity.yml's server: section for --http's
// listener config. Mirrors memoryTools' own "not a brain repo" tolerance
// (T4.1's bare-transport-smoke-test posture): outside a brain repo (no
// serenity.yml at all), or with no server: section, --http still starts,
// on the transport's own secure loopback-port-zero default. A serenity.yml
// that exists but fails config.Load's strict decode (T24.8: an unknown
// key, a malformed document) is a hard error, not a silent fallback to
// defaults: a synced config the operator cannot see the effect of is the
// exact trust gap ADR 018 closes.
func loadServerConfig(root string) (config.Server, error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if errors.Is(err, os.ErrNotExist) {
		return config.Server{}, nil
	}
	if err != nil {
		return config.Server{}, fmt.Errorf("serve --http: %w", err)
	}
	return cfg.Server, nil
}

// refuseNonLoopbackBind is `serve --http`'s own check that server.bind
// resolves to a loopback address unless server.allow_lan is set (RFC 0001
// section 14; ADR 018 decision 3). internal/server.Listen enforces the
// same rule as a last line of defense; checking here as well means the
// refusal is logged to stderr naming the bind and the missing key before
// any listener, TLS config or route is built, and the returned error
// wraps server.ErrNonLoopbackBindRefused so callers can match it.
func refuseNonLoopbackBind(sc config.Server, stderr io.Writer) error {
	if sc.AllowLAN {
		return nil
	}
	bind := sc.Bind
	if bind == "" {
		bind = server.DefaultBind
	}
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return fmt.Errorf("serve --http: parse server.bind %q: %w", bind, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	_, _ = fmt.Fprintf(stderr, "serve --http: refusing to bind %q: server.bind is not a loopback address and server.allow_lan is not set in %s (RFC 0001 section 14)\n", bind, config.FileName)
	return fmt.Errorf("serve --http: %w: %q (set server.allow_lan: true in %s to expose the daemon beyond loopback)", server.ErrNonLoopbackBindRefused, bind, config.FileName)
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
	if errors.Is(err, os.ErrNotExist) {
		_, _ = fmt.Fprintf(stderr, "serve: %s is not a brain repo -- serving MCP transport with no MEMORY_VERBS tools\n", root)
		return nil, nil, nil, nil, nil
	}
	if err != nil {
		// A serenity.yml that exists but fails the strict decode (T24.8)
		// is a real configuration error; degrading to "no tools" would
		// hide the named unknown key the operator needs to see.
		return nil, nil, nil, nil, fmt.Errorf("serve: %w", err)
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
