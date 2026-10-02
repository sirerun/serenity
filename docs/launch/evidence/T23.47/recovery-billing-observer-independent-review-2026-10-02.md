# Independent review: Serenity recovery billing observer

**Disposition: HOLD candidate `1cbf6013c39a09bb58b959771e1de1196d76adf`.** The focused package race suite passes, but the episode resolver accepts conflicting same-second subscription state transitions that the frozen contract requires it to refuse. Two permanent regression controls also fail to detect redirect-policy removal and response-byte-limit removal. Do not clear the source-review hold until the episode logic and those controls are corrected and independently re-reviewed.

## Scope and evidence

Reviewed candidate `1cbf6013c39a09bb58b959771e1de1196d76adf` against baseline `c3d491b03980310d7d59688d092b8119640e5250` in a fresh full clone at `/Volumes/BuildOffload/worktrees/serenity-billing-observer-review-20261002-0955`. The external SSD was mounted and writable, with 753 GiB available. The clone is detached at the exact candidate commit and clean after review; the three production source files are byte-identical to that commit.

The candidate adds four files: `recovery_observer.go` (227 lines), `recovery_provider.go` (1,124 lines), `recovery_observer_test.go` (717 lines), and the 17-line T23.47 implementation receipt: 2,085 insertions total. SHA-256 of the three source files in the restored clone:

- `recovery_observer.go`: `1b5da92061924e006beeab7f03e0ceea944c069fab94f4c663941b85284ee760`
- `recovery_provider.go`: `93d556e2c6a38c4a5a54645b602227ff7a8cb0dd998c7f2c3f5891306824aefb`
- `recovery_observer_test.go`: `fa02d450ce1072453f2e87067a632adf6148ee8e8c3514d25ab7f910afa56d7b`

The review used the frozen development contract, contract freeze, and independent contract rereview in the completion-progress checkout. I also read the repository `AGENTS.md`, project `ajent.social`, and local Serenity coordination board. Ajent MCP tools were unavailable in this environment, so there was no live service-feed poll. The shared skill-library entrypoint was present, but its documented lazy-lookup script and local Go/team/review skill files were unavailable. No provider credentials, network requests, or external systems were used.

## Blocking implementation finding

`internal/hosted/billing/recovery_provider.go:752-795` collects only invoice-failure times and accepted active/trialing reset times. For subscription updates it records a reset only when the snapshot is active/trialing and the previous status was non-active/non-trialing. It discards other selected-subscription status snapshots. The final ambiguity check compares reset timestamps only with failure timestamps; it never detects conflicting status transitions at the same second.

A temporary same-package control built two Basil-shaped invoice failures around two selected-subscription updates at the same timestamp: `past_due → active` and `active → past_due`. The history has a later failure after that second. The contract requires a refusal because the same-second transition order is unknowable. `resolveEpisode` returned `nil` and accepted the later failure. The control failed with:

```text
conflicting same-second status transitions were not refused: <nil>
```

The exact temporary control source is preserved at `/Volumes/BuildOffload/serenity-recovery-observer-review-20261002-mutations/observer_independent_red_test.go` (SHA-256 `1809fd99600380610dbdb366d79e16a1fe7205b5624e801ec7ee2555f4b24787`). The same file also contains the additional bound and redirect controls described below. It was removed from the review clone after preserving it externally.

The implementation should retain enough validated status-transition evidence for the selected subscription to group relevant events by `created`. If a same-second group contains mutually conflicting status changes, resets, or applicable failures whose order changes episode interpretation, it must return the zero observation with `ErrBillingProviderAmbiguous`. Do not use opaque event IDs as chronology.

## Test-control gaps

The implementation currently has the required private redirect policy and byte enforcement, but the permanent controls do not prove those safeguards survive regression.

- **Redirects:** Removing `CheckRedirect` still left `TestRecoveryObserverNeverFollowsRedirect` passing. Its injected RoundTripper rejects a redirected host before the target can observe a request, so its target counter remains zero even if `http.Client` follows the redirect. A temporary direct `work.get` control retained the real client redirect callback while substituting a RoundTripper that records every request. It passed on the candidate and failed when redirect following was enabled. The permanent test should assert there is exactly one RoundTripper invocation or otherwise observe the attempted second request.
- **Response and aggregate bytes:** Removing the `len(body) > cap` / aggregate-limit check still left `TestRecoveryObserverEnforcesResponseAndAggregateByteCeilings` passing. Its “valid-prefix-extra” body is truncated inside an unfinished JSON string, so strict JSON parsing rejects it even without byte enforcement. A temporary direct `work.get` control used a complete JSON object followed by whitespace beyond the configured cap; it passed on the candidate and failed when byte enforcement was removed. Add this valid-prefix-plus-whitespace boundary case to the permanent suite. The current implementation's limit check itself is correct.

Mutation checks that do work in the permanent suite:

- Omitting the final local reread made `TestRecoveryObserverPreflightLockAndFinalReread/final_snapshot_detects_concurrent_status_change` fail with a nonzero observation.
- Collapsing provider history to only the latest invoice failure made `TestRecoveryPastDueScansUnrelatedPagesAndUsesEarliestFailure` fail with the later, incorrect grace time. This case also exercises 405 unrelated events plus relevant history across five pages.

All deliberate production-source mutations were restored. `recovery_observer.go` and `recovery_provider.go` match their recorded candidate SHA-256 values, and the review clone has no modifications.

## Validation

- Fresh focused baseline: `go test -race ./internal/hosted/billing` passed on the unmodified candidate (`9.546s`); uptime load was below 4.
- The independent conflicting-same-second control failed as described above.
- Final-reread and latest-invoice-only mutation tests failed as expected.
- The existing redirect and byte-limit tests passed under their corresponding deliberate regression mutants; stronger temporary controls passed on the candidate and failed under those mutants.
- The author receipt reports focused `go test -race`, `go vet`, and `golangci-lint` passing. Vet and lint were not rerun independently here. No full-repository or integration qualification was performed.

This review covers source safety and focused package behavior only. It does not establish live Stripe completeness, activation authority, integration wiring, or production qualification.

## Correction on skill lookup

The earlier note that local Go/team/review skills were unavailable was an incomplete lookup. I searched the wrong root and the documented lazy-lookup script was absent there. The skills are available at `/Users/dndungu/.agents/skills/.library/engineering/go/go/SKILL.md`, `/Users/dndungu/.agents/skills/.library/work/coordination/team/SKILL.md`, and `/Users/dndungu/.agents/skills/.library/engineering/git/pr/SKILL.md`; I read them before the third review. This was a lookup failure, not an environment limitation.
