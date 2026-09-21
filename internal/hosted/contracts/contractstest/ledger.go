package contractstest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// MemLedger is the reference contracts.OperationLedger. Its single mutex
// stands for one SQL transaction: every method that changes a row does so, and
// applies every counter delta, inside one critical section. It never holds
// that mutex while calling a fence or a checker, mirroring the rule that SQL
// transactions are not held across canonical I/O.
type MemLedger struct {
	mu   sync.Mutex
	now  func() time.Time
	seq  int
	rows map[string]*contracts.OperationRecord
	used map[usageKey]int64
	// FaultBeforeApply, when set, runs inside a transition after validation
	// and before any change; a non-nil error aborts the transition with
	// nothing changed. It lets a test prove all-or-nothing behaviour.
	FaultBeforeApply func() error
}

type usageKey struct{ account, period, metric string }

func NewMemLedger(now func() time.Time) *MemLedger {
	return &MemLedger{now: now, rows: map[string]*contracts.OperationRecord{}, used: map[usageKey]int64{}}
}

func clone(r *contracts.OperationRecord) contracts.OperationRecord {
	out := *r
	out.Deltas = append([]contracts.OperationDelta(nil), r.Deltas...)
	return out
}

// Used reports committed usage, the inspector the suite needs.
func (m *MemLedger) Used(account, period, metric string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.used[usageKey{account, period, metric}]
}

// Get returns a copy of a row.
func (m *MemLedger) Get(_ context.Context, id string) (contracts.OperationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return contracts.OperationRecord{}, contracts.ErrOperationNotFound
	}
	return clone(r), nil
}

func (m *MemLedger) Reserve(_ context.Context, req contracts.ReserveRequest) (contracts.OperationRecord, error) {
	if err := req.Validate(); err != nil {
		return contracts.OperationRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if req.ClientKey != "" {
		for _, r := range m.rows {
			if r.AccountID != req.AccountID || r.BrainID != req.BrainID || r.ClientKey != req.ClientKey || r.Phase == contracts.OperationReleased {
				continue
			}
			if r.Fingerprint != req.Fingerprint || r.Source != req.Source {
				return contracts.OperationRecord{}, contracts.ErrOperationKeyReuse
			}
			switch r.Phase {
			case contracts.OperationCommitted:
				return clone(r), nil
			case contracts.OperationReserved:
				return contracts.OperationRecord{}, contracts.ErrOperationInProgress
			default:
				return contracts.OperationRecord{}, contracts.ErrOperationPendingReview
			}
		}
	}
	for _, d := range req.Deltas {
		var held int64
		for _, r := range m.rows {
			if r.AccountID != req.AccountID || r.QuotaPeriod != req.QuotaPeriod || !r.Phase.HoldsCapacity() {
				continue
			}
			for _, rd := range r.Deltas {
				if rd.Metric == d.Metric {
					held += rd.Units
				}
			}
		}
		used := m.used[usageKey{req.AccountID, req.QuotaPeriod, d.Metric}]
		if d.Units > d.Limit || used+held > d.Limit-d.Units {
			return contracts.OperationRecord{}, fmt.Errorf("%w: %s", contracts.ErrOperationLimitExceeded, d.Metric)
		}
	}
	m.seq++
	rec := &contracts.OperationRecord{
		ID: fmt.Sprintf("op-%06d", m.seq), AccountID: req.AccountID, BrainID: req.BrainID, ClientKey: req.ClientKey,
		QuotaPeriod: req.QuotaPeriod, Phase: contracts.OperationReserved, Source: req.Source, Fingerprint: req.Fingerprint,
		CreatedAt: m.now(), LeaseExpiresAt: m.now().Add(req.LeaseFor),
	}
	for _, d := range req.Deltas {
		rec.Deltas = append(rec.Deltas, contracts.OperationDelta{Metric: d.Metric, Units: d.Units})
	}
	m.rows[rec.ID] = rec
	return clone(rec), nil
}

func (m *MemLedger) EnterCanonical(_ context.Context, id string) (contracts.OperationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return contracts.OperationRecord{}, contracts.ErrOperationNotFound
	}
	if r.Phase != contracts.OperationReserved {
		return contracts.OperationRecord{}, contracts.ErrOperationNotReserved
	}
	if r.CanonicalEnteredAt.IsZero() {
		r.CanonicalEnteredAt = m.now()
	}
	return clone(r), nil
}

// transition applies a validated phase change. The caller holds m.mu. Every
// counter delta moves inside this one call, so there is no state in which one
// counter has moved and another has not.
func (m *MemLedger) transition(r *contracts.OperationRecord, to contracts.OperationPhase, ev contracts.Evidence) error {
	if m.FaultBeforeApply != nil {
		if err := m.FaultBeforeApply(); err != nil {
			return err
		}
	}
	if to == contracts.OperationCommitted {
		for _, d := range r.Deltas {
			m.used[usageKey{r.AccountID, r.QuotaPeriod, d.Metric}] += d.Units
		}
	}
	r.Phase = to
	r.Evidence = ev
	if to.Final() {
		r.FinalizedAt = m.now()
	}
	return nil
}

func (m *MemLedger) Finalize(_ context.Context, id string, outcome contracts.OperationPhase, ev contracts.Evidence) (contracts.OperationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return contracts.OperationRecord{}, contracts.ErrOperationNotFound
	}
	if r.Phase == outcome && outcome != contracts.OperationReserved {
		if r.Evidence == ev {
			return clone(r), nil
		}
		return contracts.OperationRecord{}, contracts.ErrOperationConflict
	}
	if r.Phase.Final() {
		return contracts.OperationRecord{}, contracts.ErrOperationConflict
	}
	if err := contracts.ValidateTransition(r.Phase, outcome, contracts.ActorCaller, ev); err != nil {
		return contracts.OperationRecord{}, err
	}
	if outcome == contracts.OperationReleased && !r.CanonicalEnteredAt.IsZero() {
		return contracts.OperationRecord{}, fmt.Errorf("%w: canonical writer was entered; a caller cannot release", contracts.ErrOperationEvidence)
	}
	if err := m.transition(r, outcome, ev); err != nil {
		return contracts.OperationRecord{}, err
	}
	return clone(r), nil
}

func (m *MemLedger) ResolvePendingReview(_ context.Context, id string, outcome contracts.OperationPhase, ev contracts.Evidence) (contracts.OperationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return contracts.OperationRecord{}, contracts.ErrOperationNotFound
	}
	if r.Phase == outcome && outcome.Final() && r.Evidence.Kind == contracts.EvidenceOperatorReview {
		if r.Evidence == ev {
			return clone(r), nil
		}
		return contracts.OperationRecord{}, contracts.ErrOperationConflict
	}
	if r.Phase.Final() {
		return contracts.OperationRecord{}, contracts.ErrOperationConflict
	}
	if err := contracts.ValidateTransition(r.Phase, outcome, contracts.ActorOperator, ev); err != nil {
		return contracts.OperationRecord{}, err
	}
	if err := m.transition(r, outcome, ev); err != nil {
		return contracts.OperationRecord{}, err
	}
	return clone(r), nil
}

func (m *MemLedger) ReconcilePending(ctx context.Context, checker contracts.CanonicalChecker, fence contracts.BrainFence) (contracts.ReconcileReport, error) {
	var report contracts.ReconcileReport
	m.mu.Lock()
	var ids []string
	for id, r := range m.rows {
		if r.Phase == contracts.OperationReserved && !r.LeaseExpiresAt.After(m.now()) {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	sort.Strings(ids)
	for _, id := range ids {
		if err := ctx.Err(); err != nil && errors.Is(err, context.DeadlineExceeded) {
			return report, err
		}
		m.mu.Lock()
		snap := clone(m.rows[id])
		m.mu.Unlock()
		// Fence first, then inspect, then transition, all under the fence.
		release, err := fence.Fence(ctx, snap.BrainID)
		if err != nil {
			report.Deferred++
			continue
		}
		m.reconcileOne(ctx, id, checker, &report)
		release()
	}
	return report, nil
}

// reconcileOne runs with the brain fence held by the caller.
func (m *MemLedger) reconcileOne(ctx context.Context, id string, checker contracts.CanonicalChecker, report *contracts.ReconcileReport) {
	m.mu.Lock()
	r := m.rows[id]
	if r.Phase != contracts.OperationReserved {
		m.mu.Unlock()
		return // resolved by another actor while we waited for the fence
	}
	snap := clone(r)
	m.mu.Unlock()
	verdict, err := checker.Check(ctx, snap)
	if err != nil || verdict.Ref == "" {
		report.Deferred++
		return
	}
	var to contracts.OperationPhase
	var ev contracts.Evidence
	switch verdict.Outcome {
	case contracts.CanonicalLanded:
		to, ev = contracts.OperationCommitted, contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: verdict.Ref}
	case contracts.CanonicalAbsent:
		to, ev = contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceAbsent, Ref: verdict.Ref}
	default:
		to, ev = contracts.OperationPendingReview, contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: verdict.Ref}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Phase != contracts.OperationReserved {
		return
	}
	if err := contracts.ValidateTransition(r.Phase, to, contracts.ActorReconciler, ev); err != nil {
		report.Deferred++
		return
	}
	if err := m.transition(r, to, ev); err != nil {
		report.Deferred++
		return
	}
	switch to {
	case contracts.OperationCommitted:
		report.Committed++
	case contracts.OperationReleased:
		report.Released++
	default:
		report.PendingReview++
	}
}
