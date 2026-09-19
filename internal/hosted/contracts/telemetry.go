package contracts

import "context"

// Telemetry (interfaces.md "Telemetry", owner task53, hooks41/57). FROZEN:
// fixed-cardinality structured event interface. Operation/Outcome/Unit are
// closed enumerations (task53 defines the exact allowed values in its own
// freeze evidence); request content and opaque tenant identifiers are
// forbidden fields by construction — this type has no field to carry either.
type TelemetryOutcome string

const (
	TelemetryOutcomeSuccess  TelemetryOutcome = "success"
	TelemetryOutcomeFailure  TelemetryOutcome = "failure"
	TelemetryOutcomeDeferred TelemetryOutcome = "deferred" // e.g. staged_for_review
)

type TelemetryEvent struct {
	Operation  string // fixed-cardinality producer name, e.g. "gateway.remember"
	Outcome    TelemetryOutcome
	DurationMS int64
	Count      int64
}

// Telemetry is the sink task53 implements. Emit must apply recursive
// redaction and upstream error sanitization before any value leaves this
// process, and must never block a caller past its own bounded sink queue
// (interfaces.md: "bounded sink queue").
type Telemetry interface {
	Emit(ctx context.Context, event TelemetryEvent) error
}
