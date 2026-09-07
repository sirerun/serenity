package direction

import (
	"context"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
)

// DefaultOrphanLookback is this task's own disclosed reading of "weekly"
// (RFC 0001 §10.4 names the cadence, not an exact duration -- the same
// gap briefing.DefaultMovedForwardLookback resolved for "daily" by taking
// the literal English word's duration).
const DefaultOrphanLookback = 7 * 24 * time.Hour

// Orphan is one ledger entry created within the lookback window that is
// not itself an active intent and carries no direct derives_from edge to
// one -- RFC 0001 §10.4: "weekly: activity with no derivation edge to an
// active intent -> briefing Drift section".
type Orphan struct {
	ID      string
	Kind    ledger.Kind
	Title   string
	Created time.Time
}

// DetectOrphans lists every entry in store and flags the ones created
// within [now-lookback, now] that are not themselves an active intent and
// carry no direct derives_from edge to one.
//
// Two reading calls this task makes explicit rather than silently:
//
//  1. "Direct" edge, not a transitive chain. The RFC's own words are "a
//     derivation edge" (singular), not "a derivation path"; dira's own
//     ledger.Entry.Validate constrains every edge's To field to a single
//     entry ref with no multi-hop traversal primitive anywhere in the
//     vendored package to reuse -- building one here would be new,
//     unreviewed graph semantics, not a literal reading of the clause.
//  2. Intents are excluded from the activity being checked, not just from
//     the targets an edge can point at. An intent is what "activity"
//     derives *from* in this design (T3.11's Decompose is the existing
//     precedent: a child intent gets exactly one derives_from edge back
//     to its parent) -- a root intent nobody has decomposed yet
//     legitimately has no derives_from edge of its own, and flagging it
//     would surface every top-level intent as "drift" on every render.
//
// Never mutates the ledger -- SweepRevisit (T3.10, internal/direction/
// revisit.go) is the existing precedent for "read accepted/active entries
// without ever touching the ledger", reused here. Unlike SweepRevisit,
// this function's signature carries no disposition.Store, no
// *writer.Queue and no RevisitBackend at all: there is no way for it to
// stage a disposition item even by accident, which is what plan T3.9's
// own acc line ("items are informational and create no disposition")
// asks for structurally, not just by convention -- the same shape T3.4's
// interview.Run used to make "drafts only" a property of the function
// signature rather than a promise about what the body happens to do.
func DetectOrphans(ctx context.Context, store ledger.Store, now time.Time, lookback time.Duration) ([]Orphan, error) {
	if lookback <= 0 {
		lookback = DefaultOrphanLookback
	}
	since := now.Add(-lookback)

	infos, err := store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("orphan: list: %w", err)
	}

	entries := make([]*ledger.Entry, 0, len(infos))
	activeIntents := make(map[string]bool)
	for _, info := range infos {
		entry, err := store.Get(ctx, info.ID)
		if err != nil {
			return nil, fmt.Errorf("orphan: read %s: %w", info.ID, err)
		}
		entries = append(entries, entry)
		if entry.Kind == ledger.KindIntent && entry.State == ledger.StateActive {
			activeIntents[entry.ID] = true
		}
	}

	var out []Orphan
	for _, entry := range entries {
		if entry.Kind == ledger.KindIntent {
			continue
		}
		created, err := time.Parse(time.RFC3339, entry.Created)
		if err != nil {
			return nil, fmt.Errorf("orphan: parse created for %s: %w", entry.ID, err)
		}
		if created.Before(since) || created.After(now) {
			continue
		}
		chained := false
		for _, edge := range entry.Edges {
			if edge.Type == ledger.EdgeDerivesFrom && activeIntents[edge.To] {
				chained = true
				break
			}
		}
		if !chained {
			out = append(out, Orphan{ID: entry.ID, Kind: entry.Kind, Title: entry.Title, Created: created})
		}
	}
	return out, nil
}
