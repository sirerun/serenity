package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// CanonicalActiveClaims reads a committed subject snapshot for a human review
// preview. The returned claims obey the same lifecycle/tier rules as extraction.
func (w *Writer) CanonicalActiveClaims(ctx context.Context, subject string, now time.Time) ([]domain.Claim, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, snapshot, err := w.snapshotObservations([]domain.Observation{{SubjectSlug: subject, Predicate: "review"}})
	if err != nil {
		return nil, err
	}
	if err := writer.CheckSnapshot(w.Fence.Root, snapshot); err != nil {
		return nil, err
	}
	_, active, err := w.canonicalReviewClaims(snapshot, now)
	return active[subject], err
}

// PlanHumanClaim prepares a new explicit human assertion. This never upgrades a
// machine observation in place. The caller owns human approval, conflict review,
// a durable publication receipt and commit; this method changes no canonical file.
func (w *Writer) PlanHumanClaim(ctx context.Context, c domain.Claim) ([]writer.FileChange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(c.Provenance.Actor, "human:") || c.Provenance.Actor == "human:" || c.Confidence != 1 || c.Family != c.Predicate || c.State != domain.StateActive || c.Provenance.SourceSHA256 != "" || c.Provenance.Model != "" || c.Provenance.Span != "" || c.Object == "" || c.ID != store.DerivedID(c.SubjectSlug, c.Predicate, store.NormalizeKey(c.Object), c.ValidFrom, "", store.DefaultIDWidth) {
		return nil, fmt.Errorf("ingest: claim is not an explicit human assertion")
	}
	types, snapshot, err := w.snapshotObservations([]domain.Observation{{SubjectSlug: c.SubjectSlug, Predicate: c.Predicate}})
	if err != nil {
		return nil, err
	}
	return writer.PlanFiles(w.Fence.Root, snapshot, func(preview string) error {
		q := writer.NewQueue(nil)
		defer q.Close()
		fw, ss := store.NewFenceWriter(preview), store.NewShardStore(preview)
		fw.Vocabulary = w.Fence.Vocabulary
		ss.Vocabulary = w.Shard.Vocabulary
		ss.RolloverBytes = w.Shard.RolloverBytes
		planned := New(q, fw, ss, w.Config)
		planned.EntityType = func(slug string) string { return types[slug] }
		planned.pages = map[string]*store.EntityPage{}
		planned.changedPages = map[string]bool{}
		planned.shardIDs = map[string]map[string]bool{}
		written, err := planned.writeClaim(w.Config.TierOf(c.Family), c)
		if err != nil {
			return err
		}
		if !written {
			return fmt.Errorf("ingest: human assertion identity already exists; inspect its canonical state")
		}
		return planned.renderChangedPages()
	})
}
