# Inspector API and Book Binder source qualification

PR: [361](https://github.com/sirerun/serenity/pull/361). Reviewed baseline: `e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f`.
Verified code revision: `14727a7ff6835e7361e56aefbf8be6f8dcf59dff`.

## Verification

The user authorized one-core checks above the usual shared-machine load threshold. Each heavy run acquired and released its own shared build lease; no foreign lease was removed. Tests used an isolated ownership-enabled APFS image stored on the external build volume. The shared volume itself has ownership enforcement disabled, which correctly causes private-filesystem tests to reject it.

At the verified revision, with Go 1.27.1, `GOMAXPROCS=1` and package concurrency one:

- `go test -race -p 1 -json ./...`: exit 0; 3,143 tests passed, 64 skipped, 85 packages passed and three package-level skips. No failures.
- `go vet -p 1 ./...`: exit 0.
- `golangci-lint run --concurrency 1 --timeout 15m`: exit 0, zero issues (version 2.13.2).
- `go build -p 1 ./...`: exit 0.
- Website checker: 24 pages, 406 local links, zero errors.
- Brand generator preservation test: one test passed.
- Hosted deployment/supply-chain Python tests: 22 passed.
- Immutable workflow action-pin validation passed.

Prior desktop and phone CUA checks covered public home, pricing, product, install, documentation, chat and brand pages, including the phone primary CTA. JavaScript syntax checks passed. The hosted browser CI suites did not execute; no authenticated browser acceptance is claimed. Strict MkDocs builds failed on both the candidate and clean baseline with the same seven existing warnings; this is unchanged baseline evidence, not a strict docs pass.

GitHub Actions annotations report that jobs did not start because the account is locked for billing. Source merge uses the accepted [ADR 024](../adr/024-local-validation-during-actions-billing-lock.md) local-validation fallback, without changing branch protection, billing or CI results.

## Independent headless review

Actual ephemeral, read-only Codex CLI sessions using GPT-6-Luna reviewed the full source at `b23b649124fd237aa6e6cd85abd47cc18cff6818`, then each subsequent correction. Full source review found no actionable source findings but required real verification before merge. Corrections removed an unused import and type, checked a test-helper write, preserved character-validator behavior, encoded the test request URL, and corrected test expectations for stable ordering, empty arrays and unknown capture dates. These were caught through compilation, execution and lint rather than being treated as passing from source review alone.

Superseded eligible history remains available; private, retracted and expired records remain excluded according to the existing remote-read policy. Observed dates do not silently become capture dates. No owner-browser privacy exception was authorized.

## Release boundary

This PR contains the website brand and authenticated read API. The real-data spatial frontend is the next delivery wave. Merge does not deploy the existing hosted service. Signed artifacts, runtime/startup qualification, rollback rehearsal, performance measurements and final frontend qualification remain required; see [release preflight](../tasks/inspector-release/release-preflight.md). No production hold is lifted by this receipt.

Ajent MCP tooling is unavailable. Repository coordination records and GitHub discussions were checked; no trusted hold specific to this feature patch was found. Private operator logs and provider inventory remain outside the public repository.
