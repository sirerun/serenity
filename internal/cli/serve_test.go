package cli

import (
	"bytes"
	"context"
	"encoding/json"
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

// TestServeWiresMemoryTools proves T4.5's own differentiator over
// DISPOSITION (T4.4) and DIRECTION (T4.6), both of which stayed
// package-complete but not live-wired: pointed at a real brain repo,
// `serve --stdio`'s tools/list must report all five MEMORY_VERBS v1
// verbs, not the mcp.New(Version, nil) empty set serve shipped with
// before this task.
func TestServeWiresMemoryTools(t *testing.T) {
	requireGit(t)
	root := pushFixture(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "serve", "--stdio"})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"list","method":"tools/list"}`,
	}, "\n") + "\n"
	cmd.SetIn(strings.NewReader(input))
	var out bytes.Buffer
	cmd.SetOut(&out)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("serve: %v; stderr: %s", err, stderr.String())
	}
	// pushFixture's serenity.yml pins no models (config.Default()'s
	// "none@v0"), so the embedder-unavailable degraded-mode note is
	// expected here -- what this test rules out is the OTHER note,
	// memoryTools' own "not a brain repo" fallback, which must not fire
	// against a real, freshly-scaffolded brain.
	if strings.Contains(stderr.String(), "is not a brain repo") {
		t.Fatalf("unexpected not-a-brain-repo fallback against a real brain repo, stderr: %s", stderr.String())
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 response frames (init, tools/list), got %d: %q", len(lines), out.String())
	}

	var listReply struct {
		ID     string `json:"id"`
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listReply); err != nil {
		t.Fatalf("tools/list reply is not valid JSON: %q: %v", lines[1], err)
	}
	if listReply.ID != "list" {
		t.Fatalf("tools/list reply id = %q, want %q: %s", listReply.ID, "list", lines[1])
	}
	got := make(map[string]bool, len(listReply.Result.Tools))
	for _, tool := range listReply.Result.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"recall", "remember", "entity", "synthesize", "forget"} {
		if !got[want] {
			t.Errorf("tools/list missing %q; got %v", want, got)
		}
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
