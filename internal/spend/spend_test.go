package spend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
)

func openTestChecker(t *testing.T, cfg Config) (*Checker, *index.SQLite) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(dbPath)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	ds := disposition.NewStore(eng)
	return New(eng, ds, cfg), eng
}

func fixtureRow(id string, cost float64, occurredAt time.Time) index.SpendRow {
	return index.SpendRow{
		ID:           id,
		TaskClass:    "hard_conflict_reconciliation",
		Tier:         "judgment",
		Provider:     "test-provider",
		ModelVersion: "test-model@v1",
		CostUSD:      cost,
		OccurredAt:   occurredAt,
	}
}

// TestProjectMonthGoldenNumbers is the acc line's "fixture ledger ->
// golden projection numbers": a steady $2/day rate over the first 10 days
// of a known 30-day month projects to a hand-computed monthly figure.
func TestProjectMonthGoldenNumbers(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) // September: 30 days
	var rows []index.SpendRow
	for day := 0; day < 10; day++ {
		rows = append(rows, fixtureRow(
			fmt.Sprintf("row-%02d", day),
			2.0,
			base.AddDate(0, 0, day),
		))
	}
	// A row in a different month must not leak into this month's totals.
	rows = append(rows, fixtureRow("row-august", 999, time.Date(2026, 8, 31, 23, 0, 0, 0, time.UTC)))

	now := time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC) // day 10 of September
	proj := ProjectMonth(rows, 50.0, now)

	if got, want := proj.MonthToDateUSD, 20.0; got != want {
		t.Fatalf("MonthToDateUSD = %v, want %v", got, want)
	}
	if got, want := proj.DaysElapsed, 10; got != want {
		t.Fatalf("DaysElapsed = %d, want %d", got, want)
	}
	if got, want := proj.DaysInMonth, 30; got != want {
		t.Fatalf("DaysInMonth = %d, want %d", got, want)
	}
	// $20 over 10 days -> $2/day -> $60 over 30 days.
	if got, want := proj.ProjectedUSD, 60.0; got != want {
		t.Fatalf("ProjectedUSD = %v, want %v", got, want)
	}
	if got, want := proj.CeilingUSD, 50.0; got != want {
		t.Fatalf("CeilingUSD = %v, want %v", got, want)
	}

	daily := DailyTotals(rows)
	if got, want := daily["2026-09-05"], 2.0; got != want {
		t.Fatalf("DailyTotals[2026-09-05] = %v, want %v", got, want)
	}
	if got, want := daily["2026-08-31"], 999.0; got != want {
		t.Fatalf("DailyTotals[2026-08-31] = %v, want %v", got, want)
	}
}

// TestCheckAndRecordUnderCeilingRecords is the straightforward allow path:
// a call comfortably under the ceiling is recorded and no approval item
// is staged.
func TestCheckAndRecordUnderCeilingRecords(t *testing.T) {
	c, eng := openTestChecker(t, Config{MonthlyCeilingUSD: 50})
	defer func() { _ = eng.Close() }()
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	dec, err := c.CheckAndRecord(ctx, fixtureRow("r1", 10, now), now)
	if err != nil {
		t.Fatalf("CheckAndRecord: %v", err)
	}
	if !dec.Recorded {
		t.Fatalf("Recorded = false, want true (under ceiling)")
	}
	if dec.ItemID != "" {
		t.Fatalf("ItemID = %q, want empty for a recorded call", dec.ItemID)
	}
	if got, want := dec.MonthToDateUSD, 10.0; got != want {
		t.Fatalf("MonthToDateUSD = %v, want %v", got, want)
	}

	rows, err := eng.SpendRows(ctx)
	if err != nil {
		t.Fatalf("SpendRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ledger has %d row(s), want 1", len(rows))
	}
}

// TestCheckAndRecordOverCeilingStagesEffectItemAndDoesNotRun is the acc
// line's "a judgment call that would cross the ceiling creates one
// approval item and does not run."
func TestCheckAndRecordOverCeilingStagesEffectItemAndDoesNotRun(t *testing.T) {
	c, eng := openTestChecker(t, Config{MonthlyCeilingUSD: 50})
	defer func() { _ = eng.Close() }()
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	// Seed $45 already spent this month.
	if _, err := c.CheckAndRecord(ctx, fixtureRow("seed", 45, now), now); err != nil {
		t.Fatalf("seed CheckAndRecord: %v", err)
	}

	// A further $10 call would cross the $50 ceiling ($45 + $10 = $55).
	dec, err := c.CheckAndRecord(ctx, fixtureRow("blocked", 10, now), now)
	if err != nil {
		t.Fatalf("CheckAndRecord: %v", err)
	}
	if dec.Recorded {
		t.Fatalf("Recorded = true, want false (would cross the ceiling)")
	}
	if dec.ItemID == "" {
		t.Fatalf("ItemID is empty, want a staged KindEffect item id")
	}
	if got, want := dec.MonthToDateUSD, 45.0; got != want {
		t.Fatalf("MonthToDateUSD = %v, want %v (pre-call total, the call itself did not run)", got, want)
	}

	// "does not run": the ledger must not contain the blocked row.
	rows, err := eng.SpendRows(ctx)
	if err != nil {
		t.Fatalf("SpendRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ledger has %d row(s), want 1 (only the seed call)", len(rows))
	}

	ds := disposition.NewStore(eng)
	item, err := ds.Get(ctx, dec.ItemID)
	if err != nil {
		t.Fatalf("Get staged item: %v", err)
	}
	if item.Kind != disposition.KindEffect {
		t.Fatalf("staged item Kind = %q, want %q", item.Kind, disposition.KindEffect)
	}
	var payload EffectPayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Row.ID != "blocked" || payload.Row.CostUSD != 10 {
		t.Fatalf("payload.Row = %+v, want the blocked $10 row", payload.Row)
	}
	if payload.CeilingUSD != 50 {
		t.Fatalf("payload.CeilingUSD = %v, want 50", payload.CeilingUSD)
	}

	// "accepting the item runs it": the same row is now recorded.
	disposeRes, err := ds.Dispose(ctx, item.ID, disposition.VerdictAccept, nil, "", "human:tester", "", now)
	if err != nil {
		t.Fatalf("Dispose: %v", err)
	}
	if err := c.ApplyDisposedEffect(ctx, disposeRes.Item); err != nil {
		t.Fatalf("ApplyDisposedEffect: %v", err)
	}
	rows, err = eng.SpendRows(ctx)
	if err != nil {
		t.Fatalf("SpendRows after accept: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ledger has %d row(s) after accept, want 2 (seed + the now-approved blocked call)", len(rows))
	}
	found := false
	for _, r := range rows {
		if r.ID == "blocked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("blocked row missing from the ledger after ApplyDisposedEffect: %+v", rows)
	}
}

// TestCheckAndRecordConcurrentCallsCannotOvershoot is the acc line's
// property test: N goroutines each try to record a fixed-cost call
// against a ceiling that only admits a known number of them. Run with
// -race; the recorded total must never exceed the ceiling, and exactly
// the expected count must have been recorded.
func TestCheckAndRecordConcurrentCallsCannotOvershoot(t *testing.T) {
	const (
		ceiling = 50.0
		perCall = 10.0
		callers = 20 // only 5 of these can possibly fit under the ceiling
		wantOK  = 5
	)
	c, eng := openTestChecker(t, Config{MonthlyCeilingUSD: ceiling})
	defer func() { _ = eng.Close() }()
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	var mu sync.Mutex
	recorded := 0
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			row := fixtureRow(fmt.Sprintf("row-%d", i), perCall, now)
			dec, err := c.CheckAndRecord(ctx, row, now)
			if err != nil {
				t.Errorf("CheckAndRecord goroutine %d: %v", i, err)
				return
			}
			if dec.Recorded {
				mu.Lock()
				recorded++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if recorded != wantOK {
		t.Fatalf("recorded = %d goroutines, want exactly %d (floor(%v/%v))", recorded, wantOK, ceiling, perCall)
	}
	rows, err := eng.SpendRows(ctx)
	if err != nil {
		t.Fatalf("SpendRows: %v", err)
	}
	var total float64
	for _, r := range rows {
		total += r.CostUSD
	}
	if len(rows) != wantOK {
		t.Fatalf("ledger has %d row(s), want %d", len(rows), wantOK)
	}
	if total > ceiling {
		t.Fatalf("recorded total %v overshoots ceiling %v", total, ceiling)
	}
}

func TestApplyDisposedEffectRejectsUndisposedItem(t *testing.T) {
	c, eng := openTestChecker(t, DefaultConfig())
	defer func() { _ = eng.Close() }()
	item := disposition.Item{ID: "e1", Kind: disposition.KindEffect, State: disposition.StatePending}
	if err := c.ApplyDisposedEffect(context.Background(), item); err == nil {
		t.Fatal("expected an error for a pending (undisposed) item")
	}
}

func TestApplyDisposedEffectRejectsWrongKind(t *testing.T) {
	c, eng := openTestChecker(t, DefaultConfig())
	defer func() { _ = eng.Close() }()
	item := disposition.Item{
		ID: "r1", Kind: disposition.KindReconcile,
		State: disposition.StateDisposed, Verdict: disposition.VerdictAccept,
	}
	if err := c.ApplyDisposedEffect(context.Background(), item); err == nil {
		t.Fatal("expected an error for a non-effect kind")
	}
}
