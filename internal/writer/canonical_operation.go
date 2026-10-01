package writer

import "context"

type canonicalOperationContextKey struct{}

// CanonicalOperation carries trusted hosted ledger metadata into a canonical
// writer call. It is separate from the client-facing remember operation key.
type CanonicalOperation struct {
	ID           string
	BeforeCommit func(context.Context, string) error
	// AfterFlush reports the durable identity only after the trusted source
	// write and its inline Git flush both succeeded. It runs after queue guards
	// have been released and must not perform ledger or queue I/O.
	AfterFlush func(context.Context, string, string)
}

// WithCanonicalOperation attaches metadata supplied by an internal caller.
// Request JSON must never be used to populate this value.
func WithCanonicalOperation(ctx context.Context, operation CanonicalOperation) context.Context {
	return context.WithValue(ctx, canonicalOperationContextKey{}, operation)
}

func canonicalOperationFromContext(ctx context.Context) CanonicalOperation {
	operation, _ := ctx.Value(canonicalOperationContextKey{}).(CanonicalOperation)
	return operation
}
