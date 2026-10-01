# T23.44 final after-flush candidate follow-up review

This follow-up reviews the final candidate `5e6413aef237138068c5f7c5f3397c055e13c675`, including production fix `9ea453c`, writer test correction `74593b5`, and service regression `8fc8e61`. The original independent review of `64d007f829d55ebf512e2f9bb448c31c652b660c` is recorded in `t23-44-afterflush-independent-review-2026-10-01.md`. This branch contains only this report; no production files were changed.

## Verdict

No blocker found in the final candidate's narrow hosted `remember` path. The additional trusted-entry predicate closes a real outcome-classification edge, and the new service regression now proves that the fact is present in Git `HEAD` before the injected embedder fails, that the ledger remains committed, and that retry returns the same fact without a second provider call.

## Follow-up review

- `rememberFinalizeOutcome` now requires both `operationEntered` and a nonempty durable fact ID before producing committed evidence. A fact ID without trusted entry maps to released/no canonical attempt (`internal/hosted/gateway/gateway.go`, `rememberFinalizeOutcome`). This matches the writer's contract: `AfterFlush` cannot alone attest that the gateway's canonical ledger entry succeeded.
- `MemoryFact.RememberContext` now rejects inline-flush metadata unless `RememberInput.OperationKey` exactly matches the trusted context operation ID, before the queue job runs (`internal/writer/memoryfact.go`, inline-flush validation). The gateway substitutes its reserved internal operation ID into handler args before attaching that context, so this is consistent with production routing and keeps client keys from choosing the canonical marker.
- The final service regression installs a fake embedder that reads the matching source from Git `HEAD`, verifies the source digest and nonempty canonical marker, and then returns an injected error. It asserts the ledger phase is committed, retry returns the same fact identity without a second embedder call, the projection contains one fact, and write quota is committed once (`internal/hosted/service/afterflush_regression_test.go`). This covers the previously noted post-publication indexing/provider failure gap.
- One minor strengthening opportunity remains: the new failure regression asserts the source marker is nonempty, while the existing success-path test `internal/hosted/service/canonical_operation_test.go` asserts that the stored marker exactly equals the committed ledger ID. Combining those assertions in the failure regression would make its own correspondence proof complete; the production writer's exact-key guard and existing success test support the current behavior.

## Validation

- `go test -count=1 ./internal/hosted/service -run '^TestHostedRememberCommitsBeforeProviderFailureAndReplays$'` passed (0.765s), with one-minute load sampled at 2.50 before launch.
- `go test -race -count=1 ./internal/writer -run '^TestHostedRememberAfterFlush'` passed (1.692s), with one-minute load sampled at 2.59 before launch. This is the final candidate tree.
- The earlier independent overlay mutation on the base candidate forced legacy non-flushing routing and produced runtime failures in the new writer tests; those red results and restored green checks are documented in the original review receipt.

These were focused package checks, not a full service or repository test run. The larger full-suite integration gate is owned by the coordinator.

## Remaining boundary

This change does not add the operation checker/recovery path for process death after Git publication but before callback/finalization, nor does it extend inline flush and durable-outcome semantics to Forget, other source mutations, local calls without the trusted callback, or startup/marker adoption. Those remain separate route-fencing and activation work.
