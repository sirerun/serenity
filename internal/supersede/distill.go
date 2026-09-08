package supersede

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// DistillDecision records the typed human assertion and the exact optional prior
// shown before confirmation. Original machine evidence stays in Item.Payload.
type DistillDecision struct {
	Version int          `json:"version"`
	Claim   domain.Claim `json:"claim"`
	Prior   domain.Claim `json:"prior,omitzero"`
}

func humanAssertion(item disposition.Item, object, actor string, now time.Time) (domain.Claim, error) {
	observation, ok, err := disposition.ExtractionObservation(item)
	if err != nil {
		return domain.Claim{}, err
	}
	if !ok || observation.Confidence < 0 || observation.Confidence >= extract.DistillThreshold {
		return domain.Claim{}, fmt.Errorf("distill: item is not low-confidence extraction evidence")
	}
	object = strings.TrimSpace(object)
	if object == "" || !strings.HasPrefix(actor, "human:") || actor == "human:" || now.IsZero() {
		return domain.Claim{}, fmt.Errorf("distill: a nonempty typed assertion, human actor and decision time are required")
	}
	c := ingest.ClaimFromObservation(observation)
	c.Object = object
	c.ObjectKey = store.NormalizeKey(object)
	c.Confidence = 1
	c.Provenance = domain.Provenance{Actor: actor, ObservedAt: now.UTC(), Meta: map[string]string{"distill_item_id": item.ID}}
	c.SourceRef = actor
	c.ID = store.DerivedID(c.SubjectSlug, c.Predicate, c.ObjectKey, c.ValidFrom, "", store.DefaultIDWidth)
	return c, nil
}

// PreviewDistillAssertion preflights a typed assertion without disposing the item
// or changing canonical files. The UI must show Prior and obtain confirmation.
func (w *Writer) PreviewDistillAssertion(ctx context.Context, item disposition.Item, object, actor string, now time.Time) (DistillDecision, error) {
	if item.State == disposition.StateDisposed {
		return DistillDecision{}, fmt.Errorf("distill: item already has a terminal decision")
	}
	c, err := humanAssertion(item, object, actor, now)
	if err != nil {
		return DistillDecision{}, err
	}
	prior, err := w.distillPrior(ctx, c, now)
	if err != nil {
		return DistillDecision{}, err
	}
	if _, err := w.prepareDistillPublication(ctx, c, prior, now); err != nil {
		return DistillDecision{}, err
	}
	return DistillDecision{Version: 1, Claim: c, Prior: prior}, nil
}

// ApplyAndCommitDistill publishes only a recorded, explicitly confirmed human
// assertion. It shares reconciliation's crash journal and post-commit marker.
func (w *Writer) ApplyAndCommitDistill(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	decode := func(item disposition.Item, _ time.Time) (domain.Claim, domain.Claim, error) {
		if item.State != disposition.StateDisposed || item.Verdict != disposition.VerdictEditAccept {
			return domain.Claim{}, domain.Claim{}, fmt.Errorf("distill: explicit human edit-and-confirm decision required")
		}
		var decision DistillDecision
		if err := json.Unmarshal(item.EditedPayload, &decision); err != nil {
			return domain.Claim{}, domain.Claim{}, fmt.Errorf("distill: decode human decision: %w", err)
		}
		if decision.Version != 1 {
			return domain.Claim{}, domain.Claim{}, fmt.Errorf("distill: unsupported human decision version")
		}
		expected, err := humanAssertion(item, decision.Claim.Object, item.Actor, item.DisposedAt)
		if err != nil {
			return domain.Claim{}, domain.Claim{}, err
		}
		left, err := json.Marshal(expected)
		if err != nil {
			return domain.Claim{}, domain.Claim{}, err
		}
		right, err := json.Marshal(decision.Claim)
		if err != nil {
			return domain.Claim{}, domain.Claim{}, err
		}
		if !bytes.Equal(left, right) {
			return domain.Claim{}, domain.Claim{}, fmt.Errorf("distill: human assertion does not match recorded actor/evidence/time")
		}
		return expected, decision.Prior, nil
	}
	return w.applyAndCommitDecision(ctx, ds, item, now, decode, func(c, b domain.Claim) ([]writer.FileChange, error) {
		return w.prepareDistillPublication(ctx, c, b, now)
	})
}

func (w *Writer) distillPrior(ctx context.Context, c domain.Claim, now time.Time) (domain.Claim, error) {
	iw := ingest.New(w.Queue, w.Fence, w.Shard, w.Config)
	active, err := iw.CanonicalActiveClaims(ctx, c.SubjectSlug, now)
	if err != nil {
		return domain.Claim{}, err
	}
	var prior domain.Claim
	for _, candidate := range reconcile.Candidates(c, active) {
		detection := reconcile.Detect(c, []domain.Claim{candidate})
		if detection.Verdict != reconcile.VerdictConflict && detection.Verdict != reconcile.VerdictWindowClose {
			continue
		}
		if prior.ID != "" {
			return domain.Claim{}, fmt.Errorf("distill: multiple conflicting canonical claims require separate reconciliation")
		}
		prior = candidate
	}
	return prior, nil
}

func (w *Writer) prepareDistillPublication(ctx context.Context, c, prior domain.Claim, now time.Time) ([]writer.FileChange, error) {
	current, err := w.distillPrior(ctx, c, now)
	if err != nil {
		return nil, err
	}
	left, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	right, err := json.Marshal(prior)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(left, right) {
		return nil, fmt.Errorf("distill: canonical conflict changed since confirmation; preserve the recorded decision and resolve the target before retry")
	}
	if prior.ID != "" {
		return w.preparePublication(c, prior)
	}
	iw := ingest.New(w.Queue, w.Fence, w.Shard, w.Config)
	iw.EntityType = w.EntityType
	return iw.PlanHumanClaim(ctx, c)
}
