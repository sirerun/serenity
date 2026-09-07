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
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
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
		t.Skip("git not available")
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
// the real Fence/Shard stores -- deliberately not a hand-rolled double,
// the same "test the real primitive" posture internal/supersede and
// internal/reconcile's own test fixtures already take.
func newTestHandlers(t *testing.T) (*Handlers, string) {
	t.Helper()
	root := gitRepoFixture(t)
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
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
	return New(deps), root
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return b
}

// TestRememberProvenanceRequired is this task's own acc line: "remember
// with empty provenance -> provenance_required with a populated
// suggestion." Both the wholly-absent-provenance and the
// present-but-empty-actor shapes must trip it -- Provenance.Actor is the
// field the doc comment names as the actual trigger, not mere presence of
// the object.
func TestRememberProvenanceRequired(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	cases := []struct {
		name string
		req  rememberRequest
	}{
		{"absent provenance", rememberRequest{Subject: "acme-corp", Predicate: "has_balance", Object: "$500"}},
		{"empty actor", rememberRequest{Subject: "acme-corp", Predicate: "has_balance", Object: "$500", Provenance: &rememberProvenance{Actor: ""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, isError, err := h.remember(ctx, mustMarshal(t, tc.req))
			if err != nil {
				t.Fatalf("remember: %v", err)
			}
			if !isError {
				t.Fatal("want isError=true for missing provenance")
			}
			rr, ok := resp.(rememberResponse)
			if !ok {
				t.Fatalf("response type = %T, want rememberResponse", resp)
			}
			if rr.Error == nil {
				t.Fatal("want a populated Envelope.Error")
			}
			if rr.Error.Code != "provenance_required" {
				t.Fatalf("Error.Code = %q, want %q", rr.Error.Code, "provenance_required")
			}
			if rr.Error.Suggestion == "" {
				t.Fatal("want a populated Suggestion, got empty string")
			}
		})
	}
}

// TestRememberWriteThenReconcileConflict spot-checks remember's core
// design (remember.go's own doc comment): a fresh claim writes straight
// through (Status "remembered"), while a second claim on the same
// (subject, predicate) with a differing object -- the textbook same-time
// contradiction RFC §10.2 names -- runs into internal/reconcile.Engine
// and stages a disposition item instead of writing (Status
// "staged_for_review"), never overwriting the first claim.
func TestRememberWriteThenReconcileConflict(t *testing.T) {
	h, root := newTestHandlers(t)
	ctx := context.Background()

	first := rememberRequest{
		Subject: "acme-corp", Predicate: "has_balance", Object: "$500",
		Provenance: &rememberProvenance{Actor: "human:tester"},
	}
	resp, isError, err := h.remember(ctx, mustMarshal(t, first))
	if err != nil {
		t.Fatalf("remember (first): %v", err)
	}
	if isError {
		t.Fatalf("remember (first) unexpectedly errored: %+v", resp)
	}
	rr := resp.(rememberResponse)
	if rr.Status != "remembered" {
		t.Fatalf("Status = %q, want %q", rr.Status, "remembered")
	}
	if len(rr.Evidence) != 1 || rr.Evidence[0].ClaimID == "" {
		t.Fatalf("want one evidence fact with a claim id, got %+v", rr.Evidence)
	}

	// The write actually landed on disk, through the real shard store --
	// not just reported success.
	lines, err := store.NewShardStore(root).Lines("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 1 || lines[0].Object != "$500" {
		t.Fatalf("shard lines = %+v, want one active $500 claim", lines)
	}

	second := rememberRequest{
		Subject: "acme-corp", Predicate: "has_balance", Object: "$700",
		Provenance: &rememberProvenance{Actor: "human:tester"},
	}
	resp, isError, err = h.remember(ctx, mustMarshal(t, second))
	if err != nil {
		t.Fatalf("remember (second): %v", err)
	}
	if isError {
		t.Fatalf("remember (second) unexpectedly errored: %+v", resp)
	}
	rr2 := resp.(rememberResponse)
	if rr2.Status != "staged_for_review" {
		t.Fatalf("Status = %q, want %q", rr2.Status, "staged_for_review")
	}
	if rr2.DispositionItemID == "" {
		t.Fatal("want a populated disposition_item_id")
	}
	if len(rr2.Evidence) != 2 {
		t.Fatalf("want both the new and the conflicting claim as evidence, got %d", len(rr2.Evidence))
	}

	// The conflicting claim was never written -- the shard still holds
	// exactly the first, $500 claim.
	lines, err = store.NewShardStore(root).Lines("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 1 || lines[0].Object != "$500" {
		t.Fatalf("shard lines after conflict = %+v, want the original $500 claim untouched", lines)
	}

	// And the disposition item is really there.
	item, err := h.deps.Disposition.Get(ctx, rr2.DispositionItemID)
	if err != nil {
		t.Fatalf("Disposition.Get: %v", err)
	}
	if item.Kind != disposition.KindReconcile {
		t.Fatalf("item.Kind = %q, want %q", item.Kind, disposition.KindReconcile)
	}
}

// TestEntityMissFoundFalse is this task's own acc line: "entity miss ->
// found:false not an error."
func TestEntityMissFoundFalse(t *testing.T) {
	h, _ := newTestHandlers(t)
	resp, isError, err := h.entity(context.Background(), mustMarshal(t, entityRequest{Slug: "never-existed"}))
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
		t.Fatal("Found = true, want false for a never-existing slug")
	}
	if er.Error != nil {
		t.Fatalf("want no Envelope.Error on a miss, got %+v", er.Error)
	}
}

// TestForgetIdempotent is this task's own acc line: "forget is
// idempotent." A never-existing claim id and a second forget call
// against an already-retracted claim must both report Forgotten:true and
// must not error -- a caller can retry blindly.
func TestForgetIdempotent(t *testing.T) {
	h, root := newTestHandlers(t)
	ctx := context.Background()

	// Never-existing claim id.
	resp, isError, err := h.forget(ctx, mustMarshal(t, forgetRequest{Subject: "charlie-fixture", ClaimID: "never-existed"}))
	if err != nil {
		t.Fatalf("forget (never existed): %v", err)
	}
	if isError {
		t.Fatalf("forget (never existed) unexpectedly errored: %+v", resp)
	}
	if fr, ok := resp.(forgetResponse); !ok || !fr.Forgotten {
		t.Fatalf("forget (never existed) = %+v, want Forgotten=true", resp)
	}

	// Remember a real shard-tier claim to forget.
	rememberResp, isError, err := h.remember(ctx, mustMarshal(t, rememberRequest{
		Subject: "charlie-fixture", Predicate: "has_balance", Object: "$100",
		Provenance: &rememberProvenance{Actor: "human:tester"},
	}))
	if err != nil || isError {
		t.Fatalf("remember: err=%v isError=%v resp=%+v", err, isError, rememberResp)
	}
	claimID := rememberResp.(rememberResponse).Evidence[0].ClaimID
	if claimID == "" {
		t.Fatal("remember produced no claim id to forget")
	}

	// First forget: a genuine retraction.
	resp, isError, err = h.forget(ctx, mustMarshal(t, forgetRequest{Subject: "charlie-fixture", ClaimID: claimID}))
	if err != nil {
		t.Fatalf("forget (first): %v", err)
	}
	if isError {
		t.Fatalf("forget (first) unexpectedly errored: %+v", resp)
	}
	if fr, ok := resp.(forgetResponse); !ok || !fr.Forgotten {
		t.Fatalf("forget (first) = %+v, want Forgotten=true", resp)
	}

	lines, err := store.NewShardStore(root).Lines("charlie-fixture", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) == 0 || lines[len(lines)-1].State != domain.StateRetracted {
		t.Fatalf("shard lines = %+v, want the head line retracted", lines)
	}

	// Second forget on the same, now-already-retracted claim id: still
	// Forgotten=true, still no error, and no second retraction line is
	// appended (idempotent by construction, not by luck).
	resp, isError, err = h.forget(ctx, mustMarshal(t, forgetRequest{Subject: "charlie-fixture", ClaimID: claimID}))
	if err != nil {
		t.Fatalf("forget (second): %v", err)
	}
	if isError {
		t.Fatalf("forget (second) unexpectedly errored: %+v", resp)
	}
	if fr, ok := resp.(forgetResponse); !ok || !fr.Forgotten {
		t.Fatalf("forget (second) = %+v, want Forgotten=true", resp)
	}
	linesAfter, err := store.NewShardStore(root).Lines("charlie-fixture", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(linesAfter) != len(lines) {
		t.Fatalf("second forget appended a line: before=%d after=%d", len(lines), len(linesAfter))
	}
}

// TestRecallBudgetPacking is this task's own acc line: "recall
// budget_used <= budget_tokens with dropped_count consistent." Two
// distinct fence pages give recall at least two ranked hits; a budget
// wide enough for both (observed, not hardcoded, so this stays robust to
// chunking/word-count details) establishes a baseline, then a budget one
// word short of that baseline must force at least one drop while the
// invariant itself still holds and dropped_count plus what's still
// included accounts for every hit the wide call found.
func TestRecallBudgetPacking(t *testing.T) {
	h, root := newTestHandlers(t)
	ctx := context.Background()

	for i, slug := range []string{"widget-alpha", "widget-beta"} {
		p := store.NewEntityPage(domain.Entity{Type: "project", Slug: slug})
		p.Summary = "Widget project number " + string(rune('0'+i)) + " is actively tracked, with a long summary describing its ongoing status and various details that add up to a meaningful word count for budget testing purposes here."
		if _, _, err := writer.Fence(h.deps.Queue, h.deps.Fence, p); err != nil {
			t.Fatalf("writer.Fence(%s): %v", slug, err)
		}
	}
	if err := index.Rebuild(ctx, root, h.deps.Config, h.deps.Index); err != nil {
		t.Fatalf("index.Rebuild: %v", err)
	}

	wideResp, isError, err := h.recall(ctx, mustMarshal(t, recallRequest{Query: "widget", BudgetTokens: 100000}))
	if err != nil {
		t.Fatalf("recall (wide): %v", err)
	}
	if isError {
		t.Fatalf("recall (wide) unexpectedly errored: %+v", wideResp)
	}
	wide := wideResp.(recallResponse)
	if wide.Budget.DroppedCount != 0 {
		t.Fatalf("wide budget dropped %d hits, want 0", wide.Budget.DroppedCount)
	}
	if len(wide.Evidence) < 2 {
		t.Skipf("only %d hit(s) found for \"widget\" -- not enough to test packing", len(wide.Evidence))
	}

	tightBudget := wide.Budget.BudgetUsed - 1
	tightResp, isError, err := h.recall(ctx, mustMarshal(t, recallRequest{Query: "widget", BudgetTokens: tightBudget}))
	if err != nil {
		t.Fatalf("recall (tight): %v", err)
	}
	if isError {
		t.Fatalf("recall (tight) unexpectedly errored: %+v", tightResp)
	}
	tight := tightResp.(recallResponse)
	if tight.Budget.BudgetUsed > tightBudget {
		t.Fatalf("budget_used %d exceeds budget_tokens %d", tight.Budget.BudgetUsed, tightBudget)
	}
	if tight.Budget.DroppedCount == 0 {
		t.Fatal("want at least one drop under a budget one word short of the wide baseline")
	}
	if tight.Budget.DroppedCount+len(tight.Evidence) != len(wide.Evidence) {
		t.Fatalf("dropped_count inconsistent: dropped=%d included=%d, want dropped+included = %d (the wide call's own hit count)",
			tight.Budget.DroppedCount, len(tight.Evidence), len(wide.Evidence))
	}
}
