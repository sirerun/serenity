package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/sirerun/serenity/internal/server/mcp"
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
		server, err := mcp.New(Version, nil)
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

// Inherited stdin/stdout may be blocking descriptors outside Go's runtime poller.
// Closing those files cannot interrupt an active syscall. A nonblocking duplicate
// wrapped by NewFile joins the poller, making Close interrupt concurrent pipe I/O.
// Duplicates share status flags, so cleanup restores the original flags after
// Serve has joined all I/O workers. The caller's original file stays open.
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
	var original uintptr
	if err := raw.Control(func(fd uintptr) { original = fd }); err != nil {
		return nil, nil, fmt.Errorf("access MCP descriptor: %w", err)
	}
	flags, err := unix.FcntlInt(original, unix.F_GETFL, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("read MCP stream flags: %w", err)
	}
	fd, err := unix.FcntlInt(original, unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("duplicate MCP stream: %w", err)
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
		_, restoreErr := unix.FcntlInt(original, unix.F_SETFL, flags)
		return errors.Join(closeErr, restoreErr)
	}
	return prepared, cleanup, nil
}
