package contractstest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// LedgerHarness is what RunLedgerSuite needs from an implementation under
// test: the ledger itself plus two read-only inspectors.
type LedgerHarness struct {
	Ledger contracts.OperationLedger
	// Used reports committed usage for (account, period, metric).
	Used func(account, period, metric string) int64
	// Get returns a copy of one row.
	Get func(ctx context.Context, id string) (contracts.OperationRecord, error)
}

// LedgerFactory builds a fresh, empty ledger reading time from now.
type LedgerFactory func(now func() time.Time) LedgerHarness

// ReferenceLedger is the factory for MemLedger.
func ReferenceLedger(now func() time.Time) LedgerHarness {
	m := NewMemLedger(now)
	return LedgerHarness{Ledger: m, Used: m.Used, Get: m.Get}
}

const (
	suiteAccount = "acc-1"
	suiteBrain   = "brain-1"
	suitePeriod  = "2026-09"
	suiteLease   = 5 * time.Minute
)

type rq struct {
	key, fp, source, brain string
	writes, tokens         int64
	writesLimit, tokLimit  int64
}

func (r rq) request() contracts.ReserveRequest {
	brain, source := r.brain, r.source
	if brain == "" {
		brain = suiteBrain
	}
	if source == "" {
		source = "gateway.remember"
	}
	wl, tl := r.writesLimit, r.tokLimit
	if wl == 0 {
		wl = 1000
	}
	if tl == 0 {
		tl = 1000000
	}
	return contracts.ReserveRequest{
		AccountID: suiteAccount, BrainID: brain, ClientKey: r.key, Fingerprint: r.fp, QuotaPeriod: suitePeriod, Source: source, LeaseFor: suiteLease,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: r.writes, Limit: wl}, {Metric: "input_tokens", Units: r.tokens, Limit: tl}},
	}
}

func mustReserve(t *testing.T, h LedgerHarness, r rq) contracts.OperationRecord {
	t.Helper()
	rec, err := h.Ledger.Reserve(context.Background(), r.request())
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	return rec
}

func committed(ref string) contracts.Evidence {
	return contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: ref}
}

// writeCanonical is the complete, well-behaved writer protocol: commit
// section, EnterCanonical inside it, canonical write, leave, Finalize.
func writeCanonical(t *testing.T, h LedgerHarness, fence contracts.BrainFence, canon *MemCanonical, rec contracts.OperationRecord) string {
	t.Helper()
	ctx := context.Background()
	leave, err := fence.EnterCommit(ctx, rec.BrainID)
	if err != nil {
		t.Fatalf("enter commit: %v", err)
	}
	if _, err := h.Ledger.EnterCanonical(ctx, rec.ID); err != nil {
		leave()
		t.Fatalf("enter canonical: %v", err)
	}
	ref := canon.Commit(rec.BrainID, rec.ID)
	leave()
	if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed(ref)); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	return ref
}

func expectErr(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: err=%v, want %v", what, err, want)
	}
}

// RunLedgerSuite is the executable specification of contracts.OperationLedger.
// Each subtest names the rule it pins; the quiescence group reproduces the
// paused-writer / expired-reservation / absence-check / late-commit
// interleavings deterministically.
func RunLedgerSuite(t *testing.T, factory LedgerFactory) {
	ctx := context.Background()
	newHarness := func() (LedgerHarness, *Clock) {
		c := NewClock()
		return factory(c.Now), c
	}

	t.Run("reserve_is_all_or_nothing_across_counters", func(t *testing.T) {
		h, _ := newHarness()
		mustReserve(t, h, rq{writes: 1, tokens: 8, writesLimit: 2, tokLimit: 10})
		_, err := h.Ledger.Reserve(ctx, rq{writes: 1, tokens: 8, writesLimit: 2, tokLimit: 10}.request())
		expectErr(t, "second op past the token limit", err, contracts.ErrOperationLimitExceeded)
		// If the refused op had leaked its write hold, this would exceed writes=2.
		mustReserve(t, h, rq{writes: 1, tokens: 2, writesLimit: 2, tokLimit: 10})
	})

	t.Run("finalize_applies_every_counter_once", func(t *testing.T) {
		h, _ := newHarness()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 12})
		ev := committed("commit-1")
		for i := 0; i < 3; i++ {
			if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, ev); err != nil {
				t.Fatalf("finalize #%d: %v", i, err)
			}
		}
		if w, tok := h.Used(suiteAccount, suitePeriod, "writes"), h.Used(suiteAccount, suitePeriod, "input_tokens"); w != 1 || tok != 12 {
			t.Fatalf("used writes=%d tokens=%d after repeated finalize, want 1 and 12", w, tok)
		}
		_, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed("commit-2"))
		expectErr(t, "same phase, different evidence", err, contracts.ErrOperationConflict)
		_, err = h.Ledger.Finalize(ctx, rec.ID, contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt})
		expectErr(t, "release after commit", err, contracts.ErrOperationConflict)
	})

	t.Run("release_applies_no_counter_and_frees_capacity", func(t *testing.T) {
		h, _ := newHarness()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 5, writesLimit: 1})
		if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt}); err != nil {
			t.Fatal(err)
		}
		if h.Used(suiteAccount, suitePeriod, "writes") != 0 || h.Used(suiteAccount, suitePeriod, "input_tokens") != 0 {
			t.Fatal("a released operation charged a counter")
		}
		mustReserve(t, h, rq{writes: 1, tokens: 5, writesLimit: 1})
	})

	t.Run("concurrent_reserves_admit_exactly_the_limit", func(t *testing.T) {
		h, _ := newHarness()
		var ok, refused atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := h.Ledger.Reserve(ctx, rq{writes: 1, tokens: 1, writesLimit: 60}.request())
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, contracts.ErrOperationLimitExceeded):
					refused.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 60 || refused.Load() != 40 {
			t.Fatalf("admitted %d refused %d, want 60 and 40", ok.Load(), refused.Load())
		}
	})

	t.Run("concurrent_finalize_applies_once", func(t *testing.T) {
		h, _ := newHarness()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 7})
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed("commit-1")); err != nil {
					t.Errorf("finalize: %v", err)
				}
			}()
		}
		wg.Wait()
		if w, tok := h.Used(suiteAccount, suitePeriod, "writes"), h.Used(suiteAccount, suitePeriod, "input_tokens"); w != 1 || tok != 7 {
			t.Fatalf("used writes=%d tokens=%d, want 1 and 7", w, tok)
		}
	})

	t.Run("reserve_never_releases_an_expired_row", func(t *testing.T) {
		// Regression for meter.Meter.Reserve, which releases every expired open
		// reservation as a side effect: that discards unknown canonical outcomes.
		h, clock := newHarness()
		mustReserve(t, h, rq{writes: 1, tokens: 1, writesLimit: 1})
		clock.Advance(time.Hour)
		_, err := h.Ledger.Reserve(ctx, rq{writes: 1, tokens: 1, writesLimit: 1}.request())
		expectErr(t, "reserve over an expired-but-unreconciled row", err, contracts.ErrOperationLimitExceeded)
	})

	t.Run("client_key_is_bound_to_payload_identity", func(t *testing.T) {
		h, _ := newHarness()
		first := mustReserve(t, h, rq{key: "k1", fp: "fp-fact-A", writes: 1, tokens: 4})
		_, err := h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-fact-A", writes: 1, tokens: 4}.request())
		expectErr(t, "retry while in progress", err, contracts.ErrOperationInProgress)
		// Equal token count and operation kind, different fact.
		_, err = h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-fact-B", writes: 1, tokens: 4}.request())
		expectErr(t, "changed content while in progress", err, contracts.ErrOperationKeyReuse)
		if _, err := h.Ledger.Finalize(ctx, first.ID, contracts.OperationCommitted, committed("commit-1")); err != nil {
			t.Fatal(err)
		}
		_, err = h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-fact-B", writes: 1, tokens: 4}.request())
		expectErr(t, "changed content after commit", err, contracts.ErrOperationKeyReuse)
		_, err = h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-fact-A", source: "gateway.forget", writes: 1, tokens: 4}.request())
		expectErr(t, "changed source after commit", err, contracts.ErrOperationKeyReuse)
		replay, err := h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-fact-A", writes: 1, tokens: 4}.request())
		if err != nil || replay.ID != first.ID || replay.Phase != contracts.OperationCommitted {
			t.Fatalf("replay = %+v, %v; want the original committed row", replay, err)
		}
		if h.Used(suiteAccount, suitePeriod, "writes") != 1 {
			t.Fatal("a replay charged again")
		}
		// A recomputed delta (tokenizer change between deploys) does not invalidate a retry.
		replay, err = h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-fact-A", writes: 1, tokens: 9}.request())
		if err != nil || replay.ID != first.ID {
			t.Fatalf("replay with recomputed deltas = %+v, %v", replay, err)
		}
	})

	t.Run("released_row_frees_its_key", func(t *testing.T) {
		h, _ := newHarness()
		first := mustReserve(t, h, rq{key: "k1", fp: "fp-A", writes: 1, tokens: 1})
		if _, err := h.Ledger.Finalize(ctx, first.ID, contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt}); err != nil {
			t.Fatal(err)
		}
		second := mustReserve(t, h, rq{key: "k1", fp: "fp-B", writes: 1, tokens: 1})
		if second.ID == first.ID {
			t.Fatal("released row was reused")
		}
	})

	t.Run("a_keyed_operation_requires_a_fingerprint", func(t *testing.T) {
		h, _ := newHarness()
		_, err := h.Ledger.Reserve(ctx, rq{key: "k1", writes: 1, tokens: 1}.request())
		expectErr(t, "key without fingerprint", err, contracts.ErrOperationInvalid)
	})

	t.Run("unkeyed_identical_writes_stay_distinct", func(t *testing.T) {
		h, _ := newHarness()
		a := mustReserve(t, h, rq{fp: "fp-same", writes: 1, tokens: 3})
		b := mustReserve(t, h, rq{fp: "fp-same", writes: 1, tokens: 3})
		if a.ID == b.ID {
			t.Fatal("two unkeyed operations shared an ID")
		}
	})

	t.Run("finalize_charges_the_original_quota_period", func(t *testing.T) {
		h, clock := newHarness()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 2})
		clock.Advance(40 * 24 * time.Hour) // window rolls over before finalize
		if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed("commit-1")); err != nil {
			t.Fatal(err)
		}
		if h.Used(suiteAccount, suitePeriod, "writes") != 1 || h.Used(suiteAccount, "2026-10", "writes") != 0 {
			t.Fatal("finalize charged a period other than the one reserved")
		}
	})

	t.Run("pending_review_holds_capacity_and_only_an_operator_exits", func(t *testing.T) {
		h, _ := newHarness()
		rec := mustReserve(t, h, rq{key: "k1", fp: "fp-A", writes: 1, tokens: 1, writesLimit: 1})
		if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationPendingReview, contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: "flush_failed"}); err != nil {
			t.Fatal(err)
		}
		_, err := h.Ledger.Reserve(ctx, rq{writes: 1, tokens: 1, writesLimit: 1}.request())
		expectErr(t, "capacity held by pending review", err, contracts.ErrOperationLimitExceeded)
		_, err = h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-A", writes: 1, tokens: 1, writesLimit: 1}.request())
		expectErr(t, "retry of a pending-review key", err, contracts.ErrOperationPendingReview)
		_, err = h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed("commit-1"))
		expectErr(t, "caller resolving pending review", err, contracts.ErrOperationTransition)
		if h.Used(suiteAccount, suitePeriod, "writes") != 0 {
			t.Fatal("pending review charged before an operator resolved it")
		}
		op := contracts.Evidence{Kind: contracts.EvidenceOperatorReview, Ref: "review-7"}
		if _, err := h.Ledger.ResolvePendingReview(ctx, rec.ID, contracts.OperationCommitted, op); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Ledger.ResolvePendingReview(ctx, rec.ID, contracts.OperationCommitted, op); err != nil {
			t.Fatalf("repeated operator resolution must be idempotent: %v", err)
		}
		_, err = h.Ledger.ResolvePendingReview(ctx, rec.ID, contracts.OperationReleased, op)
		expectErr(t, "conflicting second resolution", err, contracts.ErrOperationConflict)
		if h.Used(suiteAccount, suitePeriod, "writes") != 1 {
			t.Fatalf("operator commit charged %d writes, want 1", h.Used(suiteAccount, suitePeriod, "writes"))
		}
	})

	t.Run("caller_cannot_release_after_entering_canonical", func(t *testing.T) {
		h, _ := newHarness()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 1})
		first, err := h.Ledger.EnterCanonical(ctx, rec.ID)
		if err != nil || first.CanonicalEnteredAt.IsZero() {
			t.Fatalf("EnterCanonical = %+v, %v", first, err)
		}
		again, err := h.Ledger.EnterCanonical(ctx, rec.ID)
		if err != nil || !again.CanonicalEnteredAt.Equal(first.CanonicalEnteredAt) {
			t.Fatalf("EnterCanonical must be idempotent: %+v, %v", again, err)
		}
		_, err = h.Ledger.Finalize(ctx, rec.ID, contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt})
		expectErr(t, "release after canonical entry", err, contracts.ErrOperationEvidence)
	})

	t.Run("quiescence", func(t *testing.T) { runQuiescence(t, newHarness) })
}

func runQuiescence(t *testing.T, newHarness func() (LedgerHarness, *Clock)) {
	ctx := context.Background()
	expire := func(c *Clock) { c.Advance(suiteLease + time.Minute) }

	t.Run("paused_before_canonical_is_stopped_after_release", func(t *testing.T) {
		h, clock := newHarness()
		fence, canon := NewMemBrainFence(), NewMemCanonical()
		stale := mustReserve(t, h, rq{key: "k1", fp: "fp-A", writes: 1, tokens: 3}) // writer pauses here
		expire(clock)
		report, err := h.Ledger.ReconcilePending(ctx, canon.Checker(), fence)
		if err != nil || report.Released != 1 || report.Committed+report.PendingReview+report.Deferred != 0 {
			t.Fatalf("reconcile = %+v, %v; want exactly one release", report, err)
		}
		// The paused writer wakes and tries to proceed.
		leave, err := fence.EnterCommit(ctx, stale.BrainID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.Ledger.EnterCanonical(ctx, stale.ID)
		leave()
		expectErr(t, "late writer entering canonical", err, contracts.ErrOperationNotReserved)
		if canon.Landed(stale.BrainID, stale.ID) || h.Used(suiteAccount, suitePeriod, "writes") != 0 {
			t.Fatal("stale writer changed state")
		}
		// The client retries the same key and content: one write, one charge.
		retry := mustReserve(t, h, rq{key: "k1", fp: "fp-A", writes: 1, tokens: 3})
		writeCanonical(t, h, fence, canon, retry)
		if h.Used(suiteAccount, suitePeriod, "writes") != 1 || h.Used(suiteAccount, suitePeriod, "input_tokens") != 3 {
			t.Fatal("retry did not consume exactly one write and its input units")
		}
		if canon.Landed(stale.BrainID, stale.ID) || !canon.Landed(retry.BrainID, retry.ID) {
			t.Fatal("canonical state must contain the retry only")
		}
	})

	t.Run("paused_inside_commit_section_defers_reconcile", func(t *testing.T) {
		h, clock := newHarness()
		fence, canon := NewMemBrainFence(), NewMemCanonical()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 3})
		leave, err := fence.EnterCommit(ctx, rec.BrainID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Ledger.EnterCanonical(ctx, rec.ID); err != nil { // writer paused before its canonical write
			t.Fatal(err)
		}
		expire(clock)
		cancelled, cancel := context.WithCancel(ctx)
		cancel() // the reconciler will not wait for the writer
		report, err := h.Ledger.ReconcilePending(cancelled, canon.Checker(), fence)
		if err != nil || report.Deferred != 1 || report.Released+report.Committed+report.PendingReview != 0 {
			t.Fatalf("reconcile = %+v, %v; want the row deferred", report, err)
		}
		// The writer resumes, writes, and finalizes: exactly one charge.
		ref := canon.Commit(rec.BrainID, rec.ID)
		leave()
		if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed(ref)); err != nil {
			t.Fatalf("resumed writer finalize: %v", err)
		}
		if h.Used(suiteAccount, suitePeriod, "writes") != 1 {
			t.Fatal("deferred reconcile lost or duplicated the charge")
		}
		if report, _ := h.Ledger.ReconcilePending(ctx, canon.Checker(), fence); report != (contracts.ReconcileReport{}) {
			t.Fatalf("second reconcile touched a finished row: %+v", report)
		}
	})

	t.Run("unfenced_reconcile_loses_a_charge_negative_control", func(t *testing.T) {
		// The same interleaving with a fence that excludes nothing — a
		// reconciler that trusts lease expiry alone. The writer is inside its
		// canonical write, the reconciler observes absence and releases, the
		// writer's write then lands with nothing to charge. This is the defect
		// the fence exists to prevent; the assertion documents that it is real.
		h, clock := newHarness()
		canon := NewMemCanonical()
		var fence contracts.BrainFence = NoFence{}
		rec := mustReserve(t, h, rq{writes: 1, tokens: 3})
		leave, _ := fence.EnterCommit(ctx, rec.BrainID)
		if _, err := h.Ledger.EnterCanonical(ctx, rec.ID); err != nil {
			t.Fatal(err)
		}
		expire(clock)
		report, err := h.Ledger.ReconcilePending(ctx, canon.Checker(), fence)
		if err != nil || report.Released != 1 {
			t.Fatalf("reconcile = %+v, %v; the unsafe fence should have let the release through", report, err)
		}
		ref := canon.Commit(rec.BrainID, rec.ID) // the late commit
		leave()
		_, ferr := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed(ref))
		expectErr(t, "late finalize after an unfenced release", ferr, contracts.ErrOperationConflict)
		if !canon.Landed(rec.BrainID, rec.ID) || h.Used(suiteAccount, suitePeriod, "writes") != 0 {
			t.Fatal("negative control failed to reproduce the uncharged landed write; the suite would not catch the defect")
		}
	})

	t.Run("crash_after_canonical_commit_reconciles_to_committed_once", func(t *testing.T) {
		h, clock := newHarness()
		canon := NewMemCanonical()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 3})
		crashed := NewMemBrainFence()
		leave, _ := crashed.EnterCommit(ctx, rec.BrainID)
		if _, err := h.Ledger.EnterCanonical(ctx, rec.ID); err != nil {
			t.Fatal(err)
		}
		ref := canon.Commit(rec.BrainID, rec.ID)
		_ = leave // the process died before Finalize; the restarted process has a fresh fence
		expire(clock)
		report, err := h.Ledger.ReconcilePending(ctx, canon.Checker(), NewMemBrainFence())
		if err != nil || report.Committed != 1 {
			t.Fatalf("reconcile = %+v, %v; want one commit", report, err)
		}
		if _, err := h.Ledger.Finalize(ctx, rec.ID, contracts.OperationCommitted, committed(ref)); err != nil {
			t.Fatalf("a resumed writer's identical finalize must be idempotent: %v", err)
		}
		if h.Used(suiteAccount, suitePeriod, "writes") != 1 || h.Used(suiteAccount, suitePeriod, "input_tokens") != 3 {
			t.Fatal("crash recovery did not charge exactly once")
		}
	})

	t.Run("pending_working_tree_write_is_unknown_not_absent", func(t *testing.T) {
		h, clock := newHarness()
		fence, canon := NewMemBrainFence(), NewMemCanonical()
		rec := mustReserve(t, h, rq{key: "k1", fp: "fp-A", writes: 1, tokens: 1, writesLimit: 1})
		if _, err := h.Ledger.EnterCanonical(ctx, rec.ID); err != nil {
			t.Fatal(err)
		}
		canon.WritePending(rec.BrainID, rec.ID) // written, Flush failed, still in the touched queue
		expire(clock)
		report, err := h.Ledger.ReconcilePending(ctx, canon.Checker(), fence)
		if err != nil || report.PendingReview != 1 || report.Released != 0 {
			t.Fatalf("reconcile = %+v, %v; want pending review, never a release", report, err)
		}
		_, err = h.Ledger.Reserve(ctx, rq{key: "k1", fp: "fp-A", writes: 1, tokens: 1, writesLimit: 1}.request())
		expectErr(t, "retry of the unresolved key", err, contracts.ErrOperationPendingReview)
	})

	t.Run("a_live_lease_is_untouched", func(t *testing.T) {
		h, clock := newHarness()
		fence, canon := NewMemBrainFence(), NewMemCanonical()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 1})
		clock.Advance(suiteLease - time.Second)
		if report, err := h.Ledger.ReconcilePending(ctx, canon.Checker(), fence); err != nil || report != (contracts.ReconcileReport{}) {
			t.Fatalf("reconcile = %+v, %v; want nothing", report, err)
		}
		if _, err := h.Ledger.EnterCanonical(ctx, rec.ID); err != nil {
			t.Fatalf("live writer stopped: %v", err)
		}
	})

	t.Run("a_checker_error_defers_and_never_guesses", func(t *testing.T) {
		h, clock := newHarness()
		fence := NewMemBrainFence()
		rec := mustReserve(t, h, rq{writes: 1, tokens: 1})
		expire(clock)
		report, err := h.Ledger.ReconcilePending(ctx, FailingChecker{}, fence)
		if err != nil || report.Deferred != 1 {
			t.Fatalf("reconcile = %+v, %v; want deferred", report, err)
		}
		if _, err := h.Ledger.EnterCanonical(ctx, rec.ID); err != nil {
			t.Fatalf("row must remain reserved after a checker error: %v", err)
		}
	})
}
