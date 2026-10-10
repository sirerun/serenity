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
- [x] T-HU.4 Record owner and policy decisions  Owner: founder  stage: approve  deps: [T-HU.3]  acc: [accountable owner and reviewed identity journal fencing and release trust policy are explicitly accepted]
- [x] T-HU.5 Implement admitted startup and operator integration  Owner: startup-worker  stage: implement  deps: [T-HU.12, T-HU.13, T-HU.14, T-HU.15, T-HU.16]  acc: [real authority adapters and CLI lifecycle support ordinary restart and backup restore while invalid authority fails closed]
- [ ] T-HU.6 Qualify candidate and provider boundaries  Owner: coordinator  stage: verify  deps: [T-HU.5, T-HU.4, T-HU.17, T-HU.19, T-HU.20]  acc: [source pinned tests race vet lint synthetic browser and performance checks plus bounded provider fence backup and rollback receipts pass]
- [ ] T-HU.7 Review and land candidate  Owner: independent-reviewer  stage: review  deps: [T-HU.6]  acc: [independent exact head approval and guarded merge preserve holds and landed tree matches reviewed tree]
- [ ] T-HU.8 Produce authenticated release  Owner: coordinator  stage: release  deps: [T-HU.7]  acc: [latest landed archive checksum provenance and signature pass the accepted exact trust policy]
- [ ] T-HU.9 Deploy and accept existing host  Owner: coordinator  stage: deploy  deps: [T-HU.8]  acc: [existing host runs exact verified release and readiness login owner explorer and rollback evidence are recorded]

- [x] T-HU.11 Record architecture implementation approval  Owner: founder  stage: approve  acc: [owner explicitly approves DynamoDB journal authority and KMS release signing implementation while paid resources and live trust activation remain separately gated]
- [x] T-HU.12 Freeze detailed source contracts  Owner: coordinator-and-independent-reviewer  stage: design  deps: [T-HU.11]  acc: [identity process binding epoch transactions complete backup cuts and strict signing contracts have independent source-freeze review]
- [x] T-HU.13 Implement transactional authority and gateway  Owner: authority-worker  stage: implement  deps: [T-HU.12]  acc: [real DynamoDB adapter and authenticated gateway enforce atomic epoch head issuance and immutable idempotent journal mutations with bounded reads and hostile tests]
- [x] T-HU.14 Implement runtime broker and startup admission  Owner: runtime-worker  stage: implement  deps: [T-HU.12, T-HU.13]  acc: [root broker binds authenticated instance and measured process to exact admitted startup lease with replay before handlers and join before revoke]
- [x] T-HU.15 Implement exact backup and recovery authority cuts  Owner: coordinator  stage: implement  deps: [T-HU.12, T-HU.13]  acc: [backup and restore bind independently witnessed complete cuts and refuse partial stale or mismatched authority snapshots without widening recovery admission]
- [x] T-HU.16 Implement strict KMS manifest tooling  Owner: release-worker  stage: implement  deps: [T-HU.12]  acc: [real crypto signing and offline pinned verification bind canonical manifest archive source and explicit deployment approval with no Sigstore fallback]
- [ ] T-HU.17 Prepare exact activation policy and cost dossier  Owner: coordinator  stage: design  deps: [T-HU.13, T-HU.14, T-HU.15, T-HU.16]  acc: [reviewed unexecuted infrastructure and policy changes name bounded resources costs trust roots rollback and qualification calls for owner activation approval]

- [x] T-HU.18 Review source implementation for landing without activation  Owner: coordinator-and-independent-reviewer  stage: review  deps: [T-HU.5, T-HU.13, T-HU.14, T-HU.15, T-HU.16]  acc: [source build tests race vet lint and independent scoped exact-head reviews pass with no outstanding source finding while provider qualification remains open]
- [ ] T-HU.19 Design preserved legacy-data migration  Owner: coordinator-and-founder  stage: design  deps: [T-HU.17]  acc: [existing data and deletion history have a reviewed quiescence backup and adoption or separate-environment decision before any fresh enrollment or production replacement]

- [ ] T-HU.20 Implement and review preserved legacy admission  Owner: coordinator-and-independent-reviewer  stage: implement  deps: [T-HU.19]  acc: [immutable truthful checkpoint provenance complete custody inventory measured first admission strict owner transport and matching deployment data paths pass local checks and exact-head review before provider use]

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

## Approved implementation continuation

On 2026-10-10 the founder stated: "Approve DynamoDB journal authority and KMS release signing. Continue implementation." T-HU.11 records that new direction authority. T-HU.4 retains the exact identity/journal/signing policy acceptance requirement for live activation; it is not silently closed by architecture approval. The owner explicitly permits contract and code implementation before that activation decision, so T-HU.5 now depends on detailed contract and component completion, and live T-HU.6 additionally requires T-HU.4 and the exact activation dossier. All earlier task IDs, completed source evidence and deployment gates are preserved.

Implementation batches: coordinator owns canonical plan, detailed integration contract, module files, backup integration and final composition; authority worker owns new authority/gateway packages and tests; runtime worker owns new broker/startup files and designated service/CLI changes after seam freeze; release worker owns release-manifest package and new tooling, with existing verifier changes staged behind explicit policy selection. Workers use isolated DGX worktrees, inherit the session selection, and wait for reviewed seam freeze before source edits. Independent reviewers own external receipts only. No provider resources or live policies are activated by these batches.

Detailed source freeze: see hosted-aws-contract-freeze.md and the three immutable contract copies. T-HU.12 closes design review only. Implementation and all qualification gates remain open.

## Source-only landing and live migration boundary

T-HU.18 permits the independently reviewed implementation to land before live
qualification, as explicitly authorized by the owner. It does not close or
weaken T-HU.6 or T-HU.7: the final deployable candidate still needs provider,
recovery, rollback and acceptance receipts before release. Source component
completion means implemented and locally verified behavior, not AWS operation.

Read-only inventory confirms existing production accounts and ready brains.
T-HU.19 records the resulting migration dependency rather than treating that
data as a fresh namespace. The activation dossier describes the preserved
checkpoint and history-provenance decision. No empty-data bypass is permitted.

Source implementation and scoped independent review are complete through
b4510adb947f8e0cc5a69101f9fdb8c32d72c937; see
hosted-aws-source-evidence-2026-10-10.md for exact verification scope and resolved
findings. T-HU.5 and T-HU.13-16 close source implementation only. T-HU.18 closes
source-only review; guarded merge and landed-tree verification are recorded in
the delivery receipt. T-HU.4, T-HU.6-9, T-HU.17 and T-HU.19 remain open. The
activation dossier is an unexecuted reviewed proposal, not live policy approval
or a completed bounded provider qualification script.

## Owner-authorized live activation continuation

On 2026-10-10 the founder stated: ‘I approve. Deploy.’ This accepts live activation of the reviewed DynamoDB journal and KMS signing direction, necessary preserved-data migration and deployment. Initial qualification spending remains bounded by the proposed $10 allowance; no unrelated resources are authorized. Activation is limited to the existing host, one authority stack, owner-only signing and independently verified artifacts. Provider qualification, immutable prewrite checkpoint, truthful legacy-history provenance and tested recovery remain mandatory acceptance gates.

T-HU.19 implementation assignment: legacy-migration worker owns witnessed adoption receipt, deterministic complete data inventory and first-admission checks. Coordinator owns CLI and preserved checkpoint. Owner-activation worker owns current-user owner transport with strict private-file checks; restore operator remains root-only. Source changes require independent exact-head review before landing.
