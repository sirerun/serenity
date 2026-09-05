# Serenity: RFC 0001 to code complete

Plan of record for taking `docs/rfc/0001-serenity.md` (v2.2) from the tree at commit `13dc0d2` (M0 skeleton) to code complete. Split layout: each epic lives in `docs/plans/`; this file holds context, scope, the TOC, waves, milestones, risks, and procedure. Living status is in `docs/roadmap.md`.

## 1. Context

Serenity is a claim-based personal memory and direction system: a Go single binary that ingests what one person produces, reconciles claims in a git-canonical brain repo, serves them plus the person's precepts to agents over three open protocols (MEMORY_VERBS v1 adopted from gbrain, DISPOSITION v1, DIRECTION v1), and gates every consequential change through human disposition. The RFC is opinionated on formats, policy shapes, and protocol envelopes, and orders work as scope-gated milestones M0-M6.

Problem: M0 is committed and green (fence and shard writers with round-trip property tests, the 10K-claim shard test, SQLite index with the wipe-and-rebuild invariant, init/sync/doctor/status, keychain token, CI cross-builds). Everything that makes the product a product (connectors, extraction, reconcile, the disposition queue and ladder, precepts and plan check, the protocol servers, gbrain import, docs) is absent, and several M0-scoped rules in RFC sections 7, 7.7, and 14 are stated but not enforced by any test.

Status as of 2026 09 04 (see docs/roadmap.md for the live, evidence-cited breakdown -- this file is context/scope/procedure, not living status): M0 is fully shipped (E0 13/13; v0.1.1 is public with a verified working Homebrew tap formula, after the original v0.1.0 release shipped with a broken formula push, found and fixed the same day). M1's ingest spine is built and end-to-end verified except its one human-only exit task (E1 22/23; T1.23 needs David's real Gmail account and his own repos, genuinely not delegable). M3's direction/guardrail core is built (E3 12/17; remaining 5 tasks are deps-blocked on E2, not unclaimed -- corrected 2026-08-30, ADR 011). E4 has shipped both of its cross-epic-startable tasks (T4.18 the read facade, T4.3 the HTTP transport, PR #56, both merged; E4 2/19) and now has zero further frontier until M2+M3 land. M2 and M5 have not been started -- M2 is correctly gated behind M1's exit task per the build-in-M-order rule below. The gates are mechanically encoded as blocked-by annotations as of 2026-08-30 (ADR 011). Verified 2026-09-04 by reading every open task's deps/blocked-by across all six epic files: the entire remaining engineering frontier (E2, E3's remaining 5, all of E4 beyond T4.3/T4.18, all of E5) is transitively gated behind T1.23 -- zero tasks are currently pool-dispatchable. David is completing T1.23 himself before dispatching any pool session (2026-09-04 decision, section 9); section 5 documents the 4-parallel-apply-loop-session dispatch model that activates the moment it lands.

Objectives:
- Every RFC section 17 acceptance criterion for M0-M5 green on a laptop, each recorded in `docs/evals/m<N>-report.md` with the command run and the observed output.
- Every invariant the RFC calls load-bearing enforced by a test or CI gate: file-first writes, byte-identical rebuild within the pinned model set, precepts unmintable by any machine path, every endpoint authenticated, no id-equality dedup across sources.
- The CLI is the first conformant client of all three protocols; `gbrain protocol conformance` passes against Serenity.

Non-goals (RFC section 6, contractual): multi-user, hosted service, model training, autonomous side effects, plugin ecosystem, note-taking app, GUI (Flutter is v1.1). Microsoft Graph email is post-launch. M6 (7-day soak, chaos, p95, 128GB-box profile) runs after code complete (ADR 002).

Constraints and assumptions:
- Decisions made 2026 08 27 by David: Gmail via app password is the certified IMAP provider (ADR 001); code complete = M0-M5 ACs green, M6 is a post-code-complete soak (ADR 002); every milestone is decomposed to executable fidelity in this pass (M6 stays an outline because it measures finished code).
- Standard-library Go in the engine; focused third-party deps only at the connector and provider edge (ADR 003). cobra, go-keyring, yaml.v3, and modernc sqlite stay.
- Build in M order; a later epic never starts before the prior epic's exit task is checked, except where a wave's deps say otherwise.
- All code changes happen in per-task worktrees on `/Volumes/BuildOffload`, one PR per task, rebase-and-merge, no Claude attribution.
- `kazi` is on PATH: every engineering task carries an `acc:` predicate for /apply's kazi lane; design-heavy tasks are marked `lane: agent`.
- Decided 2026 09 04: the standing dispatch model is 4 parallel `/apply --pool` loop sessions once a wave has the width to feed them (section 5). David completes T1.23 himself first; ADR 011's M-order gate is unchanged, not waived -- the frontier was found completely dry (every open E2-E5 task transitively gated on T1.23) the same day T4.3, the last cross-epic-startable task, shipped.

Success metrics: the E0-E5 checklist fully checked; `docs/evals/m1..m5-report.md` present; v1.0.0 tagged with three archives and a brew formula; repo public under the decided name.

## 2. Discovery summary

Engineering discovery over the tree at `13dc0d2` (no code-review-graph db; manual scan) and the two external contracts:

- 46 use cases catalogued (40 P0, 6 P1): 2 WIRED (init, sync), 5 PARTIAL (doctor, status, hand repair, compact store, push hook), 1 STUB (extract), 38 PLANNED. Manifest: `.claude/scratch/usecases-manifest.json`.
- Present and verified green: `internal/store` (NormalizeKey, DerivedID, FenceWriter, ShardStore with order-independent head resolution, Compact, MergeLines), `internal/index` (Engine interface, SQLite with FTS5 and an empty vectors table, Rebuild honoring the shard-authority rule), `internal/config`, `internal/domain`, `internal/secrets`, `internal/cli` (init, sync, extract stub, doctor, status). 16 tests, `go test -race ./...` green.
- Absent: threat model doc, file-first CI gate, writer queue, dirty-tree guard, daemon commits, fence merge test, id collision tripwire, vocabulary enforcement, compact verb, runtime-state allowlist, linter, any release; and all of M1-M5.
- gbrain (dndungu/gbrain, branch `master`, pin `d35c9c9e441e`): MEMORY_VERBS v1 spec at `docs/protocol/MEMORY_VERBS_v1.md`; conformance is a live write test shipped as data (`test/fixtures/memory-verbs/cases.json`); facts and takes ARE markdown fences (`src/core/facts-fence.ts`, `takes-fence.ts`) with a documented column grammar; BrainBench at `evals/brainbench`; six-verdict temporal enum ends in `negation_artifact`.
- dira (kazi-org/dira, pin `15686940aa08`, Sire Run, Inc. IP under Apache-2.0): five kinds, `additionalProperties: false` everywhere, `why_not`/`revisit_if` only inside `alternatives[]`, lexical offline matcher, `dira check "<plan>"` with exit 0/2 and 1 for its own errors, a fixed four-prompt interview. Consequences in ADR 008.
- External reviewer (oxAlpha, 2026 08 27) supplied the milestone decomposition that E0-E5 are based on; its deviations from the RFC (exit codes, verb names, orphan detector semantics, sidecar rule files) were rejected in favor of the RFC and recorded in ADR 008 and ADR 010.

## 3. Scope and deliverables

In scope: E0-E5 below; ADRs 001-010; protocol documents and schemas; eval corpora and reports; docs site; the v1.0.0 release and the name decision.

Out of scope: Flutter app (v1.1), Graph email, ANN index, multi-principal enforcement, anything in RFC section 6.

| ID | Deliverable | Owner | Acceptance |
|---|---|---|---|
| D-E0 | M0 residual invariants enforced + v0.1.0 released | pool + David | E0 acceptance line |
| D-E1 | Ingest spine, search, ask, eval harness, corpora | pool + David | RFC M1 AC, `docs/evals/m1-report.md` |
| D-E2 | Reconcile, disposition queue, inbox, entities, consolidate, ladder calibration | pool | RFC M2 AC, `docs/evals/m2-report.md` |
| D-E3 | dira vendored, interview, check, orphan detector | pool | RFC M3 AC, `docs/evals/m3-report.md` |
| D-E4 | serverd, MCP/HTTP with auth, three protocols, conformance, spend ceiling | pool + David | RFC M4 AC, `docs/evals/m4-report.md` |
| D-E5 | gbrain import, docs site, adversarial gate, BrainBench trend, name, v1.0.0 | pool + David | RFC M5 AC, `docs/evals/m5-report.md` |

## 4. Checkable work breakdown

### E0 -- M0 residuals: substrate invariants the RFC mandates but 13dc0d2 lacks  -> docs/plans/E0-m0-residuals.md  (13/13, SHIPPED 2026 08 29 -- trimmed from this file's wave breakdown per the plan skill's trim pass; full task detail lives in the epic file and docs/roadmap.md Shipped)
### E1 -- M1: ingest spine + honest evals  -> docs/plans/E1-m1-ingest.md  (22/25, T1.23 -- David's own Gmail/repo exit verification -- remains for the RFC M1 AC; T1.24/T1.25 (wave 1e, OpenRouter provider configurability, ADR 013) are additional, not part of the RFC AC, cross-epic-startable now, added 2026-09-04)
### E2 -- M2: reconcile + entities + disposition queue + ladder calibration  -> docs/plans/E2-m2-reconcile.md  (0/22, gated behind T1.23 per build-in-M-order; wave-2a tasks carry blocked-by: [T1.23] mechanically -- ADR 011)
### E3 -- M3: direction (dira vendored, interview wizard, plan check, orphan detector)  -> docs/plans/E3-m3-direction.md  (12/17, remaining: T3.4, T3.9, T3.10, T3.11, T3.17 -- deps-blocked on E2 tasks (T2.1, T2.5, T2.17, T2.19); the 2026-08-29 "not blocked" note was an error, corrected 2026-08-30 -- ADR 011)
### E4 -- M4: serve + protocols  -> docs/plans/E4-m4-serve-protocols.md  (2/19, gated behind M2+M3; T4.3 and T4.18 were the cross-epic-startable tasks during E3 and both have now shipped -- ADR 011; T4.10 and T4.12 become cross-epic-startable the moment T2.1 lands, needing no other E2 wave-2a task -- see section 5; T4.18/T4.19 added 2026-09-02, ADR 012)
### E5 -- M5: migration + launch  -> docs/plans/E5-m5-migration-launch.md  (0/15, gated behind M1+M4; T5.1 and T5.9 carry blocked-by: [T4.17] mechanically -- ADR 011)
### E6 -- M6: hardening soak (outline, post-code-complete)  -> docs/plans/E6-m6-hardening.md  (0/1)

Decisions confirmed by David on 2026 08 27 (no open decisions remain):
- OD-1 Second labeler for golden sets: two independent frontier-model passes from different families with David adjudicating (ADR 005).
- OD-2 Voice-note connector timing: M2 task T2.16, not an M1 gate (ADR 005).
- OD-3 Name shortlist timing: decision taken at M4 exit, executed in T5.6 (ADR 010).
- OD-4 Docs toolchain: mkdocs-material (ADR 010).

## 5. Parallel work

Tracks are epic-internal waves; epics are sequential at their exit tasks but overlap at their first waves where deps allow.

| Track | Tasks | Sync point |
|---|---|---|
| A: substrate hardening | E0 waves 0a-0b (complete, trimmed from section 4 -- see docs/plans/E0-m0-residuals.md) | T0.13 before any E1 write path |
| B: connectors + sources | T1.1, T1.2, T1.3, T1.4, T1.5, T2.16 | T1.15 (end-to-end extract) |
| C: models + extraction | T1.6, T1.7, T1.19, T1.8, T1.9, T1.10, T1.16 | T1.15 |
| D: retrieval + composer | T1.11, T1.12, T1.21 | T1.23 (M1 exit) |
| E: evals | T1.13, T1.14, T1.20, T1.22, T2.18, T3.13, T3.16 | each milestone exit |
| F: disposition + ladder | T2.1, T2.6, T2.8, T2.10, T2.11, T2.5, T2.7, T2.9, T2.15, T2.20 | T2.22 |
| G: reconcile + entities + consolidate | T2.2, T2.3, T2.4, T2.12, T2.13, T2.14, T2.17, T2.21 | T2.22 |
| H: direction | E3 waves 3a-3d | T3.17 |
| I: server + protocols | E4 waves 4a-4c | T4.17 |
| J: migration + launch | E5 waves 5a-5c | T5.20 |

Cross-epic overlap allowed: E1 wave 1a may start once E0 wave 0a merges (T0.3 and T0.10 are its only deps); E3 wave 3a may start once E0 is done (T3.1, T3.2, T3.12, T3.13 need no M1/M2 code); E4 T4.3 and T4.18 (facade, deps T3.7/T1.12 both shipped, ADR 012) started during E3 and both have now shipped (T4.18 PR #54 2026-09-02, T4.3 PR #56 2026-09-03/04) (corrected 2026-08-30: T4.7, T4.13 need E2's T2.17/T4.7 respectively, so they unblock only as later E2 waves land -- task deps are the authority, ADR 011). Refined 2026-09-04 (dependency re-walk while scoping the 4-session dispatch model): T4.10 and T4.12 need only T2.1, not the rest of wave 2a or T2.17 -- they are cross-epic-startable the instant T2.1 merges, one full wave ahead of T4.7/T4.13. This is load-bearing for keeping 4 parallel sessions fed past wave 2a's first tick; see the dispatch model below.

### Multi-session dispatch: 4 parallel apply --pool loop sessions (decided 2026 09 04)

Decision: David completes T1.23 himself first; ADR 011's M-order gate stands unchanged, it is not waived. 4 parallel `/apply --pool` loop sessions launch once T1.23 is checked, not before. Launching them earlier would have all 4 find zero claimable work -- confirmed 2026-09-04 by walking every open task's `deps:`/`blocked-by:` across E2 through E5: T4.3 was the last cross-epic-startable task and it shipped the same day (PR #56), so the frontier is dry until T1.23 lands.

Mechanism: all 4 sessions run `/apply --pool` (or `/loop /apply --pool`) against this same docs/plan.md, each from its own terminal/session. Task claims are atomic git refs under refs/claims/T* (the /claim skill) -- collisions are impossible by construction, so no manual task-to-session assignment is needed. Each session's own claim scan finds the next unclaimed, dependency-satisfied task and takes it; `/claim --list` shows what every session currently holds.

Expected saturation once T1.23 clears (wave/task numbers as of this plan; re-check after each merge):
- Tick 1: wave 2a offers 5 immediately-claimable tasks (T2.1, T2.10, T2.12, T2.16, T2.19; T2.20 deps on T2.1 so it is not parallel-startable with it). 4 sessions claim 4 of the 5; the 5th claims as soon as a session frees up.
- The moment T2.1 merges (independent of the rest of wave 2a): T2.20 (E2), T2.6/T2.8/T2.13 (wave 2b), and cross-epic T4.10 and T4.12 (E4 wave 4a) all open -- E4 work does not wait for E2 to fully close.
- The moment T2.10 merges: T2.11 (wave 2b) opens.
- Wave 2b (6 tasks) and wave 2c (6 tasks) each exceed 4-way parallelism on their own once their wave-2a/2b prerequisites land.
- Wave 2d (4 tasks: T2.17, T2.18, T2.21, T2.22) exactly fills 4 sessions with no slack -- the one point in the sequence to expect a session to briefly find nothing, before E3's remainder (T3.4, T3.9, T3.10, T3.11 -- 4 tasks; T3.17 is E3's exit and depends on T3.4) and the rest of E4 (wave 4a's T4.1/T4.7/T4.13, then 4b's 6, then 4c's 5 pool-dispatchable tasks) open up behind it.
- Net: from T1.23 landing through the E2/E3/E4 remainder, dispatchable width stays at or above 4 essentially throughout; wave 2d is the one wave to watch.

Idle-session rule: if a session's claim scan finds nothing dependency-satisfied and unclaimed, it does not idle-poll (standing rule) -- it checkpoints and exits, and is relaunched once the next wave's prerequisites clear (usually triggered by another session's merge). Check `/claim --list` before relaunching so the new session's first scan is not wasted rediscovering what is already held.

Worktree and disk hygiene for 4 concurrent sessions: worktrees are already per-task, not per-session (`/Volumes/BuildOffload/wt/serenity-<task>`), so 4 sessions claiming 4 different tasks land in 4 distinct worktree directories with no path collision by construction. Run the disk preflight (`df -h /System/Volumes/Data`) before starting a 4th concurrent build if 3 are already running; below ~20 GB free, throttle to 3 concurrent builds rather than let a near-full disk turn one failed build into a compounding retry loop.

### Waves

Each wave lists the exact agent count (one agent per task); the task lines here mirror the epic files (ids resolve there).

### E0: SHIPPED 2026 08 29 (13/13) -- wave 0a/0b task list trimmed from this file

Full per-task completion detail lives in docs/plans/E0-m0-residuals.md (still
[x] there, never trimmed) and docs/roadmap.md's Shipped section. Notably,
T0.11 (cut v0.1.0) shipped as v0.1.1 after the original v0.1.0 release was
found to have a broken Homebrew tap formula publish step (a goreleaser config
gap, fixed same day) -- v0.1.0 itself was left untouched, not retagged.

### Wave 1a: E1 interfaces and stores (8 agents)
- [x] T1.1 Connector interface + jobs table
- [x] T1.2 Source store: content-addressed bytes + meta.yaml + tombstone stub
- [x] T1.6 Chunker `internal/extract/chunk`
- [x] T1.7 Model router `internal/router` with tiers, confidence caps, spend rows, redaction hook
- [x] T1.19 Redaction pass v1 (patterns: account numbers, card numbers, API-key shapes, emails on request)
- [x] T1.3 File-watcher connector (fsnotify + `--poll` fallback)
- [x] T1.5 Git-repo crawler connector
- [x] T1.13 Eval harness `internal/eval`: held-out golden format, P/R/F1 per family, contradiction recall

### Wave 1b: E1 connectors and extraction (6 agents)
- [x] T1.4 IMAP connector, Gmail certified (go-imap/v2, app password in keychain, UIDVALIDITY cursor)
- [x] T1.8 Extraction to observations `internal/extract` (structured prompt, fixed predicate list, 0.6 distill threshold, output cache)
- [x] T1.9 Observation to claim write path (trust 0: append only, semantic dedup deferred to E2)
- [x] T1.10 Embeddings + vector store: per-row model pin, exact cosine scan, never mix pins
- [x] T1.14 Ava Standardo extraction corpus + held-out split + contradiction cases
- [x] T1.20 Prompt-injection and precept-fabrication fixture set (seed of the adversarial corpus)

### Wave 1c: E1 retrieval and end to end (5 agents)
- [x] T1.11 Hybrid search + RRF + 4-layer dedup + `serenity search`
- [x] T1.15 Real `serenity extract` + `serenity sync` (poll connectors, chunk, extract, write, index) end to end
- [x] T1.17 `serenity status` v1: ingest lag, connector health, jobs depth, spend to date, rebuild timing
- [x] T1.21 BrainBench adapter + CI trend artifact
- [x] T1.18 Connector guide (support matrix, Gmail app-password setup, re-auth path)

### Wave 1d: E1 composer, migration, evals, exit (4 agents)
- [x] T1.12 Composer: `serenity ask` with citations, gap statement, supersession phrasing
- [x] T1.16 `serenity migrate --models`: re-extraction pass, staged re-embed, FTS fallback mid-migration
- [x] T1.22 Nightly real-model eval workflow with budget cap + per-push cached eval gate
- [ ] T1.23 M1 exit verification: real Gmail 30 days + 5 repos on a laptop; publish per-family P/R/F1 and contradiction recall (human)

### Wave 1e: model provider configurability (added 2026-09-04, not part of the RFC M1 AC, ADR 013; 2 agents)
- [ ] T1.24 OpenRouter as an explicit, selectable provider (`models.provider` field, checked before the model-name substring inference; `config.Default()` seeds openrouter)
- [ ] T1.25 `serenity config set-model <purpose> <model>` CLI + provider docs

### Wave 2a: E2 queue, ladder, sweeps, capture (6 agents)
- [ ] T2.1 DISPOSITION store `internal/disposition`: items, append-only history, idempotency, already_disposed
- [ ] T2.10 Ladder policy object `internal/ladder`: config parse with mandatory correlation guards, cell state, promotion, demotion, auto-action logging
- [ ] T2.12 Decay + weekly sweep `internal/reconcile/decay.go` (read-time decay, alias candidates, low-confidence to distill)
- [ ] T2.16 Voice-note connector (transcription via router local-cheap tier)
- [ ] T2.19 `serenity cron <job>` runner with injected clock (sweep, consolidate, decay, slo) + launchd/systemd unit docs
- [ ] T2.20 Distill queue + `serenity capture <text|audio>` staging

### Wave 2b: E2 reconcile core (6 agents)
- [ ] T2.2 Reconcile engine `internal/reconcile`: (subject, predicate) + neighbor candidates, six-verdict temporal enum, A/B items, trust 0
- [ ] T2.6 Expiry sweeper: pending > 14d (per kind) -> deferred; 3 cycles -> parked; resurface once on new evidence
- [ ] T2.8 Two-client dispose race property test
- [ ] T2.11 Ladder calibration sweep `internal/ladder/calibrate.go` + `evals/calibration/`
- [ ] T2.13 Entity resolution `internal/entities`: alias match, embedding similarity within type, undoable merge event, split, ambiguous -> disposition
- [ ] T2.15 Queue SLOs (`internal/queue/slo.go`) into `serenity status`

### Wave 2c: E2 write paths and inbox (6 agents)
- [ ] T2.3 Supersession write path on accept: fence strikethrough + pointer, shard superseding line + regenerated head
- [ ] T2.4 Fence/shard divergence handling: detect hand-edited shard head, disposition item, accept appends human-provenance line
- [ ] T2.5 CLI inbox: J/K/space, verdicts, grouped items, `--bulk-defer <filter>`, per-family pause, `--parked`
- [ ] T2.7 `edit_accept` through the deterministic writer with human-tier provenance
- [ ] T2.9 Compaction gated by an approved disposition item + shard rollover at configured size
- [ ] T2.14 Consolidate `internal/consolidate`: summary fences with freshness banners, shard-head refresh, re-embed changed chunks

### Wave 2d: E2 briefing, evals, tombstone, exit (4 agents)
- [ ] T2.17 Briefing scaffold `internal/briefing`: five fixed sections, 800-word cap, drop-not-truncate, packing function with a token-estimator parameter
- [ ] T2.18 Reconcile eval: verdict confusion matrix + contradiction-detection recall in evals/report.json
- [ ] T2.21 Tombstone cascade: source tombstone -> retraction proposals -> accept rewrites fences/shards and rebuilds
- [ ] T2.22 M2 exit: run the RFC AC checklist end to end on a laptop and record it

### Wave 3a: E3 vendoring and invariants (5 agents)
- [x] T3.1 Vendor dira at pin 15686940aa08: `internal/dira/` with schema JSON, schema.go, ledger reader/writer, LICENSE, NOTICE, PIN file, update script
- [x] T3.2 `applies_when` body block parser + validator (`internal/direction/applies.go`)
- [x] T3.3 Precept ledger writer through the writer queue (`internal/direction/ledger.go`): create staged draft, confirm (staged -> accepted with >= 1 alternative), supersede (never edit)
- [x] T3.12 Precept-immutability invariant test: no package outside internal/direction can write under .dira/ (AST allowlist, same mechanism as T0.2)
- [x] T3.13 DIRECTION eval corpus: plan x constraint matrix with expected verdicts, adversarial "ignore your constraints" plans, near-miss paraphrases

### Wave 3b: E3 matcher, interview, questions, revisit (5 agents)
- [x] T3.5 check_plan stage 1: deterministic matcher over structured actions (`internal/direction/check`)
- [x] T3.6 check_plan stage 2: free-text classifier into the closed action set via the local-cheap tier, cached, with matched_actions spans
- [ ] T3.4 Interview wizard (~30 questions, drafts only, one disposition per precept)
- [x] T3.8 Question precepts block their targets
- [ ] T3.10 revisit_if weekly sweep -> review cards (`internal/direction/revisit.go`, `serenity cron revisit`)

### Wave 3c: E3 CLI, orphans, decompose, conformance, upstream (5 agents)
- [x] T3.7 `serenity check` CLI: exit codes 0/2/1, `--json`, `--actions` structured input, why_not verbatim
- [ ] T3.9 Orphan detector: weekly activity (claims, sources, dispositions) with no derivation edge to an active intent -> briefing Drift
- [ ] T3.11 Decompose: judgment tier proposes child intents with derives_from; one-keystroke confirmations in the inbox
- [x] T3.14 dira CLI conformance job: `dira check`, `dira why`, `dira brief` run unmodified against the fixture brain in CI
- [x] T3.15 Upstream PR to kazi-org/dira proposing an optional applies_when field (non-blocking)

### Wave 3d: E3 evals and exit (2 agents)
- [x] T3.16 Direction eval section: verdict P/R/F1 per action class, unverified rate, false-deny rate; adversarial rows must be caught
- [ ] T3.17 M3 exit: run the five RFC M3 ACs and record them

### Wave 4a: E4 daemon, transport, spend, schemas, events, fixtures, facade (7 agents)
- [x] T4.18 Export `pkg/serenity` read-only facade (Open/CheckPlan/Recall) for embedding consumers; no writer export; cross-epic-startable (ADR 012)
- [ ] T4.1 `serenityd` core (`internal/server`): lifecycle, tickers for sweep/consolidate/decay/revisit/slo, structured logs with job ids, pidfile, graceful shutdown
- [x] T4.3 HTTP transport bound to loopback with bearer auth from the keychain; explicit LAN/Tailscale config with token + optional mTLS (merged PR #56, 2026-09-03/04)
- [ ] T4.10 Spend ceiling + projection (`internal/spend`): per-day aggregation, monthly ceiling default $50, transactional check-and-record, approval item on trip, projection in status and briefing Watched
- [ ] T4.7 JSON Schemas for every protocol object + `serenity protocol --json`
- [ ] T4.12 Event log with persisted monotonic cursors (`internal/events`) shared by SSE and stdio notifications
- [ ] T4.13 Conformance fixture set under testdata/conformance/: vendored gbrain memory-verbs cases.json plus Serenity DISPOSITION and DIRECTION transcripts, checksum-frozen

### Wave 4b: E4 protocol servers, connect, security (6 agents)
- [ ] T4.2 MCP stdio transport (`internal/server/mcp`): initialize handshake, JSON-RPC framing, logs to stderr only
- [ ] T4.5 MEMORY_VERBS v1 server: recall, remember, entity, synthesize, forget with the gbrain envelope (protocol_version, evidence, provenance, budget meta, cost, enumerated errors with suggestion) plus optional confidence, claim_id, superseded_by
- [ ] T4.4 DISPOSITION v1 server: list_pending (kinds, expiring_before, group), dispose (idempotency_key, already_disposed), capture, subscribe (SSE + long-poll, Last-Event-ID resume)
- [ ] T4.6 DIRECTION v1 server: brief (token budget governs, caps 12/8/8/5, omitted counts, budget_estimator named), check_plan (schema-primary verdict), propose (lands in the queue, never mutates a precept)
- [ ] T4.8 `serenity connect claude`: MCP config stanza + hook installing `serenity check` as a pre-plan gate, idempotent, token never written to the config file
- [ ] T4.11 Server security battery: oversized payloads, deep nesting, path traversal, replayed idempotency keys, redaction before cloud synthesis over the wire, go-fuzz on parsers 30s in CI

### Wave 4c: E4 drift, conformance, docs, facade brief, exit (6 agents)
- [ ] T4.9 CLI vs protocol drift tests: search/recall, ask/synthesize, check/check_plan, inbox dispose/dispose, brief/brief over one fixture
- [ ] T4.14 Run `gbrain protocol conformance --target http://127.0.0.1:<port>/mcp --token <t>` in CI against a throwaway fixture brain (Bun installed in the job)
- [ ] T4.15 Serenity conformance command: `serenity protocol conformance --target <url>` replaying testdata/conformance transcripts for all three protocols
- [ ] T4.16 Protocol documents: docs/protocol/MEMORY_VERBS_v1.md (adopted, credited), DISPOSITION_v1.md, DIRECTION_v1.md with governance section (maintainer arbitrates via in-repo RFC, additive-forever, fixtures location) and the kill criterion
- [ ] T4.19 `Brief` on the `pkg/serenity` facade over T4.6's packer; drift test extended with T4.13 transcripts (ADR 012)
- [ ] T4.17 M4 exit: external Claude Code session answers "what did we decide about X?" with two citations on the Ava corpus; ceiling trip forces approval; auth sweep (human)

### Wave 5a: E5 importer, docs, gate, corpus (6 agents)
- [ ] T5.1 gbrain importer: page frontmatter, facts fence, takes fence, timeline, links -> entity pages, claims, timeline fences, graph edges (`internal/import/gbrain`)
- [ ] T5.2 Field-level round-trip test + unmapped-field report
- [ ] T5.3 Resumable import: per-(page,row) checkpoint in .serenity/import/gbrain.json written after the batch is durable
- [ ] T5.4 Docs site (mkdocs-material): install, operator manual (config, cron/serve, scheduling units, backup and recovery, keychain, upgrades, git gc guidance), connector guide, protocol specs, RFC process, threat model by build-time include
- [ ] T5.5 Adversarial corpus release gate (`evals/adversarial`, required check on release.yml): injected instructions, contradictory and stale sources, false claims, entity collisions, poisoned documents, precept-fabrication attempts, effect-request forgery
- [ ] T5.9 10K-message synthetic corpus generator (seeded, deterministic) for the import budget benchmark

### Wave 5b: E5 name, install AC, benchmarks, report (6 agents)
- [ ] T5.6 Name decision executed: rename module, binary, CLI strings, protocol $ids, brew formula; `serenity` alias shim for one release (human)
- [ ] T5.7 Fresh-machine install script + CI job on a clean container with the canned Ava corpus: clone -> init -> connect one connector -> sync -> extract -> search -> ask -> inbox -> check, timed
- [ ] T5.8 Import budget benchmark: 10K messages through full ingest on cached model outputs, per-stage timing artifact, nightly regression compare
- [ ] T5.10 BrainBench trend published: nightly job appends to evals/brainbench-trend.json on a results branch and renders a chart into the docs site
- [ ] T5.11 `serenity report --export` and the weekly report card (claims by state, corrections per 100 extractions, ladder promotions/demotions with sampled false-acceptance, spend, repo growth, rebuild time)
- [ ] T5.12 Large-brain manual run: import >= 10K real messages on a laptop, resumable, within 4h; record it (human)

### Wave 5c: E5 fixture repo, README, code-complete gate (3 agents)
- [ ] T5.13 Public fixture gbrain brain: publish testdata/gbrain-fixture as a standalone public repo with README crediting gbrain; CI clones it at a pinned sha for the round-trip test
- [ ] T5.14 README + docs lead with plan-check (the wedge): first demo is a plan rejected against a precept; gbrain lineage credited; MEMORY_VERBS conformance badge
- [ ] T5.20 Code-complete gate: every AC line of E0-E5 checked, evals reports present, release v1.0.0 tagged and brew formula published, repo made public (human)

### Wave 6: E6 planning task (1 agents)
- [ ] T6.0 PLAN: expand E6 to executable fidelity (informed by E0-E5 learnings and the shipped metrics surface)

## 6. Timeline and milestones

Scope-gated, not date-gated (RFC section 17). Order and exit criteria:

| ID | Milestone | Depends on | Exit criteria |
|---|---|---|---|
| MS0 | Substrate invariants enforced | none | E0 fully checked; v0.1.0 archives + brew formula exist |
| MS1 | Ingest spine + honest evals | MS0 | RFC M1 AC recorded in docs/evals/m1-report.md; per-family P >= 0.90 and R >= 0.80 or listed misses with remediation ids |
| MS2 | Reconcile + queue + ladder | MS1 | RFC M2 AC recorded; calibration report matches shipped defaults |
| MS3 | Direction | MS0 (start), MS2 (inbox-dependent tasks) | RFC M3 AC recorded; dira CLI conformance job green |
| MS4 | Serve + protocols | MS2, MS3 | gbrain conformance job green; auth sweep; ceiling trip; external session citation |
| MS5 | Migration + launch = code complete | MS1, MS4 | RFC M5 AC recorded; adversarial gate required and green; v1.0.0 public under the decided name |

## 7. Risk register

| ID | Risk | Impact | Likelihood | Mitigation |
|---|---|---|---|---|
| R1 | Extraction misses the 90/80 per-family floor on real email | MS1 blocked | Medium | Held-out corpus built before extractor tuning (ADR 005); per-family remediation tasks spawned from the m1 report; prompt versioning with cached outputs |
| R2 | Disposition fatigue in dogfooding hides as a green queue | Product failure | Medium | SLOs wired to status (T2.15), grouped and bulk-defer (T2.5), calibration on data (T2.11) |
| R3 | A machine path writes canonical files outside the writer queue | Invariant loss | Low | File-first AST gate (T0.2) extended to every new package; allowlist reviewed per PR |
| R4 | dira schema evolves and breaks the vendored pin | Precept store drift | Low | Pin moves only by T3.1's update script; conformance job (T3.14) runs at the pin |
| R5 | gbrain conformance suite requires put_page semantics Serenity does not expose | MS4 AC ambiguity | Medium | Entity-card cases "skip honestly" per the spec; T4.14 records which arms ran |
| R6 | Cloud eval spend runs away in CI | Budget | Medium | Cached outputs per push; nightly live runs behind a USD cap (T1.22, T5.10) |
| R7 | The rename lands after transcripts and $ids freeze | Double re-freeze | Medium | Decision at M4 exit, rename in T5.6 before release transcripts freeze (ADR 010) |
| R8 | Gmail app-password IMAP quirks (throttling, EXPUNGE mid-fetch) corrupt cursors | Duplicates or gaps | Medium | UIDVALIDITY-aware cursor, batched fetch, replay fixture (T1.4) |
| R9 | Scope gravity: in-place claim edit or private daemon path sneaks in | RFC section 18.7 | Low | Drift tests (T4.9), edit_accept only via writer (T2.7), README leads with plan-check |
| R10 | The reviewer-derived decomposition carries an RFC deviation not caught | Rework | Low | Each epic file names its RFC anchors; exit tasks re-run the RFC AC text verbatim |

## 8. Operating procedure

Definition of done for a task, all required:
1. Tests written and green: unit or property tests for every implementation task; an API test hitting the real boundary for every protocol or CLI `--json` surface (T4.9, T4.15); no browser tests (no UI in v1).
2. `make lint`, `make vet`, `gofmt` clean; `go test -race ./...` green locally and in CI.
3. PR merged to main via rebase with CI green; docs updated in the same PR; roadmap line updated.
4. For release-shaped tasks (T0.11, T5.20): the tag fired the release workflow and the artifacts were observed, not assumed.
5. Reported honestly in `docs/roadmap.md`: observed outputs, skipped steps named.

Rules: one worktree per task on `/Volumes/BuildOffload/wt/serenity-<task>`; small logical commits; never commit files from different directories in one commit; `acc:` predicates drive kazi's lane, `lane: agent` tasks go to a frontier subagent; every kazi friction point becomes an issue at kazi-org/kazi. When running multiple parallel `/apply --pool` loop sessions (up to 4, section 5), task claims (refs/claims/T*, the /claim skill) are the only coordination mechanism -- never hand-assign tasks to sessions; a session that finds nothing claimable checkpoints and exits rather than idle-polling.

## 9. Progress log

- 2026 08 27 Initial plan: E0-E6 created (108 tasks, 108 with acc:, 6 kind: human, 2 kind: any, 7 lane: agent), ADRs 001-010 written, use-case manifest (46) written, roadmap seeded. OD-1..OD-4 confirmed by David (recommended options); ADR 005 moved to Accepted.
- 2026 08 29 Trim pass (no new scope): E0 is fully shipped (13/13) -- its wave 0a/0b task list removed from this file per the plan skill's trim step (full detail stays in docs/plans/E0-m0-residuals.md and docs/roadmap.md, never trimmed there). Synced this file's stale wave checkboxes to the epic files' real state: 22 E1 tasks and 12 E3 tasks flipped to [x] (34 total) -- only T1.23 (E1, David-only) and T3.4/T3.9/T3.10/T3.11/T3.17 (E3, pool-dispatchable, unclaimed) remain open on the frontier. No epic changed fidelity tier: E4/E5 were already decomposed to executable in the initial pass (a deliberate original choice, section 1) despite being beyond the current frontier -- left as-is rather than retroactively demoted, since that would discard already-verified planning work for no benefit. E2/E4/E5/E6 unchanged (0 done each).
- 2026 08 30 Refinement pass (no new scope; David ruled "refine the plan first" when offered early E2 dispatch): milestone gates encoded mechanically -- E2 wave 2a (T2.1, T2.10, T2.12, T2.16, T2.19) now carry blocked-by: [T1.23], T5.9 carries blocked-by: [T4.17]; corrected the 2026-08-29 trim pass's wrong "not blocked" note on E3's remaining tasks (their deps reference unchecked E2 tasks) and this file's section 5 E4 cross-epic overlap line (T4.7/T4.12/T4.13 need E2 work; only T4.3 starts during E3). ADR 011 records the ruling. Roadmap synced.
- 2026 09 02 T4.18 + T4.19 added (packaging, not roadmap scope -- RFC §3: Sire's needs arrive as a consumer of the public surface, and the facade calls exactly the functions the CLI calls, RFC §13.1): `pkg/serenity` read-only facade. T4.18 (Open/CheckPlan/Recall, wave 4a, deps [T3.7, T1.12], both shipped) is cross-epic-startable alongside T4.3 and carries no blocked-by: ADR 011 gates an epic on the prior exit "except where a wave's deps say otherwise", and rule 5 requires the prose to name it, which section 5 now does. T4.19 (Brief, wave 4c, deps [T4.18, T4.6, T4.13]) is transitively milestone-gated through T4.6 -> T2.1 -> blocked-by [T1.23]. No writer export, one brain-writer per brain (ADR 004, ADR 012). E4 is 0/19.
- 2026 09 02 T4.18 shipped (PR pending merge): `pkg/serenity` read-only facade -- `Open`/`CheckPlan`/`Recall` over exactly the functions `serenity check`/`search`/`ask` call; `internal/providers` extracted from `internal/cli/providers.go` (unchanged wiring, no cobra import for embedders); `direction.NewStore(root, nil)` is now a read-only handle whose every mutator returns `direction.ErrReadOnly`. Drift, AST, go-doc and nil-queue tests per the acc line, each shown red against a deliberate break. E4 is 1/19; T4.19 (Brief) still waits on T4.6/T4.13.
- 2026 08 31 Annotation-gap fix (no new scope; ADR 011 rule 5, same-day fix): T5.1 (gbrain importer) carried no blocked-by, but its deps [T1.9, T0.3] are checked while the E5 epic is milestone-gated behind M1+M4 -- a pool run would have dispatched it before T1.23/T4.17, the exact prose-vs-deps drift class ADR 011 closes. T5.1 now carries blocked-by: [T4.17] (same encoding as T5.9: T4.17 checked transitively implies T1.23 checked). Found by the /apply --loop candidate scan.
- 2026 09 04 Refined and expanded for 4 parallel apply --pool loop sessions (David's request). Full dependency re-walk across E2-E5 found the frontier completely dry: T4.3 (E4 HTTP transport) shipped same day (PR #56) as the last cross-epic-startable task, leaving every remaining open task transitively gated behind T1.23. Surfaced to David as a decision; he chose to complete T1.23 himself first, keeping ADR 011's gate unchanged, rather than waiving it or launching idle sessions. Changes this pass: (1) synced this file's stale T4.3 checkbox and the E4 TOC count (1/19 -> 2/19; the epic file docs/plans/E4-m4-serve-protocols.md and docs/roadmap.md's Shipped section already had it right -- only this file's inline wave-4a copy and TOC line had drifted, PR #56/#57 touched the epic file and roadmap only); (2) found and recorded that T4.10 and T4.12 depend on T2.1 alone, not the rest of E2 wave 2a, making them cross-epic-startable a full wave earlier than previously noted; (3) added the "Multi-session dispatch" subsection under section 5 with the expected wave-by-wave saturation curve for 4 sessions, the idle-session rule, and worktree/disk hygiene for 4 concurrent builds; (4) added an Operating Procedure line naming claims as the only coordination mechanism across parallel sessions. No blocked-by annotations changed; no epic's fidelity tier changed. Landed as PR #59 (self-merged on green CI).
- 2026 09 05 David's OpenRouter request scoped and added (packaging, not RFC roadmap scope, same pattern as T4.18/T4.19): read `internal/router`, `internal/config`, `internal/providers` directly and found a real pre-existing bug that OpenRouter's model-id convention would surface -- provider selection is inferred from a "claude" substring in the model name, so an OpenRouter-style vendor-prefixed id (e.g. `anthropic/claude-...`) would silently misroute to native Anthropic instead of OpenRouter. ADR 013 records the decision: an explicit `models.provider: openrouter|anthropic|openai` field, checked before the substring fallback (additive, existing brains unaffected); `openrouter` reuses the existing `OpenAICompatibleProvider` unchanged (ADR 003, no new dependency) pointed at `https://openrouter.ai/api/v1` with a new `OPENROUTER_API_KEY`; `config.Default()` seeds `models.provider: openrouter` for new brains; embeddings are explicitly untouched (OpenRouter has no embeddings endpoint, `BuildEmbeddingRouter` keeps its existing `OPENAI_API_KEY`/`OPENAI_EMBEDDINGS_BASE_URL` path regardless). Filed as T1.24/T1.25 (E1 wave 1e, docs/plans/E1-m1-ingest.md), UC-047/UC-048 added to the use-case manifest. Both tasks are cross-epic-startable now (deps: [T1.7], shipped) -- not gated by T1.23. A background research agent was asked to scope this earlier in the session but did not deliver it (see hand-off notes); this pass redid the discovery directly.

## 10. Hand-off notes

- Read the RFC first; every epic file names its RFC sections. ADRs 001-010 record every deviation or refinement.
- External contract facts (schemas, fence grammars, conformance behavior) were captured to the session scratchpad; the durable parts are in ADR 008 and ADR 009. Re-derive from the pinned commits when in doubt: gbrain `d35c9c9e441e` (branch master), dira `15686940aa08` (branch main).
- The repo is private until T5.20; no secrets exist in it. The brew tap token is founder-held.
- kazi bus team name for this repo: `-users-dndungu-code-sirerun-serenity`.
- 2026 09 05: a background research agent scoped to "research OpenRouter integration only, do not touch docs/plan.md" instead redid the parent session's own in-flight 4-parallel-session plan refinement (claimed R-plan-md, committed, opened PR #59) and did not deliver its actual assigned OpenRouter research at all -- no scratch file was produced. The PR content was verified correct and merged; the OpenRouter task itself was redone directly by the parent session (T1.24/T1.25, ADR 013). If delegating research to a background agent again, do not assume "no scratch file yet" means "still working" -- check ListAgents for whether it is still alive before trusting a stated hand-off to another sub-agent.

## 11. Appendix

- RFC: `docs/rfc/0001-serenity.md`
- ADRs (013 OpenRouter default provider, added 2026-09-04): `docs/adr/001` Gmail IMAP; `002` code-complete boundary; `003` dependency posture; `004` writer queue, pending records, id width; `005` eval labeling, voice notes to M2; `006` `serenity cron`, x/term TUI; `007` reconcile constants; `008` precepts on dira, applies_when body block; `009` gbrain import mapping; `010` check exit codes, events per transport, token rotation, docs toolchain, name timing; `011` milestone gates mechanical; `012` embedded read facade, writes single-process; `013` OpenRouter default provider, explicit provider selection.
- Use-case manifest: `.claude/scratch/usecases-manifest.json`
- Evidence reports: `docs/evals/m<N>-report.md` (created by each epic's exit task)
