# Serenity: RFC 0001 to code complete, then the hosted launch

Plan of record. Split layout: each epic lives in its own file (`docs/plans/` for E0-E22, `docs/launch/hosted-plan.md` for E23); this file holds context, scope, the TOC, open waves, milestones, risks, and procedure. Living status is in `docs/roadmap.md`. Trimmed 2026-09-11: every fully shipped wave and epic was removed from this file; their task-level detail stays untrimmed in the epic files and in `docs/roadmap.md` Shipped.

## 1. Context

Serenity is a claim-based personal memory and direction system: a Go single binary that ingests what one person produces, reconciles claims in a git-canonical brain repo, serves them plus the person's precepts to agents over three open protocols (MEMORY_VERBS v1 adopted from gbrain, DISPOSITION v1, DIRECTION v1), and gates every consequential change through human disposition. `docs/rfc/0001-serenity.md` (v2.2) is opinionated on formats, policy shapes, and protocol envelopes, and orders work as scope-gated milestones M0-M6.

Status as of 2026-09-11: M0-M4 are exit-verified (E0 13/13, E1 37/37 except the founder-only T1.23, E2 23/23, E3 17/17, E4 21/21 with T4.17 complete 2026-09-08). E5 is 11/15 (naming T5.6, the real-mailbox run T5.12, the name-dependent README T5.14, and the release gate T5.20 remain). The verification remediation epics E9-E21 all completed 2026-09-08. E22 (final import timing) stays open with two retained production timing failures. E7 launch polish is 4/6 (content packet cross-site link T7.4 and the human go/no-go T7.6 remain). The public website is live on GitHub Pages at https://serenity.sire.run with the docs-chat Lambda; v0.1.1 is the latest release.

New scope 2026-09-11 (David): **E23, the paid hosted launch.** A person signs up at serenity.sire.run, receives a private brain, connects the Rakazo adapter (elie222/rakazo PR 835) over HTTPS, uses durable semantic memory, and buys a monthly plan. The RFC's section 6 non-goal "multi-user, hosted service" is superseded for the product by this decision; the engine invariants are unchanged (one writer per brain, file-first, authenticated endpoints). Founder-ratified inputs: Resend magic-link identity (ADR 015); one ARM VM on AWS us-west-2 with EBS and S3 under a USD 60 per month ceiling (ADR 014); plan configuration v1 (Free 0 / Builder 19 / Scale 49 USD) ratified and published with the app (ADR 016); the launch-driving session merges the PR stack #220-#225 and lands `1dfecc8`. Everything E23 needs is in `docs/launch/hosted-plan.md` and `docs/tasks/T23.*.md`, written so a low-reasoning executor can drive it without asking.

Objectives (unchanged for E0-E5): every RFC section 17 acceptance criterion for M0-M5 green on a laptop and recorded in `docs/evals/m<N>-report.md`; every load-bearing invariant enforced by a test or CI gate; the CLI as the first conformant client of all three protocols. Objective for E23: the acceptance matrix in `docs/launch/hosted-acceptance.md` complete, a reproducible service that can be operated and rolled back, and an honest launch report.

Constraints and assumptions:
- Decisions by David on 2026 08 27 (ADR 001-010), 2026 08 30 (ADR 011), 2026 09 02 (ADR 012), 2026 09 05 (ADR 013), 2026 09 11 (ADR 014-016).
- Standard-library Go in the engine; focused third-party deps only at the connector and provider edge (ADR 003). The Stripe SDK is a provider-edge dependency for E23 wave 3 (ADR 016).
- All code changes happen in per-task worktrees on `/Volumes/BuildOffload`, one PR per task, rebase-and-merge, no Claude attribution. Heavy builds take the mini build lease. The internal disk was at 5.6 GB free on 2026-09-11; preflight `df -h /System/Volumes/Data` before every build.
- `kazi` is on PATH: every engineering task carries an `acc:` predicate; design-heavy tasks are marked `lane: agent`.
- Merge gate: a green PR is not clearance to merge; the lead merges, or a session merges with explicit clearance. For E23 the executor holds standing clearance for the #220-#225 stack and `1dfecc8` only.

Success metrics: E0-E5 checklist fully checked; v1.0.0 tagged under the decided name; for E23, MS-H5 (launch-ready) reached with the matrix complete, and MS-H6 (launched) on David's go.

## 2. Discovery summary

Original discovery (2026 08 27, tree at `13dc0d2`): 46 use cases (UC-001..UC-046), gbrain pin `d35c9c9e441e`, dira pin `15686940aa08`; details in ADR 008/009. UC-047/UC-048 added 2026-09-05 (OpenRouter). Hosted-launch discovery (2026-09-11) is recorded in `docs/launch/hosted-plan.md` section 2: repository and PR state, the server, MCP, secrets and storage as they exist, the Rakazo client wire contract, the website's hosting, the absence of any tenancy or billing code, and the reusable `deploy/chat` pattern. UC-049..UC-071 added for E23. Manifest: `.claude/scratch/usecases-manifest.json`.

## 3. Scope and deliverables

In scope: E5's remaining tasks, E6 outline, E7's two remaining tasks, E22, and E23 (waves 0-6). ADRs 001-016. Protocol documents and schemas. Eval corpora and reports. The docs site. The v1.0.0 release and the name decision. The hosted service on `app.serenity.sire.run`.

Out of scope: Flutter app, Graph email, ANN index, multi-principal enforcement inside one brain, and everything E23 lists as excluded (enterprise SSO, team RBAC, multi-region, marketplace, general chat, connector catalog, public knowledge sharing, model training, annual billing, overage charging).

| ID | Deliverable | Owner | Acceptance |
|---|---|---|---|
| D-E5 | gbrain import, docs site, adversarial gate, BrainBench trend, name, v1.0.0 | pool + David | RFC M5 AC, `docs/evals/m5-report.md` |
| D-E7 | Launch polish and adoption loop for the static site | pool + David | `docs/launch/checklist.md` gates |
| D-E22 | Import timing failure diagnosed and resolved | pool | `docs/plans/E22-import-budget-investigation.md` |
| D-E23 | Paid hosted service, launch-ready then launched | executor + David | `docs/launch/hosted-acceptance.md` complete; `docs/launch/hosted-release.md` |

## 4. Checkable work breakdown

### E0-E4 -- SHIPPED (M0-M4 exit-verified). Task detail: `docs/plans/E0-m0-residuals.md`, `E1-m1-ingest.md`, `E2-m2-reconcile.md`, `E3-m3-direction.md`, `E4-m4-serve-protocols.md`; never trimmed there. Only T1.23 (founder-only) remains open in E1.
### E5 -- M5: migration + launch  -> docs/plans/E5-m5-migration-launch.md  (11/15; T5.6, T5.12, T5.14, T5.20 open)
### E6 -- M6: hardening soak (outline, post-code-complete)  -> docs/plans/E6-m6-hardening.md  (0/1)
### E7 -- Launch polish and adoption loop  -> docs/plans/E7-launch-polish.md  (4/6; T7.4, T7.6 open)
### E9-E21 -- Verification remediation epics  -> docs/plans/E9-*.md .. E21-*.md  (all complete 2026-09-08; trimmed from this file)
### E22 -- Final import performance investigation  -> docs/plans/E22-import-budget-investigation.md  (0/1)
### E23 -- Hosted Serenity launch  -> docs/launch/hosted-plan.md  (0/40; executable; contracts in docs/tasks/T23.*.md)

Decisions confirmed by David on 2026 08 27 (OD-1..OD-4) stand; see ADR 005 and ADR 010. Decisions on 2026 09 11 for E23 are in ADR 014-016.

### Open waves

Task lines mirror the epic files (ids resolve there). Shipped waves are trimmed.

### Wave 1d: E1 exit (1 open)
- [ ] T1.23 M1 exit verification: real Gmail 30 days + 5 repos on a laptop; publish per-family P/R/F1 and contradiction recall (human, David)

### Wave 5b: E5 name, benchmarks (2 open)
- [ ] T5.6 Name decision executed: rename module, binary, CLI strings, protocol $ids, brew formula; `serenity` alias shim for one release (human)
- [ ] T5.12 Large-brain manual run: import >= 10K real messages on a laptop, resumable, within 4h; record it (human)

### Wave 5c: E5 README, code-complete gate (2 open)
- [ ] T5.14 README + docs lead with plan-check (the wedge): first demo is a plan rejected against a precept; gbrain lineage credited; MEMORY_VERBS conformance badge
- [ ] T5.20 Code-complete gate: every AC line of E0-E5 checked, evals reports present, release v1.0.0 tagged and brew formula published, repo made public (human)

### Wave 6: E6 planning task (1 open)
- [ ] T6.0 PLAN: expand E6 to executable fidelity (informed by E0-E5 learnings and the shipped metrics surface)

### Wave 7: E7 remaining (2 open)
- [ ] T7.4 Launch content packet (cross-site link deployment remains)
- [ ] T7.6 Launch-readiness gate for the static site (David)

### Wave 22: import timing (1 open)
- [ ] T22.1 Diagnose and resolve the final import timing failure

### E23 waves (40 open; full rows, acc lines, deps and steps in docs/launch/hosted-plan.md)
- Wave 0, reconcile and measure: T23.1 merge the PR stack; T23.2 land `1dfecc8`; T23.3 topology and cost spike; T23.4 control DB and hosted store.
- Wave 1, vertical slice local: T23.5 provisioning state machine; T23.6 client credentials; T23.7 hosted MCP gateway; T23.8 brain runtime pool; T23.9 magic-link identity and sessions; T23.10 dashboard Connect Rakazo; T23.11 `serenity hosted serve` and readiness; T23.12 slice proof harness.
- Wave 2, public deployment: T23.13 CloudFormation; T23.14 host provisioning and pinned deploy; T23.15 DNS and email domain; T23.16 Stripe test-mode objects; T23.17 first public deploy and smoke.
- Wave 3, quotas and billing: T23.18 plan config v1 and entitlement; T23.19 atomic reservations; T23.20 gateway enforcement; T23.21 Checkout, portal, webhooks; T23.22 billing lifecycle tests; T23.23 dashboard usage and billing.
- Wave 4, durability and ops: T23.24 export; T23.25 delete with retention; T23.26 backups and rehearsed restore; T23.27 crash tests; T23.28 runbook; T23.29 observability and redaction.
- Wave 5, journey and proof: T23.30 site CTA and pricing; T23.31 Rakazo instructions; T23.32 ten timing runs; T23.33 human walkthroughs (David); T23.34 memory-quality proof.
- Wave 6, qualification and release: T23.35 security battery; T23.36 load and cost re-projection; T23.37 release packet; T23.38 live Stripe cutover (David); T23.39 go/no-go (David); T23.40 PLAN re-groom of waves 4-6 after T23.17.

## 5. Parallel work

| Track | Tasks | Sync point |
|---|---|---|
| J: migration + launch | T5.6, T5.12, T5.14, T5.20 | T5.20 |
| K: static-site launch | T7.4, T7.6 | T7.6 |
| L: import timing | T22.1 | E22 |
| H: hosted launch | E23 tracks A-H (see the epic file section 5) | MS-H2, MS-H5, MS-H6 |

E23 runs on at most two executor lanes on the mini because of the build lease; E5/E7/E22 tasks are founder-gated or independent and may proceed alongside. Task claims (`refs/claims/T*`, the /claim skill) are the only coordination mechanism; a session that finds nothing claimable checkpoints and exits rather than idle-polling. Worktrees are per task on `/Volumes/BuildOffload/wt/serenity-<task>`; below 20 GB free on the internal disk, do not start another build.

## 6. Timeline and milestones

Scope-gated, not date-gated (RFC section 17), except E23, which targets a seven-day sequence with day estimates per wave.

| ID | Milestone | Depends on | Exit criteria |
|---|---|---|---|
| MS0-MS4 | Substrate through serve + protocols | done | exit-verified, see epic files |
| MS5 | Migration + launch = code complete | MS1, MS4 | RFC M5 AC recorded; adversarial gate green; v1.0.0 public under the decided name |
| MS-H0..MS-H6 | Hosted launch (base reconciled, slice local, slice public, sellable in test mode, operable, launch-ready, launched) | see `docs/launch/hosted-plan.md` section 6 | matrix rows PASS per milestone; David's go for MS-H6 |

## 7. Risk register

| ID | Risk | Impact | Likelihood | Mitigation |
|---|---|---|---|---|
| R6 | Cloud eval spend runs away in CI | Budget | Medium | Cached outputs per push; nightly live runs behind a USD cap |
| R7 | The rename lands after transcripts and $ids freeze | Double re-freeze | Medium | Decision at M4 exit, rename in T5.6 before release transcripts freeze (ADR 010) |
| R9 | Scope gravity: in-place claim edit or private daemon path sneaks in | RFC section 18.7 | Low | Drift tests, edit_accept only via writer, README leads with plan-check |
| R11 | E22 timing failure masks a real regression | Release quality | Medium | Baseline never advanced on a failing run; controlled-runner isolation next |
| RH1-RH7 | Hosted launch risks (Resend, embedding cost, stack semantics, single node, Rakazo drift, published prices, disk) | see epic | see epic | `docs/launch/hosted-plan.md` section 7 |

## 8. Operating procedure

Definition of done for a task, all required:
1. Tests written and green: unit or property tests for every implementation task; an API test hitting the real boundary for every protocol, HTTP route or CLI `--json` surface; a browser test (Playwright or agent-browser) covering the golden path and one edge for every user-facing UI change (E7 site, E23 dashboard and site).
2. `make lint`, `make vet`, `gofmt` clean; `go test -race ./...` green locally (under the build lease) and in CI.
3. PR merged to main via rebase with CI green; docs updated in the same PR; roadmap line updated; for E23 also `docs/launch/hosted-status.md` and the acceptance row.
4. For release-shaped or production-facing tasks: the tag fired the release workflow and the artifacts were observed; for E23, verified live on `app.serenity.sire.run`, not only in tests.
5. Reported honestly in `docs/roadmap.md`: observed outputs, skipped steps named.

Rules: one worktree per task on `/Volumes/BuildOffload/wt/serenity-<task>`; small logical commits; never commit files from different directories in one commit; `acc:` predicates drive kazi's lane, `lane: agent` tasks go to a frontier subagent; every kazi friction point becomes an issue at kazi-org/kazi; claims are the only cross-session coordination.

## 9. Progress log

- 2026-09-11 Hosted launch groom: added E23 (40 tasks, 38 with acc:, 5 lane: agent, 4 founder-gated kind: human/any, 1 kind: plan) in `docs/launch/hosted-plan.md` with `docs/launch/hosted-status.md` and `docs/launch/hosted-acceptance.md`; ADR 014 (tenancy topology), ADR 015 (Resend magic link), ADR 016 (plans, metering, Stripe) written; UC-049..UC-071 added; contracts `docs/tasks/T23.{1,2,4,5,6,7,8,9,10,11,12}.md` written. Trim pass: shipped waves 1a-1c, 1e, 2a-2d, 3a-3d, 4a-4c, 5a, 9 and epics E9-E21 removed from this file (detail preserved in epic files and roadmap Shipped); stale checkboxes T2.17 and T4.17 synced to shipped; progress-log history before this entry removed (in git history and roadmap).

## 10. Hand-off notes

- Read the RFC first for E0-E6; read `docs/launch/hosted-plan.md` first for E23. ADRs record every deviation or refinement.
- External contract pins: gbrain `d35c9c9e441e`, dira `15686940aa08`, Rakazo adapter `ef63e354` (PR 835).
- The repo is public; no secrets exist in it and none may be added. Hosted secrets are named, never valued, in docs.
- kazi bus team name for this repo: `-users-dndungu-code-sirerun-serenity`.
- When delegating research to a background agent, verify it is alive and delivered (ListAgents, the scratch file) before trusting a hand-off; a 2026-09-05 agent silently redid the wrong task.

## 11. Appendix

- RFC: `docs/rfc/0001-serenity.md`
- ADRs: `docs/adr/001` Gmail IMAP; `002` code-complete boundary; `003` dependency posture; `004` writer queue, pending records, id width; `005` eval labeling, voice notes to M2; `006` `serenity cron`, x/term TUI; `007` reconcile constants; `008` precepts on dira, applies_when body block; `009` gbrain import mapping; `010` check exit codes, events per transport, token rotation, docs toolchain, name timing; `011` milestone gates mechanical; `012` embedded read facade, writes single-process; `013` OpenRouter default provider; `014` hosted tenancy topology and binding chain; `015` hosted identity via Resend magic link; `016` versioned plans, atomic metering, Stripe-hosted billing.
- Use-case manifest: `.claude/scratch/usecases-manifest.json`
- Evidence reports: `docs/evals/m<N>-report.md`; hosted evidence: `docs/launch/hosted-acceptance.md`
