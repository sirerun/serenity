# Recovery Contract Plan Implementation Receipt

Branch `implementation/recovery-contract-plan-20261003`, based on frozen commit `08deb9644d78cf1f0cd9fef4e03f9fd76c8737b6`.

Implemented the additive, read-only artifact-to-contract-plan boundary in new `internal/hosted/recovery/contract_plan*.go` files. The boundary preserves legacy artifact bytes and identity, maps only the frozen artifact fields, bounds eligible scope, verifies loaded artifacts, deep-copies account slices, and computes the version-1 canonical contract hash independently of the legacy artifact hash. The canonical helper leaves the existing contract validator and legacy serialization unchanged; its SHA-shaped validation placeholder is local-only.

Focused verification passed:

- `go test -race -exec 'env TMPDIR=<owned-fixture-temp> SERENITY_RECOVERY_TEST_TMPDIR=<owned-fixture-temp>' ./internal/hosted/recovery` — stage 007.
- `go vet ./internal/hosted/recovery` — stage 008.
- `golangci-lint run ./internal/hosted/recovery` — stage 009.
- Behavioral mutant: mapping `PlanHash` into `SourceSnapshot` compiled and failed the direct frozen-mapping assertion — stage 005.
- Behavioral mutant: changing canonical `domain_version` from 1 to 2 compiled and failed the independent golden digest assertion — stage 006.

All stages ran through the durable external runner, with load below 10, a freshly won shared build lease, unchanged source fingerprint during execution, and exact lease release. Evidence is under `[external evidence store]`. An earlier stage 004 mutant attempt omitted the private fixture `-exec` environment and failed fixture setup; it is not counted as a behavioral RED. Stage 005 repeated the mutant with the required fixture environment and supplies the valid evidence.

Historical worker-only handoff: at author submission this receipt recorded focused package checks, and full-module checks and independent review were pending. The Final coordinator qualification section below supersedes that status. No provider, writer, hosted-admission, deployment, PR, merge, or full-task acceptance evidence is claimed.

Independent review at integrated `ee1cafb3996f22b805768008a58285c699ed132e` found a missing explicit regression assertion that stored contract PlanHash equals the computed contract hash for both created and loaded tokens. Source already used the correct value. The assertion was added; the Final coordinator qualification section below records the completed qualification and re-review at the corrected exact head. Earlier full qualification at ee1cafb passed 3,026 tests across 84 packages and vet/lint/Linux ARM64, with the same nine test/subtest skips and four no-test packages as PR353; it does not qualify the later test edit by itself.

## Final coordinator qualification

At exact corrected integration `1e921e5449aabf2601925f5a909efed174df7139`, full-module race passed 3,026 tests across 84 packages, including the new stored-contract-hash assertion. Full vet, lint and Linux ARM64 build passed. All four stages freshly won and exactly released their shared build leases. Nine tests/subtests skipped and four packages had no tests; the detailed skip inventory remains the same as PR353 and is preserved in raw JSON events. The owned APFS fixture is not provider or 500 GB capacity qualification. A prior final-run attempt correctly lost the lease to another project and executed no command; that history is preserved.

Independent final source review was CLEAR at that same exact head. The additional mutant storing the legacy artifact hash into contract PlanHash compiled and failed the new assertion; source was restored exactly. Together with independent mapping and canonical-domain mutants, this closes the review finding. The initial review HOLD and unexecuted extra attempts remain in the external evidence store. Subsequent qualification/report/plan changes are documentation only, with all Go/test/module bytes unchanged. Normal merge and landed verification remain open; full task50 and hosted/provider acceptance remain open.
