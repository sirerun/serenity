package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract"
)

// StageDistill retains sub-threshold observations without publishing claims.
// Repeated attempts preserve the first immutable payload and all review state.
func (w *Writer) StageDistill(ctx context.Context, ds *disposition.Store, observations []domain.Observation, now time.Time) (created, existing int, err error) {
	for _, o := range observations {
		if !safePart(o.SubjectSlug) || !safePart(o.Predicate) || o.Object == "" || math.IsNaN(o.Confidence) || o.Confidence < 0 || o.Confidence >= extract.DistillThreshold || o.SourceSHA256 == "" || o.Span == "" || o.Model == "" {
			return 0, 0, fmt.Errorf("extract distill: invalid low-confidence observation")
		}
		if _, ok := w.Config.Families[o.Predicate]; !ok {
			return 0, 0, fmt.Errorf("extract distill: predicate outside configured vocabulary")
		}
	}
	for _, o := range observations {
		key, e := observationIdentity(ClaimFromObservation(o))
		if e != nil {
			return created, existing, e
		}
		raw, e := json.Marshal(disposition.ExtractionPayload{Origin: disposition.ExtractionOrigin, Observation: o})
		if e != nil {
			return created, existing, e
		}
		_, inserted, e := ds.CreateOnce(ctx, disposition.KindDistill, raw, "", key, now)
		if e != nil {
			return created, existing, e
		}
		if inserted {
			created++
		} else {
			existing++
		}
	}
	return created, existing, nil
}
