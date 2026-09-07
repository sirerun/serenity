// Package cron implements `serenity cron <job>` (ADR 006, plan T2.19): one
// scheduled job runs to completion and exits. It is idempotent and safe to
// run concurrently with the interactive CLI because every canonical write a
// job makes goes through the writer queue (ADR 004) and the dirty-tree
// guard inside the same process — the same invariant every other write
// path in this repo already relies on. In M4, `serenityd` embeds these same
// job functions on an internal ticker (ADR 006); `serenity cron` remains
// the manual operation and test entry point.
//
// Each job is registered here as a Job function that takes an injected
// Clock, so scheduled behavior is testable without real sleeps or
// wall-clock dependence — the same shape as internal/connector/file's
// Clock/WithClock and internal/index's Clock.
//
// Decay and SLO are still placeholder bodies until their owning tasks
// land (T2.15 queue SLOs — docs/plans/E2-m2-reconcile.md; T2.12 decay
// shipped its real logic in internal/reconcile/decay.go but, as of that
// task, had not yet wired this job to call it — a disclosed gap for
// whoever picks that up next, not this package's own scope). T2.19's own
// acceptance bar only required the runner scaffold, the CLI surface, and
// idempotent run-record semantics to be real and tested; each remaining
// placeholder only records that it ran (recordRun) so `serenity cron
// <job>` on a fixture exits 0 and a second run is a genuine no-op rather
// than an error. Sweep (T2.6, expiry sweeper) and Consolidate (T2.14) are
// both real: each opens the brain's derived index, runs its own package's
// logic (disposition.Sweep / internal/consolidate.Consolidator.Run), then
// records the run exactly like every other job. Whoever ships T2.15 (and
// whoever closes T2.12's wiring gap) replaces the matching function body
// in place — same name, same signature, same registry entry — and this
// package needs no other change.
package cron

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Clock abstracts time.Now so job runs are testable without a real clock.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock is the production clock every non-test caller passes to Run.
var RealClock Clock = realClock{}

// Job is one scheduled unit of work. root is the brain repo root (the same
// root every CLI command takes via -C); clock supplies "now" so job bodies
// never call time.Now() directly.
type Job func(ctx context.Context, root string, clock Clock) error

// registry is the fixed set of jobs ADR 006 names: sweep, consolidate,
// decay, slo. serenity cron accepts no other job name.
var registry = map[string]Job{
	"sweep":       Sweep,
	"consolidate": Consolidate,
	"decay":       Decay,
	"slo":         SLO,
}

// Names lists the registered job names in sorted order, so `serenity cron`'s
// error output and any help text are deterministic.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Run resolves name in the registry and runs it against root with clock.
// An unknown name returns an error naming every valid job.
func Run(ctx context.Context, name, root string, clock Clock) error {
	job, ok := registry[name]
	if !ok {
		return fmt.Errorf("cron: unknown job %q (valid: %s)", name, strings.Join(Names(), ", "))
	}
	return job(ctx, root, clock)
}
