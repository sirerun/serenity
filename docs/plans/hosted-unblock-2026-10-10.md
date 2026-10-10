# Hosted startup and latest-code deployment

## Scope and authority

Owner requested deployment of latest code and then authorized unblocking startup, qualification, signing and deployment. Baseline is main `6ba1fb184db249a347d3b80f8e3995fcbec9eacf`, including PR368 CIMD and PR367 token binding. This plan does not replace E23/E24 or the inspector plan; it coordinates their remaining release dependencies. Existing authority contracts and recovery holds remain applicable. Work executes in an isolated DGX worktree; production remains unchanged until admission, artifact verification and rollback gates pass.

## Acceptance

Latest reviewed code starts with authentic journal authority, survives restart and qualified recovery, serves authenticated owner memory without cross-account disclosure, and is deployed with verifiable artifact identity and a tested rollback. Source, synthetic local, provider, release and production evidence remain distinct.

## Tasks

- [x] T-HU.1 Refresh source and release prerequisites  Owner: coordinator  stage: discover  acc: [remote main includes PR368 and current hosted startup and signing blockers are identified]
- [x] T-HU.2 Prepare concrete AWS authority and signing proposal  Owner: coordinator  stage: design  deps: [T-HU.1]  acc: [proposal maps each owner obligation to a concrete authority and identifies costs and policy decisions without inventing runtime evidence]
- [x] T-HU.10 Bound journal object transport  Owner: coordinator  stage: implement  deps: [T-HU.1]  acc: [S3 reads and writes enforce the existing observation size ceiling with hostile length and allocation regression checks and independent review]
- [x] T-HU.3 Review authority proposal independently  Owner: independent-reviewer  stage: review  deps: [T-HU.2]  acc: [review names reachable correctness or authority blockers and every accepted finding has a disposition]
- [ ] T-HU.4 Record owner and policy decisions  Owner: founder  stage: approve  deps: [T-HU.3]  acc: [accountable owner and reviewed identity journal fencing and release trust policy are explicitly accepted]
- [ ] T-HU.5 Implement admitted startup and operator integration  Owner: startup-worker  stage: implement  deps: [T-HU.4]  acc: [real authority adapters and CLI lifecycle support ordinary restart and backup restore while invalid authority fails closed]
- [ ] T-HU.6 Qualify candidate and provider boundaries  Owner: coordinator  stage: verify  deps: [T-HU.5]  acc: [source pinned tests race vet lint synthetic browser and performance checks plus bounded provider fence backup and rollback receipts pass]
- [ ] T-HU.7 Review and land candidate  Owner: independent-reviewer  stage: review  deps: [T-HU.6]  acc: [independent exact head approval and guarded merge preserve holds and landed tree matches reviewed tree]
- [ ] T-HU.8 Produce authenticated release  Owner: coordinator  stage: release  deps: [T-HU.7]  acc: [latest landed archive checksum provenance and signature pass the accepted exact trust policy]
- [ ] T-HU.9 Deploy and accept existing host  Owner: coordinator  stage: deploy  deps: [T-HU.8]  acc: [existing host runs exact verified release and readiness login owner explorer and rollback evidence are recorded]

## Batches and scheduling

| Batch | Tasks | Owner | Dependencies | Boundary |
| --- | --- | --- | --- | --- |
| Discovery and design | T-HU.1 T-HU.2 | coordinator | None | Read-only provider observations and proposal |
| Independent design review | T-HU.3 | independent-reviewer | T-HU.2 | Review only |
| Owner decision | T-HU.4 | founder | T-HU.3 | Concrete reviewed authority and trust changes |
| Runtime integration | T-HU.5 T-HU.6 | startup-worker and coordinator | T-HU.4 | Reviewed contract determines file ownership and bounded parallel lanes |
| Delivery | T-HU.7 T-HU.8 T-HU.9 | reviewer and coordinator | T-HU.6 | Merge then signed release then existing-host deployment |

Current session selection is inherited by any worker and independent reviewer. Up to three compatible workers may run when shared dispatch and DGX capacity permit. No additional infrastructure spend is reserved. The release workflow is billing-blocked and no repository self-hosted runner exists. Alternative signing policy and any new charged authority infrastructure require a concrete owner decision before activation.

## Evidence and handoff

GitHub main and PR368 were refreshed live. Current CLI returns startup-unavailable unconditionally. Latest release is the older e98c382 candidate without a Sigstore bundle. Actions check114305187484 reports an account billing lock before execution. Preserve the October4 owner-decision packet and October7 preflight as historical input. New evidence supersedes only the claims it verifies. Local source and all other worktrees remain preserved. Ajent MCP is unavailable; coordination uses the existing repository file.

Owner response: "Me as owner; prepare the AWS design." This assigns accountability; it does not approve the proposed AWS trust policies or new spend. T-HU.10 is an independent existing-contract hardening prerequisite; it does not activate startup.

Design artifact: `hosted-aws-authority-proposal-2026-10-10.md`. T-HU.10 candidate passes focused race/vet/lint after regression tests failed on the unchanged baseline; independent source review is clear. No production runtime gate is closed by that patch.

Independent review at f89f404 is CLEAR for source-only landing; see `hosted-unblock-review-2026-10-10.md`. T-HU.3 closes direction-only review, not implementation freeze. T-HU.4 remains open for the proposed journal-backend and trust-policy decisions; T-HU.5-9 remain dependent and incomplete.
