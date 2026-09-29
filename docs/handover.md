# Handover -- 2026-09-28 (UTC), session claude-e24-coordinator (450f1c33)

## TL;DR
E24 (deep review 001 remediation, 42 rows): all 24 wave-1 PRs are merged to main with local verification; the four follow-on fills (T24.2/T24.11/T24.19/T24.22) were claimed but their cloud dispatch never started, and their claims were released at handover. Single next action: claim and cloud-dispatch those four tasks per docs/tasks/deep-review-001/T24.NN.md contracts, on glm-5.3-flash per David's ruling.

## Done & VERIFIED
- 24 E24 PRs merged (#282-#308): every branch rebased onto main and gated (build, vet, gofmt, go test -race on touched packages) before merge; the full merged tree passed `go test -race ./...` at wave-1 integration (one cross-PR failure found and fixed via #298's 4e9ced8). Evidence: PR bodies carry genuine-red + green verbatim; merge list in .claude/scratch/handover-inventory.md.
- PR #281 (docs/roadmap E24 in-flight entries, 3 commits) merged at handover; main = f66ed29.
- Founder merge-gate rulings applied: batch clearance; #296 docs-only (ACL change reverted in #307); #284's Caddyfile.cutover deleted; seven agent flags accepted (inventory, section "Merge-session rulings").
- Wave-1 integration worktrees and merged agent worktrees deleted (19 worktrees remain, all owned by other sessions' epics).

## Done but UNVERIFIED
- All 24 merged PRs are locally verified only: CI is billing-locked (T24.41, David-only). No CI run has seen any of this code. Verify by re-running CI once billing clears.
- The final-main `go test -race ./...` run was interrupted by a sibling session's lease (LOST R-build-lease to the t25-141 desk build); per-package gates all passed on the exact merge SHAs, but a single full-suite run on the exact final main (f66ed29) is NOT on record. How to verify: claim R-build-lease, `go test -race -count=1 ./...` on main.

## In flight
- Nothing running. No background processes, no cloud routines active, no kazi converges.
- The four fills (T24.2 unit hardening/rollback, T24.11 CI supply chain, T24.19 adversarial gate real pipeline, T24.22 forget history rewrite) are UNCLAIMED and UNSTARTED. Their contracts: docs/tasks/deep-review-001/T24.{2,11,19,22}.md; plan rows in docs/plans/deep-review-001-remediation.md (lines ~111, ~112, ~121, ~135).

## Blocked
- T24.41 (restore GitHub Actions billing): David-only, pending since 2026-09-27. Unblocks CI on all future PRs and the plan's verification honesty bar.
- T24.30, T24.31-T24.38, T24.39, T24.40: wait on the hosted-completion epic's (T23.x) merges; not this session's lanes.
- T24.42 (ACME path evidence): deps met (T24.1 merged) but needs access to the hosted host; belongs to whoever holds host access.

## Running processes left alive
- None (no kazi goals, no background shells, no cloud routines).

## Landmines & context
- The scratchpad directory /private/tmp/claude-501/.../f6e74724.../scratchpad from the earlier session is GONE (system announced it unavailable); scripts referenced in old notes (e24_integrate.sh etc.) are lost -- recreate from scratch if needed (they were throwaway).
- Cloud routine dispatch recipe: RemoteTrigger create with name e24-T24.NN, run_once_at 2027-01-01T00:00:00Z, persist_session false, job_config.ccr.environment_id=env_01PQjpqZ3GnUW38YkH8oFtbQ, git source https://github.com/sirerun/serenity, prompt template in the transcript (the T24.10 prompt in this session's history is the canonical shape: READ FIRST contract pointer, branch rules, genuine-red, gates, PR with ci-blocked+e24 labels, no merge). Then action:run with the returned trigger_id. David's 2026-09-28 ruling: new fills run on glm-5.3-flash (set model accordingly).
- Claim discipline: ~/.agents/skills/claim/scripts/claim.sh with CLAUDE_CODE_SESSION_ID=claude-e24-coordinator (or a new session id); only WON: counts; release with the CLAIM-REF SHA from `git ls-remote origin refs/claims/<task>`, never the branch SHA.
- Build lease: CLAIM_REMOTE=/Users/Shared/mini-build-lease.git .../claim.sh claim R-build-lease --purpose "..."; check `holder` first; hold if 1-min load > 10; never release a lease this session did not win.
- zsh footgun: avoid bare `=`-prefixed words (echo ====== fails).
- Shared scratchpads across sibling lanes collide on filenames; use task-unique names.
- The #296+#307 pair: internal/secrets is currently byte-identical to pre-T24.28 state and SEC-L08 is disclosed as "planned" in docs/threat-model.md; do not re-land the ACL change without Security.framework reads (David's ruling).
- `gh pr merge` can auto-delete a head branch and CLOSE a PR when the mergeable computation lags (#293's incident): if a merge fails with conflicts, do NOT delete the head branch; recover commits via the integration worktree or PR headRefOid, re-push, and open a replacement PR.

## How to resume
1. cd /Users/dndungu/Code/sirerun/serenity; git fetch origin; git checkout main; git pull --ff-only (main = f66ed29 or later).
2. Read this file, .claude/scratch/handover-inventory.md, docs/plans/deep-review-001-remediation.md, docs/roadmap.md (In flight), and the four contracts under docs/tasks/deep-review-001/.
3. Claim the four fills (/claim T24.2 T24.11 T24.19 T24.22 -- only WON counts), then dispatch one cloud routine per task per the recipe above (glm-5.3-flash).
4. As each PR opens: verify labels ci-blocked+e24, genuine-red in body, release the claim with the claim-ref SHA, then rebase-merge in dependency order with tree gates (T24.11 must not fight #305's release.yml edits; T24.22 must not fight #302/#293's writer/forget changes).
5. Remaining after the four: T24.42 (needs host), then the hosted-completion-gated verify rows; T24.39 closure last.
6. David's open items: T24.41 CI billing; FUN-02/T23.44 question due 2026-09-30 (tracked in the hosted epic).
