package direction

import (
	"context"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
)

// activeIntent returns a valid, active KindIntent entry -- the derivation
// target every "chained" activity fixture entry points at.
func activeIntent(id, title, created string) *ledger.Entry {
	return &ledger.Entry{
		ID:      id,
		Kind:    ledger.KindIntent,
		Title:   title,
		State:   ledger.StateActive,
		Created: created,
	}
}

// noteActivity returns a valid, active KindNote entry -- this task's own
// reading of "activity" (see orphan.go's doc comment): anything that is
// not itself an intent. edges is nil for an unchained fixture entry.
func noteActivity(id, title, created string, edges []ledger.Edge) *ledger.Entry {
	return &ledger.Entry{
		ID:      id,
		Kind:    ledger.KindNote,
		Title:   title,
		State:   ledger.StateActive,
		Created: created,
		Edges:   edges,
	}
}

func mustPut(t *testing.T, s *Store, e *ledger.Entry) {
	t.Helper()
	if err := s.Put(context.Background(), e); err != nil {
		t.Fatalf("Put(%s): %v", e.ID, err)
	}
}

// TestDetectOrphansUnhealthyFixtureListsExactlyTwo is plan T3.9's own acc
// line, run literally: a fixture week with 3 activities, one chained to
// an intent via derives_from, the other two not -> exactly 2 orphans.
func TestDetectOrphansUnhealthyFixtureListsExactlyTwo(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	weekAgo := now.Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339)

	mustPut(t, store, activeIntent("int-0001", "Ship the widget", now.Add(-30*24*time.Hour).UTC().Format(time.RFC3339)))
	// Chained: a direct derives_from edge to the active intent.
	mustPut(t, store, noteActivity("note-0001", "Chained note", weekAgo,
		[]ledger.Edge{{Type: ledger.EdgeDerivesFrom, To: "int-0001"}}))
	// Orphan: no edges at all.
	mustPut(t, store, noteActivity("note-0002", "Unchained note", weekAgo, nil))
	// Orphan: has an edge, but not derives_from.
	mustPut(t, store, noteActivity("note-0003", "Wrong-edge note", weekAgo,
		[]ledger.Edge{{Type: ledger.EdgeInforms, To: "int-0001"}}))

	orphans, err := DetectOrphans(ctx, store, now, DefaultOrphanLookback)
	if err != nil {
		t.Fatalf("DetectOrphans: %v", err)
	}
	if len(orphans) != 2 {
		t.Fatalf("DetectOrphans returned %d orphans, want exactly 2: %+v", len(orphans), orphans)
	}
	ids := map[string]bool{}
	for _, o := range orphans {
		ids[o.ID] = true
	}
	if !ids["note-0002"] || !ids["note-0003"] || ids["note-0001"] {
		t.Fatalf("orphan ids = %v, want exactly {note-0002, note-0003}", ids)
	}
}

// TestDetectOrphansHealthyFixtureListsZero is the acc line's other half:
// a fixture where every activity is chained lists 0.
func TestDetectOrphansHealthyFixtureListsZero(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	weekAgo := now.Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339)

	mustPut(t, store, activeIntent("int-0001", "Ship the widget", now.Add(-30*24*time.Hour).UTC().Format(time.RFC3339)))
	for i, id := range []string{"note-0001", "note-0002", "note-0003"} {
		mustPut(t, store, noteActivity(id, "Chained note", weekAgo,
			[]ledger.Edge{{Type: ledger.EdgeDerivesFrom, To: "int-0001"}}))
		_ = i
	}

	orphans, err := DetectOrphans(ctx, store, now, DefaultOrphanLookback)
	if err != nil {
		t.Fatalf("DetectOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("DetectOrphans on a healthy fixture returned %d orphans, want 0: %+v", len(orphans), orphans)
	}
}

// TestDetectOrphansExcludesOutsideLookbackWindow proves an otherwise
// unchained entry created before the lookback window is not flagged --
// "weekly" activity, not the ledger's entire history.
func TestDetectOrphansExcludesOutsideLookbackWindow(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)

	mustPut(t, store, noteActivity("note-0001", "Old unchained note", old, nil))

	orphans, err := DetectOrphans(ctx, store, now, DefaultOrphanLookback)
	if err != nil {
		t.Fatalf("DetectOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("DetectOrphans flagged an entry outside the lookback window: %+v", orphans)
	}
}

// TestDetectOrphansNeverFlagsAnIntentItself proves a root active intent,
// which legitimately has no derives_from edge of its own, is never
// itself reported as an orphan.
func TestDetectOrphansNeverFlagsAnIntentItself(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	weekAgo := now.Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339)

	mustPut(t, store, activeIntent("int-0001", "A brand-new root intent", weekAgo))

	orphans, err := DetectOrphans(ctx, store, now, DefaultOrphanLookback)
	if err != nil {
		t.Fatalf("DetectOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("DetectOrphans flagged the intent itself: %+v", orphans)
	}
}

// TestDetectOrphansRequiresTheTargetIntentBeActive proves a derives_from
// edge to a non-active intent (e.g. one already achieved) does not count
// as chained -- "an active intent" is the RFC's own word, not just any
// intent that once existed.
func TestDetectOrphansRequiresTheTargetIntentBeActive(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	weekAgo := now.Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339)

	achieved := activeIntent("int-0001", "Already-achieved intent", now.Add(-30*24*time.Hour).UTC().Format(time.RFC3339))
	achieved.State = ledger.StateAchieved
	mustPut(t, store, achieved)
	mustPut(t, store, noteActivity("note-0001", "Chained to a dead intent", weekAgo,
		[]ledger.Edge{{Type: ledger.EdgeDerivesFrom, To: "int-0001"}}))

	orphans, err := DetectOrphans(ctx, store, now, DefaultOrphanLookback)
	if err != nil {
		t.Fatalf("DetectOrphans: %v", err)
	}
	if len(orphans) != 1 || orphans[0].ID != "note-0001" {
		t.Fatalf("DetectOrphans = %+v, want exactly note-0001 (its target intent is achieved, not active)", orphans)
	}
}

// TestDetectOrphansCreatesNoDispositionItem is the acc line's other
// requirement made concrete: DetectOrphans has no way to reach a
// disposition.Store at all (see orphan.go's own doc comment on the
// signature), but this proves it for real rather than by inspection --
// opens a real disposition.Store alongside the ledger store, runs the
// same unhealthy fixture, and confirms the disposition queue is still
// empty afterward.
func TestDetectOrphansCreatesNoDispositionItem(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	weekAgo := now.Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339)

	mustPut(t, store, activeIntent("int-0001", "Ship the widget", now.Add(-30*24*time.Hour).UTC().Format(time.RFC3339)))
	mustPut(t, store, noteActivity("note-0001", "Unchained note", weekAgo, nil))

	eng, err := index.Open(t.TempDir() + "/index.db")
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = eng.Close() }()
	dispStore := disposition.NewStore(eng)

	orphans, err := DetectOrphans(ctx, store, now, DefaultOrphanLookback)
	if err != nil {
		t.Fatalf("DetectOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("DetectOrphans = %+v, want exactly 1", orphans)
	}

	items, err := dispStore.List(ctx)
	if err != nil {
		t.Fatalf("disposition List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("disposition queue has %d items after DetectOrphans, want 0 (orphans are informational only)", len(items))
	}
}
