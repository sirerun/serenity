# Shared agent memory implementation

## Scope and authority

Implement RFC 0002 and RFC 0003 at design base 92b785fe6da569fa537b7ec92fb4a14197c8c3ae, preserving ADR027-029 and all existing memory interfaces. Owner explicitly authorizes implementation, independent review, merge and deployment. This attempt ends 2026-10-16 16:20 UTC with coherent checkpoints; unfinished scope remains open. No new infrastructure, unrelated UI, destructive production migration or provider substitution.

Existing Git/file SourceStore and serialized writer remain canonical; SQLite stays derived. Existing authenticated brains/grants remain the isolation boundary. Capture is gated until erasure/replay/restore qualification. No automatic real-history enrollment during qualification. Use synthetic data and explicit capability limits. No project label can authorize a brain. Hosted capture must not be activated before physical erasure and restore fences qualify.

## Discovery and execution

Existing governed HTTP gateway provides verified binding, maintenance/account/runtime locks, revocation rechecks and canonical writer publication. Existing memory v1 cannot archive conversations or physically erase them. Existing recovery claims and PR273 are foreign ownership; preserve them. PR277 proposes import, not episode implementation. No Ajent tool is available in this scoped session; disclose in candidate PR. No chief.md found in the two inspected Serenity checkouts. Recheck authenticated holds at merge.

Executor: current engineering session with bounded headless Codex helpers. Selection: gpt-6-astra, low reasoning, ChatGPT account authentication, approval never, danger-full-access. Maximum two concurrent implementation/review lanes; no recursive dispatch. Shared Mac load exceeds 10: no heavy local builds until load gate and owned lease. Existing Spark reports four available CPUs; qualify source transfer/image/toolchain before remote checks. Wazi reader, frozen schema and semantic validator gate this plan before subordinate implementation dispatch. No docs/design.md exists; authoritative design is the RFC/ADRs named above.

## Checkable work

### E1 — Archive service and safety foundation
fidelity: executable
Acceptance: additive gated archive API has exact evidence, bounded admission, existing-brain isolation and truthful deletion capabilities.

- [ ] T1.1 Freeze bounded protocol and preflight  Owner: integration  kind: agent  stage: implement  acc: [schema and capability limits are documented with canonical writer binding and current ownership and build route evidence]
- [ ] T1.2 Implement archive service foundation  Owner: archive-lane  kind: agent  stage: implement  deps: [T1.1]  acc: [synthetic HTTP fixtures verify exact event and attachment reads, stable retry conflicts, publication acknowledgment, bounded quotas, current brain authorization, fences and checkpoint context without changing v1]
- [ ] T1.3 Qualify exclusion and physical erasure recovery  Owner: integration  kind: agent  stage: implement  deps: [T1.2]  acc: [tests distinguish immediate exclusion from physical purge and prevent stale imports, indexes, derivatives and restored backups from resurrecting deleted content; capture remains disabled for unqualified paths]
- [ ] T1.4 Verify archive candidate  Owner: integration  kind: agent  stage: verify  deps: [T1.3]  acc: [affected tests and race checks, format and file-first checks pass on exact candidate with redaction, quota, retry, isolation and restart evidence; Wazi plan validates]
- [ ] T1.5 Independently review archive candidate  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.4]  acc: [separate reviewer records exact base and head, full T1.1 through T1.4 diff, findings and resolved re-review evidence]
- [ ] T1.6 Guarded archive merge  Owner: integration  kind: agent  stage: merge  deps: [T1.5]  acc: [current holds and protections permit exact reviewed head rebase merge with hosted CI or explicitly labeled authorized local fallback]
- [ ] T1.7 Verify archive landing  Owner: integration  kind: agent  stage: verify  deps: [T1.6]  acc: [landed commit is reachable from main and owned tree matches reviewed candidate with behavior checks]

- [ ] T1.8 Verify gated foundation increment  Owner: integration  kind: agent  stage: verify  deps: [T1.2, T2.0, T2.9, T2.10]  acc: [exact candidate passes source-store and gateway and queue and decoder and synthetic transport tests, race checks, build and vet; production capture remains default-off and synthetic allowlisted only]
- [ ] T1.9 Independently review gated foundation  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.8]  acc: [separate reviewer covers exact full candidate including T1.1-T1.2 and T2.0 and T2.9-T2.10 with findings fixed and re-reviewed]
- [ ] T1.10 Merge gated foundation  Owner: integration  kind: agent  stage: merge  deps: [T1.9]  acc: [exact reviewed head merges under current holds and protections and authorized ADR024 fallback without enabling unqualified capture]
- [ ] T1.11 Verify gated foundation landing  Owner: integration  kind: agent  stage: verify  deps: [T1.10]  acc: [landed owned tree matches reviewed candidate and synthetic qualification evidence is source-bound]

### E2 — Shared client and host continuity
fidelity: executable
Acceptance: enrolled histories are captured safely and can continue across qualified hosts; unsupported surfaces remain visible.

- [ ] T2.0 Implement independent capture-policy and queue primitives  Owner: client-lane  kind: agent  stage: implement  deps: [T1.1]  acc: [synthetic fixtures verify pre-staging redaction, explicit workspace enrollment, private and pause controls, bounded encrypted queue, stable visible-event identity and content-free gaps; no host enrollment or upload is enabled]
- [ ] T2.9 Prepare version-pinned visible-history reader  Owner: integration  kind: agent  stage: implement  deps: [T1.1]  acc: [Codex 0.163.0 public-source fixtures preserve covered visible messages, reject rewrites and unknown versions, exclude reasoning and report unsupported tool or attachment coverage without reading real histories]
- [ ] T2.10 Connect synthetic queue transport  Owner: integration  kind: agent  stage: implement  deps: [T1.1]  acc: [TLS HTTP synthetic fixtures verify current binding and complete fence synchronization before queued publication, exact canonical receipt validation and retry retention; no production enrollment is enabled]
- [ ] T2.1 Implement protected shared client and import  Owner: client-lane  kind: agent  stage: implement  deps: [T1.7, T2.0]  acc: [bounded protected queue redacts before staging, preserves stable history identities and progress, pauses private or unmapped sessions, verifies brain and fences before drain, and reports attachment and format gaps]
- [ ] T2.2 Integrate Zati first  Owner: integration  kind: agent  stage: implement  deps: [T2.1]  acc: [isolated Zati change budgets full requests and preserves intact tool pairs across the 200-message boundary using acknowledged checkpoint ranges with restart and cross-agent continuation tests]
- [ ] T2.3 Qualify requested host adapters  Owner: client-lane  kind: agent  stage: implement  deps: [T2.2]  acc: [Cursor, Codex, Claude Code and Antigravity each have official current capability evidence, configuration-preserving installation, actual host capture and import receipts, pause controls and honest unsupported fields]
- [ ] T2.4 Qualify durable facts and bounded recall  Owner: integration  kind: agent  stage: implement  deps: [T2.3]  acc: [standing-policy source-linked facts exclude recursive recall evidence and retain uncertainty; bounded historical retrieval and handoff retain originals and report measured coverage]
- [ ] T2.5 Verify client and host candidates  Owner: integration  kind: agent  stage: verify  deps: [T2.4]  acc: [all T2 implementation acceptance fixtures pass including revoked grants, rotation, interruption, queue capacity and malicious quoted instructions; actual host gaps are recorded]
- [ ] T2.6 Independently review client and host candidates  Owner: independent-reviewer  kind: agent  stage: review  deps: [T2.5]  acc: [separate exact-revision review covers T2.1 through T2.5 and accepted findings are fixed and re-reviewed]
- [ ] T2.7 Merge client and host candidates  Owner: integration  kind: agent  stage: merge  deps: [T2.6]  acc: [each scoped candidate passes current repository gates and exact reviewed-head guarded merge]
- [ ] T2.8 Verify client and host landing  Owner: integration  kind: agent  stage: verify  deps: [T2.7]  acc: [each landed tree matches reviewed source and narrow end-to-end continuity checks pass]

### E3 — Deployment and operational qualification
fidelity: executable
Acceptance: deployed capabilities and versions have live evidence, rollback and explicit incomplete coverage.

- [ ] T3.1 Qualify existing deployment target and rollback  Owner: integration  kind: agent  stage: verify  deps: [T1.1]  acc: [current service revision, signing and release authority, backup and rollback path are observed through existing runbooks without credentials in evidence]
- [ ] T3.2 Deploy qualified archive increment  Owner: integration  kind: agent  stage: deploy  deps: [T1.11, T3.1]  acc: [existing release process installs reviewed signed version with explicit capability gate, preserves WorkOS and billing defaults and records rollback receipt]
- [ ] T3.3 Verify live archive behavior  Owner: integration  kind: agent  stage: verify  deps: [T3.2]  acc: [actual deployed revision passes synthetic authenticated narrow API and isolation checks; unqualified capture and erasure remain unavailable]
- [ ] T3.4 Deploy qualified client and host release  Owner: integration  kind: agent  stage: deploy  deps: [T2.8, T3.3]  acc: [qualified host packages and Zati release have version-bound deployment receipts without unrelated UI changes]
- [ ] T3.5 Measure continuity and operations  Owner: integration  kind: agent  stage: verify  deps: [T3.4]  acc: [live recovery and continuation receipts, predeclared capacity workloads, purge and rebuild evidence distinguish measured limits from pending scale and soak coverage]
- [ ] T3.6 Checkpoint final delivery evidence  Owner: integration  kind: agent  stage: author  deps: [T3.5]  acc: [result records completed and pending scope, exact PR and landed and deployed versions, tests, live checks, rollback and blockers; all helper processes have stopped]

## Batches and gates

| Batch | Members | Owner | Prerequisites | Verification/review/merge/landed | Selection | Requested/actual size |
| --- | --- | --- | --- | --- | --- | --- |
| Archive | T1.1-T1.3 | integration plus isolated archive-lane | validated plan and protocol freeze | T1.4-T1.7 | gpt-6-astra / low | 3/3 |
| Client and hosts | T2.1-T2.4 | integration plus isolated client-lane | T1.7 | T2.5-T2.8 | gpt-6-astra / low | 4/4 |
| Operations | T3.1-T3.6 | integration | explicit deployment authority and dependency rows | live receipts | gpt-6-astra / low | 6/6 |

Split a candidate when independent host ownership, unsupported capabilities or review size requires it; preserve task IDs and add distinct delivery gate rows before dispatch. Deploy stages are authored obligations, not ordinary apply admission. Headless helpers stop/checkpoint before the integration deadline. No model polling unchanged jobs. Missing telemetry is unavailable, not estimated savings.

## Evidence and handoff

Session STATUS.md and result.md in the owner-designated external artifact directory hold detailed commands and receipts without secrets. This plan starts with all implementation acceptance open. Checkpoint is mandatory at deadline even if T3.6 dependencies remain incomplete. Existing authored plans and worktrees are preserved. Hierarchical retrieval is gated on comparative evidence; direct retrieval is the baseline. Physical history rewriting and managed-backup deletion are not safe to enable merely because logical exclusion passes tests.

Independent preparation: T2.0 may run concurrently with T1.2 against the documented protocol direction. It owns only new internal/episodeclient files and focused fixtures; no server edits or actual upload/enrollment. Its candidate remains unmerged until integration, verification and independent review explicitly cover it. This is a speculative source assignment, not authorization to activate capture.

T2.9 is independent speculative preparation in internal/episodehost only; it uses synthetic fixtures and current public version-pinned source. Host installation and real-history capture stay blocked by T2.1 and erasure qualification. Its source joins T2.5/T2.6 verification and exact-revision review.

Foundation split: the first candidate comprises T1.1-T1.2, T2.0, T2.9 and T2.10, with delivery gates T1.8-T1.11. It may deploy default-off under T3.2. T1.3-T1.7 remain open for full erasure/restore qualification and cannot be inferred complete from this increment. T2.10 is an isolated speculative synthetic integration under internal/episodecapture plus focused gateway fixtures; no real upload is enabled. Full host and production-capture dependencies remain unchanged. The external deletion journal currently accepts only brain/account subjects and foreign T23.48/T23.49 claims remain protected.
