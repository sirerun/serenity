package cron

import "context"

// Sweep runs the expiry-sweeper pass (plan T2.6, RFC §10.3): pending
// disposition items older than the per-kind threshold move to deferred,
// three cycles to parked, and new evidence resurfaces a parked item once.
// Placeholder until T2.6 lands (deps: [T2.1, T2.19]) — see the package doc
// comment for what that means and does not mean.
func Sweep(_ context.Context, root string, clock Clock) error {
	return recordRun(root, "sweep", clock.Now())
}

// Consolidate runs the nightly consolidate pass (plan T2.14): summary
// fences with freshness banners, shard-head refresh, and re-embedding of
// changed chunks. Placeholder until T2.14 lands — see the package doc
// comment.
func Consolidate(_ context.Context, root string, clock Clock) error {
	return recordRun(root, "consolidate", clock.Now())
}

// Decay runs the weekly decay and alias sweep (plan T2.12): read-time
// confidence decay, alias candidates, and low-confidence-to-distill.
// Placeholder until T2.12 lands — see the package doc comment.
func Decay(_ context.Context, root string, clock Clock) error {
	return recordRun(root, "decay", clock.Now())
}

// SLO computes queue SLOs for `serenity status` (plan T2.15): depth, p50
// age, time-to-dispose, and abandonment. Placeholder until T2.15 lands —
// see the package doc comment.
func SLO(_ context.Context, root string, clock Clock) error {
	return recordRun(root, "slo", clock.Now())
}
