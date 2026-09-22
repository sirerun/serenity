// Package operation implements the durable hosted mutation ledger.
package operation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type Ledger struct {
	Store *store.Store
	Clock func() time.Time
}

func (l *Ledger) now() time.Time {
	if l.Clock != nil {
		return l.Clock().UTC()
	}
	return time.Now().UTC()
}

func encodeDeltas(ds []contracts.OperationDelta) (string, error) {
	b, err := json.Marshal(ds)
	return string(b), err
}
func decodeDeltas(s string) ([]contracts.OperationDelta, error) {
	var d []contracts.OperationDelta
	err := json.Unmarshal([]byte(s), &d)
	return d, err
}

func scanRecord(row interface{ Scan(...any) error }) (contracts.OperationRecord, error) {
	var r contracts.OperationRecord
	var deltas, entered, lease, created, finalized, kind, ref string
	if err := row.Scan(&r.ID, &r.AccountID, &r.BrainID, &r.ClientKey, &r.Fingerprint, &deltas, &r.QuotaPeriod, &r.Phase, &r.Source, &kind, &ref, &entered, &lease, &created, &finalized); err != nil {
		return r, err
	}
	var err error
	if r.Deltas, err = decodeDeltas(deltas); err != nil {
		return r, err
	}
	r.Evidence = contracts.Evidence{Kind: contracts.EvidenceKind(kind), Ref: ref}
	if entered != "" {
		r.CanonicalEnteredAt, err = time.Parse(time.RFC3339Nano, entered)
		if err != nil {
			return r, err
		}
	}
	if r.LeaseExpiresAt, err = time.Parse(time.RFC3339Nano, lease); err != nil {
		return r, err
	}
	if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return r, err
	}
	if finalized != "" {
		r.FinalizedAt, err = time.Parse(time.RFC3339Nano, finalized)
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

const recordSelect = `SELECT id,account_id,brain_id,client_key,fingerprint,deltas_json,quota_period,phase,source,evidence_kind,evidence_ref,COALESCE(canonical_entered_at,''),lease_expires_at,created_at,COALESCE(finalized_at,'') FROM operations`

func (l *Ledger) Reserve(ctx context.Context, req contracts.ReserveRequest) (out contracts.OperationRecord, err error) {
	if err = req.Validate(); err != nil {
		return out, err
	}
	now := l.now()
	err = l.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if req.ClientKey != "" {
			row := tx.QueryRowContext(ctx, recordSelect+` WHERE account_id=? AND brain_id=? AND client_key=? AND phase<>'released' ORDER BY created_at LIMIT 1`, req.AccountID, req.BrainID, req.ClientKey)
			existing, e := scanRecord(row)
			if e == nil {
				if existing.Fingerprint != req.Fingerprint || existing.Source != req.Source {
					return contracts.ErrOperationKeyReuse
				}
				switch existing.Phase {
				case contracts.OperationCommitted:
					out = existing
					return nil
				case contracts.OperationReserved:
					return contracts.ErrOperationInProgress
				default:
					return contracts.ErrOperationPendingReview
				}
			}
			if !errors.Is(e, sql.ErrNoRows) {
				return e
			}
		}
		for _, d := range req.Deltas {
			var used, held int64
			if e := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(CASE WHEN phase='committed' THEN json_extract(value,'$.units') ELSE 0 END),0), COALESCE(sum(CASE WHEN phase IN ('reserved','pending_review') THEN json_extract(value,'$.units') ELSE 0 END),0) FROM operations, json_each(deltas_json) WHERE account_id=? AND quota_period=? AND phase IN ('committed','reserved','pending_review') AND json_extract(value,'$.metric')=?`, req.AccountID, req.QuotaPeriod, d.Metric).Scan(&used, &held); e != nil {
				return e
			}
			if d.Units > d.Limit || used+held > d.Limit-d.Units {
				return fmt.Errorf("%w: %s", contracts.ErrOperationLimitExceeded, d.Metric)
			}
		}
		jsonDeltas, e := encodeDeltas(func() []contracts.OperationDelta {
			x := make([]contracts.OperationDelta, len(req.Deltas))
			for i, d := range req.Deltas {
				x[i] = contracts.OperationDelta{Metric: d.Metric, Units: d.Units}
			}
			return x
		}())
		if e != nil {
			return e
		}
		out = contracts.OperationRecord{ID: store.ID(), AccountID: req.AccountID, BrainID: req.BrainID, ClientKey: req.ClientKey, Fingerprint: req.Fingerprint, Deltas: func() []contracts.OperationDelta {
			x := make([]contracts.OperationDelta, len(req.Deltas))
			for i, d := range req.Deltas {
				x[i] = contracts.OperationDelta{Metric: d.Metric, Units: d.Units}
			}
			return x
		}(), QuotaPeriod: req.QuotaPeriod, Phase: contracts.OperationReserved, Source: req.Source, CreatedAt: now, LeaseExpiresAt: now.Add(req.LeaseFor)}
		_, e = tx.ExecContext(ctx, `INSERT INTO operations(id,account_id,brain_id,client_key,fingerprint,deltas_json,quota_period,phase,source,evidence_kind,evidence_ref,lease_expires_at,created_at) VALUES(?,?,?,?,?,?,?,'reserved',?,?,?, ?,?)`, out.ID, out.AccountID, out.BrainID, out.ClientKey, out.Fingerprint, jsonDeltas, out.QuotaPeriod, out.Source, "", "", store.Stamp(out.LeaseExpiresAt), store.Stamp(out.CreatedAt))
		return e
	})
	return out, err
}

func (l *Ledger) EnterCanonical(ctx context.Context, id string) (out contracts.OperationRecord, err error) {
	now := l.now()
	err = l.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var phase string
		e := tx.QueryRowContext(ctx, `SELECT phase FROM operations WHERE id=?`, id).Scan(&phase)
		if errors.Is(e, sql.ErrNoRows) {
			return contracts.ErrOperationNotFound
		}
		if e != nil {
			return e
		}
		if contracts.OperationPhase(phase) != contracts.OperationReserved {
			return contracts.ErrOperationNotReserved
		}
		if _, e = tx.ExecContext(ctx, `UPDATE operations SET canonical_entered_at=COALESCE(canonical_entered_at,?) WHERE id=? AND phase='reserved'`, store.Stamp(now), id); e != nil {
			return e
		}
		out, e = scanRecord(tx.QueryRowContext(ctx, recordSelect+` WHERE id=?`, id))
		return e
	})
	return out, err
}

func (l *Ledger) transition(ctx context.Context, tx *sql.Tx, id string, outcome contracts.OperationPhase, actor contracts.ResolutionActor, ev contracts.Evidence) (contracts.OperationRecord, error) {
	r, err := scanRecord(tx.QueryRowContext(ctx, recordSelect+` WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, contracts.ErrOperationNotFound
	}
	if err != nil {
		return r, err
	}
	if r.Phase == outcome && outcome != contracts.OperationReserved {
		if r.Evidence == ev {
			return r, nil
		}
		return r, contracts.ErrOperationConflict
	}
	if r.Phase.Final() {
		return r, contracts.ErrOperationConflict
	}
	if err = contracts.ValidateTransition(r.Phase, outcome, actor, ev); err != nil {
		return r, err
	}
	if outcome == contracts.OperationReleased && !r.CanonicalEnteredAt.IsZero() && actor == contracts.ActorCaller {
		return r, fmt.Errorf("%w: canonical writer was entered", contracts.ErrOperationEvidence)
	}
	now := store.Stamp(l.now())
	_, err = tx.ExecContext(ctx, `UPDATE operations SET phase=?,evidence_kind=?,evidence_ref=?,finalized_at=CASE WHEN ? IN ('committed','released') THEN ? ELSE finalized_at END WHERE id=? AND phase=?`, outcome, ev.Kind, ev.Ref, outcome, now, id, r.Phase)
	if err != nil {
		return r, err
	}
	if outcome == contracts.OperationCommitted {
		for _, d := range r.Deltas {
			if _, err = tx.ExecContext(ctx, `INSERT INTO usage_windows(account_id,window_key,metric,committed) VALUES(?,?,?,?) ON CONFLICT(account_id,window_key,metric) DO UPDATE SET committed=committed+excluded.committed`, r.AccountID, r.QuotaPeriod, d.Metric, d.Units); err != nil {
				return r, err
			}
		}
	}
	r.Phase = outcome
	r.Evidence = ev
	if outcome.Final() {
		r.FinalizedAt = l.now()
	}
	return r, nil
}

func (l *Ledger) Finalize(ctx context.Context, id string, outcome contracts.OperationPhase, ev contracts.Evidence) (out contracts.OperationRecord, err error) {
	err = l.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = l.transition(ctx, tx, id, outcome, contracts.ActorCaller, ev)
		return e
	})
	return
}
func (l *Ledger) ResolvePendingReview(ctx context.Context, id string, outcome contracts.OperationPhase, ev contracts.Evidence) (out contracts.OperationRecord, err error) {
	err = l.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var e error
		out, e = l.transition(ctx, tx, id, outcome, contracts.ActorOperator, ev)
		return e
	})
	return
}

func (l *Ledger) ReconcilePending(ctx context.Context, checker contracts.CanonicalChecker, fence contracts.BrainFence) (report contracts.ReconcileReport, err error) {
	now := store.Stamp(l.now())
	rows, e := l.Store.DB().QueryContext(ctx, recordSelect+` WHERE phase='reserved' AND lease_expires_at<=? ORDER BY id`, now)
	if e != nil {
		return report, e
	}
	defer func() { _ = rows.Close() }()
	var ids []struct{ id, brain string }
	for rows.Next() {
		r, e := scanRecord(rows)
		if e != nil {
			return report, e
		}
		ids = append(ids, struct{ id, brain string }{r.ID, r.BrainID})
	}
	if e = rows.Err(); e != nil {
		return report, e
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].id < ids[j].id })
	for _, item := range ids {
		release, e := fence.Fence(ctx, item.brain)
		if e != nil {
			if errors.Is(e, contracts.ErrBrainNotQuiescent) || errors.Is(e, context.DeadlineExceeded) {
				report.Deferred++
				continue
			}
			return report, e
		}
		var checked contracts.OperationRecord
		e = l.Store.Transaction(ctx, func(tx *sql.Tx) error {
			var err error
			checked, err = scanRecord(tx.QueryRowContext(ctx, recordSelect+` WHERE id=?`, item.id))
			return err
		})
		if e != nil {
			release()
			return report, e
		}
		verdict, e := checker.Check(ctx, checked)
		if e != nil {
			release()
			report.Deferred++
			continue
		}
		var outcome contracts.OperationPhase
		var ev contracts.Evidence
		switch verdict.Outcome {
		case contracts.CanonicalLanded:
			outcome = contracts.OperationCommitted
			ev = contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: verdict.Ref}
		case contracts.CanonicalAbsent:
			outcome = contracts.OperationReleased
			ev = contracts.Evidence{Kind: contracts.EvidenceAbsent, Ref: verdict.Ref}
		default:
			outcome = contracts.OperationPendingReview
			ev = contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: verdict.Ref}
		}
		e = l.Store.Transaction(ctx, func(tx *sql.Tx) error {
			_, x := l.transition(ctx, tx, item.id, outcome, contracts.ActorReconciler, ev)
			return x
		})
		release()
		if e != nil {
			return report, e
		}
		switch outcome {
		case contracts.OperationCommitted:
			report.Committed++
		case contracts.OperationReleased:
			report.Released++
		default:
			report.PendingReview++
		}
	}
	return report, nil
}
