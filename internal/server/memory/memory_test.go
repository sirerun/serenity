package memory

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// fixedClock is the same single-method Clock double every other package
// in this repo uses for deterministic ObservedAt/CreatedAt timestamps.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var testNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// gitRepoFixture mirrors internal/supersede/supersede_test.go's own
// fixture: writer.Shard/writer.Fence's dirty-tree guard shells out to
// `git status`/`git diff --cached`, so every write path this package
// exercises needs a real, committed git repo underneath, not just a bare
// temp directory.
func gitRepoFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git required for real persistence fixture")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet")
	run("config", "user.email", "memory-test@example.com")
	run("config", "user.name", "memory test")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "seed.txt")
	run("commit", "--quiet", "-m", "seed")
	return root
}

// newTestHandlers builds a real Handlers over a fresh brain repo: a real
// *index.SQLite (providers.OpenIndex only needs root/.serenity, which it
// creates itself -- no serenity.yml required), the real writer queue, and
// the real Fence/Shard/Sources stores -- deliberately not a hand-rolled
// double, the same "test the real primitive" posture internal/supersede
// and internal/reconcile's own test fixtures already take.
func newTestHandlers(t *testing.T) (*Handlers, string) {
	t.Helper()
	root := gitRepoFixture(t)
	deps, closeFn := testDeps(t, root)
	t.Cleanup(closeFn)
	return New(deps), root
}

// testDeps builds Deps over an already-prepared git repo root, letting a
// caller (e.g. a test needing to reopen the same root after a restart)
// control the fixture's lifetime independently of Handlers construction.
func testDeps(t *testing.T, root string) (Deps, func()) {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	q := writer.NewQueue(nil)
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
	return deps, func() {
		q.Close()
		_ = eng.Close()
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return b
}

// asVerbError requires resp to be a VerbError and returns it, so every
// test asserting a specific failure shape does so uniformly against the
// new flat error type (T4.20 replaced the old nested Envelope.Error).
func asVerbError(t *testing.T, resp any, isError bool) VerbError {
	t.Helper()
	if !isError {
		t.Fatalf("want isError=true, got resp=%+v", resp)
	}
	ve, ok := resp.(VerbError)
	if !ok {
		t.Fatalf("response type = %T, want VerbError", resp)
	}
	if ve.Suggestion == "" {
		t.Fatal("want a populated Suggestion, got empty string")
	}
	return ve
}

// TestRememberProvenanceRequired is this task's own acc line, re-targeted
// at the pinned contract's own field: remember's REQUIRED field is
// provenance (free text), not the old guessed subject/predicate/object
// claim shape -- T4.20 replaced that guess wholesale (memory-compat-
// mapping.md: the old Envelope read RFC 0001 §8.1's field list, not the
// actual frozen gbrain contract). Both an absent and a blank-but-present
// provenance must trip provenance_required with a populated suggestion.
func TestRememberProvenanceRequired(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	cases := []struct {
		name string
		req  rememberRequest
	}{
		{"absent provenance", rememberRequest{Fact: "picked Stripe over Adyen"}},
		{"blank provenance", rememberRequest{Fact: "picked Stripe over Adyen", Provenance: "   "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, isError, err := h.remember(ctx, mustMarshal(t, tc.req))
			if err != nil {
				t.Fatalf("remember: %v", err)
			}
			ve := asVerbError(t, resp, isError)
			if ve.Error != ErrCodeProvenanceRequired {
				t.Fatalf("Error = %q, want %q", ve.Error, ErrCodeProvenanceRequired)
			}
		})
	}
}

// TestRememberThenRecallRoundTrip spot-checks remember's core design: a
// fresh fact writes straight through (Status "inserted"), lands on disk as
// a real memory_fact source, and is visible via recall scoped to its own
// entity -- the immediate world remember->recall round-trip the mapping
// doc's own architecture note requires.
func TestRememberThenRecallRoundTrip(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	resp, isError, err := h.remember(ctx, mustMarshal(t, rememberRequest{
		Fact:       "Acme Corp has a $500 balance",
		Provenance: "conformance run",
		Entity:     "acme-corp",
	}))
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if isError {
		t.Fatalf("remember unexpectedly errored: %+v", resp)
	}
	rr := resp.(rememberResponse)
	if rr.Status != "inserted" {
		t.Fatalf("Status = %q, want %q", rr.Status, "inserted")
	}
	if rr.ID == "" {
		t.Fatal("want a populated opaque id")
	}
	if rr.EntitySlug == nil || *rr.EntitySlug != "acme-corp" {
		t.Fatalf("EntitySlug = %v, want \"acme-corp\"", rr.EntitySlug)
	}

	recallResp, isError, err := h.recall(ctx, mustMarshal(t, recallRequest{Entity: "acme-corp"}))
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if isError {
		t.Fatalf("recall unexpectedly errored: %+v", recallResp)
	}
	rec := recallResp.(recallResponse)
	if rec.Total != 1 || len(rec.Facts) != 1 {
		t.Fatalf("recall facts = %+v, want exactly 1", rec.Facts)
	}
	if rec.Facts[0].FactID != rr.ID {
		t.Fatalf("recall fact_id = %q, want the remembered id %q", rec.Facts[0].FactID, rr.ID)
	}
	if rec.Facts[0].Fact != "Acme Corp has a $500 balance" {
		t.Fatalf("recall fact text = %q", rec.Facts[0].Fact)
	}
}

// TestEntityMissFoundFalse is this task's own acc line: "entity miss ->
// found:false not an error."
func TestEntityMissFoundFalse(t *testing.T) {
	h, _ := newTestHandlers(t)
	resp, isError, err := h.entity(context.Background(), mustMarshal(t, entityRequest{Name: "never-existed"}))
	if err != nil {
		t.Fatalf("entity: %v", err)
	}
	if isError {
		t.Fatalf("entity miss unexpectedly reported isError=true: %+v", resp)
	}
	er, ok := resp.(entityResponse)
	if !ok {
		t.Fatalf("response type = %T, want entityResponse", resp)
	}
	if er.Found {
		t.Fatal("Found = true, want false for a never-existing name")
	}
	if er.Suggestions == nil {
		t.Fatal("want a non-nil (possibly empty) suggestions array on a miss")
	}
}

// TestForgetIdempotent is this task's own acc line: "forget is
// idempotent." A never-existing id and a second forget call against an
// already-expired fact must both report Expired:false/true correctly per
// the pinned semantics (fresh=true, repeat=false, unknown=not_found) and
// must never error on the repeat path -- a caller can retry blindly.
func TestForgetIdempotent(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	// Unknown id -> not_found.
	resp, isError, err := h.forget(ctx, mustMarshal(t, forgetRequest{ID: "never-existed"}))
	if err != nil {
		t.Fatalf("forget (never existed): %v", err)
	}
	ve := asVerbError(t, resp, isError)
	if ve.Error != ErrCodeNotFound {
		t.Fatalf("Error = %q, want %q", ve.Error, ErrCodeNotFound)
	}

	// Remember a real fact to forget.
	rememberResp, isError, err := h.remember(ctx, mustMarshal(t, rememberRequest{
		Fact: "Charlie has a $100 balance", Provenance: "conformance run",
	}))
	if err != nil || isError {
		t.Fatalf("remember: err=%v isError=%v resp=%+v", err, isError, rememberResp)
	}
	id := rememberResp.(rememberResponse).ID
	if id == "" {
		t.Fatal("remember produced no id to forget")
	}

	// First forget: a genuine expiry.
	resp, isError, err = h.forget(ctx, mustMarshal(t, forgetRequest{ID: id}))
	if err != nil {
		t.Fatalf("forget (first): %v", err)
	}
	if isError {
		t.Fatalf("forget (first) unexpectedly errored: %+v", resp)
	}
	fr, ok := resp.(forgetResponse)
	if !ok || !fr.Expired {
		t.Fatalf("forget (first) = %+v, want Expired=true", resp)
	}

	// Second forget on the same, now-already-expired id: Expired=false,
	// still no error.
	resp, isError, err = h.forget(ctx, mustMarshal(t, forgetRequest{ID: id}))
	if err != nil {
		t.Fatalf("forget (second): %v", err)
	}
	if isError {
		t.Fatalf("forget (second) unexpectedly errored: %+v", resp)
	}
	fr, ok = resp.(forgetResponse)
	if !ok || fr.Expired {
		t.Fatalf("forget (second) = %+v, want Expired=false (already expired)", resp)
	}

	// The fact no longer round-trips through recall.
	recallResp, isError, err := h.recall(ctx, mustMarshal(t, recallRequest{}))
	if err != nil || isError {
		t.Fatalf("recall: err=%v isError=%v", err, isError)
	}
	for _, f := range recallResp.(recallResponse).Facts {
		if f.FactID == id {
			t.Fatalf("forgotten fact %s still visible via recall", id)
		}
	}
}
