package check

// WireResult is check_plan's wire response shape (RFC 0001 §8.3), field
// names mirroring the RFC's own vocabulary (precept_id, why_not,
// revisit_if, matched_actions, spans) rather than inventing new ones.
// This is the single source of truth both `serenity check --json`
// (internal/cli/check.go) and DIRECTION v1's check_plan HTTP handler
// (internal/server/direction, T4.6) marshal -- neither package declares
// its own copy of these fields, so "check_plan wire verdicts equal
// `serenity check --json` on the same fixture" (T4.6's own acc line)
// holds by construction, not by two implementations happening to agree.
type WireResult struct {
	Status          string              `json:"status"`
	ConsideredCount int                 `json:"considered_count"`
	Constraints     []WireConstraint    `json:"constraints,omitempty"`
	Warnings        []WireWarning       `json:"warnings,omitempty"`
	MatchedActions  []WireMatchedAction `json:"matched_actions,omitempty"`
	Confidence      *float64            `json:"confidence,omitempty"`
}

// WireConstraint is one ConstraintVerdict's wire form.
type WireConstraint struct {
	PreceptID string `json:"precept_id"`
	Outcome   string `json:"outcome"`
	WhyNot    string `json:"why_not,omitempty"`
	RevisitIf string `json:"revisit_if,omitempty"`
}

// WireWarning is one QuestionWarning's wire form.
type WireWarning struct {
	PreceptID string `json:"precept_id"`
	Title     string `json:"title"`
	Action    string `json:"action"`
}

// WireSpan is one Span's wire form.
type WireSpan struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// WireMatchedAction is one MatchedAction's wire form.
type WireMatchedAction struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
	Span   WireSpan       `json:"span"`
}

// ToWire converts a Result (plus stage 2's optional MatchedActions/
// Confidence, present only when haveConfidence is true -- a structured
// --actions call never classifies, so it never has one) into WireResult.
// Every caller marshaling check_plan's verdict to JSON goes through this
// function so the two callers (the CLI and the HTTP server) can never
// drift apart on field names, omission rules, or nil-vs-empty-slice
// behavior.
func ToWire(result Result, matched []MatchedAction, confidence float64, haveConfidence bool) WireResult {
	w := WireResult{
		Status:          string(result.Status),
		ConsideredCount: result.ConsideredCount,
	}
	for _, c := range result.Constraints {
		w.Constraints = append(w.Constraints, WireConstraint{
			PreceptID: c.PreceptID,
			Outcome:   string(c.Outcome),
			WhyNot:    c.WhyNot,
			RevisitIf: c.RevisitIf,
		})
	}
	for _, warn := range result.Warnings {
		w.Warnings = append(w.Warnings, WireWarning(warn))
	}
	if haveConfidence {
		w.Confidence = &confidence
		for _, ma := range matched {
			w.MatchedActions = append(w.MatchedActions, WireMatchedAction{
				Action: ma.Action.Action,
				Params: ma.Action.Params,
				Span:   WireSpan{Start: ma.Span.Start, End: ma.Span.End, Text: ma.Span.Text},
			})
		}
	}
	return w
}
