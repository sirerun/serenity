package router

import (
	"context"
	"time"
)

// SpendEntry is one row of the spend ledger (RFC section 16): every
// judgment- and local-cheap-tier call writes one, carrying provider and
// model@version for audit and the T4.10 spend-ceiling projection.
// Confidence/ConfidenceClamped are the CLAMPED values (Result.Confidence
// after NewConfidence), not the provider's raw report.
type SpendEntry struct {
	ID                string
	TaskClass         TaskClass
	Tier              Tier
	Provider          string
	ModelVersion      string
	InputTokens       int
	OutputTokens      int
	CostUSD           float64
	Confidence        float64
	ConfidenceClamped bool
	OccurredAt        time.Time
}

// SpendLedger persists spend entries. RFC section 7.5 places the spend
// ledger in the derived index's runtime-only state (DB-only, never
// canonical) -- a concrete implementation backing this interface with
// the index engine is wired by whichever subsystem first holds a live
// index.Engine handle in production (T1.8 extraction, or T4.10's spend
// ceiling, which already depends on this package). Keeping the
// dependency direction this way means internal/router never imports
// internal/index.
//
// Record, not Append: internal/gate's file-first CI gate (RFC section 7,
// ADR 004) flags any call site named "Append" outside the writer-queue
// allowlist, because store.ShardStore.Append is a canonical-file write
// primitive. SpendLedger is unrelated runtime-only state, not a
// canonical-file write, so it is named to stay clear of that gate rather
// than fight it.
type SpendLedger interface {
	Record(ctx context.Context, e SpendEntry) error
}
