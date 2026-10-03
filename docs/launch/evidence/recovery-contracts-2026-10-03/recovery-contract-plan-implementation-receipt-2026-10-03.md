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

This receipt records only focused package checks. Full-module checks and independent exact-head review remain with the coordinator. No provider, writer, hosted-admission, deployment, PR, merge, or full-task acceptance evidence is claimed.
