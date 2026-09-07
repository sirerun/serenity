package cli

import (
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
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/server/memory"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "serve", Short: "Serve MCP over standard input and output", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) (runErr error) {
		stdio, err := cmd.Flags().GetBool("stdio")
		if err != nil {
			return err
		}
		if !stdio {
			return fmt.Errorf("choose --stdio to serve MCP")
		}
		tools, closeDeps, err := memoryTools(flagRoot, cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		if closeDeps != nil {
			defer func() { runErr = errors.Join(runErr, closeDeps()) }()
		}
		server, err := mcp.New(Version, tools)
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
		return server.Serve(ctx, closer, output)
	}}
	cmd.Flags().Bool("stdio", false, "read and write newline-delimited MCP JSON-RPC")
	return cmd
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
func memoryTools(root string, stderr io.Writer) ([]mcp.Tool, func() error, error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "serve: %s is not a brain repo -- serving MCP transport with no MEMORY_VERBS tools\n", root)
		return nil, nil, nil
	}

	eng, err := providers.OpenIndex(root)
	if err != nil {
		return nil, nil, fmt.Errorf("serve: open index: %w", err)
	}
	// The writer queue is this daemon's own owned resource (memory-compat-
	// mapping.md coordinator refinement #2: "stdio serve must close its
	// owned queue") -- closed alongside the index on shutdown, never left
	// running past Serve's own return.
	q := writer.NewQueue(nil)
	closeDeps := func() error {
		q.Close()
		return eng.Close()
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
	return memory.New(deps).Tools(), closeDeps, nil
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
