package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// MergeCandidatePayload is the JSON payload of a disposition.KindEntityMerge
// item (Engine.Resolve's VerdictAmbiguous outcome): both candidate
// entities plus the Decision that routed them here, so a human reviewing
// the item sees exactly why it was flagged without re-running Evaluate.
type MergeCandidatePayload struct {
	A      domain.Entity `json:"a"`
	B      domain.Entity `json:"b"`
	Stage  Stage         `json:"stage"`
	Score  float64       `json:"score"`
	Reason string        `json:"reason"`
}

// StageAmbiguous stages one disposition.KindEntityMerge item for a pair
// Evaluate routed to VerdictAmbiguous. Nothing is merged -- RFC §10.5's
// "ambiguous cases as low-priority disposition items" -- the item's
// eventual accept is a human deciding whether to call Merge themselves
// (out of this package's scope: T2.1's disposition wire has no built-in
// notion of "verdict accept calls back into an arbitrary Go function",
// the same reason T2.2/T2.7's reconcile items need their own
// internal/supersede write-through rather than disposition.Store applying
// anything itself).
func StageAmbiguous(ctx context.Context, s *disposition.Store, a, b domain.Entity, d Decision, now time.Time) (disposition.Item, error) {
	payload := MergeCandidatePayload{A: a, B: b, Stage: d.Stage, Score: d.Score, Reason: d.Reason}
	raw, err := json.Marshal(payload)
	if err != nil {
		return disposition.Item{}, fmt.Errorf("entities: stage ambiguous: marshal payload: %w", err)
	}
	item, err := s.Create(ctx, disposition.KindEntityMerge, raw, "", now)
	if err != nil {
		return disposition.Item{}, fmt.Errorf("entities: stage ambiguous: create disposition item: %w", err)
	}
	return item, nil
}

// Engine wires Evaluate's pure classification to the two write paths RFC
// §10.5's staged pipeline needs: Merge for VerdictAutoMerge, StageAmbiguous
// for VerdictAmbiguous -- the same "pure decision, then Engine dispatches
// verdict to write path" shape internal/reconcile.Engine.Process already
// established for T2.2's Detect. Construct with NewEngine; the zero value
// is not usable (every field but Thresholds is required).
type Engine struct {
	Queue    *writer.Queue
	Fence    *store.FenceWriter
	Shard    *store.ShardStore
	Store    *disposition.Store
	Embedder embed.Embedder
	// Thresholds defaults to DefaultThresholds() when left zero-valued.
	Thresholds Thresholds
}

// NewEngine builds an Engine over already-open dependencies. q, fw, ss,
// s, and embedder must be non-nil.
func NewEngine(q *writer.Queue, fw *store.FenceWriter, ss *store.ShardStore, s *disposition.Store, embedder embed.Embedder) *Engine {
	return &Engine{Queue: q, Fence: fw, Shard: ss, Store: s, Embedder: embedder, Thresholds: DefaultThresholds()}
}

func (e *Engine) thresholds() Thresholds {
	if e.Thresholds == (Thresholds{}) {
		return DefaultThresholds()
	}
	return e.Thresholds
}

// Resolve runs RFC §10.5's staged pipeline for one candidate pair:
// Evaluate classifies, then Resolve dispatches on the verdict --
// VerdictAutoMerge applies Merge immediately (returning the resulting
// MergeEvent), VerdictAmbiguous stages a disposition item (returning it;
// nothing is merged), VerdictNoMatch does nothing (both returns nil).
// Exactly one of the two return pointers is non-nil, and only for
// AutoMerge/Ambiguous respectively.
func (e *Engine) Resolve(ctx context.Context, a, b domain.Entity, now time.Time, actor string) (Decision, *MergeEvent, *disposition.Item, error) {
	d, err := Evaluate(ctx, e.Embedder, a, b, e.thresholds())
	if err != nil {
		return Decision{}, nil, nil, err
	}

	switch d.Verdict {
	case VerdictAutoMerge:
		ev, err := Merge(e.Queue, e.Fence, e.Shard, a, b, now, actor)
		if err != nil {
			return d, nil, nil, err
		}
		return d, &ev, nil, nil
	case VerdictAmbiguous:
		item, err := StageAmbiguous(ctx, e.Store, a, b, d, now)
		if err != nil {
			return d, nil, nil, err
		}
		return d, nil, &item, nil
	default:
		return d, nil, nil, nil
	}
}
