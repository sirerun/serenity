# Hosted Git migration final local qualification

Pinned tested source: `dd17a56bdbfbf6ba2c669bfd62ed86ed2a421643`; Go/module content is byte-identical to independently reviewed corrected source `2296a958`. Canonical/lifecycle source f6733a15 is independently CLEAR; isolated pool review and its supplemental original-e6 baseline mutations clear pool source 2296a958. Positive service-fixture changes were independently inspected.

Full `go test -race -count=1 -json ./...` passed 84 packages and 2,976 tests/subtests. Four packages have no tests. Seven preexisting helper/gated skips remain, including live S3 qualification; these are not acceptance of provider/live/physical quota or full T23.45/46/48. `go vet ./...`, `golangci-lint run ./...` and Linux ARM64 no-CGO CLI build all exited 0. Fresh uptime stayed <=10 at each stage, SSD cache/temp and -p=2 were set, and the APFS private ownership fixture was used for executable test temp roots. The fixture is not physical quota proof.

Shared build lease `1fe9466ebc567799dc94d63a70dee4944e09984c` was actually WON, exactly reverified before each stage and CAS-released in the runner finally block. Immutable external hosted-git-validation log SHA-256 values:

- race: `f791faf277aa01688a179436414f0e529577fa4d00581c4bbdc2c215ac1e5f12`
- vet: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- lint: `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`
- linux-arm64-build: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`

The 34-contract completion checker passed with both profile DAGs acyclic and original40 requirements covered; it remains preview only, not task completion. gofmt and diff checks are clean. The founder-authorized local-validation route is used for GitHub billing limitations; no green Actions claim is made.

Only documentation was appended while full gates ran: the independently cleared v1.2 transport source ruling. No Go/module source changed. Report copies with normalized Markdown trailing spaces: none. Cited independent report SHAs identify immutable originals. The initial nonisolated pool review and incorrect absent-feed/claim-remote accounts are preserved as historical observations and superseded by explicit corrective evidence, not silently credited as valid isolated checks.

This qualifies the bounded local source changes. The new transport remains a separate unwired source lane; no installed scripts, provider requests, deployment, purge, spend, recovery activation or full-task acceptance occurred. Evals still have four raw Git callsites; whole-module T24.30 coverage remains a separate future lane.
