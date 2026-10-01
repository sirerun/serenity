# T23.44 production operation evidence routing design

Date: 2026-10-01  
Scope: read-only design audit against the isolated core-writer queue seam. This receipt makes no production, contract, assembly, module, or live-service changes and does not declare T23.44 accepted.

## Finding

The queue seam is available but no hosted remember/forget path uses it. The canonical positive signal must be emitted only after the memory source mutation and the Git commit containing it have both completed. Today remember invokes a source `Queue.Submit`, then a second queue job calls the embedder/index, and only after the handler returns does the gateway call `Runtime.Flush`. Consequently the outer flush cannot safely mark the source operation committed before indexing, and the callback currently records only entry before the source write.

The existing checker test double is also not a valid model for production: `internal/hosted/contracts/contractstest/canonical.go` returns Unknown when it sees pending working-tree state and otherwise uses an in-memory sequence. A production checker must inspect the canonical committed Git `HEAD`; neither working-tree files nor touched queue paths prove durability. A missing or unreadable HEAD/source is Unknown unless a durable content-free marker proves the operation’s terminal state.

## Queue and lifecycle inventory

The memory-specific mutation paths are:

| Path | Current behavior | Required routing |
| --- | --- | --- |
| `internal/writer/memoryfact.go:RememberContext` | `Queue.Submit` calls `rememberLocked`; source may be written before the caller later flushes. The trusted `BeforeCommit` callback enters the operation ledger immediately before the first source write. | Use `SubmitAndFlush` for hosted canonical remember. Keep `BeforeCommit` before the first source write. Add a trusted after-flush signal that runs only after the requested commit/no-op flush succeeds and both queue guards are released. A no-op flush is valid only when the matching canonical operation was already present in committed HEAD; it must not turn an unrelated no-op into success. |
| `internal/server/memory/remember.go:111-117` | A second `Queue.Submit` invokes `RefreshMemoryFactSearch`, including embedding, before gateway `Runtime.Flush`. | Run only after the source-flush signal. Keep provider and index work outside `WithCommitFence` and outside any commit guard. Its failure affects search status, not the already established source durability outcome. |
| `internal/writer/memoryfact.go:Forget` and `forgetLocked` | A `Queue.Submit` writes an expiry, possibly a cancellation fence, purges the index, removes the fact, and calls `rewriteForgottenPath`; gateway flush occurs after the handler. | Ensure forget’s erase marker and history rewrite are published as one explicit durability outcome before success is reported. No provider call should be in this protected section. Define how an already-erased retry is idempotently represented. |
| `internal/writer/memoryfact.go:CancelRemoteOperation` | A separate `Queue.Submit` writes an operation-key cancellation fence and may remove a matching fact. | Treat the key as a trusted internal operation ID only. Publish the cancellation marker before claiming durable absence. A client operation key must first resolve through the ledger’s authoritative `(brain, quota period, client key) -> operation ID` mapping. |

Other queue writers exist and matter to the guard boundary: `internal/writer/tombstone.go`, `publish.go`, `import.go`, `dirtytree.go`, `internal/cli/sync.go`, `internal/direction/ledger.go`, `internal/direction/publication.go`, and `internal/entities/merge.go`; normal `writer.Flush` callers also exist in CLI, supersede, consolidate, hosted gateway lifecycle, and `internal/hosted/pool/pool.go`. They are not automatically protected by the new guard. Scope a checker to one brain queue and do not activate it until every mutation/flush capable of changing the evidence tree is either routed through the shared guard or serialized by an equivalent invariant. An ordinary provider-only `Submit` may hold `runMu`; the seam intentionally does not claim that it is protected by the commit guard.

## Required operation ordering

For a new hosted remember operation, keep the existing trusted ledger identity substitution: the gateway reserves the operation, rewrites the writer argument to the ledger-generated operation ID, and passes that same ID through the unforgeable internal context metadata. The client’s request key remains ledger-only; it is not the canonical identity.

1. Validate and normalize the full request before reserving/entering the canonical operation.
2. Inside `SubmitAndFlush`, invoke `BeforeCommit` immediately before the first source mutation. It calls `EnterCanonical` with the trusted ledger ID.
3. Render the memory fact with `OperationKey` and `CanonicalOperationID` equal to that internal ID; await the commit result. A successful result means the matching source content is in `HEAD`, not merely rendered or staged.
4. After the flush returns successfully and the queue’s run and shared-commit guards are released, invoke the trusted after-flush callback. It records the committed outcome/evidence before any index or provider work begins. The callback must not call queue methods or block on queue locks.
5. Run the current secondary index/embedding job after that signal. Its failure may degrade search; it must not downgrade a durable source commit to PendingReview.
6. Flush any separately touched search/index state under its own existing semantics. This flush is not the source durability signal.

For forget, the source path must atomically establish a content-free erase/cancel marker, remove the fact and scrub its history, then commit the resulting canonical tree before returning durable success. A checker finding the fact in HEAD can report landed. If it is absent and an operation-ID marker in HEAD proves it was erased/cancelled, it can report the corresponding terminal outcome. If the marker was lost, never written, unreadable, or present only in the working tree, return Unknown. Do not infer absence from the erased payload’s old SHA or from a clean working tree.

## Evidence and privacy contract

The source payload already carries `OperationKey` and `CanonicalOperationID`; hosted writes set them to the ledger ID. The operations ledger is authoritative for translating a client idempotency key into that internal ID. A checker must query that mapping from the ledger row for the correct brain and quota period, then inspect committed source evidence by the internal ID. It must not trust a caller-supplied `operation_key` as proof of identity.

Current forget markers are `MemoryExpiryPayload` records. A marker with only the opaque internal operation ID and timestamp can be content-free; expiry records can also carry `TargetSHA256` and free-text `Reason`, which may identify erased content or contain sensitive details. Do not copy those fields into operation-ledger evidence, logs, reconciliation reports, or new marker metadata. Prefer a dedicated minimal lifecycle marker keyed by operation ID and marker kind, or strictly validate/use the existing operation-key-only marker form. Preserve it through `rewriteForgottenPath` and subsequent commits. Keep evidence refs content-free (for example, operation ID plus a fixed evidence kind); do not persist fact text, provenance, entity, client secret/key, customer data, or fact SHA in a reconciliation ref.

Clarify `CanonicalVerdict` documentation in a later contract change: `CanonicalLanded` requires committed HEAD evidence; `CanonicalAbsent` requires positive proof of no canonical attempt/entry or a durable terminal absence marker; any unresolved entered operation with no intact marker is `CanonicalUnknown`. `internal/hosted/contracts/contractstest/canonical.go` should model committed state and explicit marker state instead of working-tree presence. Reconciliation must leave Unknown rows pending for review.

## Minimal implementation ownership and review tests

The following is a proposed later integration boundary, subject to coordinator approval:

- `internal/writer/memoryfact.go`: expose the hosted remember submit-and-flush outcome; retain the existing callback placement before the first source write. Route forget and cancellation through a durable marker+erase outcome. Do not change generic writers as part of this narrow lane until their relationship to the checker’s queue/tree is mapped.
- `internal/server/memory/remember.go`: sequence search/index work strictly after the source durability callback/result; keep it optional and outside the commit fence.
- `internal/server/memory/forget.go` and `cancel.go`: preserve current public response semantics while requiring a durable erase/cancel result for a success response; obtain trusted IDs from internal gateway context/ledger, never the client payload.
- `internal/hosted/gateway/gateway.go`: move remember’s committed transition to the after-flush callback; retain safe pending-review behavior for flush uncertainty and released behavior when no canonical entry occurred. Do not hold the gateway-wide mutex or brain fence over provider/index work.
- `internal/hosted/pool/pool.go`: expose the brain’s canonical queue/fence and a HEAD-backed checker source to the hosted service. Route or exclude all other flushes before enabling reconciliation for that tree.
- `internal/hosted/contracts/operations.go`, `internal/hosted/operation/ledger.go`, and store/source marker code: tighten verdict semantics, resolve internal IDs authoritatively, and preserve content-free erasure evidence across history rewrite. Any marker-format or schema change needs a separate reviewed migration decision.

Focused tests required before production routing is considered:

1. Writer tests prove `BeforeCommit` precedes the first source write, the matching source is committed in HEAD before the after-flush callback, callback runs after queue guards are released, and commit failure/cancellation never reports committed.
2. A service-level test blocks the embedder and observes that the source is already in HEAD and ledger phase committed before provider work starts; provider failure leaves the durable operation committed and returns degraded search state.
3. Forget tests prove the fact payload and its history are absent, a content-free operation marker remains in HEAD, retries are stable, and no erased fact text/provenance/entity/SHA/client key appears in marker or ledger evidence.
4. Checker tests cover landed in HEAD, explicit durable erase/cancel marker, genuine no-entry/released state, working-tree-only data, missing marker after history purge, corrupt/unavailable HEAD, mismatched client-key mapping, and unknown preservation. Every ambiguous case must stay Unknown/PendingReview.
5. Queue integration tests ensure ordinary unrelated writers/flushes cannot race the checker once it is enabled, while provider-only work remains outside the exclusive fence and does not deadlock or extend the protected interval.

This design does not establish assembled pool wiring, migration readiness, hosted reconciliation acceptance, provider qualification, or task acceptance.
