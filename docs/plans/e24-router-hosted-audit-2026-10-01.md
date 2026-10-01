# E24 router and hosted verification readiness audit

Date: 2026-10-01  
Scope: T24.12, T24.13, T24.25, T24.27, T24.29, and the hosted dependency gate for T24.31–T24.38. This is a source and evidence audit; no implementation files were changed.

## Outcome

The router work is locally verifiable in this checkout. T24.12, T24.13, and T24.25 have the expected source paths and regression tests, and their package tests pass. T24.27's local handler and template tests pass, but its stated post-deploy Lambda environment check is not evidenced here. T24.29's disposition table is pinned to an older tree and its counts no longer match this checkout.

T24.31–T24.38 remain blocked. The hosted-completion registry marks all required upstream tasks T23.44–T23.54 as `planned` (T23.41 and T23.60 are `ready`). Local-process receipts exist only for T23.44, T23.46, and T23.47; each says `PARTIAL`. Their artifact hashes do not all match current source, and the other required result receipts are absent. None of these facts establishes acceptance.

## Router and docs-chat findings

- **T24.12 — locally ready; no accepted-task claim.** `internal/router/router.go` redacts both prompt roles in `Complete`; `internal/embed/embed.go` sends every embedding input through `Complete`. `internal/router/redaction_test.go` exercises the recording-provider wire for Anthropic, OpenAI-compatible, OpenRouter, OpenAI-compatible embeddings, and OpenRouter embeddings. Anthropic has no embeddings endpoint, which the test documents. Pattern/config coverage is in `internal/redact/keyshapes_test.go`, `internal/router/redaction_config_test.go`, and `internal/config/redact_test.go`. The router, redact, and config package tests pass. The acceptance record still needs the required genuine-red and green outputs in the task PR body; this audit does not supply those PR artifacts.
- **T24.13 — locally ready; no accepted-task claim.** `internal/router/prices.go`, `router_test.go`, `retry_test.go`, and `internal/server/memory/synthesize_limits_test.go` cover priced token-only usage, fail-closed unlisted models, `MaxUSD`, context cancellation during backoff, the synthesis token limit, and the 61st-call limit. Router and memory package tests pass. The reportable “real cost” is a per-call router result and spend row; the implementation does not claim to prevent a provider call already in progress.
- **T24.25 — locally ready; no accepted-task claim.** `internal/router/system.go` and `system_test.go` place instructions in a system role and generate per-call nonce-delimited document fences. The router package tests pass. The task's frozen extraction-eval check and genuine-red/green PR evidence are not established by this audit.
- **T24.27 — local behavior ready; deployment check open.** `deploy/chat/handler.py`, `test_handler.py`, `test_operations.py`, and `stack.json` implement user-only history, atomic rate limits, and a Secrets Manager salt reference; the test suite passes (26 tests). The acceptance criterion requiring read-only confirmation that the deployed Lambda environment has no plaintext `RATE_SALT` is not present in the inspected evidence, so the task cannot be called complete from these local tests.
- **T24.29 — not ready for review closure.** `docs/deep-reviews/001-remediation-status.md:14-24` pins counts and line numbers to commit `2bf0d98` and reports 79 repository sites / 66 under `internal cmd pkg`. A fresh count in this task worktree is 83 repository sites / 70 under that scoped path. The table therefore needs a current-tree reconciliation before it can satisfy the “one row per site” criterion. The current code also still has the three billing Scan discards at `internal/hosted/billing/billing.go:330,388,1047`; those are the material hosted FUN-05 sites called out by the task and T23.47 receipt.

## Hosted dependency gate: T24.31–T24.38

The plan explicitly makes T24.30–T24.38 wait for their hosted tasks to be marked accepted (`docs/plans/deep-review-001-remediation.md:133-145`). In `docs/launch/hosted-completion/tasks.json`, each upstream task below is still `planned`:

| E24 verification | Required hosted task(s) | Registry status | Evidence available in this checkout | Readiness gap |
|---|---|---|---|---|
| T24.31 | T23.44 | planned | `T23.44/result.json`: PARTIAL, local-process | Receipt itself leaves physical storage bound blocked and phase barriers not run. `gateway.go` artifact digest differs from the current file. |
| T24.32 | T23.45 | planned | No `T23.45/result.json` | Required dependency receipt is absent. |
| T24.33 | T23.46 | planned | `T23.46/result.json`: PARTIAL, local-process | Receipt says registration mode is not wired through service config/assembly. All three listed artifact digests differ from current files. |
| T24.34 | T23.47 | planned | `T23.47/result.json`: PARTIAL, local-process | Receipt records a FAIL on payment-failure timing and NOT_RUN lifecycle criteria; source still has all three ignored `Scan` errors. Digests for `billing.go`, `billing_test.go`, and `checkout_interruption_test.go` differ from current files. |
| T24.35 | T23.48 | planned | No `T23.48/result.json` | Required hosted deletion/export receipt is absent. |
| T24.36 | T23.50 | planned | No `T23.50/result.json` | T23.47, T23.48, and T23.49 are also planned; no restore receipt is present. |
| T24.37 | T23.54 and T23.52 | planned | No result receipt for either task | T23.51 and T23.53 are planned dependencies of T23.54; no source-pinned INF regression result is present. |
| T24.38 | T23.51 | planned | No `T23.51/result.json` | Required bootstrap receipt is absent. |

No current hosted test function names containing the required finding identifiers (for example `FUN01`, `CON01`, `SEC...`, `PRIV01`, or `INF01`) were found by source search. This does not prove equivalent behavioral tests are absent, but it means the task documents' named-regression-test condition is not evidenced by the current tree. Run the individual hosted packages and record source-pinned receipts only after the registry dependencies are accepted and their source digests match.

## Verification run

These local tests passed, each as a single Go package; the docs-chat suite also passed:

- `go test ./internal/router`
- `go test ./internal/redact`
- `go test ./internal/config`
- `go test ./internal/server/memory`
- `python3 -m unittest discover -s deploy/chat -p 'test*.py'` — 26 tests

No hosted/billing package build or live cloud/provider operation was run. No Ajent MCP tools or project-root `ajent.social` file were available, so the repository's additional Ajent feed poll could not be performed.
