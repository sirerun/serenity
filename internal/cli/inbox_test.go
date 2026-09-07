package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/supersede"
	"github.com/sirerun/serenity/internal/writer"
)

var inboxFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// openInboxTestStore opens a real disposition.Store over a real brain
// repo's derived index (initBrainRepo/providers.OpenIndex -- the same
// path runInbox itself takes), not a synthetic double. It also returns
// root -- callers that need to build a *supersede.Writer (T2.7's 'e' key)
// or seed real fence/shard fixtures use it; callers that don't may
// discard it with `_`.
func openInboxTestStore(t *testing.T) (*disposition.Store, string) {
	t.Helper()
	root := initBrainRepo(t)
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng), root
}

// newTestSupersedeWriter builds a *supersede.Writer over root the same
// way runInbox itself does (T2.7): a fresh writer.Queue, a FenceWriter
// and ShardStore rooted at root, and root's own loaded serenity.yml for
// tier assignment.
func newTestSupersedeWriter(t *testing.T, root string) *supersede.Writer {
	t.Helper()
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	return supersede.New(q, store.NewFenceWriter(root), store.NewShardStore(root), cfg)
}

// newTestDirectionStore returns a *direction.Store rooted at root with
// its own writer queue -- production wiring (runInbox's default case)
// shares one queue between sw and this store so a session's edit_accept
// and decompose-accept writes commit together, but none of these
// existing tests exercise KindDecompose, so a separate queue here is
// harmless and keeps this helper independent of newTestSupersedeWriter's
// own private queue.
func newTestDirectionStore(t *testing.T, root string) *direction.Store {
	t.Helper()
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	return direction.NewStore(root, q)
}

// seedReconcileItem stages one KindReconcile item shaped exactly like
// internal/reconcile.Engine.Process's own output (T2.2): both claims
// share (subject, predicate), and Family carries the value inbox.go's
// itemFamily/bulk-defer filtering reads.
func seedReconcileItem(t *testing.T, dispStore *disposition.Store, ctx context.Context, now time.Time, subject, predicate, aObject, bObject, groupID string) disposition.Item {
	t.Helper()
	a := domain.Claim{
		ID: "a-" + subject + "-" + predicate + "-" + aObject, SubjectSlug: subject,
		Predicate: predicate, Object: aObject, Family: predicate, State: domain.StateActive,
	}
	b := domain.Claim{
		ID: "b-" + subject + "-" + predicate + "-" + bObject, SubjectSlug: subject,
		Predicate: predicate, Object: bObject, Family: predicate, State: domain.StateActive,
	}
	payload, err := json.Marshal(reconcile.ReconcilePayload{Verdict: reconcile.VerdictConflict, Reason: "test fixture", A: a, B: b})
	if err != nil {
		t.Fatalf("marshal ReconcilePayload: %v", err)
	}
	item, err := dispStore.Create(ctx, disposition.KindReconcile, payload, groupID, now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return item
}

// TestInboxInteractiveDrivesJKSpaceRecordsOneDispositionPerGroupMember is
// T2.5's acc line, word for word: "a scripted-TTY test drives J/K/space
// and records one disposition per group member." Four reviewable rows are
// seeded: an ungrouped item, a two-member group ("g1"), and a second
// ungrouped item -- the script moves the cursor down twice then back up
// once (landing back on the group row) before disposing with space, so
// j, k, and space are all genuinely exercised, not just j+space.
func TestInboxInteractiveDrivesJKSpaceRecordsOneDispositionPerGroupMember(t *testing.T) {
	dispStore, root := openInboxTestStore(t)
	sw := newTestSupersedeWriter(t, root)
	dirStore := newTestDirectionStore(t, root)
	ctx := context.Background()

	untouchedBefore := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "alice-tan", "works_at", "acme-corp", "initech", "")
	groupA := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "acme-corp", "has_balance", "$700", "$500", "g1")
	groupB := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "acme-corp", "has_balance", "$900", "$500", "g1")
	untouchedAfter := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "bob-lee", "works_at", "globex", "initrode", "")

	var out bytes.Buffer
	in := strings.NewReader("jjk ") // down to group row, down to last row, back up to group row, dispose
	if err := runInteractive(ctx, dispStore, sw, dirStore, in, &out, "human:test", inboxFixedNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}

	for _, id := range []string{groupA.ID, groupB.ID} {
		got, err := dispStore.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if got.State != disposition.StateDisposed {
			t.Errorf("group member %s State = %q, want disposed", id, got.State)
		}
		if got.Verdict != disposition.VerdictAccept {
			t.Errorf("group member %s Verdict = %q, want accept", id, got.Verdict)
		}
		hist, err := dispStore.HistoryFor(ctx, id)
		if err != nil {
			t.Fatalf("HistoryFor %s: %v", id, err)
		}
		if len(hist) != 1 {
			t.Errorf("group member %s has %d history row(s), want exactly 1", id, len(hist))
		}
	}

	for _, id := range []string{untouchedBefore.ID, untouchedAfter.ID} {
		got, err := dispStore.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if got.State != disposition.StatePending {
			t.Errorf("ungrouped item %s State = %q, want still pending (never under the cursor when space was pressed)", id, got.State)
		}
	}

	if n := strings.Count(out.String(), "disposed "); n != 2 {
		t.Fatalf("expected exactly 2 \"disposed\" lines (one per group member), got %d:\n%s", n, out.String())
	}
}

// TestInboxInteractiveGroupOfOneRecordsExactlyOneDisposition covers the
// degenerate "group" case (GroupID == "", a singleton) -- "one disposition
// per group member" trivially holding for a group of one.
func TestInboxInteractiveGroupOfOneRecordsExactlyOneDisposition(t *testing.T) {
	dispStore, root := openInboxTestStore(t)
	sw := newTestSupersedeWriter(t, root)
	dirStore := newTestDirectionStore(t, root)
	ctx := context.Background()

	item := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "carol-diaz", "works_at", "umbrella", "oscorp", "")

	var out bytes.Buffer
	if err := runInteractive(ctx, dispStore, sw, dirStore, strings.NewReader(" "), &out, "human:test", inboxFixedNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}
	got, err := dispStore.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != disposition.StateDisposed || got.Verdict != disposition.VerdictAccept {
		t.Fatalf("item State=%q Verdict=%q, want disposed/accept", got.State, got.Verdict)
	}
	if n := strings.Count(out.String(), "disposed "); n != 1 {
		t.Fatalf("expected exactly 1 \"disposed\" line, got %d:\n%s", n, out.String())
	}
}

// TestBulkDeferDefersExactlyMatchingItems is T2.5's acc line, word for
// word: "--bulk-defer family=has_balance defers exactly the matching
// items." Grouping plays no role in bulk-defer (unlike the interactive
// loop): two ungrouped has_balance items are seeded alongside an
// unrelated works_at item, and only the two matching items move.
func TestBulkDeferDefersExactlyMatchingItems(t *testing.T) {
	dispStore, _ := openInboxTestStore(t)
	ctx := context.Background()

	match1 := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "acme-corp", "has_balance", "$700", "$500", "")
	match2 := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "globex", "has_balance", "$300", "$200", "")
	other := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "alice-tan", "works_at", "acme-corp", "initech", "")

	var out bytes.Buffer
	if err := runBulkDefer(ctx, dispStore, "family=has_balance", "human:test", &out, inboxFixedNow); err != nil {
		t.Fatalf("runBulkDefer: %v", err)
	}

	for _, id := range []string{match1.ID, match2.ID} {
		got, err := dispStore.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if got.State != disposition.StateDeferred || got.Verdict != disposition.VerdictDefer {
			t.Errorf("matching item %s State=%q Verdict=%q, want deferred/defer", id, got.State, got.Verdict)
		}
	}

	gotOther, err := dispStore.Get(ctx, other.ID)
	if err != nil {
		t.Fatalf("Get other: %v", err)
	}
	if gotOther.State != disposition.StatePending {
		t.Fatalf("non-matching item %s was touched: State = %q, want still pending", other.ID, gotOther.State)
	}

	if !strings.Contains(out.String(), "deferred 2 item(s) matching family=has_balance") {
		t.Fatalf("expected a summary counting exactly 2 deferred items, got: %q", out.String())
	}
}

func TestBulkDeferRejectsUnsupportedFilterKey(t *testing.T) {
	dispStore, _ := openInboxTestStore(t)
	ctx := context.Background()

	var out bytes.Buffer
	err := runBulkDefer(ctx, dispStore, "kind=reconcile", "human:test", &out, inboxFixedNow)
	if !errors.Is(err, errUnsupportedBulkDeferFilter) {
		t.Fatalf("err = %v, want errUnsupportedBulkDeferFilter", err)
	}
}

// TestListParkedListsParkedItemsOnly is T2.5's acc line, word for word:
// "--parked lists parked items only." One item is driven to StateParked
// via three real Sweep passes (T2.6's own expiry mechanics -- MaxDeferCycles
// pending->deferred->deferred->parked), exactly how a real item would get
// there; a second, freshly-created item stays pending and must not appear.
func TestListParkedListsParkedItemsOnly(t *testing.T) {
	dispStore, _ := openInboxTestStore(t)
	ctx := context.Background()

	toPark := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "dave-kim", "has_balance", "$100", "$50", "")

	thresholds := disposition.Thresholds{disposition.KindReconcile: time.Minute}
	now := inboxFixedNow
	for i := 0; i < disposition.MaxDeferCycles; i++ {
		now = now.Add(2 * time.Minute)
		if _, err := disposition.Sweep(ctx, dispStore, thresholds, now); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
	}
	gotParked, err := dispStore.Get(ctx, toPark.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotParked.State != disposition.StateParked {
		t.Fatalf("fixture setup failed: item State = %q, want parked", gotParked.State)
	}

	// Freshly created at the sweep's own final "now" -- not yet threshold-old,
	// so it stays pending and must never show up in a parked-only listing.
	stillPending := seedReconcileItem(t, dispStore, ctx, now, "carol-diaz", "works_at", "umbrella", "oscorp", "")

	var out bytes.Buffer
	if err := runListParked(ctx, dispStore, &out); err != nil {
		t.Fatalf("runListParked: %v", err)
	}
	if !strings.Contains(out.String(), "has_balance") {
		t.Fatalf("expected the parked item's family in the listing, got: %q", out.String())
	}
	if strings.Contains(out.String(), "works_at") {
		t.Fatalf("parked-only listing leaked the still-pending item %s, got: %q", stillPending.ID, out.String())
	}
}

func TestListParkedReportsNoneWhenEmpty(t *testing.T) {
	dispStore, _ := openInboxTestStore(t)
	ctx := context.Background()

	var out bytes.Buffer
	if err := runListParked(ctx, dispStore, &out); err != nil {
		t.Fatalf("runListParked: %v", err)
	}
	if !strings.Contains(out.String(), "no parked items") {
		t.Fatalf("expected an explicit empty-state message, got: %q", out.String())
	}
}

// TestInboxInteractiveEKeyEditAcceptWritesThroughToBrainRepo is T2.7's
// CLI-level acc-line check: the new 'e' key drives edit_accept through
// runInteractive's own scripted-TTY loop exactly like T2.5's j/k/space
// test does for accept/defer/reject, and the resulting write-through
// lands on disk via internal/supersede -- not just recorded in the
// disposition queue.
func TestInboxInteractiveEKeyEditAcceptWritesThroughToBrainRepo(t *testing.T) {
	dispStore, root := openInboxTestStore(t)
	sw := newTestSupersedeWriter(t, root)
	dirStore := newTestDirectionStore(t, root)
	ctx := context.Background()

	// Seed B on disk exactly as it must already exist for a fence-tier
	// apply (internal/supersede.Writer's own applyFence precondition) --
	// same subject/predicate/object seedReconcileItem below uses, so its
	// derived b.ID lands on the very row Apply looks for.
	b := domain.Claim{
		ID: "b-alice-tan-works_at-initech", SubjectSlug: "alice-tan",
		Predicate: "works_at", Object: "initech", Family: "works_at", State: domain.StateActive,
	}
	fw := store.NewFenceWriter(root)
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "alice-tan"})
	page.Claims = []domain.Claim{b}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}

	item := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "alice-tan", "works_at", "acme-corp", "initech", "")

	var out bytes.Buffer
	// 'e', then the replacement object, then Enter -- runInteractive's
	// own 'e' case reads exactly this shape (see its doc comment).
	in := strings.NewReader("eglobex-corp\n")
	if err := runInteractive(ctx, dispStore, sw, dirStore, in, &out, "human:test", inboxFixedNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}

	if !strings.Contains(out.String(), "verdict=edit_accept") {
		t.Fatalf("expected a disposed/edit_accept line, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "-> claim written to brain repo") {
		t.Fatalf("expected an applied/write-through line, got: %q", out.String())
	}

	got, err := dispStore.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != disposition.StateDisposed || got.Verdict != disposition.VerdictEditAccept {
		t.Fatalf("item State=%q Verdict=%q, want disposed/edit_accept", got.State, got.Verdict)
	}
	if got.AppliedClaimID == "" {
		t.Fatal("AppliedClaimID empty after a successful 'e' apply -- the disposition row must reference the new claim id")
	}

	p, err := fw.ParseEntity(fw.PathFor("topic", "alice-tan"))
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	var found bool
	for _, c := range p.Claims {
		if c.ID == got.AppliedClaimID && c.State == domain.StateActive && c.Object == "globex-corp" {
			found = true
		}
	}
	if !found {
		t.Fatalf("edited object %q not found as the active row referenced by AppliedClaimID on the fence page: %+v", "globex-corp", p.Claims)
	}
}

// seedDecomposeItem stages one KindDecompose item shaped exactly like
// direction.Decompose's own output (T3.11): payload carries the parent
// id plus one proposed child.
func seedDecomposeItem(t *testing.T, dispStore *disposition.Store, ctx context.Context, now time.Time, parentID, title, rationale, groupID string) disposition.Item {
	t.Helper()
	payload, err := json.Marshal(direction.DecomposePayload{
		ParentID: parentID,
		Child:    direction.ChildIntentDraft{Title: title, Rationale: rationale},
	})
	if err != nil {
		t.Fatalf("marshal DecomposePayload: %v", err)
	}
	item, err := dispStore.Create(ctx, disposition.KindDecompose, payload, groupID, now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return item
}

// TestInboxInteractiveSpaceAcceptWritesDecomposedChildIntoLedger is
// T3.11's own acc-line clause, exercised through the real CLI review
// surface: "confirming writes valid dira entries with the edge." Two
// KindDecompose children share one GroupID (the batch a real Decompose
// call would have staged) -- a single space keystroke on the grouped row
// disposes both AND writes both, proving "one-keystroke confirmations in
// the inbox" for a kind whose whole batch needs exactly one press.
func TestInboxInteractiveSpaceAcceptWritesDecomposedChildIntoLedger(t *testing.T) {
	dispStore, root := openInboxTestStore(t)
	sw := newTestSupersedeWriter(t, root)
	dirStore := newTestDirectionStore(t, root)
	ctx := context.Background()

	parent := &ledger.Entry{
		Kind: ledger.KindIntent, Title: "Ship the Q3 launch",
		State: ledger.StateActive, Created: inboxFixedNow.UTC().Format(time.RFC3339),
	}
	if err := ledger.Add(ctx, dirStore, parent); err != nil {
		t.Fatalf("seed parent intent: %v", err)
	}

	itemA := seedDecomposeItem(t, dispStore, ctx, inboxFixedNow, parent.ID, "Write the launch doc", "needed first", "g-decompose")
	itemB := seedDecomposeItem(t, dispStore, ctx, inboxFixedNow, parent.ID, "Line up the demo env", "referenced by the doc", "g-decompose")

	var out bytes.Buffer
	if err := runInteractive(ctx, dispStore, sw, dirStore, strings.NewReader(" "), &out, "human:test", inboxFixedNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}

	if !strings.Contains(out.String(), "-> ") || !strings.Contains(out.String(), "written to ledger") {
		t.Fatalf("expected an applied/written-to-ledger line for each child, got: %q", out.String())
	}

	var newTitles []string
	for _, id := range []string{itemA.ID, itemB.ID} {
		got, err := dispStore.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if got.State != disposition.StateDisposed || got.Verdict != disposition.VerdictAccept {
			t.Fatalf("item %s State=%q Verdict=%q, want disposed/accept", id, got.State, got.Verdict)
		}
	}

	entries, err := dirStore.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range entries {
		if info.ID == parent.ID {
			continue
		}
		e, err := dirStore.Get(ctx, info.ID)
		if err != nil {
			t.Fatal(err)
		}
		newTitles = append(newTitles, e.Title)
		if e.Kind != ledger.KindIntent || e.State != ledger.StateActive {
			t.Fatalf("entry %s: Kind=%q State=%q, want intent/active", info.ID, e.Kind, e.State)
		}
		if len(e.Edges) != 1 || e.Edges[0].Type != ledger.EdgeDerivesFrom || e.Edges[0].To != parent.ID {
			t.Fatalf("entry %s Edges=%+v, want one derives_from edge to %s", info.ID, e.Edges, parent.ID)
		}
	}
	if len(newTitles) != 2 {
		t.Fatalf("ledger has %d new child entries, want 2 (got titles: %v)", len(newTitles), newTitles)
	}
}
