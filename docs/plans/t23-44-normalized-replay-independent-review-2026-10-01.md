# Independent review: normalized hosted remember replay

**Reviewed revision:** `1815e70b8c0f0378f119fc286ebffb0a46854564`, the integrated normalized-replay patch atop coordinator `4161c212513168d05345982574335da6bba73054`.

## Verdict

The patch closes the committed-replay fingerprint defect at the hosted gateway. It shares validation and defaulting through `memory.NormalizeRememberRequest`, so the gateway fingerprints the same semantic values the remember handler passes to the canonical writer. I found no changed-field path that can replay a committed operation with different normalized remember content. This is focused local evidence only; T23.44 remains partial and has separate acceptance/documentation gaps below.

`internal/server/memory/remember_normalize.go:21-92` extracts the former remember validation/defaulting path without writing. `internal/server/memory/remember.go:76-95` consumes that same normalized result for the writer. In `internal/hosted/gateway/gateway.go:405-436`, the gateway validates before reservation and fingerprints brain, exact fact and provenance, visibility, the parsed absolute expiry rendered in UTC RFC3339Nano, canonical entity type and slug, and defaulted kind. This covers every remember payload field that affects the canonical writer. Account, key and original quota period are enforced by the operation ledger's namespace/query/index; the hosted source is fixed, and the writer principal falls back to `LocalWriter` in this path. The gateway maps `ErrOperationKeyReuse` to the same stable remember conflict response as the local writer.

Semantic normalization preserves compatibility: absent kind/visibility become `fact`/`world`; entity aliases are canonicalized before comparison; two absolute TTL strings denoting the same instant fingerprint identically; unrecognized extra JSON fields remain ignored by the handler and do not affect the stored request. Explicit defaults therefore replay the original ID. Validation runs before a committed replay can bypass the handler, so malformed or invalid requests do not replay a committed result. The source projection assertion in the regression test confirms rejected retries did not add another canonical fact.

## Regression evidence

The added `TestCommittedRememberReplayBindsNormalizedPayload` (`internal/hosted/service/fingerprint_regression_test.go:20-126`) checks explicit defaults against an initial commit; changed visibility, provenance, entity, kind and TTL all return `operation_conflict`; invalid changed requests fail; and only one canonical fact and committed ledger row remain.

Focused checks passed:

- `go test -race -count=1 ./internal/hosted/service -run '^TestCommittedRememberReplayBindsNormalizedPayload$'`
- `go test -race -count=1 ./internal/server/memory -run 'Test(RememberRecordsCallerAsWriter|RememberValidationRunsBeforeCanonicalOperationCallback|RememberOperationWireContract|MemoryV1TTLValidation)$'`

The requested mutation was reproduced in this isolated worktree: replacing the semantic fingerprint field set with the former brain+fact-only fields made the new regression test fail for all five changed payload fields because each replay returned the old committed result. The mutation was restored; no production files are changed in this review branch.

## Remaining acceptance and contract gaps

This review does not mark T23.44 complete. Its task record still requires an unkeyed hosted `remember` with `ttl: "30d"` to store the fact (`docs/launch/hosted-completion/tasks.json:188`). The gateway creates an internal operation key for unkeyed calls before invoking `NormalizeRememberRequest`; the shared validator then treats that internal key like a client key and rejects duration shorthand. This is preexisting behavior, not introduced by this fingerprint patch, and remains an acceptance gap until the internal idempotency path preserves relative-TTL behavior or stores a fixed expiry without changing client semantics. The separately tracked cross-period writer identity scope is also not resolved by this patch.

The implementation contract needs reconciliation before task acceptance: `docs/launch/hosted-completion/interfaces.md:141` describes remember fingerprint input as only fact bytes and brain, while this necessary correction binds all normalized stored fields. ADR017 at `docs/adr/017-hosted-operation-and-recovery-contracts.md:38-46` approves fingerprint-bound retry keys but does not enumerate their fields. The task record at `docs/launch/hosted-completion/tasks.json:188` already requires a changed-visibility retry to conflict. Update the interface description to enumerate normalized semantic fields and retain the task requirement; no acceptance wording should be weakened.

No live hosted endpoint, provider, or deployment qualification was exercised.
