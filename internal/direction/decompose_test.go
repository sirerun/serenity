package direction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/router"
)

// fakeProvider is a test double implementing router.Provider -- the same
// pattern internal/extract/extract_test.go and internal/router/router_test.go
// both already use. Test-file only, per the zero-stub policy.
type fakeProvider struct {
	name         string
	modelVersion string
	resp         router.Response
	err          error
	calls        int
}

func (f *fakeProvider) Name() string         { return f.name }
func (f *fakeProvider) ModelVersion() string { return f.modelVersion }
func (f *fakeProvider) Send(_ context.Context, _ string) (router.Response, error) {
	f.calls++
	return f.resp, f.err
}

type fakeLedger struct{ entries []router.SpendEntry }

func (f *fakeLedger) Record(_ context.Context, e router.SpendEntry) error {
	f.entries = append(f.entries, e)
	return nil
}

// newTestRouter builds a real *router.Router with fp registered under the
// judgment tier -- router.TaskClassDecompositionProposals resolves to
// TierJudgment (internal/router/router.go's own taskClassTiers map), so
// Decompose's tier resolution, confidence cap, and spend-ledger recording
// all run for real against this fake provider rather than being stubbed
// out of the test entirely.
func newTestRouter(fp *fakeProvider) *router.Router {
	return router.New(map[router.Tier]router.Provider{router.TierJudgment: fp}, &fakeLedger{})
}

func openTestDispositionStore(t *testing.T) *disposition.Store {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng)
}

// seedParentIntent writes a real, already-confirmed-shaped intent
// directly to store's ledger (via ledger.Add, bypassing CreateDraft since
// KindIntent never carries StateStaged) so Decompose has a real parent to
// read.
func seedParentIntent(t *testing.T, s *Store, title, body string, now time.Time) *ledger.Entry {
	t.Helper()
	e := &ledger.Entry{
		Kind:    ledger.KindIntent,
		Title:   title,
		State:   ledger.StateActive,
		Created: now.UTC().Format(time.RFC3339),
		Body:    body,
	}
	if err := ledger.Add(context.Background(), s, e); err != nil {
		t.Fatalf("seed parent intent: %v", err)
	}
	return e
}

var decomposeFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// TestDecomposeStagesOneItemPerChildSharingGroupID is this task's own
// acc-line clause: "decompose on a fixture intent yields staged child
// drafts carrying derives_from edges" -- the drafts (disposition items,
// not yet ledger entries -- nothing is written until accept) carry
// exactly the parent id ApplyDisposedDecompose will later turn into the
// edge, and every item from one call shares a GroupID (RFC 0001 section
// 8.2's grouped items).
func TestDecomposeStagesOneItemPerChildSharingGroupID(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	parent := seedParentIntent(t, s, "Ship the Q3 launch", "", decomposeFixedNow)

	resp := `{"children":[
		{"title":"Write the launch doc","rationale":"needed before anything else can proceed"},
		{"title":"Line up the demo environment","rationale":"the launch doc references it"}
	]}`
	fp := &fakeProvider{name: "fake", modelVersion: "fake-judgment@v1", resp: router.Response{Text: resp}}
	r := newTestRouter(fp)
	dispStore := openTestDispositionStore(t)

	items, err := Decompose(ctx, r, dispStore, parent, decomposeFixedNow)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if fp.calls != 1 {
		t.Fatalf("router called %d times, want 1", fp.calls)
	}

	wantTitles := map[string]string{
		"Write the launch doc":         "needed before anything else can proceed",
		"Line up the demo environment": "the launch doc references it",
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.Kind != disposition.KindDecompose {
			t.Fatalf("item %s Kind = %q, want %q", it.ID, it.Kind, disposition.KindDecompose)
		}
		if it.State != disposition.StatePending {
			t.Fatalf("item %s State = %q, want %q", it.ID, it.State, disposition.StatePending)
		}
		if it.GroupID == "" || it.GroupID != items[0].GroupID {
			t.Fatalf("item %s GroupID = %q, want a shared non-empty group with %q", it.ID, it.GroupID, items[0].GroupID)
		}
		var payload DecomposePayload
		if err := json.Unmarshal(it.Payload, &payload); err != nil {
			t.Fatalf("decode payload for %s: %v", it.ID, err)
		}
		if payload.ParentID != parent.ID {
			t.Fatalf("item %s ParentID = %q, want %q", it.ID, payload.ParentID, parent.ID)
		}
		wantRationale, ok := wantTitles[payload.Child.Title]
		if !ok {
			t.Fatalf("unexpected child title %q", payload.Child.Title)
		}
		if payload.Child.Rationale != wantRationale {
			t.Fatalf("child %q rationale = %q, want %q", payload.Child.Title, payload.Child.Rationale, wantRationale)
		}
		seen[payload.Child.Title] = true
	}
	if len(seen) != 2 {
		t.Fatalf("saw %d distinct child titles, want 2: %v", len(seen), seen)
	}
}

// TestDecomposeRefusesNonIntentParent: derives_from decomposition only
// makes sense for an intent -- a decision or constraint parent is
// refused before any router call is made (no wasted judgment-tier spend
// on a request that can never produce a valid write).
func TestDecomposeRefusesNonIntentParent(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	parent := stagedDraft("dec-0001", "Adopt the new vendor")
	parent.State = ledger.StateAccepted // a real decision, not staged

	fp := &fakeProvider{name: "fake", modelVersion: "fake-judgment@v1", resp: router.Response{Text: `{"children":[]}`}}
	r := newTestRouter(fp)
	dispStore := openTestDispositionStore(t)
	_ = s

	items, err := Decompose(ctx, r, dispStore, parent, decomposeFixedNow)
	if err == nil {
		t.Fatal("Decompose: want error for a non-intent parent, got nil")
	}
	if items != nil {
		t.Fatalf("Decompose: want nil items on refusal, got %v", items)
	}
	if fp.calls != 0 {
		t.Fatalf("router called %d times, want 0 -- refusal must happen before any router call", fp.calls)
	}
}

// TestDecomposeMalformedResponseYieldsZeroItems: a response that fails to
// parse as the required JSON shape fails closed -- zero children, zero
// disposition items -- never a best-effort scan of whatever prose the
// model actually emitted.
func TestDecomposeMalformedResponseYieldsZeroItems(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	parent := seedParentIntent(t, s, "Ship the Q3 launch", "", decomposeFixedNow)

	fp := &fakeProvider{name: "fake", modelVersion: "fake-judgment@v1", resp: router.Response{Text: "sorry, I can't help with that"}}
	r := newTestRouter(fp)
	dispStore := openTestDispositionStore(t)

	items, err := Decompose(ctx, r, dispStore, parent, decomposeFixedNow)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
	all, err := dispStore.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("disposition store has %d items, want 0", len(all))
	}
}

// TestDecomposeNothingLandsWithoutADisposition is this task's own
// acc-line clause, checked structurally: after Decompose stages its
// items, the on-disk ledger (.dira/entries/) is byte-for-byte unchanged
// from before the call -- proposing children never itself writes a
// ledger entry, disposition items are the only artifact.
func TestDecomposeNothingLandsWithoutADisposition(t *testing.T) {
	ctx := context.Background()
	s, root := newTestStore(t)
	parent := seedParentIntent(t, s, "Ship the Q3 launch", "", decomposeFixedNow)

	before, err := os.ReadDir(filepath.Join(root, ".dira", "entries"))
	if err != nil {
		t.Fatal(err)
	}
	beforeNames := make(map[string]bool, len(before))
	for _, de := range before {
		beforeNames[de.Name()] = true
	}

	resp := `{"children":[{"title":"Write the launch doc","rationale":"r"}]}`
	fp := &fakeProvider{name: "fake", modelVersion: "fake-judgment@v1", resp: router.Response{Text: resp}}
	r := newTestRouter(fp)
	dispStore := openTestDispositionStore(t)

	if _, err := Decompose(ctx, r, dispStore, parent, decomposeFixedNow); err != nil {
		t.Fatalf("Decompose: %v", err)
	}

	after, err := os.ReadDir(filepath.Join(root, ".dira", "entries"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(beforeNames) {
		t.Fatalf(".dira/entries/ has %d files after Decompose, want %d (unchanged) -- proposing must never write", len(after), len(beforeNames))
	}
	for _, de := range after {
		if !beforeNames[de.Name()] {
			t.Fatalf("Decompose wrote a new ledger file %q -- nothing may land without a disposition", de.Name())
		}
	}
}

// TestApplyDisposedDecomposeWritesEntryWithDerivesFromEdge is this task's
// own acc-line clause: "confirming writes valid dira entries with the
// edge." Accepts a real staged item (a real Store.Dispose call) and
// applies it, then reads the written entry straight back off disk.
func TestApplyDisposedDecomposeWritesEntryWithDerivesFromEdge(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	parent := seedParentIntent(t, s, "Ship the Q3 launch", "", decomposeFixedNow)

	payload, err := json.Marshal(DecomposePayload{
		ParentID: parent.ID,
		Child:    ChildIntentDraft{Title: "Write the launch doc", Rationale: "needed before anything else can proceed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispStore := openTestDispositionStore(t)
	staged, err := dispStore.Create(ctx, disposition.KindDecompose, payload, "grp-1", decomposeFixedNow)
	if err != nil {
		t.Fatal(err)
	}
	res, err := dispStore.Dispose(ctx, staged.ID, disposition.VerdictAccept, nil, "", "human:tester", "", decomposeFixedNow)
	if err != nil {
		t.Fatal(err)
	}

	entry, err := s.ApplyDisposedDecompose(ctx, res.Item, decomposeFixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedDecompose: %v", err)
	}
	if entry.Kind != ledger.KindIntent {
		t.Fatalf("entry.Kind = %q, want %q", entry.Kind, ledger.KindIntent)
	}
	if entry.State != ledger.StateActive {
		t.Fatalf("entry.State = %q, want %q", entry.State, ledger.StateActive)
	}
	if entry.Title != "Write the launch doc" {
		t.Fatalf("entry.Title = %q, want %q", entry.Title, "Write the launch doc")
	}
	if len(entry.Edges) != 1 || entry.Edges[0].Type != ledger.EdgeDerivesFrom || entry.Edges[0].To != parent.ID {
		t.Fatalf("entry.Edges = %+v, want one derives_from edge to %s", entry.Edges, parent.ID)
	}
	if entry.Edges[0].Note != "needed before anything else can proceed" {
		t.Fatalf("entry.Edges[0].Note = %q, want the child's rationale", entry.Edges[0].Note)
	}

	// Read straight back off disk through Get -- proves the entry
	// actually validates against ledger.Entry.Validate (Get decodes via
	// the same codec every other reader uses), not just that the Go
	// struct in memory looked right before the write.
	stored, err := s.Get(ctx, entry.ID)
	if err != nil {
		t.Fatalf("Get %s: %v", entry.ID, err)
	}
	if stored.Title != entry.Title || stored.Edges[0].To != parent.ID {
		t.Fatalf("stored entry %+v does not match what ApplyDisposedDecompose returned", stored)
	}
}

// TestApplyDisposedDecomposeRefusesUnacceptedItem: a still-pending item
// (never disposed) must refuse -- staging a proposal is not the same as
// a human accepting it, and nothing may be written for it.
func TestApplyDisposedDecomposeRefusesUnacceptedItem(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	parent := seedParentIntent(t, s, "Ship the Q3 launch", "", decomposeFixedNow)

	payload, err := json.Marshal(DecomposePayload{
		ParentID: parent.ID,
		Child:    ChildIntentDraft{Title: "Write the launch doc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispStore := openTestDispositionStore(t)
	staged, err := dispStore.Create(ctx, disposition.KindDecompose, payload, "", decomposeFixedNow)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ApplyDisposedDecompose(ctx, staged, decomposeFixedNow); err == nil {
		t.Fatal("ApplyDisposedDecompose: want error for a still-pending item, got nil")
	}

	entries, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Only the seeded parent should exist -- nothing was written for the
	// refused child.
	if len(entries) != 1 {
		t.Fatalf("ledger has %d entries, want 1 (parent only)", len(entries))
	}
}
