# Hosted freeze recovery inventory — 2026-10-01

## Finding

The freeze work is recoverable from preserved source and evidence. PR #274 is merged; PR #273 is an open draft and must remain held pending recovery and review. The three preserved legacy worktrees are clean. They contain useful unmerged documentation and T23.44 implementation, but none of those commits by itself establishes hosted acceptance.

## GitHub state checked read-only

- [PR #273](https://github.com/sirerun/serenity/pull/273), “Harden hosted backup and restore integrity (T23.49)”, is open and draft. Its head is `f41b88214356d13cc6914738afeaa95c17fef8ea` (`hosted/t23-49-20260924`), based on `d0502ff1cf9adf45349a1998ca066d5e9eee9814`; 23 commits and 26 changed files. GitHub returned no issue comments, review comments, or reviews for this PR. This means no trusted hold/lift was visible in those PR surfaces during this read; draft state itself remains a merge stop. Checks include failures for `eval-cached`, `gbrain-protocol-conformance`, `lint`, and `test`; crossbuild checks are cancelled. Do not treat this head as green or accepted.
- [PR #274](https://github.com/sirerun/serenity/pull/274), “Strengthen hosted billing lifecycle recovery (T23.47)”, is merged (2026-09-24) at `8dd43a61a0edf4665b1abe8315cbf5a9e8ba4d88` (`hosted/t23-47-20260924`), based on `b4febdf7bbc0d3c33f9939c79099dc64cce89e84`; 37 commits and 17 changed files. GitHub returned no issue comments, review comments, or reviews for this PR. The body describes recovery for the 72-hour grace deadline tied to the exact first invoice-failure event/latest invoice, a separate persisted billing-period projection, and immutable checkout request identity with recoverable provider session. This is merged code evidence, not proof of task receipt/acceptance.
- PR #273's changed files add T23.48 integration request and T23.49 evidence, manifest-v2 restore validation/staging, and production deletion-journal wiring. The evidence needs a fresh source-pinned receipt against the current base after the failures are resolved; cached PR body/CI claims do not replace that.

## Preserved local work

All three named legacy worktrees were read-only inspected and are clean (`git status --porcelain` empty):

- `serenity-launch-coordinator-20260918`: six commits ahead of its recorded `origin/main`, all launch/hosted measurement and coordination docs (`988a7b0` through `4d9ebc6`). Preserve these constraints as review inputs; they are not implementation completion.
- `serenity-operation-wiring-20260921`: one commit ahead, `41c3675` (`docs: refresh repository overview and hosted status`).
- `serenity-t23-44-20260921`: four commits ahead, including `92082c6` operation ledger/staging implementation, `a48dc15` checked ledger-close errors, and T23.44 evidence commits. The source-pinned T23.44 result at `4c98269` is explicitly `PARTIAL`: 8/8 local cases pass, while the production canonical checker is pending, the physical storage ceiling lacks an OS-enforced filesystem and allocated-byte measurement, and gateway ledger/canonical-operation-marker wiring is blocked. Do not infer those gaps closed from PR #273's deletion journal work; deletion journaling and operation billing ledger are distinct seams.

The project coordination record reports ownership transfer of the Integrator 41/57 lane into this recovery lane. The old local worktrees and their claims were left untouched. The report does not rely on unverified Ajent hold/lift text as authority; current PR review/comment surfaces showed no hold comments, while PR #273 remains draft.

## Exact open gaps

1. **Operation schema and recovery:** T23.44 ledger/staging code exists in the preserved branch, but its evidence identifies no production gateway wiring, canonical operation markers, or production canonical-checker integration. Establish the current branch/base relationship and reconcile this implementation before attempting integration; do not transplant the old branch wholesale.
2. **Deletion journal and backup:** PR #273 contains deletion-journal and manifest-v2 work but remains draft with failed checks. Verify migration/schema compatibility against current main and the T23.48 integration boundary, then run the founder-authorized local equivalents and regenerate source-pinned evidence. Confirm journal storage immutability, adapter behavior, and restore completeness from the actual merged candidate and tests.
3. **Billing lifecycle:** PR #274's billing recovery implementation is merged, but its old T23.47 receipt/status remains a separate acceptance artifact. Recheck current task dependencies and source-pinned receipt; verify retry/idempotency and billing projection integration on current main. Do not reopen or alter the merged PR.
4. **Physical staging bound:** no preserved T23.44 evidence proves an OS-enforced quota or measured allocated-byte ceiling. This requires an infrastructure decision and deployment evidence, not another in-process accounting test.

## Safe recovery sequence

1. Keep PR #273 in draft. Reconcile its base and changed files against current main and the frozen task/dependency record; do not push or merge during the inventory.
2. Recover T23.44 from its clean historical commits by review/cherry-pick planning only. First settle the canonical-writer/shared-seam ownership and migration version, then integrate the ledger and canonical markers with gateway code and canonical-checker behavior.
3. Resolve the physical staging filesystem/quota and capture allocated-byte measurements before changing its evidence from blocked/partial.
4. Review PR #273's journal/backup schema changes against the selected migration and restore contract; resolve implementation blockers in its owned branch and run exact package tests plus the founder-authorized local equivalents. Regenerate receipts pinned to final code SHAs.
5. Audit the merged T23.47 code against its current dependencies and produce a fresh source-pinned receipt. Keep billing and journal evidence independent.
6. Only after these artifacts are reviewed should the coordinator update the shared task/roadmap status or reconsider the draft PR. No provider/billing calls are part of this sequence absent a separately authorized test plan.

## Coordinator continuation evidence

PR325 merged at `42f6dfd8615b193fffb797175daf4b5f5d02dc94`; its tree exactly matches locally reviewed `1dd8d57c892f0a1f0684be68b9956a4f2213e8fc`. Legacy recovery starts from this baseline.

The Oct1 fresh check annotations for PR273 test run `107884501700` and lint run `107884501702` explicitly say the jobs never started because the account is locked for billing. These failures are infrastructure evidence, not executed test failures. David authorized local validation instead of paid Actions; recovery does not require billing restoration. Real draft/review and correctness blockers still apply.

On this baseline, `go test -race -tags hostedtest -count=1 ./internal/hosted/contracts/... ./internal/hosted/testhooks/...` passed all three packages. The shared build lease was acquired, verified and released in the same foreground shell. This verifies the tagged seam tests; it does not establish assembled crash-recovery acceptance.

The Sept24 Integrator41/57 transfer is recorded in the root project channel. Its unchanged expired assembly/schema claims were released through the canonical compare-and-swap primitive, briefly reacquired for this inventory checkpoint, then released. Unrelated claims and all legacy source remain preserved.
