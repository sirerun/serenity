package pool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func canonicalTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"brain/sources", "brain/entities", "brain/claims"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	git("config", "user.name", "canonical test")
	git("config", "user.email", "canonical@example.test")
	if err := os.WriteFile(filepath.Join(root, "brain", "seed.md"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "brain")
	git("commit", "--quiet", "-m", "seed")
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	return &Runtime{Root: root, brainID: "brain_1234567890abcd", queue: q}
}

func TestCanonicalCheckerRequiresEnteredFactInHEAD(t *testing.T) {
	runtime := canonicalTestRuntime(t)
	base := contracts.OperationRecord{ID: "operation-123", BrainID: runtime.brainID, Source: "gateway.remember"}
	verdict, err := runtime.Check(context.Background(), base)
	if err != nil || verdict.Outcome != contracts.CanonicalUnknown || verdict.Ref != "working_tree_absence_unverified" {
		t.Fatalf("no-entry verdict = %+v, %v; want conservative Unknown", verdict, err)
	}

	writerFact := writer.MemoryFact{Queue: runtime.queue, Sources: store.NewSourceStore(runtime.Root)}
	operation := writer.WithCanonicalOperation(context.Background(), writer.CanonicalOperation{
		ID:           "operation-123",
		BeforeCommit: func(context.Context, string) error { return nil },
		AfterFlush:   func(context.Context, string, string) {},
	})
	written, err := writerFact.RememberContext(operation, writer.RememberInput{
		OperationKey: "operation-123",
		Fact:         "The canonical marker is cobalt",
		Provenance:   "checker fixture",
		Kind:         store.MemoryFactKindFact,
		Visibility:   store.MemoryVisibilityWorld,
	}, time.Now())
	if err != nil {
		t.Fatalf("RememberContext: %v", err)
	}
	base.CanonicalEnteredAt = time.Now()
	verdict, err = runtime.Check(context.Background(), base)
	if err != nil || verdict.Outcome != contracts.CanonicalLanded || verdict.Ref != "fact:"+written.Record.SHA256 {
		t.Fatalf("landed verdict = %+v, %v; want fact %s", verdict, err, written.Record.SHA256)
	}
	base.CanonicalEnteredAt = time.Time{}
	verdict, err = runtime.Check(context.Background(), base)
	if err != nil || verdict.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("fact without ledger entry verdict = %+v, %v; want contradiction Unknown", verdict, err)
	}
}

func TestCanonicalCheckerDoesNotInferAbsenceAfterForget(t *testing.T) {
	runtime := canonicalTestRuntime(t)
	writerFact := writer.MemoryFact{Queue: runtime.queue, Sources: store.NewSourceStore(runtime.Root)}
	const operationID = "forget-check-operation"
	ctx := writer.WithCanonicalOperation(context.Background(), writer.CanonicalOperation{
		ID:           operationID,
		BeforeCommit: func(context.Context, string) error { return nil },
		AfterFlush:   func(context.Context, string, string) {},
	})
	written, err := writerFact.RememberContext(ctx, writer.RememberInput{
		OperationKey: operationID,
		Fact:         "Forget must leave the entered outcome unknown",
		Provenance:   "checker fixture",
		Kind:         store.MemoryFactKindFact,
		Visibility:   store.MemoryVisibilityWorld,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec := contracts.OperationRecord{ID: operationID, BrainID: runtime.brainID, Source: "gateway.remember", CanonicalEnteredAt: time.Now()}
	got, err := runtime.Check(context.Background(), rec)
	if err != nil || got.Outcome != contracts.CanonicalLanded {
		t.Fatalf("pre-forget check = %+v, %v", got, err)
	}
	if _, err := writerFact.ForgetContext(context.Background(), written.Record.SHA256, "requested", time.Now()); err != nil {
		t.Fatalf("ForgetContext: %v", err)
	}
	got, err = runtime.Check(context.Background(), rec)
	if err != nil || got.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("post-forget check = %+v, %v; want Unknown", got, err)
	}
}

func TestCanonicalCheckerTreatsMalformedMemoryEvidenceAsUnknown(t *testing.T) {
	runtime := canonicalTestRuntime(t)
	bad := []byte("{malformed memory fact")
	digest := sha256.Sum256(bad)
	sha := hex.EncodeToString(digest[:])
	dir := filepath.Join(runtime.Root, "brain", "sources", sha[:2], sha)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte("kind: memory_fact\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bytes"), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", runtime.Root, "add", "brain/sources")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add malformed fixture: %v: %s", err, out)
	}
	cmd = exec.Command("git", "-C", runtime.Root, "commit", "--quiet", "-m", "malformed source fixture")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit malformed fixture: %v: %s", err, out)
	}
	got, err := runtime.Check(context.Background(), contracts.OperationRecord{ID: "missing-operation", BrainID: runtime.brainID, Source: "gateway.remember"})
	if err != nil || got.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("malformed source verdict = %+v, %v; want Unknown", got, err)
	}
}

func TestCanonicalFenceValidatesBrainAndExcludesCommits(t *testing.T) {
	runtime := canonicalTestRuntime(t)
	if _, err := runtime.EnterCommit(context.Background(), "wrong-brain"); !errors.Is(err, ErrBrainFenceID) {
		t.Fatalf("wrong-brain enter error = %v", err)
	}
	release, err := runtime.Fence(context.Background(), runtime.brainID)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		leave, err := runtime.EnterCommit(context.Background(), runtime.brainID)
		if err == nil {
			close(entered)
			leave()
		}
		done <- err
	}()
	select {
	case <-entered:
		t.Fatal("shared commit entered while exclusive fence held")
	case <-time.After(30 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shared commit after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shared commit did not resume after fence release")
	}
}

func TestCanonicalGitConfigAllowsOnlyPinnedLocalOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	for _, tc := range []struct {
		name string
		body string
		ok   bool
	}{
		{
			name: "platform booleans",
			body: "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n\tlogallrefupdates = true\n\tignorecase = true\n\tprecomposeunicode = true\n[user]\n\tname = test\n\temail = test@example.test\n",
			ok:   true,
		},
		{
			name: "invalid platform boolean",
			body: "[core]\n\trepositoryformatversion = 0\n\tignorecase = yes\n",
		},
		{
			name: "fsmonitor command",
			body: "[core]\n\trepositoryformatversion = 0\n\tfsmonitor = /tmp/untrusted\n",
		},
		{
			name: "config include",
			body: "[core]\n\trepositoryformatversion = 0\n[include]\n\tpath = /tmp/untrusted\n",
		},
		{
			name: "remote rewrite",
			body: "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = https://invalid.example\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			err := validateCanonicalGitConfig(path)
			if (err == nil) != tc.ok {
				t.Fatalf("validateCanonicalGitConfig error = %v, want success %v", err, tc.ok)
			}
		})
	}
}

func TestCanonicalCheckerRejectsObjectAlternates(t *testing.T) {
	runtime := canonicalTestRuntime(t)
	info := filepath.Join(runtime.Root, ".git", "objects", "info")
	if err := os.MkdirAll(info, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info, "alternates"), []byte("/outside/objects\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := runtime.Check(context.Background(), contracts.OperationRecord{ID: "operation-123", BrainID: runtime.brainID, Source: "gateway.remember"})
	if err != nil || got.Outcome != contracts.CanonicalUnknown {
		t.Fatalf("alternate object repository verdict = %+v, %v; want Unknown", got, err)
	}
}
