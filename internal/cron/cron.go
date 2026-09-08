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
// Every registered job invokes its shipped behavior before recording success.
// Decay stages deduplicated human review without changing canonical confidence;
// SLO records the same queue snapshot status computes directly.
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
// decay, slo, plus the T3.10 revisit sweep. serenity cron accepts no other job name.
var registry = map[string]Job{
	"sweep":       Sweep,
	"revisit":     Revisit,
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
