package disposition

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultExpiryThreshold is RFC 0001 §8.2's default: "An expired item
// (default 14 days, configurable per kind) drops to a low-priority aged
// state with visible aging and periodic resurfacing."
const DefaultExpiryThreshold = 14 * 24 * time.Hour

// MaxDeferCycles is RFC 0001 §8.2's default: "after N defer cycles
// (default 3, configurable) an item moves to parked." Read here as the
// Nth cycle itself being the terminal one -- the defer that would bring
// Item.DeferCount to MaxDeferCycles parks the item instead of deferring
// it again, rather than deferring a 3rd time and parking on a 4th check.
const MaxDeferCycles = 3

// Thresholds maps a Kind to its own expiry threshold -- RFC 0001 §8.2's
// "configurable per kind." A kind absent from the map, or a nil
// Thresholds, uses DefaultExpiryThreshold. No config surface exists yet
// for this in serenity.yml, so every caller today effectively gets
// DefaultExpiryThreshold for every kind; Thresholds exists so a future
// config-driven caller can override per kind without this package's
// signature changing.
type Thresholds map[Kind]time.Duration

func (t Thresholds) forKind(kind Kind) time.Duration {
	if d, ok := t[kind]; ok {
		return d
	}
	return DefaultExpiryThreshold
}

// SweepResult reports what one Sweep pass did, so a caller (internal/cron's
// sweep job) can report or log an outcome without re-querying the store.
type SweepResult struct {
	Deferred int
	Parked   int
}

// Sweep is the expiry sweeper's one pass (RFC 0001 §8.2, plan T2.6): every
// non-terminal item (pending or deferred -- parked and disposed items are
// left alone) whose last transition (Item.UpdatedAt) is at least its
// kind's threshold old advances one cycle. Item.DeferCount (already
// present on Item, reused unchanged -- see its own doc comment) counts
// cycles regardless of whether they came from Sweep or a human's explicit
// dispose(verdict=defer): the cycle that would bring it to MaxDeferCycles
// parks the item instead of deferring it again.
//
// Sweep never disposes anything and never touches Verdict/Note/Actor --
// expiry is auto-DEFER, never auto-decline (RFC 0001 §8.2): no claim
// state and no disposition verdict is ever set by this function, only
// State/DeferCount/UpdatedAt.
func Sweep(ctx context.Context, s *Store, thresholds Thresholds, now time.Time) (SweepResult, error) {
	items, err := s.List(ctx)
	if err != nil {
		return SweepResult{}, fmt.Errorf("disposition: sweep: %w", err)
	}
	now = now.UTC()
	var res SweepResult
	for _, listed := range items {
		item, changed, err := s.updateItem(ctx, listed.ID, func(item *Item) (bool, error) {
			if item.State != StatePending && item.State != StateDeferred {
				return false, nil
			}
			if now.Sub(item.UpdatedAt) < thresholds.forKind(item.Kind) {
				return false, nil
			}
			item.DeferCount++
			item.UpdatedAt = now
			if item.DeferCount >= MaxDeferCycles {
				item.State = StateParked
			} else {
				item.State = StateDeferred
			}
			return true, nil
		})
		if err != nil {
			return res, fmt.Errorf("disposition: sweep: write %s: %w", listed.ID, err)
		}
		if changed {
			if item.State == StateParked {
				res.Parked++
			} else {
				res.Deferred++
			}
		}
	}
	return res, nil
}

// ErrNotParked is returned by Resurface when id does not name an item
// currently in StateParked.
var ErrNotParked = errors.New("disposition: item is not parked")

// ErrAlreadyResurfaced is returned by Resurface when id has already been
// resurfaced once.
var ErrAlreadyResurfaced = errors.New("disposition: item already resurfaced once")

// Resurface moves a parked item back to pending, exactly once ever (RFC
// 0001 §8.2: "resurfaced only by explicit filter or by new evidence
// arriving on the same (subject, predicate). Nothing resurfaces
// forever."). Correlating "new evidence" with a parked item's (subject,
// predicate) is domain knowledge this package's opaque Payload cannot see
// on its own -- that correlation belongs to whichever caller owns the
// item's kind (T2.2's reconcile engine for KindReconcile, T2.13's entity
// resolution for merge candidates, both separate, later tasks). Resurface
// only enforces the mechanical guarantee once a caller has decided a
// parked item should come back: exactly one resurface, structurally
// enforced via Item.Resurfaced rather than trusted to callers.
func (s *Store) Resurface(ctx context.Context, id string, now time.Time) (Item, error) {
	item, _, err := s.updateItem(ctx, id, func(item *Item) (bool, error) {
		if item.Resurfaced {
			return false, fmt.Errorf("%w: %s", ErrAlreadyResurfaced, id)
		}
		if item.State != StateParked {
			return false, fmt.Errorf("%w: %s is %q", ErrNotParked, id, item.State)
		}
		item.State = StatePending
		item.Resurfaced = true
		item.UpdatedAt = now.UTC()
		return true, nil
	})
	return item, err
}

// PendingDepth counts items in a non-terminal state (pending or deferred).
// RFC 0001 §8.2: a parked item is "excluded from queue-depth/age SLO
// metrics" -- this is the minimal counting primitive that makes that
// clause true today, tested directly. T2.15 (Queue SLOs, deps: [T2.1,
// T2.6, T1.17]) owns the real `serenity status` surface and is expected to
// build on or extend this rather than reimplement the exclusion rule.
func (s *Store) PendingDepth(ctx context.Context) (int, error) {
	items, err := s.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("disposition: pending depth: %w", err)
	}
	n := 0
	for _, item := range items {
		if item.State == StatePending || item.State == StateDeferred {
			n++
		}
	}
	return n, nil
}

// OldestPendingAge returns the age (relative to now) of the oldest
// non-terminal item by Item.CreatedAt, excluding parked items per the same
// RFC clause PendingDepth documents. ok is false when there are no
// pending/deferred items to measure.
func (s *Store) OldestPendingAge(ctx context.Context, now time.Time) (age time.Duration, ok bool, err error) {
	items, err := s.List(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("disposition: oldest pending age: %w", err)
	}
	var oldest time.Time
	for _, item := range items {
		if item.State != StatePending && item.State != StateDeferred {
			continue
		}
		if !ok || item.CreatedAt.Before(oldest) {
			oldest = item.CreatedAt
			ok = true
		}
	}
	if !ok {
		return 0, false, nil
	}
	return now.UTC().Sub(oldest), true, nil
}
