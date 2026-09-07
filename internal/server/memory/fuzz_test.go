package memory

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// fuzzFixture is gitRepoFixture/newTestHandlers's own logic
// (memory_test.go), duplicated here rather than shared: those two
// helpers take a *testing.T, and *testing.F has no common concrete-type
// relationship to *testing.T even though it exposes the same
// Helper/TempDir/Fatalf/Cleanup methods -- Go's parameter typing is
// nominal, not structural, so sharing them would mean changing those
// existing, already-tested helpers to take an interface instead.
// Duplicating ~20 lines here is the smaller, safer change, and it is
// called once (not per fuzz execution) -- the returned *Handlers is
// reused across every iteration, the same way a fuzz target over a
// stateful service normally amortizes its fixture cost.
func fuzzFixture(f *testing.F) *Handlers {
	f.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		f.Fatal("git is required for the memory parser fixture")
	}
	root := f.TempDir()
	run := func(args ...string) {
		f.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			f.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet")
	run("config", "user.email", "memory-fuzz@example.com")
	run("config", "user.name", "memory fuzz")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		f.Fatal(err)
	}
	run("add", "seed.txt")
	run("commit", "--quiet", "-m", "seed")

	eng, err := providers.OpenIndex(root)
	if err != nil {
		f.Fatalf("OpenIndex: %v", err)
	}
	f.Cleanup(func() { _ = eng.Close() })
	q := writer.NewQueue(nil)
	f.Cleanup(q.Close)
	deps := Deps{
		Root:    root,
		Config:  config.Default(),
		Index:   eng,
		Queue:   q,
		Sources: store.NewSourceStore(root),
		Fence:   store.NewFenceWriter(root),
		Shard:   store.NewShardStore(root),
		Clock:   fixedClock{testNow},
	}
	return New(deps)
}

// FuzzVerbs exercises the five request parsers with canonical v1 fields,
// including traversal-shaped names and IDs. Every domain failure must remain
// a structured VerbError; arbitrary bytes must never panic.
func FuzzVerbs(f *testing.F) {
	seeds := []struct {
		verb string
		args string
	}{
		{"recall", `{"query":"hello","budget_tokens":100}`},
		{"recall", `{}`},
		{"remember", `{"fact":"Acme has $1","entity":"acme-corp","provenance":"human:t"}`},
		{"remember", `{}`},
		{"remember", `{"fact":"traversal probe","entity":"../../etc/passwd","provenance":"human:t"}`},
		{"entity", `{"name":"acme-corp"}`},
		{"entity", `{}`},
		{"entity", `{"name":"../../etc/passwd"}`},
		{"synthesize", `{"question":"what"}`},
		{"synthesize", `{}`},
		{"forget", `{"id":"unknown-id"}`},
		{"forget", `{}`},
		{"forget", `{"id":"../../etc/passwd"}`},
		{"unknown-verb", `{}`},
		{"", ``},
	}
	for _, s := range seeds {
		f.Add(s.verb, []byte(s.args))
	}

	h := fuzzFixture(f)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, verb string, args []byte) {
		for _, tool := range h.Tools() {
			if tool.Name != verb {
				continue
			}
			result, err := tool.Handler(ctx, args)
			if err != nil {
				t.Fatalf("%s returned an unhandled public error: %v", verb, err)
			}
			if len(result.Content) != 1 {
				t.Fatalf("%s returned %d content blocks", verb, len(result.Content))
			}
			if result.IsError {
				var response VerbError
				if err := json.Unmarshal([]byte(result.Content[0].Text), &response); err != nil {
					t.Fatalf("%s error is not a flat VerbError: %v", verb, err)
				}
				asVerbError(t, response, true)
			}
			return
		}

	})
}
