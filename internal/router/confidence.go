package router

// Confidence caps (RFC 0001 section 9): machine-assigned confidence is
// capped at 0.90 for local-cheap and 0.95 for judgment. Only human
// confirmation can set confidence above 0.95 -- no machine path in this
// package ever does, since NewConfidence is the only constructor and it
// always clamps a raw, machine-reported figure.
const (
	CapLocalCheap = 0.90
	CapJudgment   = 0.95
)

// Confidence is a clamped confidence value. It is only ever produced by
// NewConfidence, which is the type boundary RFC section 9 requires the
// cap be enforced at -- never by convention scattered across call sites.
type Confidence struct {
	// Value is the confidence after clamping to the tier's cap.
	Value float64
	// Clamped is true when the raw value exceeded the tier's cap and was
	// reduced to it.
	Clamped bool
}

func capFor(tier Tier) float64 {
	if tier == TierJudgment {
		return CapJudgment
	}
	return CapLocalCheap
}

// NewConfidence clamps a raw, machine-reported confidence to the given
// tier's cap. A value at or under the cap passes through unchanged and
// unclamped.
func NewConfidence(raw float64, tier Tier) Confidence {
	limit := capFor(tier)
	if raw > limit {
		return Confidence{Value: limit, Clamped: true}
	}
	return Confidence{Value: raw, Clamped: false}
}
