package memory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
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
		f.Skip("git not available")
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
	deps := Deps{
		Root:        root,
		Config:      config.Default(),
		Index:       eng,
		Disposition: disposition.NewStore(eng),
		Queue:       writer.NewQueue(nil),
		Fence:       store.NewFenceWriter(root),
		Shard:       store.NewShardStore(root),
		Clock:       fixedClock{testNow},
	}
	return New(deps)
}

// FuzzVerbs is T4.11's "go-fuzz on parsers 30s in CI" acc-line clause,
// applied to MEMORY_VERBS's own five request-decoding entry points.
// Each verb's json.Unmarshal(args, &req) call is itself a parser over
// attacker-controlled MCP tool-call arguments (RFC §14 adversary 2: a
// malicious or compromised MCP client), and this is also where this
// task's own validSlug hardening lives (memory.go) -- so this target
// directly fuzzes that new code path alongside the pre-existing
// decode/validate logic in entity/forget/remember/recall/synthesize.
// verb selects which of the five handlers runs; args is fuzzed as the
// raw JSON request body for whichever verb is selected. Every handler
// must return cleanly for arbitrary bytes -- a Go-level error is this
// package's own documented signal for a bug in this package (verbFunc's
// own doc comment: every domain/validation outcome is reported through
// the response's embedded Envelope.Error, never a Go error), so a
// non-nil err from any of these five calls indicates something worth
// investigating by hand, but the crash this acc line's "run without
// crash" actually names is a panic -- which go test's fuzzing runtime
// detects and reports natively, the same as FuzzParse.
func FuzzVerbs(f *testing.F) {
	seeds := []struct {
		verb string
		args string
	}{
		{"recall", `{"query":"hello","budget_tokens":100}`},
		{"recall", `{}`},
		{"remember", `{"subject":"acme-corp","predicate":"has_balance","object":"$1","provenance":{"actor":"human:t"}}`},
		{"remember", `{}`},
		{"remember", `{"subject":"../../etc/passwd","predicate":"has_balance","object":"$1","provenance":{"actor":"human:t"}}`},
		{"entity", `{"slug":"acme-corp"}`},
		{"entity", `{}`},
		{"entity", `{"slug":"../../etc/passwd"}`},
		{"synthesize", `{"query":"what"}`},
		{"synthesize", `{}`},
		{"forget", `{"subject":"acme-corp","claim_id":"x"}`},
		{"forget", `{}`},
		{"forget", `{"subject":"..","claim_id":"x"}`},
		{"unknown-verb", `{}`},
		{"", ``},
	}
	for _, s := range seeds {
		f.Add(s.verb, []byte(s.args))
	}

	h := fuzzFixture(f)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, verb string, args []byte) {
		switch verb {
		case "recall":
			_, _, _ = h.recall(ctx, args)
		case "remember":
			_, _, _ = h.remember(ctx, args)
		case "entity":
			_, _, _ = h.entity(ctx, args)
		case "synthesize":
			_, _, _ = h.synthesize(ctx, args)
		case "forget":
			_, _, _ = h.forget(ctx, args)
		}
	})
}
