# Independent review: hosted canonical retry-key scoping

Reviewed `3fe8a06` and `d2f30ef` on `hosted/journal-recovery-20261001` in an isolated worktree at `89f07e5`. Review scope was the gateway/writer key boundary, ledger preservation, cleanup ordering, and the real service regression. No production files were changed.

## Review result

The scoped change is correct for the period-collision defect. The ledger continues reserving the client-supplied key with its account, brain, fingerprint, and quota period. Once reserved, the gateway replaces only the forwarded canonical `operation_key` with that reservation's stable operation ID. This prevents the writer's brain-wide canonical key namespace from replaying a previous period's fact. The original client key remains in the ledger and remains the input to replay/conflict decisions. Committed ledger retries return the stored outcome before re-entering the writer.

`d2f30ef` also moves the reservation-finalization defer ahead of JSON argument rewriting. Thus any failure after reservation but before the writer attempt executes cleanup: deterministic pre-canonical errors release the reservation; an entered operation becomes committed only after successful canonical completion, otherwise pending review. Finalization uses a five-second context detached from caller cancellation. Cancellation before `EnterCanonical` does not begin the source mutation; after entry, the detached finalizer preserves an unknown outcome rather than releasing its charge.

The real assembled-service regression `internal/hosted/service/period_writer_regression_test.go` exercises the same client key in September and October, then repeats October. It asserts two distinct canonical fact IDs, same-period replay of the October ID, two source records, one committed write per period, and agreement between each canonical operation ID and the corresponding ledger row while the client key remains `monthly-key`. Focused validation passed:

```text
GOCACHE=/Volumes/BuildOffload/.cache/hosted-key-review/gocache \
GOTMPDIR=/Volumes/BuildOffload/.cache/hosted-key-review/tmp \
go test -race ./internal/hosted/service -run '^TestHostedWriterRetryKeyIsScopedToQuotaPeriod$' -count=1
ok   github.com/sirerun/serenity/internal/hosted/service  2.198s
```

I found no blocker in these two commits. This does not close T23.44 or establish recovery acceptance. The task's pending-operation reconciliation remains a separate required lifecycle integration; absent that integration, interrupted reservations are not proven to resolve automatically at service startup. Likewise the period regression is local service evidence, not full hosted acceptance.

## Handoff

Keep the existing `OperationKey` public writer behavior for non-hosted callers. The gateway's internal substitution applies only after a hosted ledger reservation and does not expose the ledger ID as the client key. Retain the current contract test and this real-service period regression when integrating. No merge or acceptance decision is made by this review.
