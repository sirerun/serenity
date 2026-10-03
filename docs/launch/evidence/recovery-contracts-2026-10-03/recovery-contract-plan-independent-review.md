# Independent review: recovery contract-plan component

**Verdict: CLEAR for the bounded recovery artifact-to-contract-plan component at exact integrated head `1e921e5449aabf2601925f5a909efed174df7139`.** This is a component-level source review only. It does not qualify the full recovery coordinator, immutable envelope/store, epoch or admission verifier, startup/service wiring, provider behavior, or hosted acceptance.

## Exact scope and source review

A fresh detached external-SSD clone is clean at the exact head. The source baseline is landed main `e119d251686627a4d6d29e002cdd8b89bdddbc99`; the three new `internal/hosted/recovery/contract_plan*.go` files match author handoff `4a22932e506c652200263fe2f086d3fedad39330`, with the one later test assertion correction in `contract_plan_test.go`. Other integration changes are documentation and delivery status. `git diff --check` passes.

The implementation matches the frozen field mapping: legacy artifact `SnapshotSHA256` becomes contract `SourceSnapshot`; journal watermark and fence generation are preserved; provider time is exact canonical UTC; and accounts are sorted, bounded and copied. The legacy `FormatVersion`, artifact PlanHash and contract PlanHash remain in their separate domains. Version-1 canonical JSON field order and names match the golden fixture; the input `PlanHash` is excluded. The validation placeholder is confined to a private value copy. Create and load paths use the existing artifact API, re-open the artifact, revalidate it, verify identity and cancellation boundaries, and preserve legacy bytes. `VerifiedEligiblePlan` has private fields and no arbitrary-plan constructor. This is not a signed approval or provider/writer authority.

I previously held review at `ee1cafb3996f22b805768008a58285c699ed132e` because the token test did not assert that stored contract `PlanHash` equaled the separate computed hash. The corrected head adds that equality and distinctness assertion for both created and loaded tokens. A compiled mutant that writes the legacy artifact hash into `contractPlan.PlanHash` now fails at the new assertion.

## Independent behavioral controls

Three independent mutations were applied only in the detached review clone, compiled, failed at their intended runtime assertions, and were restored with the original source hashes verified:

- Mapping `artifact.PlanHash` into `SourceSnapshot` failed `TestVerifiedEligiblePlanCreateLoadAndArtifactIdentity`. Evidence: `control-map-red-20261003T0530Z/`.
- Changing the canonical payload `domain_version` from 1 to 2 failed `TestCanonicalContractRecoveryPlanHashVersionOneGolden`. Evidence: `control-domain-red-20261003T0532Z/`.
- Writing the artifact hash into `contractPlan.PlanHash` failed the new created/loaded token identity assertion. Evidence: `control-contracthash-red-20261003T0541Z/`.

All three runner leases were won and released exactly. A separate attempted focused green rerun lost admission to the R02 holder before invoking Go; it is not counted as test evidence. The coordinator’s subsequent full race run includes the corrected focused test and records it passing.

## Full local qualification and remaining boundary

At this exact head, the coordinator’s results record four exit-0 stages: full `go test -race ./...` (3,026 test passes across 84 packages), `go vet ./...`, `golangci-lint run ./...`, and Linux ARM64 CLI build. The race run disclosed nine skipped tests/subtests and four packages without tests; the receipt preserves those skips and does not claim S3 qualification. The full run’s four exact build leases were released, and the private fixture does not establish physical capacity or provider acceptance.

Raw root qualification evidence is under `[external evidence store]`; independent clone, mutant controls and restoration fingerprints are under this review directory. Public receipt searches found no host paths, email addresses, IPs or credential-shaped values. No source edit, PR change, push, merge, source claim operation or provider action was made by this review.
