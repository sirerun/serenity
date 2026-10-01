# Hosted operation lookup receipt

Base: PR338 candidate `65612db1a9a23e1c0e86cdcdc6b302f3f421c7a6`. This isolated change adds a concrete, read-only `(*operation.Ledger).Lookup(ctx, operationID)` without changing the frozen `contracts.OperationLedger` interface.

`Lookup` uses the ledger's canonical record projection and scanner so it returns the complete internal `OperationRecord`, including original quota deltas, phase, evidence, source, and timestamps. It binds the opaque ID as a SQL parameter; empty and absent IDs map to `contracts.ErrOperationNotFound`. A canceled context is returned before query work begins. The method has no transaction or mutation path. Tests cover a committed record with all fields populated, canceled context, empty/missing IDs, and stable usage counters after repeated reads.

This lookup is an internal prerequisite for a later operator-resolution adapter. Possession of an operation ID grants no transition authority; lookup does not alter state and the frozen ledger interface remains unchanged. The source/test commit is not acceptance of an HTTP endpoint, operator authorization, pending-review policy, or T23.44.

Validation status at source banking: `gofmt` and `git diff --check` passed. The focused package race test is pending host-load confirmation; no build or Go test has been run for this change yet.
