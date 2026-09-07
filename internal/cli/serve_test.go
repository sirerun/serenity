package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
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
