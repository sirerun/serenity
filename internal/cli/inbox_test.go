package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/reconcile"
)

var inboxFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// openInboxTestStore opens a real disposition.Store over a real brain
// repo's derived index (initBrainRepo/providers.OpenIndex -- the same
// path runInbox itself takes), not a synthetic double.
func openInboxTestStore(t *testing.T) *disposition.Store {
	t.Helper()
	root := initBrainRepo(t)
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng)
}

// seedReconcileItem stages one KindReconcile item shaped exactly like
// internal/reconcile.Engine.Process's own output (T2.2): both claims
// share (subject, predicate), and Family carries the value inbox.go's
// itemFamily/bulk-defer filtering reads.
func seedReconcileItem(t *testing.T, store *disposition.Store, ctx context.Context, now time.Time, subject, predicate, aObject, bObject, groupID string) disposition.Item {
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
	item, err := store.Create(ctx, disposition.KindReconcile, payload, groupID, now)
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
	store := openInboxTestStore(t)
	ctx := context.Background()

	untouchedBefore := seedReconcileItem(t, store, ctx, inboxFixedNow, "alice-tan", "works_at", "acme-corp", "initech", "")
	groupA := seedReconcileItem(t, store, ctx, inboxFixedNow, "acme-corp", "has_balance", "$700", "$500", "g1")
	groupB := seedReconcileItem(t, store, ctx, inboxFixedNow, "acme-corp", "has_balance", "$900", "$500", "g1")
	untouchedAfter := seedReconcileItem(t, store, ctx, inboxFixedNow, "bob-lee", "works_at", "globex", "initrode", "")

	var out bytes.Buffer
	in := strings.NewReader("jjk ") // down to group row, down to last row, back up to group row, dispose
	if err := runInteractive(ctx, store, in, &out, "human:test", inboxFixedNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}

	for _, id := range []string{groupA.ID, groupB.ID} {
		got, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if got.State != disposition.StateDisposed {
			t.Errorf("group member %s State = %q, want disposed", id, got.State)
		}
		if got.Verdict != disposition.VerdictAccept {
			t.Errorf("group member %s Verdict = %q, want accept", id, got.Verdict)
		}
		hist, err := store.HistoryFor(ctx, id)
		if err != nil {
			t.Fatalf("HistoryFor %s: %v", id, err)
		}
		if len(hist) != 1 {
			t.Errorf("group member %s has %d history row(s), want exactly 1", id, len(hist))
		}
	}

	for _, id := range []string{untouchedBefore.ID, untouchedAfter.ID} {
		got, err := store.Get(ctx, id)
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
	store := openInboxTestStore(t)
	ctx := context.Background()

	item := seedReconcileItem(t, store, ctx, inboxFixedNow, "carol-diaz", "works_at", "umbrella", "oscorp", "")

	var out bytes.Buffer
	if err := runInteractive(ctx, store, strings.NewReader(" "), &out, "human:test", inboxFixedNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}
	got, err := store.Get(ctx, item.ID)
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
	store := openInboxTestStore(t)
	ctx := context.Background()

	match1 := seedReconcileItem(t, store, ctx, inboxFixedNow, "acme-corp", "has_balance", "$700", "$500", "")
	match2 := seedReconcileItem(t, store, ctx, inboxFixedNow, "globex", "has_balance", "$300", "$200", "")
	other := seedReconcileItem(t, store, ctx, inboxFixedNow, "alice-tan", "works_at", "acme-corp", "initech", "")

	var out bytes.Buffer
	if err := runBulkDefer(ctx, store, "family=has_balance", "human:test", &out, inboxFixedNow); err != nil {
		t.Fatalf("runBulkDefer: %v", err)
	}

	for _, id := range []string{match1.ID, match2.ID} {
		got, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if got.State != disposition.StateDeferred || got.Verdict != disposition.VerdictDefer {
			t.Errorf("matching item %s State=%q Verdict=%q, want deferred/defer", id, got.State, got.Verdict)
		}
	}

	gotOther, err := store.Get(ctx, other.ID)
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
	store := openInboxTestStore(t)
	ctx := context.Background()

	var out bytes.Buffer
	err := runBulkDefer(ctx, store, "kind=reconcile", "human:test", &out, inboxFixedNow)
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
	store := openInboxTestStore(t)
	ctx := context.Background()

	toPark := seedReconcileItem(t, store, ctx, inboxFixedNow, "dave-kim", "has_balance", "$100", "$50", "")

	thresholds := disposition.Thresholds{disposition.KindReconcile: time.Minute}
	now := inboxFixedNow
	for i := 0; i < disposition.MaxDeferCycles; i++ {
		now = now.Add(2 * time.Minute)
		if _, err := disposition.Sweep(ctx, store, thresholds, now); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
	}
	gotParked, err := store.Get(ctx, toPark.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotParked.State != disposition.StateParked {
		t.Fatalf("fixture setup failed: item State = %q, want parked", gotParked.State)
	}

	// Freshly created at the sweep's own final "now" -- not yet threshold-old,
	// so it stays pending and must never show up in a parked-only listing.
	stillPending := seedReconcileItem(t, store, ctx, now, "carol-diaz", "works_at", "umbrella", "oscorp", "")

	var out bytes.Buffer
	if err := runListParked(ctx, store, &out); err != nil {
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
	store := openInboxTestStore(t)
	ctx := context.Background()

	var out bytes.Buffer
	if err := runListParked(ctx, store, &out); err != nil {
		t.Fatalf("runListParked: %v", err)
	}
	if !strings.Contains(out.String(), "no parked items") {
		t.Fatalf("expected an explicit empty-state message, got: %q", out.String())
	}
}
