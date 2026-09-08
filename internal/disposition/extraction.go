package disposition

import (
	"encoding/json"
	"fmt"

	"github.com/sirerun/serenity/internal/domain"
)

const ExtractionOrigin = "extraction-observation-v1"

// ExtractionPayload retains the immutable machine observation, including its
// original confidence and exact source/model/span provenance. A later human
// assertion is a separate EditedPayload, never a rewrite of this evidence.
type ExtractionPayload struct {
	Origin      string             `json:"origin"`
	Observation domain.Observation `json:"observation"`
}

// ExtractionObservation distinguishes extraction evidence from capture text and
// other distill producers. A recognized but malformed payload fails explicitly.
func ExtractionObservation(item Item) (domain.Observation, bool, error) {
	if item.Kind != KindDistill {
		return domain.Observation{}, false, nil
	}
	var header struct {
		Origin string `json:"origin"`
	}
	if err := json.Unmarshal(item.Payload, &header); err != nil {
		return domain.Observation{}, false, fmt.Errorf("disposition: decode distill origin: %w", err)
	}
	if header.Origin != ExtractionOrigin {
		return domain.Observation{}, false, nil
	}
	var p ExtractionPayload
	if err := json.Unmarshal(item.Payload, &p); err != nil {
		return domain.Observation{}, true, fmt.Errorf("disposition: decode extraction observation: %w", err)
	}
	o := p.Observation
	if o.SubjectSlug == "" || o.Predicate == "" || o.Object == "" || o.SourceSHA256 == "" || o.Model == "" || o.Span == "" {
		return domain.Observation{}, true, fmt.Errorf("disposition: incomplete extraction evidence")
	}
	return o, true, nil
}
