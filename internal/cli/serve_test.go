package cli

import (
	"bytes"
	"context"
	"io"
	"os"

	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestServeRequiresStdio(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"serve"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--stdio") {
		t.Fatalf("missing mode error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("error wrote stdout")
	}
}
func TestServeMemoryInput(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"serve", "--stdio"})
	cmd.SetIn(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"jsonrpc":"2.0","id":1,"result":{}}`+"\n" {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
}
func TestServeCancelledContext(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"serve", "--stdio"})
	r, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() })
	cmd.SetIn(r)
	cmd.SetOut(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMCPPipeFlagsRestored(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	raw, err := r.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var fd uintptr
	if err := raw.Control(func(n uintptr) { fd = n }); err != nil {
		t.Fatal(err)
	}
	before, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err != nil {
		t.Fatal(err)
	}
	prepared, cleanup, err := pollableMCPFile(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	after, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("descriptor flags changed: %d -> %d", before, after)
	}
}
