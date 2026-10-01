# Private operator resolution service slice

Implementation base: `3ed28b556707dbf0cb4208000684b74777be3f55`, in isolated full clone `/Volumes/BuildOffload/worktrees/serenity-operator-review-admission-20261001`.

This receipt covers only the service implementation slice in `internal/hosted/service/**`. It does not add a production authenticator, immutable case store, private listener, or default adapter, and it is not T23.44 or T23.47 end-to-end acceptance.

The optional `LifecycleDependencies.OperatorReviewAdmission` is deliberately excluded from required startup validation. The private `AdminHandler` route `POST /operations/resolve` returns 503 when the dependency is absent, including typed nil. It strictly parses a bounded request containing only operation ID and review reference, asks the trusted admission to authenticate and return an immutable case before ledger lookup or fencing, verifies the committed-only case binding, and then requires a live matching Landed canonical reference while holding the exact runtime fence through the ledger transition. It stores the unchanged review reference as `EvidenceOperatorReview`. Exact committed retries require reauthorization against the same binding and preserve usage counters. The public service handler has no resolution action.

## Validation

- Focused operator-resolution tests passed with `go test -count=1 -p 1 ./internal/hosted/service -run '^(TestParseOperatorReviewRequestStrict|TestAdminOperatorReviewAdmissionBeforeLookupAndNoAdapter|TestAdminOperatorReviewHoldsFenceThroughCheckAndTransition|TestAdminOperatorReviewCancellationDuringProofKeepsPendingReview|TestAdminOperatorReviewRejectsCaseAndProofMismatch|TestAdminOperatorReviewCommitsOnlyMatchingLandedProofOnce|TestAdminOperatorReviewColdInactiveAndCanceledKeepPendingReview)$'`.
- Full service package passed with `go test -count=1 -p 1 ./internal/hosted/service` (`9.118s`). The integration test writes and checks an actual canonical fact, confirms the original quota period is charged exactly once, and verifies same-case replay leaves counters unchanged. Other tests cover denied admission before lookup/fence, strict parser rejection, proof/case mismatch, Unknown proof, cold/inactive runtimes, cancellation, fence retention through transition, and public-handler non-mutation.
- `gofmt` and `git diff --check` passed. No race test or multi-package gate was run in this worker lane; the coordinator owns final integrated gates.
- Go test commands used external GOCACHE, GOMODCACHE, GOTMPDIR, and TMPDIR and were each preceded by `uptime`; one-minute load remained within the configured limit.

## Ownership record

The first `R-hosted-service` attempt returned `9ce6d49f7796a99a8d22dd92e6a25125a71d424a` against the local shared-build claim remote, not the canonical fleet claim remote. I paused edits, acquired the canonical GitHub `R-hosted-service` claim at `425b85392d8a1a6394891dbfc6c25b6213df5236`, verified that exact SHA with `git ls-remote`, then released the earlier local claim using its exact SHA before continuing. The canonical claim remains held until this implementation commit is banked.
