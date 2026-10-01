# Hosted operation lookup receipt

Base: PR338 candidate `65612db1a9a23e1c0e86cdcdc6b302f3f421c7a6`. This isolated change adds a concrete, read-only `(*operation.Ledger).Lookup(ctx, operationID)` without changing the frozen `contracts.OperationLedger` interface.

`Lookup` uses the ledger's canonical record projection and scanner so it returns the complete internal `OperationRecord`, including original quota deltas, phase, evidence, source, and timestamps. It binds the opaque ID as a SQL parameter; empty and absent IDs map to `contracts.ErrOperationNotFound`. A canceled context is returned before query work begins. The method has no transaction or mutation path. Tests cover a committed record with all fields populated, canceled context, empty/missing IDs, and stable usage counters after repeated reads.

This lookup is an internal prerequisite for a later operator-resolution adapter. Possession of an operation ID grants no transition authority; lookup does not alter state and the frozen ledger interface remains unchanged. The source/test commit is not acceptance of an HTTP endpoint, operator authorization, pending-review policy, or T23.44.

Validation: `gofmt` and `git diff --check` passed. With one-minute host load below 10 and no competing Go process, `go test -race ./internal/hosted/operation` passed (2.151s). Go temporary files and caches are preserved as an untracked `.validation/` artifact on the external SSD.
