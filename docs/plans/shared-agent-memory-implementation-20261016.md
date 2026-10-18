# Shared agent memory implementation

## Scope and authority

Implement RFC 0002 and RFC 0003 at design base 92b785fe6da569fa537b7ec92fb4a14197c8c3ae, preserving ADR027-029 and all existing memory interfaces. Owner explicitly authorizes implementation, independent review, merge and deployment. The first attempt checkpointed its default-off foundation. Resume 01 runs 2026-10-16 16:58–22:58 UTC, with helpers stopped by 22:55 UTC; unfinished scope remains open. No new infrastructure, unrelated UI, destructive production migration or provider substitution.

Existing Git/file SourceStore and serialized writer remain canonical; SQLite stays derived. Existing authenticated brains/grants remain the isolation boundary. Capture is gated until erasure/replay/restore qualification. No automatic real-history enrollment during qualification. Use synthetic data and explicit capability limits. No project label can authorize a brain. Hosted capture must not be activated before physical erasure and restore fences qualify.

## Zati experience and efficiency refinement — 2026-10-16

The owner requires a great Zati experience with token efficiency and responsive
execution. Zati is the main user-facing system; Serenity supplies durable memory
and historical evidence; Gist supplies discoverable versioned skills, complete
packages, exact schemas and capability resolution. See
[ADR 030](../adr/030-zati-context-and-gist-boundaries.md). This refinement is
ordinary repository planning, not runtime enrollment, application execution or a
renewal of the elapsed Resume 01 window. Prior implementation/deployment grants
remain scoped to their original task and conditions; new cross-repository rows
require their owning repository's accepted handoff and actual execution binding.

All 79 prior task rows, their authored statuses, acceptance, owners and dependency
edges are retained unchanged. E4 adds a coordinated acceptance slice; it does not
replace Zati, Gist or Foundry task state. Existing T2.2 still owns checkpointed
Zati continuity; T4.2 consumes that result rather than implementing a second
archive or context scheduler. Each external implementation needs a canonical
owner/task/receipt mapping before dispatch. Unassigned IDs are genuinely TBD.
The old Operations/T3 release obligations remain distinct; this refinement
creates no new release, deployment, provider-spend or enrollment grant.

Executor for this planning artifact: current attended Codex desktop session,
engineering profile selected for capability discovery; existing loaded desktop
tools remain unchanged. Helpers inherit the current model, reasoning and account
route without overrides. Future E4 batches use current-session inheritance at
execution time, not the earlier attempt's gpt-6-astra/low selection. No unattended
route is qualified. One author and one independent read-only reviewer suffice.
No Ajent MCP is exposed; inspect repository coordination and PR comments and
explicitly disclose that limitation in the planning PR. Existing graph snapshot
predates this source; use bounded source/caller/test inspection instead.

### User journeys and observable acceptance

| Use case | Zati behavior to qualify |
| --- | --- |
| ZUX-01 Fresh conversation or authorized worker | Relevant preferences and project decisions appear without a reminder; unrelated/restricted material does not; a useful no-match remains possible |
| ZUX-02 Accomplish a task using a skill | Relevant summaries lead to exact package and asset retrieval, capability checks and admitted execution; missing capabilities have a clear next step; an ordinary user need not understand backend product names |
| ZUX-03 Remember, correct, inspect or stop using information | Natural conversation leads through existing policy to an evidenced save/correction/retraction or explicit unavailable/unknown status; skill version changes use normal Zati bindings |
| ZUX-04 Continue long-running work | A fresh session, restart and another authorized worker recover cited context and unfinished work with intact tool pairs; historical approvals are never replayed as grants |
| ZUX-05 Slow/offline dependencies and revocation | Progress is intelligible; bounded retries and queues preserve custody; optional enrichment can be omitted visibly, while required or revoked context prevents unsafe dispatch |
| ZUX-06 Efficient repeated work | Unchanged context is not repeatedly injected or fetched; exact skill caches and stable prompt prefixes reduce overhead without stale authorization or lower task quality |

Measurement records compare the same task set, authorized catalog, model/version,
provider, generation settings and environment. Record end-to-end and time-to-first
useful-response p50/p95, context preparation and dependency latency, tool round
trips, input/output/cached tokens, and cost per successfully completed task.
Separate model steps, tool effects, retrieval/embedding and background capture or
summary costs. Count failures/retries and report unavailable provider billing or
child usage instead of inferring savings from input-token cache percentage.
Freeze sample sizes, numerical context/latency/cost budgets and correctness and
privacy thresholds in T4.0 before candidate runs; no invented baseline or universal
percentage target. A speed/token improvement cannot compensate for failed task,
provenance, permission or deletion acceptance. Whole-request accounting includes
instructions, history/checkpoints, memory, complete selected skill requirements,
tool schemas, output allowance and provider overhead with tokenizer estimates
labeled. Required schemas/instructions and call/result pairs are never truncated.

## Discovery and execution

Existing governed HTTP gateway provides verified binding, maintenance/account/runtime locks, revocation rechecks and canonical writer publication. Existing memory v1 cannot archive conversations or physically erase them. Existing recovery claims and PR273 are foreign ownership; preserve them. PR277 proposes import, not episode implementation. No Ajent tool is available in this scoped session; disclose in candidate PR. No chief.md found in the two inspected Serenity checkouts. Recheck authenticated holds at merge.

Executor: current engineering session with bounded headless Codex helpers. Selection: gpt-6-astra, low reasoning, ChatGPT account authentication, approval never, danger-full-access. Maximum two concurrent implementation/review lanes; no recursive dispatch. Shared Mac load exceeds 10: no heavy local builds until load gate and owned lease. Existing Spark reports four available CPUs; qualify source transfer/image/toolchain before remote checks. Wazi reader, frozen schema and semantic validator gate this plan before subordinate implementation dispatch. No docs/design.md exists; authoritative design is the RFC/ADRs named above.

## Reliable automatic memory and recovery delivery — 2026-10-17

This is the current delivery refinement of the same authoritative plan, identity
`serenity:shared-agent-memory`, authority `github:sirerun/serenity`, revision
`automatic-memory-r3`. It implements the owner's existing RFC0002/0003 outcomes:
automatic enrolled-history import/capture and policy-covered durable knowledge,
useful bounded recall, acknowledged archive recovery, and four qualified host
adapters with Zati first. This planning invocation delivers documents only;
it does not renew Resume01, launch workers, enroll real histories, spend on
providers, install, release or deploy. Existing grants are reused only within
current target, scope, budget and expiry; missing live authority blocks that
operation, not all source work. No new infrastructure is selected.

Use the existing Git/file writer, hosted brain grants, operation identity and
external deletion journal. The synthetic API at this source explicitly reports
real_history_capture=false, physical_purge=false, managed_restore=false,
portable_export=false and automatic_extraction=false. `episodeerase.Build`
produces an inactive standalone episode-only candidate; mixed brains and unknown
derivatives are refused. `episodeclient` supplies encrypted queue/policy/import
primitives, and `episodecapture` supplies synthetic HTTP/decoder composition;
neither is a complete enrolled service. Generic FTS/vector/RRF exists, but the
actual episode API searches a bounded direct lexical scan. Extend these owners;
source presence is not service/host acceptance. No shared write engine, task
scheduler, second Gist preparer or mandatory inference service is introduced.

### Delivery sequence and useful increments

1. T5.0-T5.3 complete the erasure/restore/copy design and implementation feeding
   the existing T1.3-T1.7 safety gates. No real-history endpoint becomes available
   on inactive-candidate or exclusion-only evidence.
2. T5.4-T5.11 deliver the enrolled shared client, policy-covered facts and bounded
   retrieval through one source candidate. Covered import and new capture share
   stable event identity; background processing cannot recursively archive its
   own recall or block replies unnecessarily. Failures preserve bounded progress.
3. T5.12 and T2.2 consume actual Zati continuation owner receipts and qualify the
   archive bridge. SDK fixtures and existing governed-memory UX can proceed
   before that external runtime gate; full cross-host recovery cannot.
4. T5.13-T5.33 freeze actual host/version/surface profiles and deliver Codex,
   Claude Code, Cursor and Antigravity independently. Each has its own source
   checks, independent review and landing. Capability gaps remain visible;
   no host receives a complete-support claim from another host's receipt.
5. T5.34-T5.40 and existing T3.14/T3.15 complete the current-state maintenance
   and bounded synthetic release exercise. T5.41-T5.45 deliver separately gated
   production-enrollment code; T5.46 reviews the actual release/activation packet,
   T3.4 deploys only under current scoped authority, T5.47 verifies enrollment,
   and T3.5/T3.6 retain operations/soak and honest final delivery.

Real-history capture is a capability transition requiring source, exact current
release, erasure/restore qualification and an enrolled policy. Never remove the
synthetic request marker or widen its allowlist to fake that transition. The
extension design must version/negotiate new semantics while preserving old clients.
If mixed-brain purge or a required managed copy cannot be qualified, keep capture
unavailable for that scope and expose the limitation rather than weakening the
accepted retention/deletion requirement. Explicit residual user-held exports or
unreachable devices are reported; controlled service copies need actual cleanup
or a frozen disclosed deadline and proof. New file/blob formats and unsupported
large attachments require measured need, not speculative implementation.

### Acceptance and measurements

| Outcome | Completion evidence |
| --- | --- |
| RAM-01 Automatic enrolled import/capture | Covered old events plus concurrent new events survive crash/restart and rotation without duplicate publication; policy/private/pause/exclusions and unsupported fields are visible |
| RAM-02 Useful durable knowledge | Explicitly covered facts retain exact source, time, scope and uncertainty; correction and withdrawal affect dependent facts; repeated summaries/speculation do not become corroboration |
| RAM-03 Relevant bounded recall | The actually called API returns authorized cited evidence or truthful no-match/incomplete/degraded state within complete response/request budgets; old rare facts remain findable |
| RAM-04 Recoverable work | Exact acknowledged events, attachment bytes and checkpoints can be exported/read and consumed after origin loss by a qualified receiver; pending effects and approvals are revalidated, never replayed |
| RAM-05 Safe deletion and queues | Exclusion wins races with reads/uploads, controlled purge is evidenced, current deletion history fences rebuild/restore/offline rejoin, residual copies are disclosed |
| RAM-06 Operable host integration | Supported install/upgrade/removal preserves unrelated config; status distinguishes locally queued, remotely acknowledged, deriving, paused, partial, failed and unknown |

T5.0 and T4.0 freeze actual numerical storage, queue, attachment, import/capture,
retrieval and provider budgets before qualification. T4.0 continues to own shared
Zati end-to-end token/cost/latency comparison; do not create another scoring harness.
Use RFC0002's declared 10k/100k/1M synthetic event workloads with text/attachment
bytes and supported envelope recorded; bounded failures narrow support claims,
not labels or thresholds. Measure p50/p95 reply/context latency, retrieval quality,
continuation correctness, source coverage, token input/output/cache and full task
cost including extraction/recovery, CPU/RAM, queue lag, repository growth and
purge/rebuild/restore duration. Unavailable provider billing is unavailable.
No speedup is inferred from task counts, model caches or a catalog benchmark.

### Source ownership and verification recipes

| Slice | Existing owners and proportional recipe to qualify at preflight |
| --- | --- |
| Archive safety | `internal/episodeerase`, `store`, `writer`, hosted gateway/journal/backup; actual Git synthetic mixed-history and real HTTP status/body tests, focused race/vet and existing replay/restore fixtures |
| Shared client | `episodeclient`, `episodecapture`, `episodehost`; policy/queue/import/receipt and controlled HTTP tests with fsync failure, stable retry, outage, rotation and redaction |
| Facts/retrieval | Existing extraction/reconciliation, source eligibility, index/search and actual gateway; source-linked extraction and deletion fixtures, exact read/export and actual-path lexical/hybrid comparison |
| Zati bridge | Owning Zati focused-worker E-FO0/E-FO1 and memory/execution/controller contracts; no sibling SQL or checkpoint implementation in Serenity; consume independently landed owner tests |
| Host packages | One shared client plus thin separately owned host adapters; official installed-version evidence, controlled native hooks/import/injection/compaction and config rollback tests; missing recipes frozen in T5.13 |
| Release/operations | Existing signed release and private custody owner; actual current-state maintenance entry/closeout, cold restore, scoped API/capability denial and credential cleanup receipts |

No docs/design.md exists; RFC0002/0003 and ADR027-030 remain design authority.
The stale graph is not refreshed for document work; bounded source and direct
caller/test inspection supplies discovery. Executor is current attended engineering
session; future batches and independent reviewers inherit actual session model,
reasoning and account route, not the historical Resume01 selection. Task owners
are role assignments, not new service authority. `stage: deploy` remains an
explicit blocked operational obligation until its existing owner route qualifies.
The current plan's 91 original IDs/statuses/owners/evidence remain; additive rows
and strengthened dependencies are recorded in the preservation receipt.

## Checkable work

### E1 — Archive service and safety foundation
fidelity: executable
Acceptance: additive gated archive API has exact evidence, bounded admission, existing-brain isolation and truthful deletion capabilities.

- [x] T1.1 Freeze bounded protocol and preflight  Owner: integration  kind: agent  stage: implement  acc: [schema and capability limits are documented with canonical writer binding and current ownership and build route evidence]
- [x] T1.2 Implement archive service foundation  Owner: archive-lane  kind: agent  stage: implement  deps: [T1.1]  acc: [synthetic HTTP fixtures verify exact event and attachment reads, stable retry conflicts, publication acknowledgment, bounded quotas, current brain authorization, fences and checkpoint context without changing v1]
- [ ] T1.3 Qualify exclusion and physical erasure recovery  Owner: integration  kind: agent  stage: implement  deps: [T1.2, T5.2, T5.3]  acc: [tests distinguish immediate exclusion from physical purge and prevent stale imports, indexes, derivatives and restored backups from resurrecting deleted content; capture remains disabled for unqualified paths Complete original mixed-brain, controlled-copy and activation coverage from T5.0-T5.3; unsupported required safety paths keep capture unavailable.]
- [ ] T1.4 Verify archive candidate  Owner: integration  kind: agent  stage: verify  deps: [T1.3]  acc: [affected tests and race checks, format and file-first checks pass on exact candidate with redaction, quota, retry, isolation and restart evidence; Wazi plan validates Coverage includes T5.0-T5.3 and actual API/export/restore responses, preserved unrelated sources and interrupted physical cleanup.]
- [ ] T1.5 Independently review archive candidate  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.4]  acc: [separate reviewer records exact base and head, full T1.1 through T1.4 diff, findings and resolved re-review evidence Review scope explicitly includes T5.0-T5.3, all current erasure/backup/copy contracts and the complete candidate.]
- [ ] T1.6 Guarded archive merge  Owner: integration  kind: agent  stage: merge  deps: [T1.5]  acc: [current holds and protections permit exact reviewed head rebase merge with hosted CI or explicitly labeled authorized local fallback]
- [ ] T1.7 Verify archive landing  Owner: integration  kind: agent  stage: verify  deps: [T1.6]  acc: [landed commit is reachable from main and owned tree matches reviewed candidate with behavior checks]

- [x] T1.8 Verify gated foundation increment  Owner: integration  kind: agent  stage: verify  deps: [T1.2, T2.0, T2.9, T2.10]  acc: [exact candidate passes source-store and gateway and queue and decoder and synthetic transport tests, race checks, build and vet; production capture remains default-off and synthetic allowlisted only]
- [x] T1.9 Independently review gated foundation  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.8]  acc: [separate reviewer covers exact full candidate including T1.1-T1.2 and T2.0 and T2.9-T2.10 with findings fixed and re-reviewed]
- [x] T1.10 Merge gated foundation  Owner: integration  kind: agent  stage: merge  deps: [T1.9]  acc: [exact reviewed head merges under current holds and protections and authorized ADR024 fallback without enabling unqualified capture]
- [x] T1.11 Verify gated foundation landing  Owner: integration  kind: agent  stage: verify  deps: [T1.10]  acc: [landed owned tree matches reviewed candidate and synthetic qualification evidence is source-bound]

- [x] T1.12 Implement isolated episode erasure and replay primitives  Owner: erasure-lane  kind: agent  stage: implement  deps: [T1.2]  acc: [isolated synthetic episode-only repositories produce inactive candidates without excluded current or historical sources and with current supplied fences; unknown derivatives and mixed brains are refused, legacy journal formats are unchanged and no activation or physical completion is claimed]
- [x] T1.13 Verify erasure primitive candidate  Owner: integration  kind: agent  stage: verify  deps: [T1.12]  acc: [affected race and failure-injection checks validate snapshot-before-delete replay, interrupted retry, foreign-brain refusal and retained-copy limits on exact candidate]
- [x] T1.14 Independently review erasure primitive candidate  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.13]  acc: [independent exact base and head review covers T1.12 and all plan changes with findings resolved]
- [x] T1.15 Merge erasure primitive candidate  Owner: integration  kind: agent  stage: merge  deps: [T1.14]  acc: [exact reviewed head passes current holds and protection gates and guarded rebase merge]
- [x] T1.16 Verify erasure primitive landing  Owner: integration  kind: agent  stage: verify  deps: [T1.15]  acc: [landed tree equals reviewed source and main reachability and scoped behavior are recorded without claiming T1.3 complete]

- [x] T1.17 Integrate external episode replay fences  Owner: replay-lane  kind: agent  stage: implement  deps: [T1.2]  acc: [existing independently durable journal records opaque conversation exclusion before acknowledgment and verified startup history prevents older snapshots from serving or republishing excluded episodes; legacy journal bytes remain compatible and old readers fail closed on new subjects]
- [x] T1.18 Verify external episode fence candidate  Owner: integration  kind: agent  stage: verify  deps: [T1.17, T3.12]  acc: [journal ambiguity and outage, snapshot restore, startup admission, denied brain access and quota-full exclusion tests pass on exact candidate without physical purge claims]
- [x] T1.19 Independently review external episode fences  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.18]  acc: [independent exact base and head review covers T1.17 and T3.12 and compatibility and deployment rollback requirements with findings resolved]
- [x] T1.20 Merge external episode fence candidate  Owner: integration  kind: agent  stage: merge  deps: [T1.19]  acc: [current holds and protection gates permit exact reviewed-head guarded rebase merge]
- [x] T1.21 Verify external episode fence landing  Owner: integration  kind: agent  stage: verify  deps: [T1.20]  acc: [reviewed tree equals landed main tree and qualified source behavior is recorded]

### E1c — Restore-to-erasure cross-component qualification
fidelity: executable
Acceptance: a synthetic pre-exclusion snapshot restored behind complete admitted external history cannot regain readable evidence; inactive candidate construction removes excluded historical objects without pretending to activate or purge managed copies.

- [x] T1.22 Add restore-to-erasure qualification fixture  Owner: erasure-chain-lane  kind: agent  stage: implement  deps: [T1.16, T1.21]  acc: [isolated actual Git and existing journal/gateway/store paths exercise pre-exclusion snapshot restore, complete external replay, stale queue and dependent checkpoint refusal, inactive candidate construction and preserved unrelated sources; missing or conflicting history refuses preparation; no production or managed-backup mutation]
- [x] T1.23 Verify restore-to-erasure candidate  Owner: integration  kind: agent  stage: verify  deps: [T1.22]  acc: [focused affected race and vet and required static checks pass on exact source; controlled old-copy retention and physical-completion gaps remain explicit]
- [x] T1.24 Independently review restore-to-erasure candidate  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.23]  acc: [separate exact base and head review resolves findings without inferring authority from a local snapshot or weakening existing refusal boundaries]
- [x] T1.25 Merge restore-to-erasure candidate  Owner: integration  kind: agent  stage: merge  deps: [T1.24]  acc: [current claims and protections permit exact reviewed-head guarded rebase merge with accurately labeled verification]
- [x] T1.26 Verify restore-to-erasure landing  Owner: integration  kind: agent  stage: verify  deps: [T1.25]  acc: [landed tree equals reviewed candidate and main ancestry is verified; full T1.3 and real-capture gates remain open]

### E1d — Portable backup restore and exclusion replay qualification
fidelity: executable
Acceptance: actual existing manifest-v2 backup and frozen restore paths preserve their admission boundary while complete admitted external episode history excludes old canonical evidence before any synthetic read path.

- [x] T1.27 Exercise portable snapshot and episode replay  Owner: portable-restore-lane  kind: agent  stage: implement  deps: [T1.26]  acc: [isolated actual backup Create and Restore APIs snapshot before exclusion and restore frozen with revoked credentials; complete caller-admitted external history fences old episode and dependent checkpoint reads; missing history fails closed; no backup interface edit or production activation]
- [x] T1.28 Verify portable restore fixture  Owner: integration  kind: agent  stage: verify  deps: [T1.27]  acc: [focused cross-component race and vet and required static checks pass on exact source; restored accounts stay pending, retained snapshot and historical bytes are disclosed and mixed-repository candidate refusal remains intact]
- [x] T1.29 Independently review portable restore fixture  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.28]  acc: [separate exact base and head review verifies actual portable APIs and admission boundaries without promoting caller-trusted seals to provider authority]
- [x] T1.30 Merge portable restore fixture  Owner: integration  kind: agent  stage: merge  deps: [T1.29]  acc: [current claims and holds and protections permit exact reviewed-head guarded rebase merge]
- [x] T1.31 Verify portable restore landing  Owner: integration  kind: agent  stage: verify  deps: [T1.30]  acc: [landed tree equals reviewed source and main ancestry is verified; full T1.3 and managed production restore remain open]

### E2 — Shared client and host continuity
fidelity: executable
Acceptance: enrolled histories are captured safely and can continue across qualified hosts; unsupported surfaces remain visible.

- [x] T2.0 Implement independent capture-policy and queue primitives  Owner: client-lane  kind: agent  stage: implement  deps: [T1.1]  acc: [synthetic fixtures verify pre-staging redaction, explicit workspace enrollment, private and pause controls, bounded encrypted queue, stable visible-event identity and content-free gaps; no host enrollment or upload is enabled]
- [x] T2.9 Prepare version-pinned visible-history reader  Owner: integration  kind: agent  stage: implement  deps: [T1.1]  acc: [Codex 0.163.0 public-source fixtures preserve covered visible messages, reject rewrites and unknown versions, exclude reasoning and report unsupported tool or attachment coverage without reading real histories]
- [x] T2.10 Connect synthetic queue transport  Owner: integration  kind: agent  stage: implement  deps: [T1.1]  acc: [TLS HTTP synthetic fixtures verify current binding and complete fence synchronization before queued publication, exact canonical receipt validation and retry retention; no production enrollment is enabled]
- [ ] T2.1 Implement protected shared client and import  Owner: client-lane  kind: agent  stage: implement  deps: [T1.7, T2.0, T5.5]  acc: [bounded protected queue redacts before staging, preserves stable history identities and progress, pauses private or unmapped sessions, verifies brain and fences before drain, and reports attachment and format gaps T5.5 supplies the complete enrolled client; local queue acknowledgment, remote canonical acknowledgment and derived knowledge remain distinct.]
- [ ] T2.2 Qualify the archive bridge to Zati-owned continuation  Owner: integration  kind: agent  stage: implement  deps: [T2.1, T5.11, T5.12]  acc: [owning Zati E-FO0 contract and E-FO1 runtime receipts are pinned without advancing upstream tasks; map authorized canonical history ranges, complete current work and pending snapshots, actual artifact refs and checkpoints to acknowledged Serenity archive bytes with current scope and deletion fences; joined tests preserve whole-request budgets, intact tool pairs, original effect identity and cumulative limits across greater-than-200-message continuation, restart and cross-agent recovery; contract-only or local continuation never proves archive custody Any Zati code follows its owning canonical review/merge/landed lifecycle; this row consumes those receipts and qualifies the archive bridge rather than advancing foreign tasks.]
- [ ] T2.3 Qualify requested host adapters  Owner: client-lane  kind: agent  stage: implement  deps: [T2.2, T5.18, T5.23, T5.28, T5.33]  acc: [Cursor, Codex, Claude Code and Antigravity each have official current capability evidence, configuration-preserving installation, actual host capture and import receipts, pause controls and honest unsupported fields Aggregate all four T5.14-T5.33 host receipts without substituting one version or surface for another.]
- [ ] T2.4 Qualify durable facts and bounded recall  Owner: integration  kind: agent  stage: implement  deps: [T2.3, T5.11]  acc: [standing-policy source-linked facts exclude recursive recall evidence and retain uncertainty; bounded historical retrieval and handoff retain originals and report measured coverage T5.6-T5.7 own the fact/retrieval source delivered at T5.11; this original row closes aggregate host and handoff coverage, not duplicate implementation.]
- [ ] T2.5 Verify client and host candidates  Owner: integration  kind: agent  stage: verify  deps: [T2.4]  acc: [all T2 implementation acceptance fixtures pass including revoked grants, rotation, interruption, queue capacity and malicious quoted instructions; actual host gaps are recorded Include independently landed SDK and host receipts plus the actual Zati bridge owner candidate; a new joined code change must be reviewed before T2.7.]
- [ ] T2.6 Independently review client and host candidates  Owner: independent-reviewer  kind: agent  stage: review  deps: [T2.5]  acc: [separate exact-revision review covers T2.1 through T2.5 and accepted findings are fixed and re-reviewed Review original joined scope and any T2.2 bridge changes plus all E5 source receipts; never self-approve prior shared or host code.]
- [ ] T2.7 Merge client and host candidates  Owner: integration  kind: agent  stage: merge  deps: [T2.6]  acc: [each scoped candidate passes current repository gates and exact reviewed-head guarded merge]
- [ ] T2.8 Verify client and host landing  Owner: integration  kind: agent  stage: verify  deps: [T2.7]  acc: [each landed tree matches reviewed source and narrow end-to-end continuity checks pass]

- [x] T2.11 Persist atomic protected import progress  Owner: integration  kind: agent  stage: implement  deps: [T2.0, T2.9]  acc: [synthetic Codex pages atomically persist redacted records and content-free cursor and gaps in encrypted queue state; restart, capacity failure, competing cursor updates, pause and fences cannot silently skip unadmitted evidence]
- [x] T2.12 Verify protected import progress  Owner: integration  kind: agent  stage: verify  deps: [T2.11]  acc: [focused queue and adapter race tests plus format and static checks pass on exact candidate without host enrollment]
- [x] T2.13 Independently review protected import progress  Owner: independent-reviewer  kind: agent  stage: review  deps: [T2.12]  acc: [separate exact-revision review covers T2.11 and compatibility with findings resolved]
- [x] T2.14 Merge protected import progress  Owner: integration  kind: agent  stage: merge  deps: [T2.13]  acc: [current repository gates permit exact reviewed-head guarded rebase merge]
- [x] T2.15 Verify protected import progress landing  Owner: integration  kind: agent  stage: verify  deps: [T2.14]  acc: [landed tree matches reviewed source with source-bound synthetic acceptance and no real host completion claim]

### E2c — Version-pinned visible message phase coverage
fidelity: executable
Acceptance: documented Codex visible partial answers use the same safe decoder and protected capture path, with exclusions and source-only limits preserved.

- [x] T2.16 Cover documented Codex partial answers  Owner: codex-phase-lane  kind: agent  stage: implement  deps: [T2.9, T2.15]  acc: [pinned official 0.163.0 message phase is decoded with stable event identity and existing provenance, role, channel, encryption and content exclusions; no discovery, enrollment, upload or native-host claim]
- [x] T2.17 Verify partial-answer capture contract  Owner: integration  kind: agent  stage: verify  deps: [T2.16]  acc: [focused decoder and protected import fixtures verify pagination, retry, redaction and excluded metadata and unknown phases on exact source with affected race and required static checks]
- [x] T2.18 Independently review partial-answer increment  Owner: independent-reviewer  kind: agent  stage: review  deps: [T2.17]  acc: [separate exact base and head review covers phase semantics and all exclusions with findings resolved]
- [x] T2.19 Merge partial-answer increment  Owner: integration  kind: agent  stage: merge  deps: [T2.18]  acc: [current claims, holds and protection gates permit exact reviewed-head guarded rebase merge]
- [x] T2.20 Verify partial-answer landing  Owner: integration  kind: agent  stage: verify  deps: [T2.19]  acc: [landed tree equals reviewed source and main ancestry is verified without closing full T2.1 or T2.3 host qualification]

- [x] T2.21 Observe a controlled native Codex fixture  Owner: integration  kind: agent  stage: discover  deps: [T2.20]  acc: [a bounded explicitly synthetic session preserves selected model and scoped profile and exposes only known visible markers; no existing history, tool evidence, attachments or hidden reasoning is exported]
- [x] T2.22 Verify native fixture coverage and gaps  Owner: integration  kind: agent  stage: verify  deps: [T2.21]  acc: [actual installed version and content-free session provenance bind a sanitized visible projection to decoder results with exact exclusions; projected synthetic evidence is not full-history, plugin installation or T2.3 acceptance]

### E3 — Deployment and operational qualification
fidelity: executable
Acceptance: deployed capabilities and versions have live evidence, rollback and explicit incomplete coverage.

- [x] T3.1 Qualify existing deployment target and rollback  Owner: integration  kind: agent  stage: verify  deps: [T1.1]  acc: [current service revision, signing and release authority, backup and rollback path are observed through existing runbooks without credentials in evidence]
- [x] T3.2 Deploy qualified archive increment  Owner: integration  kind: agent  stage: deploy  deps: [T1.11, T3.1]  acc: [existing release process installs reviewed signed version with explicit capability gate, preserves WorkOS and billing defaults and records rollback receipt]
- [ ] T3.3 Verify live archive behavior  Owner: integration  kind: agent  stage: verify  deps: [T3.2, T3.15, T1.7]  acc: [actual deployed revision passes synthetic authenticated narrow API and isolation checks; unqualified capture and erasure remain unavailable Actual synthetic API acceptance includes current safety source and cleanup receipts; source deployment alone does not close this gate.]
- [ ] T3.4 Deploy qualified client and host release  Owner: integration  kind: agent  stage: deploy  deps: [T2.8, T3.3, T5.46]  acc: [qualified host packages and Zati release have version-bound deployment receipts without unrelated UI changes Use the current T5.46 reviewed release and qualified capability/enrollment envelope; no old synthetic gate is used as a real-history bypass.]
- [ ] T3.5 Measure continuity and operations  Owner: integration  kind: agent  stage: verify  deps: [T3.4, T5.47]  acc: [live recovery and continuation receipts, predeclared capacity workloads, purge and rebuild evidence distinguish measured limits from pending scale and soak coverage Include cold restore and origin-loss recovery, RAM-01 through RAM-06, per-host supported limits and RFC0002 scale/soak receipts; failure leaves the affected support claim open.]
- [ ] T3.6 Checkpoint final delivery evidence  Owner: integration  kind: agent  stage: author  deps: [T3.5]  acc: [result records completed and pending scope, exact PR and landed and deployed versions, tests, live checks, rollback and blockers; all helper processes have stopped]

- [x] T3.7 Prepare scoped synthetic production qualification tooling  Owner: fixture-lane  kind: agent  stage: implement  deps: [T1.1]  acc: [reviewable offline operator fixture uses existing store and provisioner and issuer for exactly two dedicated synthetic identities with private credentials, explicit maintenance guards, no email assertions or sends and no real-user credential reuse]
- [x] T3.8 Verify synthetic qualification tooling  Owner: integration  kind: agent  stage: verify  deps: [T3.7]  acc: [isolated interruption and retry and foreign-identity refusal and quota and credential custody tests pass; production mutation remains blocked until exact operational review]
- [x] T3.9 Independently review synthetic qualification tooling  Owner: independent-reviewer  kind: agent  stage: review  deps: [T3.8]  acc: [independent exact source and offline custody contract review accepts provisioning scope, maintenance preconditions, cleanup and credential custody with findings resolved; final live packet remains separately gated by T3.13]
- [x] T3.10 Merge synthetic qualification tooling  Owner: integration  kind: agent  stage: merge  deps: [T3.9]  acc: [current holds and protections permit exact reviewed-head guarded merge without authorizing any unreviewed live operation]
- [x] T3.11 Verify qualification tooling landing  Owner: integration  kind: agent  stage: verify  deps: [T3.10]  acc: [landed tree equals reviewed source and live execution remains contingent on refreshed target and signed deployment and rollback qualification]
- [x] T3.12 Implement expiring private synthetic qualification gate  Owner: integration  kind: agent  stage: implement  deps: [T1.17]  acc: [existing private admin transport can temporarily allow at most two dedicated synthetic fixture brains for at most fifteen minutes, public transport cannot set it, expiry or disable closes access, safety build refuses enablement, private cleanup revokes only explicitly named dedicated fixture credentials, and permanent config is unchanged]
- [x] T3.13 Independently review bounded synthetic deployment packet  Owner: independent-reviewer  kind: agent  stage: review  deps: [T1.21, T3.11]  acc: [exact landed source and signed feature and compatible safety artifacts, actual authority code change set and policies, root custody producer and consumer, target pins and cleanup and recovery packet are independently accepted before execution]
- [ ] T3.14 Deploy and exercise bounded synthetic replay increment  Owner: integration  kind: agent  stage: deploy  deps: [T3.13, T5.40]  acc: [fresh verified target and rollback authorization permit guarded existing-capacity authority and host deployment plus exactly two scoped synthetic fixtures; no real capture or customer-data migration is enabled T5.40 is the new exact operational review; the earlier T3.13 approval is retained as historical evidence and cannot authorize changed current-state packets.]
- [ ] T3.15 Verify synthetic replay release and cleanup receipts  Owner: integration  kind: agent  stage: verify  deps: [T3.14]  acc: [exact deployed authority and host versions, public health and disabled defaults, scoped authenticated API and restart replay checks, revoked fixture keys, retained synthetic data, rollback compatibility and all unqualified coverage are recorded; full T3.3 remains open until its original coverage passes]

### E3b — Recovery of already-revoked synthetic fixture credentials
fidelity: executable
Acceptance: cold offline custody can revoke or reissue exactly recorded fixture credentials after independent revocation, without treating revoked credentials as live authority.

- [x] T3.16 Recover externally revoked fixture custody  Owner: integration  kind: agent  stage: implement  deps: [T3.11]  acc: [offline reissue and revoke accept only correctly verified revoked secrets bound to the exact recorded fixture account and brain and credential ID; prepare still refuses retained revoked keys; public credential authentication and cold custody gates remain unchanged]
- [x] T3.17 Verify revoked fixture recovery  Owner: integration  kind: agent  stage: verify  deps: [T3.16]  acc: [affected race and vet and static checks cover private-admin-style external revocation, exact-two reissue, idempotent revoke, wrong secrets and swapped or foreign revoked keys with no unintended mutations]
- [x] T3.18 Independently review revoked fixture recovery  Owner: independent-reviewer  kind: agent  stage: review  deps: [T3.17]  acc: [separate exact base and head review covers custody and refusal boundaries with findings resolved]
- [x] T3.19 Merge revoked fixture recovery  Owner: integration  kind: agent  stage: merge  deps: [T3.18]  acc: [current claim and hold and protection gates permit exact reviewed-head guarded rebase merge]
- [x] T3.20 Verify revoked fixture recovery landing  Owner: integration  kind: agent  stage: verify  deps: [T3.19]  acc: [landed tree equals reviewed source and main ancestry is verified; existing deployment and cross-tool-source manifest migration remain separately gated]


### E3c — Explicit offline fixture source adoption
fidelity: executable
Acceptance: an explicitly reviewed new operator can adopt an exact older fixture manifest under fresh inherited root custody without granting live credentials, retagging arbitrary state or changing customer data.

- [x] T3.21 Implement exact offline fixture source adoption  Owner: fixture-adoption-lane  kind: agent  stage: implement  deps: [T3.20]  acc: [a distinct root-authorized operation binds old source and exact old private manifest hash plus current tool source; only complete dedicated fixture identities with all recorded credentials revoked may be adopted; other target fields and identities remain exact; no account or credential issuance and no implicit migration]
- [x] T3.22 Verify offline source adoption  Owner: integration  kind: agent  stage: verify  deps: [T3.21]  acc: [focused race and vet and operator models and required static checks cover wrong source or hash, live or foreign keys, incomplete state, interruption, changed custody and retry outcomes without secrets in evidence]
- [x] T3.23 Independently review source adoption  Owner: independent-reviewer  kind: agent  stage: review  deps: [T3.22]  acc: [separate exact base and head review covers producer and consumer custody, confined manifest-only mutation and all refusal boundaries with findings resolved]
- [x] T3.24 Merge source adoption increment  Owner: integration  kind: agent  stage: merge  deps: [T3.23]  acc: [current claims, holds and protections permit exact reviewed-head guarded rebase merge]
- [x] T3.25 Verify source adoption landing  Owner: integration  kind: agent  stage: verify  deps: [T3.24]  acc: [landed tree equals reviewed source and main ancestry is verified; live installation, current cold maintenance, reissue and full API qualification remain separate gates]

### E4 — Unified Zati context, skills and measured experience
fidelity: executable
Acceptance: ZUX-01 through ZUX-06 work through Zati with bounded relevant context, exact skills and truthful progress, and measured task quality, tokens, cost and latency. Source acceptance and owner-observed behavior remain separate from T3 deployment.

- [ ] T4.0 Freeze journeys and measure the current baseline  Owner: zati-integration  kind: agent  stage: discover  acc: [ZUX-01 through ZUX-06 have reproducible synthetic fixtures and owner-selected representative tasks; current source and installed app identities, sample sizes, numerical whole-request and p50/p95 latency and cost budgets, correctness thresholds, measurement boundaries and missing telemetry are recorded before candidate comparison; no baseline or savings is fabricated]
- [ ] T4.1 Reconcile current consumer and context contracts  Owner: integration  kind: agent  stage: author  deps: [T4.0]  acc: [pin current public Gist catalog/preparation/composition APIs and Zati revision49 continuation owners with exact repository task/file/receipt mappings; reuse selected zati/inbox-briefing v1.0.0 template and explicit v2.0.0 composition plus supported schemas and real admitted capability rather than rediscovering identity; freeze only uncovered memory/context interfaces, byte/token bounds, cancellation, no-match and required versus optional behavior; no duplicate preparer, archive builder or scheduler]
- [ ] T4.2 Integrate one bounded context preparation path  Owner: zati-integration  kind: agent  stage: implement  deps: [T4.1]  acc: [ZUX-01 uses existing Zati context preparation and governed memory with the Gist-prepared complete skill/schema/guide bundle; independent authorized reads overlap within deadlines and whole-request budget preserves required instructions, tool pairs and output reserve; local continuation consumes qualified Zati E-FO1 runtime receipts without requiring archive activation; missing archive/continuation capability is explicit and full ZUX-04 waits for T2.2 and T4.7; no unconditional recent-fact dump or second preparation scheduler]
- [ ] T4.3 Consume the delivered Gist skill and composition owners  Owner: gist-integration  kind: agent  stage: implement  deps: [T4.1]  acc: [map Gist T-COMPOSE.8-.14 and owning Zati source receipts into ZUX-02 and identify only uncovered Serenity integration deltas; current consumer pin and exact immutable package, template/composition assets, explicit v1/v2 selection and existing Zati import/binding/effect path remain intact; preparation requires current positive task coverage and exact required capabilities, schemas and account/connection affinity, with explicit non-readiness and bounded partial evidence; authorized-before-exposure discovery uses embedded APIs or explicitly selected hosted catalog without mandatory Serenity backend, silent fallback or new Gist executor; source evidence is not real Gmail/provider acceptance]
- [ ] T4.4 Make memory and skill state understandable in Zati  Owner: zati-experience  kind: agent  stage: implement  deps: [T4.2, T4.3]  acc: [ZUX-02 and ZUX-03 and ZUX-05 expose relevant sources and applied skill versions on demand, actionable missing connections/capabilities and bounded progress without backend setup jargon; remember, inspect, correct and stop-using journeys use existing policy and version operations without redundant approvals; saved, queued, blocked and unknown remain distinct; memory exclusion versus physical erasure and chat deletion are truthful; actual native preview acceptance is recorded separately from widget fixtures]
- [ ] T4.5 Qualify reuse and remove avoidable context overhead  Owner: zati-integration  kind: agent  stage: implement  deps: [T4.2, T4.3]  acc: [ZUX-06 retrieves summaries then exact content on demand, avoids unchanged excerpt reinjection, caches immutable packages by version and digest, and keys any retrieval reuse by authenticated brain, authorized scope and current policy/version/freshness state; revocation, deletion, expiry and changed bindings invalidate reuse before exposure or action; stable prompt ordering supports caching and deterministic checks avoid extra model calls; correctness survives cold cache and cache failure]
- [ ] T4.6 Compare actual memory retrieval against scoped upstream evidence  Owner: serenity-retrieval  kind: agent  stage: verify  deps: [T4.2, T4.3]  acc: [same labeled ZUX tasks measure the actual governed/episodic memory call path with BM25/configured hybrid where implemented; reuse pinned Gist preparation and 10k/100k index receipts only for their named catalog workload, retaining failures and unavailable model/token data; local embedding adapter source qualification does not establish default adoption, cross-modal memory or Zati latency; revalidate runtime and numerical authority before any new trial; shared retrieval backend requires measured benefit and preserves exact packages, current policy and explicit fallback]
- [ ] T4.7 Verify the integrated candidate and efficiency budgets  Owner: integration  kind: agent  stage: verify  deps: [T4.4, T4.5, T4.6, T2.2]  acc: [all T4.1-T4.6 outcomes and T2.2 archive bridge have owning source/test receipts; Zati E-FO1 runtime and source coverage, current full child/pending snapshots and fenced continuation are qualified before ZUX-04 acceptance; run existing context/memory/Gist composition/skill tests plus affected Serenity retrieval tests and native preview journeys; wrong brain, tampering, no-match, stale/revoked bindings, deletion, timeout, restart, cancellation and unknown effects pass; fixed T4.0 quality/cost/latency budgets pass or remain blocked; background capture does not unnecessarily delay replies]
- [ ] T4.8 Independently review the coordinated candidate  Owner: independent-reviewer  kind: agent  stage: review  deps: [T4.7]  acc: [independent reviewers record exact base/head and scope for every owning repository implementation including plan changes, tests and ZUX-01 through ZUX-06 coverage; findings have tracked fixes, affected verification and re-review before merge; no author self-approval or transferred execution authority]
- [ ] T4.9 Merge reviewed owning repository candidates  Owner: integration  kind: agent  stage: merge  deps: [T4.8]  acc: [each owning repository accepts its mapped candidate and current claims, holds, required checks and protection; independent exact-head approval and refreshed base permit guarded rebase merge without cross-repository bypass]
- [ ] T4.10 Verify source landing and record the operational handoff  Owner: integration  kind: agent  stage: verify  deps: [T4.9]  acc: [all mapped commits are reachable on intended targets and reviewed/landed trees and affected checks agree; exact source, pending installation/live gates, reversible configuration path and prerequisite edges into existing T3.4/T3.5 and owning Zati release tasks are recorded before release dispatch; no source receipt is called deployment]
- [ ] T4.11 Accept the observed Zati journey and publish comparison evidence  Owner: zati-experience  kind: agent  stage: verify  deps: [T4.10]  acc: [owner observes ZUX-01 through ZUX-06 in the identified native development app with controlled accounts or separately authorized live bindings; task quality and full token/cost/latency comparison meet predeclared budgets; absent installed/live/long-soak evidence is explicit and corresponding release gates stay open; unsupported host behavior and remaining work have an owning task]

### E5 — Complete automatic memory and recovery
fidelity: executable
Acceptance: RAM-01 through RAM-06 have separately source-bound and released receipts, all four qualified host profiles and the Zati bridge preserve scoped evidence and current authority, and incomplete or unsupported capabilities stay visibly unavailable.

- [x] T5.0 Reconcile remaining capabilities and freeze qualification envelope  Owner: integration  Est: TBD  kind: agent  stage: preflight  acc: [RAM-01 through RAM-06 have exact source and owner/task mappings, supported brain/attachment/host scopes, numerical capacity and purge/restore/latency limits and controlled API test recipes; active claims, current deployment capability and pending maintenance prerequisites are recorded; no provider or real-history admission follows]
- [x] T5.1 Freeze safe erasure and production capability extension  Owner: archive-lane  Est: TBD  kind: agent  stage: author  deps: [T5.0]  acc: [versioned API/capability, complete authoritative deletion-history producer/consumer, mixed-brain and shared-attachment handling, bounded maintenance activation/rollback, exact export/restore and controlled-copy deadlines are frozen before code; synthetic-only compatibility remains and unsupported paths stay unavailable]
- [ ] T5.2 Complete canonical purge and derivative-safe activation  Owner: archive-lane  Est: TBD  kind: agent  stage: implement  deps: [T5.1]  acc: [under existing writer/maintenance locks actual mixed-brain candidates preserve unrelated canonical bytes and remove excluded original/history/derivative content; trusted current external fences bind activation and crash recovery; all controlled stages and old object copies have bounded cleanup or explicit residual state; exclusion-only and inactive stages never emit physical-complete receipts]
- [ ] T5.3 Fence controlled backups exports restores and device rejoin  Owner: archive-lane  Est: TBD  kind: agent  stage: implement  deps: [T5.1]  acc: [actual supported backup/export/restore APIs apply current independently admitted deletion history before disclosure or publication; content-free receipt identity survives erasure without payload retention; shared references and controlled copy lifecycle have exact evidence; old snapshots, queues and stale derivatives cannot resurrect content and unsupported custody refuses readiness]
- [ ] T5.4 Freeze enrolled-client and factual-retrieval source assignment  Owner: client-lane  Est: TBD  kind: agent  stage: preflight  deps: [T1.7]  acc: [existing queue/import/HTTP decoder components, credential/key custody and versioned archive API are mapped to exact owned files with safe installation/enrollment inputs; fact policy, query-relevance and incomplete-budget semantics and required checks are fixed; current SDK and legacy interfaces remain compatible]
- [ ] T5.5 Deliver automatic import continuous capture and recovery client  Owner: client-lane  Est: TBD  kind: agent  stage: implement  deps: [T5.4]  acc: [RAM-01 and RAM-06 use explicit stable enrolled brain/policy/workspace identities and existing secure custody; import and live events share durable encrypted dedup/progress, redact before staging, pause private/unmapped work and bound queues/retries; HTTP binding/fence synchronization precedes each drain and exact remote receipts precede removal; timeout/absence is unknown and restart/rotation cannot skip evidence]
- [ ] T5.6 Derive and correct policy-covered durable knowledge  Owner: memory-lane  Est: TBD  kind: agent  stage: implement  deps: [T5.4]  acc: [RAM-02 uses existing extraction/reconciliation outside writer commits under explicit current policy and provider limits; exact event provenance and historical time survive with uncertainty; repeated speculative/model/recall content is not independent evidence; correction, withdrawal and deletion invalidate dependent derivatives while uncovered or sensitive saves retain existing consent requirements]
- [ ] T5.7 Deliver bounded actual-path recall exact export and continuation evidence  Owner: memory-lane  Est: TBD  kind: agent  stage: implement  deps: [T5.4]  acc: [RAM-03 and RAM-04 use the actual hosted memory/episode route with current brain/expiry/deletion eligibility, source citations and whole-response budgets; relevant older facts beat unrelated recent ones; lexical fallback is explicit and hybrid remains configured/evaluated; exact export preserves admitted original and attachment bytes or refuses unsupported size, with incomplete scans distinct from true no-match and no summary-only recovery claim]
- [ ] T5.8 Verify shared-client and knowledge candidate  Owner: integration  Est: TBD  kind: agent  stage: verify  deps: [T2.1, T5.6, T5.7]  acc: [T5.5-T5.7 and T2.1 pass source-bound policy/queue/import, actual HTTP status/body, extraction and actual-path retrieval/export fixtures plus affected race/vet/static checks; tests cover no-match, wrong brain, malicious text, deletion/revocation during I/O, quota/fsync failure and restart; default real capture remains off]
- [ ] T5.9 Independently review shared-client and knowledge candidate  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.8]  acc: [non-author records exact base/head and complete T5.4-T5.8 plus T2.1 scope including new protocols and custody boundaries; accepted findings have tracked fix, affected checks and current re-review without weakening capture gates]
- [ ] T5.10 Merge reviewed shared-client candidate  Owner: integration  Est: TBD  kind: agent  stage: merge  deps: [T5.9]  acc: [current claims/holds/protections and required checks permit exact-reviewed-head guarded rebase with source and verification provenance]
- [ ] T5.11 Verify landed shared-client candidate  Owner: integration  Est: TBD  kind: agent  stage: verify  deps: [T5.10]  acc: [landed source is reachable and tree-equivalent to reviewed candidate with focused client/knowledge checks; exported integration package and capability limits are recorded without installing a host or enabling capture]
- [ ] T5.12 Bind the Zati archive consumer to current runtime owners  Owner: integration  Est: TBD  kind: agent  stage: preflight  deps: [T5.11]  acc: [exact Zati E-FO0 contract and independently reviewed E-FO1 runtime receipt identify history ranges, complete work/pending snapshots, checkpoint artifacts and source coverage; owning tasks and bridge file assignments are accepted before T2.2 implementation; absent runtime receipts block joined recovery rather than start a second scheduler]
- [ ] T5.13 Freeze four actual host profiles and install contracts  Owner: client-lane  Est: TBD  kind: agent  stage: preflight  deps: [T5.11, T2.2]  acc: [Codex, Claude Code, Cursor and Antigravity each name installed product/version/surface, supported lifecycle/history/attachment/injection/compaction access, native trust and secure custody, preserve/rollback/remove manifest and synthetic conformance recipe; no guessed hooks, arbitrary app-db scraping or unsupported full-history claim]
- [ ] T5.14 Implement Codex shared-memory adapter  Owner: codex-adapter  Est: TBD  kind: agent  stage: implement  deps: [T5.13]  acc: [RAM-01 and RAM-06 use the frozen Codex profile and shared client for enrollment, covered historical import, incremental capture, bounded relevant injection and checkpoint/handoff; installer owns only declared config, respects native trust, pause/private/redaction and secure custody; unsupported tool/attachment/native-resume fields produce visible gaps]
- [ ] T5.15 Qualify actual Codex adapter  Owner: codex-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.14]  acc: [actual frozen Codex surface runs controlled synthetic install/import/live capture/compaction/stop/restart and configuration-preserving upgrade/removal; stable event identity, queue overflow, credentials/revocation, deletion-before-rejoin and receiver handoff have source-bound receipts; native trust unavailable leaves qualification open and no real history is enrolled]
- [ ] T5.16 Independently review Codex adapter  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.15]  acc: [non-author reviews exact base/head and complete Codex installer/adapter/shared-client diff, actual host evidence and exclusions; findings are fixed, checked and re-reviewed before merge]
- [ ] T5.17 Merge reviewed Codex adapter  Owner: integration  Est: TBD  kind: agent  stage: merge  deps: [T5.16]  acc: [current repository holds/checks/protections permit guarded rebase of exactly reviewed Codex head; no runtime enablement or unrelated config changes]
- [ ] T5.18 Verify landed Codex adapter  Owner: codex-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.17]  acc: [reviewed/landed source correspondence and reachability plus affected Codex configuration and conformance checks pass; exact distributable version and honest supported-surface matrix are recorded, installation/deployment remaining separate]
- [ ] T5.19 Implement Claude Code shared-memory adapter  Owner: claude-code-adapter  Est: TBD  kind: agent  stage: implement  deps: [T5.13]  acc: [RAM-01 and RAM-06 use the frozen Claude Code profile and shared client for enrollment, covered historical import, incremental capture, bounded relevant injection and checkpoint/handoff; installer owns only declared config, respects native trust, pause/private/redaction and secure custody; unsupported tool/attachment/native-resume fields produce visible gaps]
- [ ] T5.20 Qualify actual Claude Code adapter  Owner: claude-code-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.19]  acc: [actual frozen Claude Code surface runs controlled synthetic install/import/live capture/compaction/stop/restart and configuration-preserving upgrade/removal; stable event identity, queue overflow, credentials/revocation, deletion-before-rejoin and receiver handoff have source-bound receipts; native trust unavailable leaves qualification open and no real history is enrolled]
- [ ] T5.21 Independently review Claude Code adapter  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.20]  acc: [non-author reviews exact base/head and complete Claude Code installer/adapter/shared-client diff, actual host evidence and exclusions; findings are fixed, checked and re-reviewed before merge]
- [ ] T5.22 Merge reviewed Claude Code adapter  Owner: integration  Est: TBD  kind: agent  stage: merge  deps: [T5.21]  acc: [current repository holds/checks/protections permit guarded rebase of exactly reviewed Claude Code head; no runtime enablement or unrelated config changes]
- [ ] T5.23 Verify landed Claude Code adapter  Owner: claude-code-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.22]  acc: [reviewed/landed source correspondence and reachability plus affected Claude Code configuration and conformance checks pass; exact distributable version and honest supported-surface matrix are recorded, installation/deployment remaining separate]
- [ ] T5.24 Implement Cursor shared-memory adapter  Owner: cursor-adapter  Est: TBD  kind: agent  stage: implement  deps: [T5.13]  acc: [RAM-01 and RAM-06 use the frozen Cursor profile and shared client for enrollment, covered historical import, incremental capture, bounded relevant injection and checkpoint/handoff; installer owns only declared config, respects native trust, pause/private/redaction and secure custody; unsupported tool/attachment/native-resume fields produce visible gaps]
- [ ] T5.25 Qualify actual Cursor adapter  Owner: cursor-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.24]  acc: [actual frozen Cursor surface runs controlled synthetic install/import/live capture/compaction/stop/restart and configuration-preserving upgrade/removal; stable event identity, queue overflow, credentials/revocation, deletion-before-rejoin and receiver handoff have source-bound receipts; native trust unavailable leaves qualification open and no real history is enrolled]
- [ ] T5.26 Independently review Cursor adapter  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.25]  acc: [non-author reviews exact base/head and complete Cursor installer/adapter/shared-client diff, actual host evidence and exclusions; findings are fixed, checked and re-reviewed before merge]
- [ ] T5.27 Merge reviewed Cursor adapter  Owner: integration  Est: TBD  kind: agent  stage: merge  deps: [T5.26]  acc: [current repository holds/checks/protections permit guarded rebase of exactly reviewed Cursor head; no runtime enablement or unrelated config changes]
- [ ] T5.28 Verify landed Cursor adapter  Owner: cursor-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.27]  acc: [reviewed/landed source correspondence and reachability plus affected Cursor configuration and conformance checks pass; exact distributable version and honest supported-surface matrix are recorded, installation/deployment remaining separate]
- [ ] T5.29 Implement Google Antigravity shared-memory adapter  Owner: google-antigravity-adapter  Est: TBD  kind: agent  stage: implement  deps: [T5.13]  acc: [RAM-01 and RAM-06 use the frozen Google Antigravity profile and shared client for enrollment, covered historical import, incremental capture, bounded relevant injection and checkpoint/handoff; installer owns only declared config, respects native trust, pause/private/redaction and secure custody; unsupported tool/attachment/native-resume fields produce visible gaps]
- [ ] T5.30 Qualify actual Google Antigravity adapter  Owner: google-antigravity-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.29]  acc: [actual frozen Google Antigravity surface runs controlled synthetic install/import/live capture/compaction/stop/restart and configuration-preserving upgrade/removal; stable event identity, queue overflow, credentials/revocation, deletion-before-rejoin and receiver handoff have source-bound receipts; native trust unavailable leaves qualification open and no real history is enrolled]
- [ ] T5.31 Independently review Google Antigravity adapter  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.30]  acc: [non-author reviews exact base/head and complete Google Antigravity installer/adapter/shared-client diff, actual host evidence and exclusions; findings are fixed, checked and re-reviewed before merge]
- [ ] T5.32 Merge reviewed Google Antigravity adapter  Owner: integration  Est: TBD  kind: agent  stage: merge  deps: [T5.31]  acc: [current repository holds/checks/protections permit guarded rebase of exactly reviewed Google Antigravity head; no runtime enablement or unrelated config changes]
- [ ] T5.33 Verify landed Google Antigravity adapter  Owner: google-antigravity-adapter  Est: TBD  kind: agent  stage: verify  deps: [T5.32]  acc: [reviewed/landed source correspondence and reachability plus affected Google Antigravity configuration and conformance checks pass; exact distributable version and honest supported-surface matrix are recorded, installation/deployment remaining separate]
- [ ] T5.34 Freeze current-state maintenance and operational source boundary  Owner: integration  Est: TBD  kind: agent  stage: preflight  deps: [T3.25]  acc: [current existing host/release and custody owner confirms the missing same-release maintenance entry/closeout route and exact source/task mapping, rollback and finite authority envelope; no old release6-to8 fields, credentials or expired approval is reused to fabricate current readiness]
- [ ] T5.35 Complete current-state maintenance through its authoritative owner  Owner: release-owner  Est: TBD  kind: agent  stage: implement  deps: [T5.34]  acc: [existing private release/custody owner implements same-release entry/closeout and safe operator adoption/reissue with preserved journals and cold fences; source receipts and operator schema bind actual current state without manual policy changes or code duplication in Serenity]
- [ ] T5.36 Verify current-state operational producer and consumer  Owner: integration  Est: TBD  kind: agent  stage: verify  deps: [T5.35]  acc: [actual supported producer/consumer fixture exercises current-state entry, interrupted closeout, rollback, wrong source/custody/pair, uncertain outcome and exact recorded credentials; no production mutation or synthetic-source seal is passed off as current authority]
- [ ] T5.37 Independently review maintenance source candidate  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.36]  acc: [separate reviewer records owning repository base/head, T5.34-T5.36 scope and complete custody/recovery contract; fixes and affected re-review close all blockers before source merge]
- [ ] T5.38 Merge reviewed maintenance owner candidate  Owner: release-owner  Est: TBD  kind: agent  stage: merge  deps: [T5.37]  acc: [owning repository accepts exact reviewed source under current gates without widening scope, keys, policy or live authority]
- [ ] T5.39 Verify landed operational owner contract  Owner: integration  Est: TBD  kind: agent  stage: verify  deps: [T5.38]  acc: [actual owning-source ancestry and tree correspondence plus affected fixture checks are recorded; this is not installed operator adoption, live credential reissue or a runtime permission receipt]
- [ ] T5.40 Review refreshed bounded synthetic release packet  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.39, T1.7]  acc: [exact current host/operator/authority versions, cold custody, finite grant, synthetic identities, historical/current credential inventory, signed feature/safety pair, cleanup and rollback are reviewed before T3.14; unknown old commands are reconciled rather than replayed and current authority is never inferred from source checks]
- [ ] T5.41 Implement explicit production enrollment and truthful capabilities  Owner: integration  Est: TBD  kind: agent  stage: implement  deps: [T5.1]  acc: [additive negotiated protocol distinguishes unsupported, synthetic and qualified enrolled production capability using existing brain authority and policy; allowlisted synthetic API is not silently widened; default remains off, declared scope/quotas and pause/revocation/rollback persist, and full safety receipts are required before real data admission]
- [ ] T5.42 Verify production-enrollment capability transition  Owner: integration  Est: TBD  kind: agent  stage: verify  deps: [T5.41, T1.7, T5.11]  acc: [controlled actual API fixtures deny missing safety evidence, stale release/policy, wrong brain and removed consent; supported opt-in and rollback preserve receipts/fences and bounded queue without upload to unqualified paths; affected source/race/static and compatibility checks pass with default off]
- [ ] T5.43 Independently review production-enrollment candidate  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.42]  acc: [non-author exact base/head review covers full T5.41-T5.42 source and protocol/capability transition with findings fixed and re-reviewed; no deploy or enrollment authority is minted]
- [ ] T5.44 Merge reviewed production-enrollment source  Owner: integration  Est: TBD  kind: agent  stage: merge  deps: [T5.43]  acc: [current holds/protections/checks permit guarded exact-head rebase of default-off production-enrollment source]
- [ ] T5.45 Verify production-enrollment source landing  Owner: integration  Est: TBD  kind: agent  stage: verify  deps: [T5.44]  acc: [landed source equals reviewed tree, is reachable and passes affected capability checks; release artifact identity remains separate and real-history activation is not claimed]
- [ ] T5.46 Review exact client host and capture activation release  Owner: independent-reviewer  Est: TBD  kind: agent  stage: review  deps: [T5.45, T2.8, T3.3, T4.10]  acc: [source-pinned signed service and qualified host/client artifacts, current erasure/restore/copy evidence, measured limits, existing target/cost/expiry grants, enrollment policy, canary bounds, pause and rollback are independently accepted before T3.4; absent permission blocks only the dependent operation and no new account/infrastructure is assumed]
- [ ] T5.47 Verify enrolled automatic-memory and origin-loss recovery canary  Owner: integration  Est: TBD  kind: agent  stage: verify  deps: [T3.4]  acc: [at actual released versions the scoped synthetic canary proves import plus concurrent capture, exact read/export, origin-loss receiver continuation, correction, pause/private, outage replay, deletion and cleanup; any subsequent real enrollment uses current owner-covered standing policy and qualified scope, never qualification data; consent, queued versus acknowledged/derived and unknown states are visible and failed canary parks the capability without silently replaying work]

### E6 — Bounded mixed-source preparation increment
fidelity: executable
Acceptance: An additive inactive candidate can preserve explicitly pinned independent raw file/email/voice sources while removing excluded episode history; unsupported hosted config/derivatives/activation remain refused and full T5.2/T1.3 stay open.

- [x] T5.48 Implement pinned mixed raw-source candidate  Owner: mixed-source-lane  kind: agent  stage: implement  deps: [T5.1]  acc: [additive BuildMixed preserves exact content and metadata of explicitly trusted SHA256-pinned independent raw sources across actual Git history, refuses missing/duplicate/mismatched/unsupported pins, foreign brain and unknown derivatives, removes excluded episode/checkpoint objects, and keeps Build legacy refusal and no-activation/purge semantics]
- [x] T5.49 Verify mixed-source candidate increment  Owner: integration  kind: agent  stage: verify  deps: [T5.48]  acc: [meaningful real-Git tests cover mixed retained/deleted history, raw metadata identity and changed/missing/unused pins, unlisted/derived sources, preservation and object closure; focused package race/vet, format and Wazi checks pass on exact source under qualified bounds]
- [ ] T5.50 Independently review mixed-source increment  Owner: independent-reviewer  kind: agent  stage: review  deps: [T5.49]  acc: [non-author exact base/head review covers full source/tests/contract/plan diff and no false authority or physical completion; accepted findings have tracked fixes and affected checks and re-review]
- [ ] T5.51 Merge reviewed mixed-source increment  Owner: integration  kind: agent  stage: merge  deps: [T5.50]  acc: [current holds/protections/checks permit guarded exact-head rebase of the reviewed inactive-source candidate]
- [ ] T5.52 Verify mixed-source source landing  Owner: integration  kind: agent  stage: verify  deps: [T5.51]  acc: [reviewed/landed tree and main reachability plus affected checks pass; broader mixed-derivative activation and managed-copy purge remain explicitly incomplete]

This bounded source increment is one implementation member under ceiling3. It
adds no production caller. Independent-source pins are a trusted maintenance
caller assertion bound to exact data/metadata, not automatic provenance or fence
authentication. Full T5.2 still requires active-pointer integration and controlled
old-copy cleanup; T5.3 owns backup/export/device handling. A prepared candidate
must never open reads, enable capture or become a physical erasure receipt.

## Batches and gates

| Batch | Members | Owner | Prerequisites | Verification/review/merge/landed | Selection | Requested/actual size |
| --- | --- | --- | --- | --- | --- | --- |
| Pinned mixed raw-source preparation | T5.48 | mixed-source-lane, isolated source worktree | T5.1; explicit trusted pins, no hosted owner mutation | T5.49 / T5.50 / T5.51 / T5.52 | inherited current session | ceiling3 / actual1 implementation member |
| Gated foundation | T1.1-T1.2, T2.0, T2.9-T2.10 | integration plus two completed implementation lanes | validated plan and frozen protocol | T1.8-T1.11 | gpt-6-astra / low | 5/5 |
| Archive | T1.1-T1.3 | integration plus isolated archive-lane | validated plan and protocol freeze | T1.4-T1.7 | gpt-6-astra / low | 3/3 |
| Client and hosts | T2.1-T2.4 | integration plus isolated client-lane | T1.7 | T2.5-T2.8 | gpt-6-astra / low | 4/4 |
| Operations | T3.1-T3.6 | integration | explicit deployment authority and dependency rows | live receipts | gpt-6-astra / low | 6/6 |

Split a candidate when independent host ownership, unsupported capabilities or review size requires it; preserve task IDs and add distinct delivery gate rows before dispatch. Deploy stages are authored obligations, not ordinary apply admission. Headless helpers stop/checkpoint before the integration deadline. No model polling unchanged jobs. Missing telemetry is unavailable, not estimated savings.

E4 is a coordinated outcome batch proposal, not permission to combine foreign
writers. Context/efficiency owns T4.2/T4.5; Gist owns T4.3; experience owns T4.4;
retrieval comparison owns T4.6. Each external candidate must map its own
verification/review/merge/landing rows under T4.7-T4.10 before dispatch, retaining
canonical upstream task IDs rather than creating a parallel scheduler. T4.0/T4.1
freeze the actual batch split and current inherited selection. Use one session
unless independent ownership justifies helpers. No speculative worker count or
paid execution route is implied. T4.11 measures a native development experience;
installed or deployed claims additionally require the applicable existing T3 and
Zati release gates. E4 does not reopen retired Gist execution or paused hosting.

## Evidence and handoff

Session STATUS.md and result.md in the owner-designated external artifact directory hold detailed commands and receipts without secrets. This plan starts with all implementation acceptance open. Checkpoint is mandatory at deadline even if T3.6 dependencies remain incomplete. Existing authored plans and worktrees are preserved. Hierarchical retrieval is gated on comparative evidence; direct retrieval is the baseline. Physical history rewriting and managed-backup deletion are not safe to enable merely because logical exclusion passes tests.

Independent preparation: T2.0 may run concurrently with T1.2 against the documented protocol direction. It owns only new internal/episodeclient files and focused fixtures; no server edits or actual upload/enrollment. Its candidate remains unmerged until integration, verification and independent review explicitly cover it. This is a speculative source assignment, not authorization to activate capture.

T2.9 is independent speculative preparation in internal/episodehost only; it uses synthetic fixtures and current public version-pinned source. Host installation and real-history capture stay blocked by T2.1 and erasure qualification. Its source joins T2.5/T2.6 verification and exact-revision review.

Foundation split: the first candidate comprises T1.1-T1.2, T2.0, T2.9 and T2.10, with delivery gates T1.8-T1.11. It may deploy default-off under T3.2. T1.3-T1.7 remain open for full erasure/restore qualification and cannot be inferred complete from this increment. T2.10 is an isolated speculative synthetic integration under internal/episodecapture plus focused gateway fixtures; no real upload is enabled. Full host and production-capture dependencies remain unchanged. The external deletion journal currently accepts only brain/account subjects and foreign T23.48/T23.49 claims remain protected.

T1.1 evidence: EPISODES_v1.md and request/response/error schemas freeze the synthetic-only protocol; existing writer and brain scope remain authoritative. Current PR386 and ownership audit preserve foreign recovery/deletion claims. The immutable Go1.26.9 Linux arm64 image and source-hash transfer were exercised on existing Spark capacity, with bounded two-CPU jobs and no heavy Mac build. Independent deployment preparation owns only external task artifacts and read-only target evidence; integration retains T3.1 execution ownership.

Foundation verification: full build/vet/race passed at8ac22eeb; final equivalent boolean simplifications at6618cf59 passed affected store/gateway race checks, golangci-lint2.11.4 (zero issues) and govulncheck1.1.4 (zero reachable vulnerabilities; two required-module-only advisories). Protected queue, decoder and TLS integration acceptance passed in the full run. Twenty-two deployment unit tests and immutable workflow-pin checks passed. Gitleaks8.30.1 passes with an exact-two-public-fixture-hash AND exact-file exception; negative fixtures still detect different values and other paths. These are isolated Linux arm64/root checks with source-defined provider/native/browser skips, not hosted Actions or real-host acceptance. PR386 Actions did not start due to billing lock; ADR024 fallback applies. Full erasure, restore and enrolled-host dependencies remain open.

Independent review of9c221183 requested changes: canonical JSON storage admission and local queue/wire size, sequence and empty-byte compatibility. T1.8 reopened for affected verification; fixes include HTTP no-mutation/refused-staging regressions and explicit empty Codex occurrence handling. No review or deployment acceptance is inferred from earlier passing tests.

Review closure: separate review of44b0ff22 accepted the canonical quota, queue/wire compatibility and empty-message fixes with no source blockers. Affected build/vet and CLI/client/capture/host/JSON/store/writer/schema race checks passed atff0b8c65; the test-only pool lease correction44b0ff22 then passed store/gateway race, lint (zero issues) and govulncheck (zero reachable vulnerabilities). The original full suite remains applicable outside these changes. Final plan-only evidence update receives independent re-review before guarded merge. GitHub Actions remains unavailable due account billing; ADR024 fallback is qualified isolated execution, not hosted CI.

T1.10-T1.11 evidence: PR386 guarded-rebase merged at 8242459bc6a8689d81c96d0b66ed7f751a0de090 on 2026-10-16; landed tree 024eb6efd0b22c363deb2a8698f35e1dd696b990 equals independently reviewed e0064a646bdea2bf6370d361828b8b57a204d005 and is reachable from main. Actions did not start because of account billing; proportionate owner-authorized ADR024 fallback is recorded separately from hosted CI.

Reader follow-up: PR387 independently reviewed 9e48b85320e246123daa2ab2d62d95fc4f4df63f and landed efdaf3da592e6fea9ddda453d08abc163400bc3c with equal tree 04b4efa627d11df9910372f24c10d876273f27a0. Nonempty Codex harness metadata and non-null encrypted-message records are excluded with content-free coverage gaps. Focused race/vet/build/static checks passed; these client libraries are not linked into the hosted command. No real host capture or T2.3 completion is inferred.

T3.1-T3.2 evidence: the [delivery receipt](../launch/shared-agent-memory-delivery-2026-10-16.md) records actual sequence 6/source 8242459bc6a8689d81c96d0b66ed7f751a0de090 installation, root signature/state/pair checks, authority version 7 policy/readback/direct-denial evidence, default-off flags, successful postdeployment backup and root verification of the schema 10 fallback against actual state 6. T3.1 qualifies the observed rollback path; no executed production rollback or authenticated recall is claimed. T3.3 remains open because the authorized synthetic production fixture is unavailable. Full capture, erasure/restore and downstream host tasks remain open.

## Resume 01 admission

Fresh origin/main base is 7033342c15bee75aabec16fe56922f7697440d0a. T23.48 and T23.49 claims still exist as of 16:59 UTC with October 1 purposes; PR273 remains open unchanged since September24. These observations establish reserved scopes, not a running worker. Preserve their exclusive lifecycle/deletion/backup paths and journal bytes. T1.12 proceeds in new internal/episodeerase files plus episode-specific store/writer helpers only, using synthetic isolated repositories. A separate external replay abstraction must not reinterpret existing account/brain journal subjects. Integration into activation and managed copies remains T1.3 and requires a compatible reviewed handoff. Do not treat claim age as permission to steal it.

Resume batch: T1.12, one isolated implementer; T1.13–T1.16 delivery gates. Selection remains gpt-6-astra/low/ChatGPT. Parent owns plan, ownership reconciliation, live target refresh and later integration. Actual production deletion, restore and capture remain blocked until full end-to-end gates pass. All existing 28 task IDs and completion evidence are preserved; five bounded delivery rows are added rather than closing partial T1.3. Parent may advance independent host preparation only after a scoped admission update.

Shared contract ownership reconciliation: current integration owner acquired previously unowned R-hosted-schema and R-hosted-assembly resource claims. T23.41 retains its historical task claim; its documented protocol assigns subsequent shared-interface requests to the coordinator. This new additive memory contract request is applied as scoped integration work, not a takeover or completion of T23.41. Preserve T23.48 deletion adapter/lifecycle and T23.49 backup files. The external-fence batch T1.17 uses existing journal transport/authority, an explicit new subject validated in shared contracts, and episode-specific gateway plus service assembly replay; old account/brain bytes do not change. Prior readers must refuse new-subject history, so any later deployment requires an episode-aware signed fallback before writes. No synthetic gate is lifted. T1.18–T1.21 own verification through landing. Two implementer lanes have disjoint files; parent alone owns shared plan and delivery.

Independent client preparation T2.11 is parent-owned in internal/episodeclient and internal/episodecapture, disjoint from both helpers. It remains synthetic-only and does not discharge T2.1 or its erasure dependency. T2.12–T2.15 provide separate delivery gates; requested/actual implementation member count 1/1. No third helper is launched.

T2.11–T2.12 resume evidence: atomic protected progress candidate fd967b8768c144ccd43f341722b79a257456429d passes episodeclient/episodecapture/episodehost race and vet, hosted build with client libraries absent, golangci-lint2.11.4 with zero issues and govulncheck1.9.0 with no vulnerabilities. Test/documentation follow-up 5a25d55b04387581e3620955c1f81db6f1f1f66b passes affected episodeclient race/vet including format1-to2 migration. Checks ran on bounded existing Linux arm64 DGX capacity, not hosted Actions or native host acceptance. Full T2.1 and real capture remain open.

Resume protected progress landing: PR389 independently accepted df959ef2299c6ee89a38162bf2d8119be0ea9f21 after R1 retry-no-op correction; affected client race/vet passed. Guarded rebase landed b680354243d70f0d088e1fe7dced95b846174a01 with equal reviewed tree and main reachability. Actions never started due billing lock; fresh protection404/rulesets empty, ADR024 fallback, no bypass. No host installation or production activation.

T1.12–T1.13 bounded preparation: erasure candidate source53bbad449b9e2a25668b80e19bbaa8a6172785cc passes actual disposable-Git episodeerase/store/writer/gitrun race and vet on existing DGX. Lint-only explicit ignored hash-write return9ef6d247ffdf5fb001afaa7d24064f833aeda9fb passes lint2.11.4 and vulnerability1.9.0 checks. Rebase onto PR389 changes no owned source or its dependencies. The new T1.12 acceptance is clarified to the delivered inactive episode-only candidate: no live source rewrite, unknown derivatives/mixed brains refused, supplied fences are not authenticated by this library, no backup/device/export purge, activation or physical-completion claim. Original full T1.3 remains unchanged and open.
T3.12 is part of the external-fence candidate and its T1.18–T1.21 verification/review/merge/landing gates. It grants no credentials and creates no accounts. It uses the existing private admin socket, validates reserved fixture account identity and free/unbilled state, expires without permanent config changes and cannot override the archive-disabled safety build. The separate operator fixture and live packet still need T3.7–T3.11 and T3.3 qualification. No public endpoint or unrelated UI is added.

Authority integration correction: R-hosted-authority was unowned and acquired for this batch. T1.17 also updates the authority server entry admission for the explicit episode subject; its strict shared validation refuses malformed identities and physical-purge outcomes, preserving existing account/brain admission. T1.18 tests the actual authority append route. Deployment must replace the authority Lambda code as well as approve both signed episode-aware host build profiles before episode intents are written. Existing legacy Lambda code is incompatible and cannot be relabeled as qualified.

Resume erasure preparation landing: PR390 independently accepted exact c4baa80ae95612f5d7af07b74e945dee04a07227 after R1 duplicate historical paths was fixed. Four malformed historical duplicate variants and historical gitlink with clean later HEAD pass affected race/vet; no destination is published and source hashes remain unchanged. Guarded rebase landed77196ba3f7098985496439ad3b4d3d87d3dce1c7; reviewed tree equality and main reachability verified. Actions billing lock persists; proportional ADR024 fallback and no protection bypass. This closes only T1.14–T1.16; full T1.3 remains open.

Synthetic cleanup contract: T3.12 also exposes a bounded private-only revocation operation for explicitly named credentials belonging to the same dedicated free/unbilled/nonpartner fixture identities. Validation and revocation share a transaction; ordinary or unknown keys cause the entire request to fail. This avoids leaving live synthetic credentials or requiring an unrelated second release transaction just to clean them up. It grants no credential, email assertion or public authority.

External fence verification: prior aggregate92b1a934 hosted/... writer/store/gitrun race passed except the backup test private-directory prerequisite; backup race and aggregate vet passed with the required explicit private fixture directory. Independent R1 accepted: startup discovery must not apply the interactive65536entry ceiling to ordinary legacy brains. Fixbe143538 uses streaming complete replay discovery while preserving interactive and episode bounds. Exact231a94dae4147c881a83a466cc6344d64412224d passes affected Episode race/vet, the22000source legacy startup/foundation-refusal/external-replay regression in normal and archive-disabled profiles, private cleanup atomicity/scope checks, and lint2.11.4 plus govuln1.9.0. No provider or physical-purge acceptance follows from these checks.
Synthetic credential gate: existing public signup requires email delivery, and partner provisioning requires an actual verified-email assertion. Neither is fabricated. T3.7 prepares a separate offline operator fixture over existing provisioning interfaces, not a new public authentication bypass. The owner authorized scoped synthetic provisioning; execution is still gated on exact code/operational review, existing maintenance/runbook guards and refreshed signed release/fallback evidence. Exactly two dedicated reserved synthetic accounts may be retained inert with revoked credentials for future qualification; retained bytes are disclosed and not called erased. Real-user accounts, plan limits and optional WorkOS/billing flags remain unchanged. This source preparation is independent of real capture and does not close T3.3.

Resume replay landing: PR391 independently accepted5d0c9d1cbcd36f3fdb8f4b57313ae147d90f28e6 after large ordinary-brain startup R1 and new private cleanup review. Guarded rebase landed5a5dc91f6065acadc08e9a2fc99832d3470159b9; equal reviewed tree and main reachability verified. Actions billing blocked, protection404/rulesets empty, ADR024 fallback; no production acceptance.

Fixture source verification: ecbdcb01 Python12 filesystem/systemd model tests passed. DGX identified the missing existing checkout immutability trigger in the strict schema inventory; corrected325bf7ec with required-trigger regression. Cleanup/static findings fixed608bdb3551825cb0a1d17ddcce1c7ac77408eb4d; package race/vet, lint2.11.4 and govuln1.9.0 pass. An explicitly injected source-only schema harness produced9bd9153fd18f433eb1d0416c7faa38ca6eefdc29ebcd8585f3a021b8b63ed963 from a fresh synthetic schema10 database. Rebase preserves fixture source byte-for-byte. Model tests do not establish real UID/FD/systemd/provider behavior.

Source merge and live packet sequencing: T3.9 reviews the source and offline custody contract; the exact signed artifacts and fresh operational packet can only be finalized after source lands. Their original mandatory independent review is preserved explicitly in T3.13, before bounded T3.14 execution and T3.15 receipt verification. These new rows do not close or replace original full T3.3 (all API/privacy/restore/physical-erasure gates), T3.4–T3.6, or any remaining client/host task.

Resume01 additional bounded lane at20:10UTC: parent retains live authority/host recovery ownership; isolated erasure-chain lane owns only new synthetic cross-component qualification and necessary narrow episode-specific helpers. It cannot activate or purge a production source, edit foreign lifecycle/backup interfaces, change capture capabilities or launch helpers. Existing57taskIDs include all prior52 unchanged. No live or managed-restore acceptance is implied.

T1.22–T1.23 source6d599cc3c56eff69d470be00f645b2a9388e99ef: actual isolated Git/writer/journal/gateway restore-to-inactive-candidate chain passes focused race tests, affected gateway vet and lint2.11.4 on existing qualified DGX. The synthetic caller pins a trusted sealed cut; it does not qualify service/provider recovery admission. Mixed hosted repository refusal and retained source copies are explicit. Candidate cancellation is before construction, not power-loss durability. No production implementation changes or physical-erasure completion. Independent exact review and guarded landing remain T1.24–T1.26.

Resume01 additional client increment at20:43UTC: owned shared-memory resource and current PR/claim audit permit isolated internal/episodehost and episodecapture source work. Parent owns deployment and final operational review; one bounded client implementer owns only T2.16. Pinned official Codex0.163.0 enum source identifies partial_answer; its semantics and fixtures must be checked before widening admission. Existing57 task IDs and all unfinished whole-design gates remain intact; five additive task IDs provide scoped delivery evidence. No actual host/capture installation follows from source fixture acceptance.

T2.16–T2.17 sourcebbb55370f7d95fad538f48f78b6f527884365677: immutable public Codex0.163.0 MessagePhase documents partial_answer as stable visible answer text, not turn completion. Existing safe decoder now covers that phase; equal-text partial/final occurrences retain distinct stable identities. Protected import tests verify redaction, cursor/restart/retry and pause/private/real-history refusal. Existing provenance, encrypted/reasoning/tool/attachment/channel exclusions remain. Actual affected three-package race/vet, lint2.11.4 and govuln1.9.0 passed on existing bounded DGX Go1.26.9 arm64; no installed host or backfill acceptance. Previously acknowledged unsupported-phase gaps are not reset or silently re-imported.

Resume01 scoped recovery source work: actual private synthetic cleanup revoked fixture keys while retaining their private manifest strings. Existing offline inspect calls live credential Verify and therefore rejects reissue/revoke before reaching their own revocation path. T3.16 fixes only explicitly requested offline recovery modes after secret verification and exact recorded binding checks. No production reissue, new account, source-version migration or cold-fence bypass is authorized by a passing source test. All prior62 task IDs remain, including incomplete live API and complete-design gates.

PR394 independently accepted3b90f59a4ba983d5f8259f9c590a7ed758a620f9 and guarded rebase landed25d74ce0c64e0032516b38a6902d579c43eb8c1a with equal treee26b6f514119fbf5a06b0ab1ec5e7896aed920c2 and main ancestry verified. Actions billing failure had zero test steps; protections and holds rechecked, no bypass. T2.21–T2.22 add only controlled synthetic discovery independent of full host lifecycle and erasure gates; max two subordinate sessions remains in force.

T3.16–T3.17 source98e01437376f6433db83870d3935229d46f9423f passes affected fixture/credential race and vet, lint2.11.4 and govuln1.9.0 on bounded existing DGX. Recovery modes recognize only an already secret-verified revoked key and recheck its exact recorded account/brain/credential binding. Prepare still refuses stale revoked strings; public Verify remains revoked. This is source-only, not installed fixture-tool compatibility or cross-source manifest migration.

T2.21–T2.22 controlled native discovery: actual Codex0.163.0 selected-model synthetic session emitted two known markers with sibling retained-source/client-authored/order metadata. No model tool calls or existing-history reads; original history was not exported. A sanitized visible projection preserving line positions and nonempty metadata keys was checked against landed25d74ce0 on DGX: zero admitted messages, two explicit provenance gaps and stable acknowledged cursor. This is negative coverage evidence, not full native capture. Original SHA1636e970b5a792e612341a0de2e82645d5bdd2a4002c95e05ee2f0d2b121e727; projected fixture SHA9b813aeec28728b13828d8f2c0b49852fd00f8582ef3d4871c7f7c185c0d9b88. T2.3 remains open; no broad metadata admission is inferred.

T1.24–T1.26 source3b5b22e0 independently approved and PR393 landedf6efd727 with equal reviewed tree and main ancestry. New T1.27 lane owns only a new gateway cross-component test using unchanged public backup Create/Restore APIs; existing foreign T23.48/T23.49 claims and backup source are preserved. Original snapshot test used whole-Git copying, so this separate portable-manifest/frozen-control-store boundary is not duplicate production acceptance. Parent owns all runner/review/merge/provider actions. Maximum two subordinate sessions; lane checkpoint deadline21:45UTC, attempt deadline22:58:27UTC.

Source delivery reconciliation: PR392 independently accepted95fa53b5bb81869b862d537ec13abf3cb4d9fbb9 and landed30919c2fe1581ff2c0e31475dc0b49cb2fd0926a with equal tree and main ancestry. PR395 independently accepted6c06849c9375796bf7b1b7c74db93592097185bf and landed5dcef0a9e5d3b9091ced4e7ab99fe24c2af63bc0 with equal treedaf89ec03d35d82a78c961debea239ad58b32e46 and main ancestry. These close only their source review/merge/landing rows. Hosted source30919 and installed fixture tooling do not include PR395; new reissue still requires an explicit cross-tool-source custody contract and fresh cold maintenance proof. Full live qualification and real capture remain open.

T1.27–T1.28 exact sourceb6e3b072711929f63496a4caa58a28abb58a98a4 passes portable restore and prior erasure-chain race, existing backup round-trip race, gateway/backup vet and lint2.11.4 on qualified bounded DGX Go1.26.9 arm64. Actual manifest-v2 Create/Restore retains the complete hosted repository and freezes accounts. The initial assertion expected ErrRevoked incorrectly: active-account lookup hides restore_pending rows. Corrected tests separately prove the exact old key identity/verifier and nonzero revocation timestamp, ordinary invalid-credential denial, and frozen-account HTTP refusal. Complete explicitly caller-trusted external replay excludes old evidence and dependent checkpoints; missing/unsealed/wrong/conflicting history refuses. Retained bundle/history hashes remain unchanged; mixed hosted candidate construction refuses. No provider seal admission, account activation, queue-generation acceptance behind the frozen gate, physical purge or managed production restore is claimed.

Resume01 at21:30UTC: T3.21 source-only lane owns evals/episode-qualification exclusively. Parent retains provider/release authority and integration; portable fixture lane is finished pending verification/review. All74 task IDs remain; five additive source-adoption gates do not close original live API or erasure tasks. Exact old manifest binding must be an explicit inherited root-custody field, never a CLI/environment bypass or automatic change on ordinary prepare/reissue/revoke. Unknown/pending/incomplete/live-key state refuses. Private data and keys stay on-host. A later actual reissue needs fresh cold custody, compatible operational packets and separate exact review. Deadline22:58:27UTC; this implementer must checkpoint by22:05UTC.

T3.21 integration ownership: the existing hosted store Open always initializes/migrates and selects WAL, so it cannot substantiate manifest-only adoption. Parent may add a separate narrowly scoped offline read-only store opener in new store files, preserving Open and all ordinary interfaces. It must require a caller-held offline fence, refuse any WAL/SHM/journal or unsafe file state, open immutable read-only without initialization, validate the existing exact supported schema and reject mutations. This is part of the same candidate and independent review, not permission to use immutable reads on a live database. The subordinate still owns only evals/episode-qualification.

PR396 independently accepted8e753dad3cffdce38e4118d68f82b9eedfb664fe and guarded rebase landede1f12e3c7ba7ec5e8807adf156a87f3c07793c47 with equal treefaefb53bca804f9a0f6be0297475407b8629b40f and main ancestry. Actions run38969634355 had zero job steps because of account billing lock; proportional ADR024 checks remain source fallback. Full T1.3 is open.

T3.21 source d90d0bf18c9eda5c5fc6eee74fd6646b0c968742 provides explicit v2 inherited root custody for adopt-source and exact old manifest/source binding. Ordinary v1 operations remain unchanged/source-strict. The separate offline read-only opener refuses journal sidecars and requires a caller-held cold fence; actual main-database writes still fail after query_only is disabled. Narrow reader race/vet passed on Go-identical c1949b99. Actual unmodified d90d0bf1 Linux binary passed22 supplemental UID1800/empty-group/six-FD cases, including old released30919 preparation, exact retained revoked keys, manifest-only source/epoch update, byte/metadata-preserved control data, old-proof retry, malformed proof/source/UID/lock and empty/nonempty WAL/SHM/journal refusals. Real systemd/provider authority and production installation/adoption are not qualified by this synthetic harness. Aggregate source checks and exact independent review remain mandatory before merge.

T3.14 preparation only: a separate external retry-template copy preserves the failed original packet. Sixty offline Python models cover truthful Serenity HTTP identification, an explicit new fixture operator source distinct from fixed hosted30919/state8, and exactly two recorded credentials per fixture account after one reissue. Exact raw manifest/custody pins, bounded cleanup, TLS/no-proxy/no-redirect and401/404 requirements remain. No actual input packet is rendered, uploaded or invoked; current cold adoption/reissue, inventory observation and exact operational review remain before execution. T3.14/T3.15 stay open.

T3.22 final sourcec1fd0a9e403e72ced2a625c3243a6bf58cb9bff2 passes24 Python operator models, affected fixture race/vet, lint2.11.4 with zero issues, govuln1.9.0 with no vulnerabilities and Darwin arm64 cross-build on qualified DGX Go1.26.9. Prior d90d0bf1 full fixture/credential/store race and vet passed; its lint found an unused test assignment and equivalent version-branch switch suggestion, both corrected and affected verification rerun. Read-only store/credential code is unchanged. Exact c1fd0a9e binary SHA3ae2e47a2c0f89343ea6f5335ae343f0329be025d85121f1f776b72b5752285d again passes all22 actual Linux UID/FD adoption cases. Archive SHA0d74535dd671218a1d237410986707bec22190d15bac8a7c0417d4a7278a63e2 binds these source results. This is synthetic source/process qualification, not installed root-wrapper/provider/cold maintenance or production adoption/reissue acceptance.

Synthetic fixture source landing: PR392 independently accepted exact95fa53b5bb81869b862d537ec13abf3cb4d9fbb9 after effective typed startup-guard and actual busctl representation corrections. Guarded rebase landed30919c2fe1581ff2c0e31475dc0b49cb2fd0926a; both trees equal d625e823beba6e32e29d55b45c21dd168c5c5ede and main reachability is verified. Source race/vet, static/vulnerability and18 Python adapter models passed; the actual source30919 release fixture binary also passed12 isolated Linux UID/GID/FD custody and refusal cases. Actions remained billing-blocked under ADR024 fallback, without protection bypass. This closes only T3.9–T3.11. Production provisioning, authority rollout, host activation and authenticated qualification still require exact operational review and fresh target receipts; physical erasure, managed restore and real capture remain unqualified.

T1.24–T1.26 subsequently completed: independent review approved exact3b5b22e0ecbf45f4d20d897398b6c2eb2c4ed3b7. PR393 guarded rebase landedf6efd727577728161200a93becf8919a76adca30 at20:31UTC; reviewed-tree equality and main ancestry verified. Current Actions run38962392550 has zero test steps because billing is locked; proportional ADR024 source checks are labeled fallback, not hosted CI. Main protection/rulesets and posted holds were rechecked without bypass. Full T1.3 remains open.

Portable source landing: PR396 independently accepted8e753dad3cffdce38e4118d68f82b9eedfb664fe and guarded rebase landede1f12e3c7ba7ec5e8807adf156a87f3c07793c47 at21:31UTC. Equal reviewed/landed treefaefb53bca804f9a0f6be0297475407b8629b40f and main ancestry verified. Actions run38969634355 had zero job steps with explicit account billing-lock annotation; ADR024 fallback remains accurately labeled. Current protection404/rulesets empty and no review/coordinator hold; no bypass.

Operational packet review T3.13 is complete for the actually executed source30919 feature8/safety7 pair, authority8, scoped fixture and separate safe cleanup recovery. Exact code/packet/invocation and actual cleanup receipts received independent review. Host sequence8/high8 and authoritypublished8/open1 are observed, but original full API run failed at initial public-disabled check before append/exclusion/restart; T3.14 and T3.15 remain open for that incomplete exercise. Reviewed recovery proved both revoked keys401, gateclosed404, timeractive and healthy exact release, preserving the original failed journal. Two synthetic accounts/brains and private revoked-key strings remain retained. Full T3.3, T1.3 and real capture remain open. The signed compatible safety7/high8 approval expires2026-10-20T18:49:01UTC and was verified against actual committed8, not exercised as a production rollback.

PR397 independently approved exact49a88d7ee5d888f0c7e9fe9ab01a4cf55ff5c4ea and guarded rebase landedf4c091e42fdebc493555093f9e3baf8a0f216a4d at21:55UTC. Equal tree1f87efbe30225ebef9da40f2ce5f8120d71bb7ef and main ancestry verified. Actions38972136831 billing-locked with zero job steps; protections404/rulesets empty, no applicable hold or bypass. The exact landed Linux operator SHAa459cda480f168f39004e6a057fb25c1fd2370d088f3f0ffd69b6301ddadac50 also passed22 actual UID/FD synthetic adoption cases and is retained uninstalled. T3.23–T3.25 are source-complete; no live source adoption/reissue occurred.

Resume01 closing evidence: fresh21:53UTC root/public observations confirm release8/high8/source30919 and healthy default-off service, timeractive, no pending/hold; fresh21:54 published-authority configuration/code/methods/stage match with concurrency1. These do not repeat the accepted21:06 two-key HTTP401 cleanup or refresh every policy/journal/stack claim. The separately reviewed retry-template copy passed60 offline models; one documentation source-binding contradiction was corrected and independently closed. No actual current-state packet was rendered/uploaded/invoked. T3.14/T3.15 and full T1.3/T2/T3 remain open, preserving all79 task IDs. Remaining live work requires fresh cold custody, qualified landed operator installation/adoption/reissue, exact private inventory pins and final operational review before synthetic API execution.

T3.14 final bounded preparation: independent current8 contract inspection confirms the installed6-to8 stage_hold/previous-pair runtime is not an already-qualified same-release8 cold entry route. A separately isolated external preparation lane may extend the existing fixture-proof renderer for explicit v2 adopt-source binding and fail-closed outcome reporting, with offline positive/negative tests only. It must preserve original artifacts and all current transition checks, cannot manufacture current8 maintenance authority or invoke/render real target inputs, and must explicitly retain the missing same-release producer/closeout as a blocker. Parent owns exact independent review. Lane checkpoint22:30UTC, allhelpersstop22:55UTC, attempt22:58:27UTC. No new task completion or live acceptance follows from preparation.

## Upstream realignment — 2026-10-17

The [inspection and owner crosswalk](../receipts/2026-10-17-gist-zati-alignment.md)
pins current Gist and Zati main. This section supersedes the older current-state
assumptions in E4 and historical batches without deleting their evidence. Gist
has public package/preparation/composition APIs and a controlled Zati inbox
consumer; select its current pins and unresolved qualification instead of
reimplementing that work. Hosted Library delivery has scoped owner receipts and
is no longer globally described as paused. Embedded use remains supported;
Serenity is not a mandatory Gist backend and Gist execution stays retired.

Zati E-FO0 freezes the runtime continuation contract; E-FO1 still owns runtime
implementation and acceptance. T2.2 qualifies Serenity's archive bridge, never
Zati's internal history/tasks/execution implementation. T4.2 may improve context
using existing governed memory before archive enablement. Full continuity and
T4.7 remain dependent on T2.2 and actual owner runtime receipts. Local context
segmentation does not grant full-history capture or prove recovery after loss of
the canonical host history. Keep all physical erasure/replay/restore and host
qualification gates. Reporting hierarchy and proactive mandates grant no memory
access and are not restored as current authority from archive content.

All91 IDs/statuses/owners remain. Six open rows have evidence-backed amended
acceptance, with old/new rows and dependency delta preserved in the sibling
preservation receipt. No previous completed row changes and no task is marked
complete from another repository. This task delivers documentation only; elapsed
Resume01 authority is not renewed and no external runtime task is advanced.

## Automatic-memory batch proposal and handoff — revision r3

These batches refine the historical table above. Default source member ceiling
is3; gate rows do not pad it. One attended coordinator owns plan/integration;
one independent reviewer handles each frozen candidate. Future implementation
workers inherit the actual current selection without model/provider fallback.
Dispatch is not performed by this planning task. Split only at actual package,
repository, trust or independently releasable boundaries and give each resulting
candidate its own gate chain before dispatch.

| Batch / intended result | Implementation members | Prerequisites | Owner | Verify / review / merge / landed | Size / split |
| --- | --- | --- | --- | --- | --- |
| Safe archive | T5.2-T5.3, original T1.3 closure | T5.0-T5.1 | archive-lane | T1.4 / T1.5 / T1.6 / T1.7 | 3 members; serialized existing writer/backup ownership |
| Shared automatic client and knowledge | T5.5-T5.7, T2.1 aggregate closure | T5.4 | client/memory roles under one integration owner | T5.8 / T5.9 / T5.10 / T5.11 | 3 implementation members; T2.1 is aggregate evidence, not fourth dispatch |
| Zati bridge | original T2.2 | T5.11-T5.12 | existing Zati and Serenity owners | canonical Zati gates plus joined T2.5 / T2.6 / T2.7 / T2.8 | 1 outcome; no second runtime scheduler |
| Codex | T5.14 | T5.13 | codex-adapter | T5.15 / T5.16 / T5.17 / T5.18 | 1; target version/surface fixed |
| Claude Code | T5.19 | T5.13 | claude-code-adapter | T5.20 / T5.21 / T5.22 / T5.23 | 1; target version/surface fixed |
| Cursor | T5.24 | T5.13 | cursor-adapter | T5.25 / T5.26 / T5.27 / T5.28 | 1; target version/surface fixed |
| Antigravity | T5.29 | T5.13 | google-antigravity-adapter | T5.30 / T5.31 / T5.32 / T5.33 | 1; target version/surface fixed |
| Same-release operational prerequisite | T5.35 | T5.34 | existing private release owner | T5.36 / T5.37 / T5.38 / T5.39 | 1; separate source owner and no live action |
| Default-off production enrollment | T5.41 | T5.1 | integration | T5.42 / T5.43 / T5.44 / T5.45 | 1; no activation at source landing |
| Controlled release and real-use envelope | existing T3.14, T3.4 operational transitions | T5.40, T5.46 and current recorded authority | existing release owner | T3.15, T3.3, T5.47, T3.5, T3.6 | distinct synthetic and real capture gates, not a source PR |

T5.0 and T4.0 can start with read-only source/fixture work; T5.1 follows T5.0.
Current-state maintenance preflight T5.34 can proceed independently from archive
implementation using existing T3.25 source, but no physical operation follows.
Existing governed-memory UX and Gist consumption remain available through E4;
archive custody and cross-host recovery remain gated. A missing host hook/trust,
Zati runtime receipt, authenticated fence producer, current-state maintenance
contract or finite activation envelope is an explicit blocker for its dependent
row, not a reason to retry unsafe alternatives or invent success.

Before/after identity/status/dependency/criteria inventory is retained in
[the preservation receipt](../receipts/2026-10-17-automatic-memory-plan-preservation.json).
All91 original task rows survive; amended open gates retain their prior acceptance
verbatim with additional coverage and prerequisites. No original completed row
changes. Review/merge/landed evidence for this planning artifact is recorded in
its PR; source implementation, provider behavior, installation and deployment
remain future receipts. The primary checkout and foreign PR273/277 work are
preserved; no lifecycle is enrolled by this document.

## First current-session source batch — 2026-10-17

T5.0/T5.1 completed their source inspection and design deliverables in
[the preflight receipt](../receipts/2026-10-17-automatic-memory-preflight.md) and
[erasure extension contract](../protocol/EPISODE_ERASURE_EXTENSION.md). All prior
acceptance and source gates remain; this closes no implementation, purge,
production capability or installed host. Requested --batch-size omitted N, so
use default ceiling3; this batch has2 members. Current attended engineering
executor and independent reviewers inherit actual session selection; no paid
fallback, resumed prior deadline or unattended service admission. R-shared-memory
and foreign backup/import ownership remain preserved. Continue with T5.2/T5.3
within the frozen scope and keep partial source increments and operational
blockers explicit. Actual checks and exact review/merge/landing are recorded
in this candidate PR before delivery is claimed.

Accepted review finding AM-CR01 required a concrete production negotiation and
admission contract before T5.1 could unlock production-capability source. The
extension now freezes v2 route/schema, exact capability/reference/owner-record
fields, policy/release/version/expiry bindings, current publication rechecks,
legacy/unknown-client refusal and sanitized denial semantics. This is a contract
correction only; no endpoint is implemented or enabled. Existing T5.41-T5.45
remain the code/check/review/landing owners.

## 2026-10-17 — Pinned mixed raw-source preparation

T5.48-T5.49 source snapshot fbaa1e626f285c13142232fcc8e061c6ac4405e7
passes real-Git episodeerase/store/gitrun race checks, existing gateway erasure
and portable-restore race scenarios, affected vet, episodeerase lint and scoped
vulnerability checks on bounded Linux ARM64 Go1.26.9. All139 prior rows retain
exact status/criteria; five additive source-delivery rows cover this inactive
increment. Legacy Build still refuses mixed sources. Full T5.2/T5.3/T1.3 stay
open; no authenticated activation, hosted derivative/copy purge or live recovery
claim follows. See the mixed-source preparation receipt for the exact boundary.
