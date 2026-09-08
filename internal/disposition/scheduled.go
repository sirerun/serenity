package disposition

import (
	"encoding/json"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

const DecayOrigin = "decayed-claim-v1"
const LexicalAliasOrigin = "lexical-alias-v1"

// DecayPayload retains the canonical evidence and the separate aged review
// signal. Accepting this review never upgrades the stored claim's confidence.
type DecayPayload struct {
	Origin            string       `json:"origin"`
	Text              string       `json:"text"`
	Claim             domain.Claim `json:"claim"`
	DecayedConfidence float64      `json:"decayed_confidence"`
	HalfLifeDays      int          `json:"half_life_days"`
	EvaluatedAt       time.Time    `json:"evaluated_at"`
}

// LexicalAliasPayload is a spelling heuristic, not an embedding score or merge.
type LexicalAliasPayload struct {
	Origin string `json:"origin"`
	A      string `json:"a"`
	B      string `json:"b"`
	Reason string `json:"reason"`
}

func DecayedClaim(item Item) (DecayPayload, bool) {
	var p DecayPayload
	if item.Kind != KindDistill || json.Unmarshal(item.Payload, &p) != nil || p.Origin != DecayOrigin {
		return p, false
	}
	return p, true
}
