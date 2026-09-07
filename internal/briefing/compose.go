package briefing

import (
	"context"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/queue"
	"github.com/sirerun/serenity/internal/spend"
)

// DefaultWordBudget is RFC 0001 §7's own number: "hard cap 800 words".
const DefaultWordBudget = 800

// DefaultMovedForwardLookback bounds "Moved forward" to items disposed
// within this long of now. RFC 0001 §7 gives no explicit lookback window
// for a daily briefing (unlike the fixed 800-word cap it does give); 24h
// is this task's own disclosed reading -- "daily" implies "since
// yesterday's briefing," and no per-briefing checkpoint exists yet to
// track the actual previous-render time (there is no "last briefing
// rendered at" record anywhere in this codebase to read instead). The
// same "no config surface yet, overridable via Go, zero means default"
// shape queue.Config and spend.Config already carry.
const DefaultMovedForwardLookback = 24 * time.Hour

// Config holds Compose's policy numbers.
type Config struct {
	WordBudget           int
	MovedForwardLookback time.Duration
	// OrphanLookback bounds "Drift" to activity created within this long
	// of now. Zero means direction.DefaultOrphanLookback (RFC 0001
	// §10.4's own "weekly").
	OrphanLookback time.Duration
}

// DefaultConfig returns RFC 0001 §7's own word cap and this task's own
// disclosed 24h Moved-forward lookback.
func DefaultConfig() Config {
	return Config{WordBudget: DefaultWordBudget, MovedForwardLookback: DefaultMovedForwardLookback, OrphanLookback: direction.DefaultOrphanLookback}
}

func (c Config) orDefault() Config {
	if c.WordBudget <= 0 {
		c.WordBudget = DefaultWordBudget
	}
	if c.OrphanLookback <= 0 {
		c.OrphanLookback = direction.DefaultOrphanLookback
	}
	if c.MovedForwardLookback <= 0 {
		c.MovedForwardLookback = DefaultMovedForwardLookback
	}
	return c
}

// Compose gathers the five fixed sections' real content from eng (the
// same single *index.SQLite handle `serenity status`'s runStatus already
// opens, internal/cli/status.go) and packs them against cfg's word
// budget (WordEstimator) -- "render the daily briefing" (RFC 0001 §7).
//
// Blocked sources queue.Compute's own Snapshot (T2.15, a literal dep,
// whose own doc comment names this package as the SLOs' second renderer
// after `serenity status`): one line per tripped alert (depth or p50
// age), empty when neither trips -- RFC 0001 §7's "queue hygiene is
// structural... alert in the briefing when p50 age > 3 days or depth >
// 50".
//
// Needs you sources disposition.Store.List filtered to State pending or
// deferred (excluding parked -- the same exclusion queue.Snapshot.Depth
// itself already applies, per PendingDepth's own doc comment): the queue
// a human still owes a verdict on.
//
// Moved forward sources the same List filtered to State disposed with
// DisposedAt within cfg.MovedForwardLookback of now: what actually got
// resolved recently (a human's or automation's verdict), not what is
// still outstanding.
//
// Watched sources spend.ProjectMonth over eng.SpendRows (T4.10, already
// shipped in this codebase; RFC 0001 §7: "projected-spend shown in ...
// the briefing before the ceiling trips") -- this is the first live
// wiring of T4.10's own disclosed gap ("the briefing's Watched section
// is untouched since internal/briefing (T2.17) has not shipped yet").
//
// Drift sources direction.DetectOrphans (T3.9, RFC 0001 §10.4 "weekly:
// activity with no derivation edge to an active intent -> briefing Drift
// section") over dirStore, one line per orphaned entry -- the first live
// wiring of T3.9's own detector, the same "package shipped standalone,
// wired the moment its own upstream section exists" shape T4.10's Watched
// carried until this package closed it. dirStore may be nil: no dira
// ledger store wired in yet at a given call site leaves Drift empty
// exactly as before, rather than erroring -- the same disclosed-gap
// tolerance Watched itself needed before T4.10 existed.
//
// T2.14 (consolidate)'s role here is establishing that the brain Compose
// renders over has already had its entity summary fences and shard heads
// refreshed -- RFC 0001 §7's own nightly sequence runs consolidate
// immediately before "render the daily briefing" -- not literal content
// Compose reads out of consolidate.Result (Pages/Embedded are two bare
// counts with no per-item detail a section line could render). This
// package's own tests build their fixture the same way
// internal/cli/status_test.go already does (runInit + providers.OpenIndex
// + seeded disposition/spend rows) rather than also driving a full
// consolidate.Run pass (git-committed EntityPage, writer.Queue, embedder)
// -- disclosed: Compose's golden test does not itself exercise the
// consolidate step, only the disposition/queue/spend state a real nightly
// run would leave behind by the time briefing rendering's turn comes.
func Compose(ctx context.Context, eng *index.SQLite, dirStore ledger.Store, cfg Config, now time.Time) (Briefing, error) {
	cfg = cfg.orDefault()
	ds := disposition.NewStore(eng)

	items, err := ds.List(ctx)
	if err != nil {
		return Briefing{}, fmt.Errorf("briefing: list disposition items: %w", err)
	}

	qcfg := queue.DefaultConfig()
	snap, err := queue.Compute(ctx, ds, qcfg, now)
	if err != nil {
		return Briefing{}, fmt.Errorf("briefing: compute queue SLOs: %w", err)
	}

	spendRows, err := eng.SpendRows(ctx)
	if err != nil {
		return Briefing{}, fmt.Errorf("briefing: read spend rows: %w", err)
	}

	var drift []Item
	if dirStore != nil {
		orphans, err := direction.DetectOrphans(ctx, dirStore, now, cfg.OrphanLookback)
		if err != nil {
			return Briefing{}, fmt.Errorf("briefing: detect orphans: %w", err)
		}
		drift = driftItems(orphans)
	}

	sections := []Section{
		{Name: SectionBlocked, Items: blockedItems(snap, qcfg)},
		{Name: SectionNeedsYou, Items: needsYouItems(items, now)},
		{Name: SectionMovedForward, Items: movedForwardItems(items, now, cfg.MovedForwardLookback)},
		{Name: SectionWatched, Items: watchedItems(spendRows, now)},
		{Name: SectionDrift, Items: drift},
	}
	return Pack(sections, cfg.WordBudget, WordEstimator), nil
}

func blockedItems(snap queue.Snapshot, cfg queue.Config) []Item {
	var out []Item
	if snap.DepthAlert {
		out = append(out, Item{Text: fmt.Sprintf(
			"queue depth %d exceeds the %d-item threshold", snap.Depth, cfg.DepthThreshold)})
	}
	if snap.AgeAlert {
		out = append(out, Item{Text: fmt.Sprintf(
			"queue p50 pending age %s exceeds the %s threshold",
			snap.P50Age.Round(time.Minute), cfg.AgeThreshold)})
	}
	return out
}

func needsYouItems(items []disposition.Item, now time.Time) []Item {
	var out []Item
	for _, it := range items {
		if it.State != disposition.StatePending && it.State != disposition.StateDeferred {
			continue
		}
		out = append(out, Item{Text: fmt.Sprintf(
			"%s %s (%s old)", it.Kind, it.ID, now.Sub(it.CreatedAt).Round(time.Minute))})
	}
	return out
}

func movedForwardItems(items []disposition.Item, now time.Time, lookback time.Duration) []Item {
	var out []Item
	for _, it := range items {
		if it.State != disposition.StateDisposed {
			continue
		}
		if now.Sub(it.DisposedAt) > lookback {
			continue
		}
		out = append(out, Item{Text: fmt.Sprintf(
			"%s %s -> %s (%s ago)", it.Kind, it.ID, it.Verdict, now.Sub(it.DisposedAt).Round(time.Minute))})
	}
	return out
}

func watchedItems(rows []index.SpendRow, now time.Time) []Item {
	proj := spend.ProjectMonth(rows, spend.DefaultConfig().MonthlyCeilingUSD, now)
	return []Item{{Text: fmt.Sprintf(
		"spend $%.2f month-to-date, projected $%.2f of $%.2f ceiling",
		proj.MonthToDateUSD, proj.ProjectedUSD, proj.CeilingUSD)}}
}

func driftItems(orphans []direction.Orphan) []Item {
	var out []Item
	for _, o := range orphans {
		out = append(out, Item{Text: fmt.Sprintf("%s %s %q has no derivation edge to an active intent", o.Kind, o.ID, o.Title)})
	}
	return out
}
