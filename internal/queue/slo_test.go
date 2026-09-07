package queue

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
)

func openTestStore(t *testing.T) *disposition.Store {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng)
}

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// parkItem drives item through Sweep with a threshold small enough to park
// it in exactly disposition.MaxDeferCycles calls, without needing days of
// wall-clock advance per cycle -- same mechanism
// TestSweepWalksItemThroughDeferredToParked (internal/disposition) uses,
// just with a threshold sized for this package's fixtures.
func parkItem(t *testing.T, s *disposition.Store, id string, start time.Time) {
	t.Helper()
	ctx := context.Background()
	thresholds := disposition.Thresholds{disposition.KindReconcile: time.Minute}
	t0 := start
	for i := 0; i < disposition.MaxDeferCycles; i++ {
		t0 = t0.Add(time.Minute)
		if _, err := disposition.Sweep(ctx, s, thresholds, t0); err != nil {
			t.Fatalf("Sweep (park %s, cycle %d): %v", id, i+1, err)
		}
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get %s: %v", id, err)
	}
	if got.State != disposition.StateParked {
		t.Fatalf("parkItem: %s ended in state %q, want parked", id, got.State)
	}
}

// TestQueueSLODepthAlertTripsAt60PendingItems is T2.15's own acc-line
// clause: "60 pending items trip the depth alert" -- 60 > RFC 0001 §7's
// depth threshold of 50.
func TestQueueSLODepthAlertTripsAt60PendingItems(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	for i := 0; i < 60; i++ {
		if _, err := s.Create(ctx, disposition.KindReconcile, nil, "", fixedNow); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	snap, err := Compute(ctx, s, DefaultConfig(), fixedNow)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.Depth != 60 {
		t.Fatalf("Depth = %d, want 60", snap.Depth)
	}
	if !snap.DepthAlert {
		t.Fatal("DepthAlert = false, want true (60 > 50)")
	}
	// All 60 items were just created (age 0) -- the age alert must stay
	// clear so this test isolates the depth alert alone.
	if snap.AgeAlert {
		t.Fatal("AgeAlert = true, want false -- items are brand new")
	}
}

// TestQueueSLOAgeAlertTripsWith40ItemsOlderThan3Days is T2.15's own
// acc-line clause: "40 items older than 3 days trip the age alert." 40 is
// below the depth threshold (50) -- chosen so this fixture isolates the
// age alert without also tripping the depth alert, proving they are
// independent signals.
func TestQueueSLOAgeAlertTripsWith40ItemsOlderThan3Days(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	created := fixedNow.Add(-4 * 24 * time.Hour) // every item is 4 days old
	for i := 0; i < 40; i++ {
		if _, err := s.Create(ctx, disposition.KindReconcile, nil, "", created); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	snap, err := Compute(ctx, s, DefaultConfig(), fixedNow)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.Depth != 40 {
		t.Fatalf("Depth = %d, want 40", snap.Depth)
	}
	if snap.DepthAlert {
		t.Fatal("DepthAlert = true, want false -- 40 does not exceed the 50 depth threshold")
	}
	if !snap.P50AgeOK || snap.P50Age != 4*24*time.Hour {
		t.Fatalf("P50Age = %v (ok=%v), want 96h0m0s", snap.P50Age, snap.P50AgeOK)
	}
	if !snap.AgeAlert {
		t.Fatal("AgeAlert = false, want true (p50 age 4d > 3d threshold)")
	}
}

// TestQueueSLOParkedItemsCountInNeither is T2.15's own acc-line clause:
// "parked items count in neither" (depth nor age). A parked item that is
// both very old and part of a population that would otherwise trip both
// alerts must be invisible to Depth and P50Age -- RFC 0001 §8.2: parked is
// "excluded from queue-depth/age SLO metrics."
func TestQueueSLOParkedItemsCountInNeither(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	veryOld := fixedNow.Add(-365 * 24 * time.Hour)

	item, err := s.Create(ctx, disposition.KindReconcile, nil, "", veryOld)
	if err != nil {
		t.Fatal(err)
	}
	parkItem(t, s, item.ID, veryOld)

	// One live pending item, recent, so Depth/P50Age reflect only it.
	if _, err := s.Create(ctx, disposition.KindReconcile, nil, "", fixedNow); err != nil {
		t.Fatal(err)
	}

	snap, err := Compute(ctx, s, DefaultConfig(), fixedNow)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.Depth != 1 {
		t.Fatalf("Depth = %d, want 1 (parked item excluded)", snap.Depth)
	}
	if snap.P50AgeOK && snap.P50Age >= 24*time.Hour {
		t.Fatalf("P50Age = %v, want ~0 (parked item's huge age must not leak in)", snap.P50Age)
	}
	if snap.DepthAlert || snap.AgeAlert {
		t.Fatalf("alerts tripped on a single fresh item: depth=%v age=%v", snap.DepthAlert, snap.AgeAlert)
	}
}

// TestQueueSLORendersTimeToDisposeAndAbandonment covers the acc line's
// final clause: "status renders depth, p50 age, time-to-dispose,
// abandonment." Seeds a population that has been disposed, parked, and
// left pending, and checks each of the four Snapshot metrics against a
// hand-computed expectation.
func TestQueueSLORendersTimeToDisposeAndAbandonment(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	// Two disposed items: one took 1h to dispose, the other 3h -- median
	// (p50) of {1h, 3h} is 2h.
	a, err := s.Create(ctx, disposition.KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dispose(ctx, a.ID, disposition.VerdictAccept, nil, "", "human:t", "", fixedNow.Add(1*time.Hour)); err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(ctx, disposition.KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dispose(ctx, b.ID, disposition.VerdictReject, nil, "no", "human:t", "", fixedNow.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// One parked item -- abandonment population becomes parked=1,
	// disposed=2, so abandonment = 1/3.
	parked, err := s.Create(ctx, disposition.KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	parkItem(t, s, parked.ID, fixedNow)

	snap, err := Compute(ctx, s, DefaultConfig(), fixedNow.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !snap.TimeToDisposeOK || snap.TimeToDispose != 2*time.Hour {
		t.Fatalf("TimeToDispose = %v (ok=%v), want 2h0m0s", snap.TimeToDispose, snap.TimeToDisposeOK)
	}
	wantAbandonment := 1.0 / 3.0
	if !snap.AbandonmentOK || snap.Abandonment != wantAbandonment {
		t.Fatalf("Abandonment = %v (ok=%v), want %v", snap.Abandonment, snap.AbandonmentOK, wantAbandonment)
	}
	// Nothing pending/deferred is left in this fixture (both non-parked
	// items were disposed) -- P50Age must report "no data" honestly
	// rather than a misleading zero.
	if snap.P50AgeOK {
		t.Fatalf("P50AgeOK = true, want false -- no pending/deferred items in this fixture")
	}
}

// TestQueueSLOEmptyStoreReportsNoDataNotZero: a brand-new store (no items
// at all) must report every *OK flag false, never a bare zero that would
// misrepresent "no data yet" as "healthy queue, 0 pending."
func TestQueueSLOEmptyStoreReportsNoDataNotZero(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	snap, err := Compute(ctx, s, DefaultConfig(), fixedNow)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.Depth != 0 || snap.DepthAlert {
		t.Fatalf("Depth/DepthAlert = %d/%v, want 0/false", snap.Depth, snap.DepthAlert)
	}
	if snap.P50AgeOK || snap.AgeAlert {
		t.Fatalf("P50AgeOK/AgeAlert = %v/%v, want false/false", snap.P50AgeOK, snap.AgeAlert)
	}
	if snap.TimeToDisposeOK {
		t.Fatal("TimeToDisposeOK = true, want false")
	}
	if snap.AbandonmentOK {
		t.Fatal("AbandonmentOK = true, want false")
	}
}
