package contractstest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// RunStagingSuite is the executable specification of contracts.StagingGate.
func RunStagingSuite(t *testing.T, factory StagingFactory) {
	ctx := context.Background()
	const maxStage = 100
	cfg := func(budgetStages int64, quota map[string]int64) StagingConfig {
		return StagingConfig{Budget: budgetStages * maxStage, MaxStage: maxStage, Headroom: 0, Quota: quota, Free: func() int64 { return 1 << 40 }}
	}
	reserve := func(t *testing.T, h StagingHarness, acct string) contracts.StageTicket {
		t.Helper()
		tk, err := h.Gate.ReserveStage(ctx, contracts.StageRequest{AccountID: acct, BrainID: "b-" + acct, OperationID: "op-" + acct})
		if err != nil {
			t.Fatalf("reserve stage for %s: %v", acct, err)
		}
		return tk
	}

	t.Run("one_byte_over_the_quota_is_refused_and_pending_growth_counts", func(t *testing.T) {
		h := factory(cfg(10, map[string]int64{"a": 1000}))
		h.SetPhysical("a", 998)
		first, second := reserve(t, h, "a"), reserve(t, h, "a")
		if err := h.Gate.AdmitMeasured(ctx, first, 2); err != nil {
			t.Fatalf("growth landing exactly on the quota must be admitted: %v", err)
		}
		if err := h.Gate.AdmitMeasured(ctx, second, 1); !errors.Is(err, contracts.ErrStorageQuotaExceeded) {
			t.Fatalf("second admit against the same remaining byte = %v, want ErrStorageQuotaExceeded", err)
		}
		if err := h.Gate.Release(ctx, first, contracts.StagePublished); err != nil {
			t.Fatal(err)
		}
		if h.Physical("a") != 1000 {
			t.Fatalf("physical = %d after publishing, want 1000", h.Physical("a"))
		}
		third := reserve(t, h, "a")
		if err := h.Gate.AdmitMeasured(ctx, third, 1); !errors.Is(err, contracts.ErrStorageQuotaExceeded) {
			t.Fatalf("admit past a full quota = %v, want ErrStorageQuotaExceeded", err)
		}
		h.SetPhysical("a", 999)
		fourth := reserve(t, h, "a")
		if err := h.Gate.AdmitMeasured(ctx, fourth, 2); !errors.Is(err, contracts.ErrStorageQuotaExceeded) {
			t.Fatalf("one byte over the quota = %v, want ErrStorageQuotaExceeded", err)
		}
	})

	t.Run("aggregate_staging_is_bounded_across_accounts", func(t *testing.T) {
		h := factory(cfg(3, map[string]int64{"a": 1 << 30, "b": 1 << 30, "c": 1 << 30, "d": 1 << 30}))
		held := []contracts.StageTicket{reserve(t, h, "a"), reserve(t, h, "b"), reserve(t, h, "c")}
		if _, err := h.Gate.ReserveStage(ctx, contracts.StageRequest{AccountID: "d", BrainID: "b-d", OperationID: "op-d"}); !errors.Is(err, contracts.ErrStagingBusy) {
			t.Fatalf("fourth stage across accounts = %v, want ErrStagingBusy", err)
		}
		if h.Outstanding() != 3*maxStage {
			t.Fatalf("outstanding = %d, want %d", h.Outstanding(), 3*maxStage)
		}
		if err := h.Gate.Release(ctx, held[0], contracts.StageDiscarded); err != nil {
			t.Fatal(err)
		}
		reserve(t, h, "d")
	})

	t.Run("concurrent_stages_never_exceed_the_budget", func(t *testing.T) {
		accounts := map[string]int64{}
		for i := 0; i < 10; i++ {
			accounts[fmt.Sprintf("acc-%d", i)] = 1 << 30
		}
		h := factory(cfg(7, accounts))
		var ok, busy atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := h.Gate.ReserveStage(ctx, contracts.StageRequest{AccountID: fmt.Sprintf("acc-%d", i%10), BrainID: "b", OperationID: fmt.Sprintf("op-%d", i)})
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, contracts.ErrStagingBusy):
					busy.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}(i)
		}
		wg.Wait()
		if ok.Load() != 7 || busy.Load() != 43 || h.Outstanding() != 7*maxStage {
			t.Fatalf("granted %d refused %d outstanding %d; want 7, 43 and %d", ok.Load(), busy.Load(), h.Outstanding(), 7*maxStage)
		}
	})

	t.Run("operator_headroom_counts_stages_that_are_reserved_but_not_yet_written", func(t *testing.T) {
		c := cfg(100, map[string]int64{"a": 1 << 30})
		c.Free = func() int64 { return 10 * maxStage } // the device reports the same free space throughout
		c.Headroom = 8 * maxStage
		h := factory(c)
		reserve(t, h, "a")
		reserve(t, h, "a")
		if _, err := h.Gate.ReserveStage(ctx, contracts.StageRequest{AccountID: "a", BrainID: "b", OperationID: "op-3"}); !errors.Is(err, contracts.ErrStagingBusy) {
			t.Fatalf("third stage = %v, want ErrStagingBusy: two unwritten ceilings already consume the headroom margin", err)
		}
	})

	t.Run("a_stage_past_its_ceiling_is_refused_and_returns_its_budget", func(t *testing.T) {
		h := factory(cfg(1, map[string]int64{"a": 1 << 30}))
		tk := reserve(t, h, "a")
		m := contracts.NewStageMeter(tk)
		if err := m.Add(maxStage); err != nil {
			t.Fatal(err)
		}
		if err := m.Add(1); !errors.Is(err, contracts.ErrStageCeilingExceeded) {
			t.Fatalf("stager write past the ceiling = %v, want ErrStageCeilingExceeded", err)
		}
		if err := h.Gate.AdmitMeasured(ctx, tk, maxStage+1); !errors.Is(err, contracts.ErrStageCeilingExceeded) {
			t.Fatalf("admit above the ceiling = %v, want ErrStageCeilingExceeded", err)
		}
		if err := h.Gate.Release(ctx, tk, contracts.StageDiscarded); err != nil {
			t.Fatal(err)
		}
		reserve(t, h, "a") // the single-stage budget is free again
		if h.Physical("a") != 0 {
			t.Fatal("a discarded stage changed physical usage")
		}
	})

	t.Run("discard_returns_admitted_growth_and_release_is_idempotent", func(t *testing.T) {
		h := factory(cfg(4, map[string]int64{"a": 100}))
		first := reserve(t, h, "a")
		if err := h.Gate.AdmitMeasured(ctx, first, 100); err != nil {
			t.Fatal(err)
		}
		second := reserve(t, h, "a")
		if err := h.Gate.AdmitMeasured(ctx, second, 1); !errors.Is(err, contracts.ErrStorageQuotaExceeded) {
			t.Fatalf("admit while 100 of 100 bytes are held = %v, want ErrStorageQuotaExceeded", err)
		}
		for i := 0; i < 2; i++ {
			if err := h.Gate.Release(ctx, first, contracts.StageDiscarded); err != nil {
				t.Fatalf("release #%d: %v", i, err)
			}
		}
		if err := h.Gate.AdmitMeasured(ctx, second, 100); err != nil {
			t.Fatalf("admit after the held growth was discarded = %v", err)
		}
		if err := h.Gate.AdmitMeasured(ctx, first, 1); !errors.Is(err, contracts.ErrStageTicketUnknown) {
			t.Fatalf("admit on a released ticket = %v, want ErrStageTicketUnknown", err)
		}
		if h.Outstanding() != maxStage {
			t.Fatalf("outstanding = %d after a double release, want %d", h.Outstanding(), maxStage)
		}
	})
}
