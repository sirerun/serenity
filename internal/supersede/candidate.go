package supersede

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/writer"
)

// PreviewClaimCandidate checks, before any decision is recorded, that accepting
// a claim_candidate item as actor would activate its pending claim (ADR 022,
// T24.16). It changes nothing.
func (w *Writer) PreviewClaimCandidate(ctx context.Context, item disposition.Item, actor string, now time.Time) error {
	payload, ok, err := ingest.ClaimCandidate(item)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("claim candidate: item %s is kind %q", item.ID, item.Kind)
	}
	_, err = w.candidateWriter().PlanActivateCandidate(ctx, payload.Claim, actor, now)
	return err
}

// ApplyAndCommitClaimCandidate activates the pending claim of an accepted
// claim_candidate item with the disposing human as actor, through the same
// receipt, lock and commit protocol as reconciliation.
func (w *Writer) ApplyAndCommitClaimCandidate(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	return w.applyAndCommitDecision(ctx, ds, item, now, acceptedClaimCandidate, func(a, _ domain.Claim) ([]writer.FileChange, error) {
		pending := a
		pending.State = domain.StatePending
		pending.Provenance.Actor = "machine"
		return w.candidateWriter().PlanActivateCandidate(ctx, pending, a.Provenance.Actor, now)
	})
}

func (w *Writer) candidateWriter() *ingest.Writer {
	iw := ingest.New(w.Queue, w.Fence, w.Shard, w.Config)
	iw.EntityType = w.EntityType
	return iw
}

func acceptedClaimCandidate(item disposition.Item, _ time.Time) (domain.Claim, domain.Claim, error) {
	payload, ok, err := ingest.ClaimCandidate(item)
	if err != nil {
		return domain.Claim{}, domain.Claim{}, err
	}
	if !ok || item.State != disposition.StateDisposed || item.Verdict != disposition.VerdictAccept {
		return domain.Claim{}, domain.Claim{}, fmt.Errorf("claim candidate: item %s needs a recorded accept", item.ID)
	}
	if !strings.HasPrefix(item.Actor, "human:") || item.Actor == "human:" {
		return domain.Claim{}, domain.Claim{}, fmt.Errorf("claim candidate: item %s was not accepted by a human", item.ID)
	}
	return ingest.ActivatedClaim(payload.Claim, item.Actor), domain.Claim{}, nil
}
