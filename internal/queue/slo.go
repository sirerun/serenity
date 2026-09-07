// Package queue computes the DISPOSITION queue's SLO metrics (RFC 0001
// §8.2, §10.3, §16, plan T2.15): queue depth, p50 (median) pending age,
// time-to-dispose, and abandonment. `serenity status` (internal/cli) and,
// later, the briefing (T2.17/E4) both render this package's Snapshot
// rather than recomputing it.
//
// RFC 0001 §7 preamble ("Queue hygiene is structural"): "queue depth and
// age carry SLOs (alert in the briefing when p50 age > 3 days or depth >
// 50)". Those two numbers -- DefaultDepthThreshold=50, DefaultAgeThreshold
// = 3 days -- are this package's defaults, kept overridable via Config the
// same way internal/store.ShardStore.RolloverBytes and internal/spend's
// MonthlyCeilingUSD are: zero-value Config falls back to DefaultConfig()
// rather than computing against a zero threshold.
package queue

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
)

// DefaultDepthThreshold is RFC 0001 §7's queue-depth SLO: "depth > 50".
const DefaultDepthThreshold = 50

// DefaultAgeThreshold is RFC 0001 §7's queue-age SLO: "p50 age > 3 days".
const DefaultAgeThreshold = 3 * 24 * time.Hour

// Config holds the two alert thresholds RFC 0001 §7 names. A zero Config
// (DepthThreshold == 0) is not a valid "always alert" setting -- Compute
// substitutes DefaultConfig() for it, mirroring ShardStore.RolloverBytes's
// own "zero means use the seeded default" convention.
type Config struct {
	DepthThreshold int
	AgeThreshold   time.Duration
}

// DefaultConfig returns RFC 0001 §7's own numbers. No config surface exists
// yet for overriding these in serenity.yml (same disclosed gap
// disposition.Thresholds already carries for per-kind expiry) -- every
// caller today effectively gets these defaults.
func DefaultConfig() Config {
	return Config{DepthThreshold: DefaultDepthThreshold, AgeThreshold: DefaultAgeThreshold}
}

func (c Config) orDefault() Config {
	if c.DepthThreshold <= 0 {
		c.DepthThreshold = DefaultDepthThreshold
	}
	if c.AgeThreshold <= 0 {
		c.AgeThreshold = DefaultAgeThreshold
	}
	return c
}

// Snapshot is one Compute call's result. Every *OK field is false when the
// underlying population is empty (no pending/deferred items for the age
// stats, no disposed items for time-to-dispose, no parked-or-disposed
// items for abandonment) -- callers must render an explicit "n/a", never a
// bare zero duration/ratio that would misreport "healthy" for "no data
// yet" (the same discipline internal/cli.printRebuildTiming already
// applies to "never rebuilt").
type Snapshot struct {
	// Depth is the queue-depth SLO's population: every item in
	// StatePending or StateDeferred. StateParked is excluded by RFC 0001
	// §8.2's own words ("excluded from queue-depth/age SLO metrics");
	// StateDisposed items have left the queue entirely. This is the exact
	// count disposition.Store.PendingDepth already computes -- Compute
	// calls it directly rather than re-deriving the exclusion rule, per
	// PendingDepth's own doc comment ("T2.15 ... is expected to build on
	// or extend this rather than reimplement the exclusion rule").
	Depth      int
	DepthAlert bool

	// P50Age is the median age (now - CreatedAt) across the same
	// pending/deferred population Depth counts. P50AgeOK is false when
	// Depth == 0 (no population to take a median of).
	P50Age   time.Duration
	P50AgeOK bool
	AgeAlert bool

	// TimeToDispose is the median (p50) time from CreatedAt to DisposedAt
	// across every StateDisposed item -- RFC 0001 §7's own metric name
	// ("disposition queue depth/age and time-to-dispose"). The RFC names
	// the metric but not a specific statistic; p50 is chosen here for
	// consistency with P50Age rather than a mean, which a single
	// slow-to-dispose outlier would distort. TimeToDisposeOK is false
	// when no item has ever been disposed.
	TimeToDispose   time.Duration
	TimeToDisposeOK bool

	// Abandonment is the fraction of items that left the queue via
	// StateParked (ran out its defer cycles with no human verdict) rather
	// than StateDisposed (a human actually decided it): parked /
	// (parked + disposed). RFC 0001 §7 names "abandonment" as a tracked
	// metric without defining its formula; this is the natural reading --
	// "abandoned" items are exactly the ones RFC 0001 §8.2 calls
	// terminal-but-reversible parking, the queue's failure mode.
	// AbandonmentOK is false when parked+disposed == 0 (nothing has ever
	// left the queue either way).
	Abandonment   float64
	AbandonmentOK bool
}

// Compute reads every disposition item and derives the RFC 0001 §7 queue
// SLOs against cfg (DefaultConfig() when cfg's zero value is passed).
func Compute(ctx context.Context, store *disposition.Store, cfg Config, now time.Time) (Snapshot, error) {
	cfg = cfg.orDefault()
	now = now.UTC()

	depth, err := store.PendingDepth(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("queue: compute: %w", err)
	}
	snap := Snapshot{Depth: depth, DepthAlert: depth > cfg.DepthThreshold}

	items, err := store.List(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("queue: compute: %w", err)
	}

	var pendingAges, disposeTimes []time.Duration
	var parked, disposed int
	for _, item := range items {
		switch item.State {
		case disposition.StatePending, disposition.StateDeferred:
			pendingAges = append(pendingAges, now.Sub(item.CreatedAt.UTC()))
		case disposition.StateParked:
			parked++
		case disposition.StateDisposed:
			disposed++
			disposeTimes = append(disposeTimes, item.DisposedAt.UTC().Sub(item.CreatedAt.UTC()))
		}
	}

	if p50, ok := median(pendingAges); ok {
		snap.P50Age, snap.P50AgeOK = p50, true
		snap.AgeAlert = p50 > cfg.AgeThreshold
	}
	if p50, ok := median(disposeTimes); ok {
		snap.TimeToDispose, snap.TimeToDisposeOK = p50, true
	}
	if total := parked + disposed; total > 0 {
		snap.Abandonment, snap.AbandonmentOK = float64(parked)/float64(total), true
	}

	return snap, nil
}

// median returns the standard median of ds (average of the two middle
// values on an even-length input), ok=false for an empty input. ds is
// copied before sorting so Compute's own iteration order is never
// mutated by a caller's slice reuse.
func median(ds []time.Duration) (time.Duration, bool) {
	if len(ds) == 0 {
		return 0, false
	}
	sorted := make([]time.Duration, len(ds))
	copy(sorted, ds)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid], true
	}
	return (sorted[mid-1] + sorted[mid]) / 2, true
}
