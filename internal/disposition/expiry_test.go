package disposition

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSweepWalksItemThroughDeferredToParked is the acc-line's first
// clause: "fake-clock test walks an item through deferred to parked."
func TestSweepWalksItemThroughDeferredToParked(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	start := fixedNow
	item, err := s.Create(ctx, KindReconcile, nil, "", start)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Cycle 1: still pending, 15 days old -> deferred (DeferCount 1).
	t1 := start.Add(15 * 24 * time.Hour)
	res, err := Sweep(ctx, s, nil, t1)
	if err != nil {
		t.Fatalf("Sweep (cycle 1): %v", err)
	}
	if res.Deferred != 1 || res.Parked != 0 {
		t.Fatalf("Sweep (cycle 1) = %+v, want Deferred=1 Parked=0", res)
	}
	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateDeferred || got.DeferCount != 1 {
		t.Fatalf("after cycle 1: State=%q DeferCount=%d, want deferred/1", got.State, got.DeferCount)
	}

	// Cycle 2: deferred again, another 15 days -> deferred (DeferCount 2).
	t2 := t1.Add(15 * 24 * time.Hour)
	res, err = Sweep(ctx, s, nil, t2)
	if err != nil {
		t.Fatalf("Sweep (cycle 2): %v", err)
	}
	if res.Deferred != 1 || res.Parked != 0 {
		t.Fatalf("Sweep (cycle 2) = %+v, want Deferred=1 Parked=0", res)
	}
	got, err = s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateDeferred || got.DeferCount != 2 {
		t.Fatalf("after cycle 2: State=%q DeferCount=%d, want deferred/2", got.State, got.DeferCount)
	}

	// Cycle 3: DeferCount would reach MaxDeferCycles (3) -> parked.
	t3 := t2.Add(15 * 24 * time.Hour)
	res, err = Sweep(ctx, s, nil, t3)
	if err != nil {
		t.Fatalf("Sweep (cycle 3): %v", err)
	}
	if res.Deferred != 0 || res.Parked != 1 {
		t.Fatalf("Sweep (cycle 3) = %+v, want Deferred=0 Parked=1", res)
	}
	got, err = s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateParked || got.DeferCount != 3 {
		t.Fatalf("after cycle 3: State=%q DeferCount=%d, want parked/3", got.State, got.DeferCount)
	}

	// Parked is terminal to Sweep: a 4th cycle, far past threshold, must
	// not touch the item again.
	t4 := t3.Add(30 * 24 * time.Hour)
	res, err = Sweep(ctx, s, nil, t4)
	if err != nil {
		t.Fatalf("Sweep (cycle 4): %v", err)
	}
	if res.Deferred != 0 || res.Parked != 0 {
		t.Fatalf("Sweep (cycle 4) = %+v, want no-op (item already parked)", res)
	}
	got, err = s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateParked || got.DeferCount != 3 {
		t.Fatalf("after cycle 4: State=%q DeferCount=%d, want unchanged parked/3", got.State, got.DeferCount)
	}
}

func TestSweepLeavesFreshItemsAlone(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindDistill, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// One hour later -- nowhere near the 14-day default threshold.
	res, err := Sweep(ctx, s, nil, fixedNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Deferred != 0 || res.Parked != 0 {
		t.Fatalf("Sweep on a fresh item = %+v, want no-op", res)
	}
	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StatePending || got.DeferCount != 0 {
		t.Fatalf("fresh item State=%q DeferCount=%d, want unchanged pending/0", got.State, got.DeferCount)
	}
}

func TestSweepNeverDisposesAnything(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindEffect, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for i := 1; i <= 4; i++ {
		if _, err := Sweep(ctx, s, nil, fixedNow.Add(time.Duration(i)*15*24*time.Hour)); err != nil {
			t.Fatalf("Sweep cycle %d: %v", i, err)
		}
	}

	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateParked {
		t.Fatalf("State = %q, want parked", got.State)
	}
	if got.Verdict != "" || !got.DisposedAt.IsZero() {
		t.Fatalf("Sweep set Verdict=%q DisposedAt=%v, want both unset -- expiry is auto-DEFER, never auto-decline", got.Verdict, got.DisposedAt)
	}
}

func TestSweepUsesPerKindThreshold(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindTombstone, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	thresholds := Thresholds{KindTombstone: 24 * time.Hour}

	// 12h old under a 24h threshold: still fresh.
	res, err := Sweep(ctx, s, thresholds, fixedNow.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Deferred != 0 {
		t.Fatalf("Sweep at 12h = %+v, want no-op under a 24h threshold", res)
	}

	// 25h old: past the configured 24h threshold, even though it is far
	// short of DefaultExpiryThreshold (14 days).
	res, err = Sweep(ctx, s, thresholds, fixedNow.Add(25*time.Hour))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Deferred != 1 {
		t.Fatalf("Sweep at 25h = %+v, want Deferred=1 under a 24h threshold", res)
	}
	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateDeferred || got.DeferCount != 1 {
		t.Fatalf("after 25h: State=%q DeferCount=%d, want deferred/1", got.State, got.DeferCount)
	}
}

// TestResurfaceOnce is the acc-line's second clause: "a new claim on the
// same (subject, predicate) resurfaces it exactly once" -- exercised here
// at the mechanical layer Resurface owns (a caller decides when to call
// it; Resurface enforces the "exactly once" cap).
func TestResurfaceOnce(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := Sweep(ctx, s, nil, fixedNow.Add(time.Duration(i)*15*24*time.Hour)); err != nil {
			t.Fatalf("Sweep cycle %d: %v", i, err)
		}
	}
	got, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateParked {
		t.Fatalf("State = %q, want parked before resurfacing", got.State)
	}

	resurfaceAt := fixedNow.Add(60 * 24 * time.Hour)
	resurfaced, err := s.Resurface(ctx, item.ID, resurfaceAt)
	if err != nil {
		t.Fatalf("Resurface: %v", err)
	}
	if resurfaced.State != StatePending || !resurfaced.Resurfaced {
		t.Fatalf("after Resurface: State=%q Resurfaced=%v, want pending/true", resurfaced.State, resurfaced.Resurfaced)
	}

	// A second resurface attempt on the same item must be refused --
	// "nothing resurfaces forever."
	_, err = s.Resurface(ctx, item.ID, resurfaceAt.Add(time.Hour))
	if !errors.Is(err, ErrAlreadyResurfaced) {
		t.Fatalf("second Resurface: err = %v, want ErrAlreadyResurfaced", err)
	}
}

func TestResurfaceNonParkedItemErrors(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	item, err := s.Create(ctx, KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = s.Resurface(ctx, item.ID, fixedNow)
	if !errors.Is(err, ErrNotParked) {
		t.Fatalf("Resurface(pending item): err = %v, want ErrNotParked", err)
	}
}

// TestParkedItemsExcludedFromDepthAndAgeMetrics is the acc-line's third
// clause verbatim.
func TestParkedItemsExcludedFromDepthAndAgeMetrics(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	// One item aged through all 3 cycles into parked.
	parked, err := s.Create(ctx, KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatalf("Create (parked): %v", err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := Sweep(ctx, s, nil, fixedNow.Add(time.Duration(i)*15*24*time.Hour)); err != nil {
			t.Fatalf("Sweep cycle %d: %v", i, err)
		}
	}
	got, err := s.Get(ctx, parked.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateParked {
		t.Fatalf("State = %q, want parked", got.State)
	}

	// A second item created fresh, after the parked item's aging is done
	// -- never touched by any Sweep call, so it stays pending.
	stillPending, err := s.Create(ctx, KindDistill, nil, "", fixedNow.Add(45*24*time.Hour))
	if err != nil {
		t.Fatalf("Create (pending): %v", err)
	}
	pendingGot, err := s.Get(ctx, stillPending.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if pendingGot.State != StatePending {
		t.Fatalf("stillPending State = %q, want pending", pendingGot.State)
	}

	depth, err := s.PendingDepth(ctx)
	if err != nil {
		t.Fatalf("PendingDepth: %v", err)
	}
	if depth != 1 {
		t.Fatalf("PendingDepth = %d, want 1 (the parked item must not count)", depth)
	}

	now := fixedNow.Add(90 * 24 * time.Hour)
	age, ok, err := s.OldestPendingAge(ctx, now)
	if err != nil {
		t.Fatalf("OldestPendingAge: %v", err)
	}
	if !ok {
		t.Fatal("OldestPendingAge: ok = false, want true (stillPending exists)")
	}
	wantAge := now.Sub(stillPending.CreatedAt.UTC())
	if age != wantAge {
		t.Fatalf("OldestPendingAge = %v, want %v (the parked item's older CreatedAt must not win)", age, wantAge)
	}
}

func TestPendingDepthAndOldestAgeOnEmptyStore(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	depth, err := s.PendingDepth(ctx)
	if err != nil {
		t.Fatalf("PendingDepth: %v", err)
	}
	if depth != 0 {
		t.Fatalf("PendingDepth = %d, want 0", depth)
	}
	_, ok, err := s.OldestPendingAge(ctx, fixedNow)
	if err != nil {
		t.Fatalf("OldestPendingAge: %v", err)
	}
	if ok {
		t.Fatal("OldestPendingAge: ok = true on an empty store, want false")
	}
}
