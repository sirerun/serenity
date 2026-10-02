# Independent source rereview: recovery billing observer `f29d076`

**Disposition: HOLD `f29d076cac3af4ea59b9beb2730bb3fd90f7ff9c` pending the coordinator's cross-kind transition correction and a third source rereview.** The original `1cbf601` blocker is fixed. The corrected source still accepts a same-second `customer.subscription.created` reset and `customer.subscription.updated` snapshot with different selected current statuses. The coordinator has explicitly chosen the frozen contract's conservative policy: reject differing selected current outcomes within the same second across created and updated events, without claiming that provider event order is unknowable. Identical final active/trialing outcomes may be accepted when tests prove the reset anchor is stable.

## Scope and verification

Reviewed the exact candidate `f29d076cac3af4ea59b9beb2730bb3fd90f7ff9c` in a fresh full SSD clone at `/Volumes/BuildOffload/worktrees/serenity-billing-observer-rereview-20261002-1015`, compared with baseline `c3d491b03980310d7d59688d092b8119640e5250`. SSD was mounted and writable with 753 GiB available. The clone was detached at the exact candidate SHA and clean after temporary controls were removed. No live provider, network, credential, deploy, merge, or other external operation occurred.

The candidate adds four files, 2,259 lines: 227 observer source, 1,151 provider source, 854 focused tests, and a 27-line implementation receipt. SHA-256 in the clean clone:

- `recovery_observer.go`: `1b5da92061924e006beeab7f03e0ceea944c069fab94f4c663941b85284ee760`
- `recovery_provider.go`: `8ee98127de22f71ff6358c728a78e93e0dbc93248b0236aafbab2018ff9ef317`
- `recovery_observer_test.go`: `2bc738b0baa0f80a8758ceb1f03ff13b6f7b758267865a6336199829891e6c8e`

The review used the frozen development contract, contract freeze, independent contract rereview, the repository `AGENTS.md`, project `ajent.social`, and the local Serenity coordination board. Ajent MCP tools were unavailable, so no live service-feed poll was possible. The shared skill library was available, but its documented lazy lookup command and local Go/team/review skills were unavailable.

## Findings

The original `1cbf601` defect was that the resolver discarded non-reset selected-subscription updates and accepted conflicting same-second update transitions. `f29d076` now records selected current status and previous status, rejects multiple distinct update transition projections at one second, rejects an applicable failure tied to a selected status transition, and keeps identical duplicate transitions supported. Its permanent cases retain the positive reset-to-next-failure example, reject conflicting updates and failure ties, and accept identical duplicate resets.

A separate public-HTTP probe in this rereview clone put an earlier failure, a selected `customer.subscription.created` snapshot with status `active`, a selected `customer.subscription.updated` snapshot with status `past_due` (previous status `active`), and a later failure in one complete history. The created and updated events shared the same `created` second. The candidate returned a populated eligible observation anchored at the later failure. The exact probe source is preserved at `/Volumes/BuildOffload/serenity-billing-observer-rereview-20261002-evidence/created-reset-update-probe.go`.

I initially treated this as a potential ambiguous-order blocker, then considered the causal implication that a subscription must exist before an update. The coordinator has resolved the policy conservatively across event kinds: the observation will refuse differing selected current outcomes at one second rather than infer a cross-kind order for eligibility. The source correction should include `customer.subscription.created` in the projection comparison. Identical active/trialing final outcomes from created plus updated evidence can be accepted when they establish the same reset second and preserve the episode anchor; tests should cover this positive case. This policy decision is recorded as coordinator interpretation of the frozen contract, not a claim that provider event chronology is impossible to infer.

## Runtime controls

The corrected candidate's focused race run passed: `go test -race ./internal/hosted/billing -count=1` (`11.058s`; uptime load 5.20 before invocation).

I independently exercised the new regression guards with one controlled mutation at a time in the isolated clone. Each was restored byte-for-byte afterward:

- Disabling the conflicting-update guard made `TestRecoveryPastDueRefusesConflictingSameSecondTransitions` fail with a populated eligible observation.
- Disabling the response-byte check made `TestRecoveryObserverEnforcesResponseAndAggregateByteCeilings` fail because the valid JSON plus oversized whitespace was accepted.
- Removing the redirect callback made `TestRecoveryObserverNeverFollowsRedirect` fail because the synthetic redirected response was followed and returned successfully.

The clone's production files match the above candidate SHA-256 values, and `git status` is clean. The author receipt reports focused race, vet, and lint gates passing; vet and lint were not rerun independently in this review. No full-repository or integration qualification was performed.

The prior `1cbf601` review receipt remains at `/Volumes/BuildOffload/serenity-recovery-billing-observer-independent-review-20261002.md`; that candidate's hold remains historical and is not cleared by this rereview.

## Correction on skill lookup

The earlier note that local Go/team/review skills were unavailable was an incomplete lookup. I searched the wrong root and the documented lazy-lookup script was absent there. The skills are available at `/Users/dndungu/.agents/skills/.library/engineering/go/go/SKILL.md`, `/Users/dndungu/.agents/skills/.library/work/coordination/team/SKILL.md`, and `/Users/dndungu/.agents/skills/.library/engineering/git/pr/SKILL.md`; I read them before the third review. This was a lookup failure, not an environment limitation.
