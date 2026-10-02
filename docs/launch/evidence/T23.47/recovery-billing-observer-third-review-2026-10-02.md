# Independent source review: recovery billing observer `cc44ad0`

**Disposition: CLEAR for the exact source-only candidate `cc44ad06f2160f30044a6c8b3be9a409214e563c`.** This clears only the new billing observer source and focused tests for the next delivery step. It does not qualify or authorize service assembly, activation, provider calls, merge, or full T23.47 acceptance. Earlier holds on `1cbf601` and `f29d076` remain historical and are not retroactively cleared.

## Scope and source identity

Reviewed `cc44ad06f2160f30044a6c8b3be9a409214e563c` against baseline `c3d491b03980310d7d59688d092b8119640e5250` in a fresh full SSD clone at `/Volumes/BuildOffload/worktrees/serenity-billing-observer-third-review-20261002-1030`. The SSD was mounted and writable with 753 GiB available. The clone is detached at the exact candidate SHA and clean after all temporary mutations were restored.

The candidate adds 2,328 lines across five files: `recovery_observer.go` (227 lines), `recovery_provider.go` (1,158 lines), `recovery_observer_test.go` (898 lines), the 27-line implementation receipt, and the 18-line coordinator-policy record. SHA-256 of production source in the clean clone:

- `recovery_observer.go`: `1b5da92061924e006beeab7f03e0ceea944c069fab94f4c663941b85284ee760`
- `recovery_provider.go`: `101e6c7cc80c52e73b53fea820ed362285c37c6af06d7d37471111d63efa0f41`
- `recovery_observer_test.go`: `39c796e8d0bb3fefde79d903cc6b28b24ceafcaa31c527c25b76c4c31d5a4dd4`

I reviewed the frozen development contract, contract freeze, contract rereview, the coordinator-policy record, repository `AGENTS.md`, project `ajent.social`, and the local Serenity coordination board. Ajent MCP tools were unavailable, so I could not poll the live service feed. The Go, team coordination, and Git PR review skills were read from `/Users/dndungu/.agents/skills/.library/` before this third review. Earlier receipts now carry a correction explaining that the first skill lookup searched the wrong root.

## Findings

The remaining cross-kind ambiguity is fixed. The resolver groups selected subscription current statuses by event second across both `customer.subscription.created` and `customer.subscription.updated`; a differing status at one second returns ambiguity before deriving grace. Same-second created/updated active and trialing outcomes are covered as positive controls and preserve the post-reset failure anchor. The separate update-transition projection continues to reject distinct update transitions at one second, and failure ties remain refused. The original same-second update conflict, latest-invoice-only, final-reread, redirect, and response-byte findings are addressed.

The public-HTTP test for a created active snapshot tied to a past-due update now returns the zero observation and `ErrBillingProviderAmbiguous`. An intentional omission of the new current-status guard made that test fail with a populated eligible observation, confirming the guard is necessary. The exact original boundary probe source remains at `/Volumes/BuildOffload/serenity-billing-observer-rereview-20261002-evidence/created-reset-update-probe.go`.

## Validation

Fresh focused validation on the unmodified candidate: `go test -race ./internal/hosted/billing -count=1` passed (`11.032s`; uptime load 2.71 before invocation).

With one deliberate mutation at a time in the isolated clone, these permanent controls failed as intended; every source mutation was restored byte-for-byte:

- Omitting the created/updated current-status guard failed `TestRecoveryPastDueRefusesCreatedResetTiedToConflictingUpdate` with a nonzero eligible observation.
- Removing redirect refusal failed `TestRecoveryObserverNeverFollowsRedirect` because the redirected synthetic response was followed.
- Removing response and aggregate byte enforcement failed `TestRecoveryObserverEnforcesResponseAndAggregateByteCeilings` because valid JSON plus oversized whitespace was accepted.
- Collapsing full event history to the latest invoice failure failed `TestRecoveryPastDueScansUnrelatedPagesAndUsesEarliestFailure` with the wrong later grace anchor.
- Omitting final SQL reread failed `TestRecoveryObserverPreflightLockAndFinalReread/final_snapshot_detects_concurrent_status_change` with a nonzero observation.

The author/coordinator receipt also reports focused vet and lint success. I independently reran the focused package race suite but did not rerun vet or lint. No full-repository integration gate, live Stripe completeness, assembly wiring, or production activation was established here.
