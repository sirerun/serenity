# RFC 0001 — Serenity (v2.2)

**Status:** Draft for implementation — supersedes the v1 draft (pre-repo, not published here).
v2.1 folded in the accepted findings of a six-model external review council
(2026-08-27; the council's consensus matrix, ranked findings, and rejected
feedback are internal review artifacts, referenced by date). v2.2 resolves
the findings of a final line review (2026-08-27): shard-head authority,
claim-id/dedup semantics, the plan-check matcher, and the smaller items in
the v2.2 changelog below.
**Name:** "Serenity" is a **working title** (collides with SerenityOS); the
rename decision is a blocking acceptance criterion of the launch milestone (M5).
**License / distribution:** Apache-2.0, standalone open-source project
**Audience:** Engineering and design, no prior context assumed
**Definition of done:** Code complete = every P0/P1 milestone below passes its acceptance criteria on commodity hardware (a laptop), with the scale profile verified on a 128GB-class box.

### Changelog (v2.1 → v2.2, final line review)

- **Shard-head authority rule** (§7.2a): for shard-tier families the shard is canonical and the fence head is derived; "the file is truth" is scoped to fence-tier. Wipe-and-rebuild test now covers fence/shard disagreement.
- **Claim-id derivation and dedup semantics specified** (§7.2): ids identify provenance-bearing extractions; dedup is semantic, at reconcile, never id equality.
- **Plan-check matcher specified** (§8.3): deterministic over structured actions (offline); model-classified for free text; `unverified`, never a silent pass, when no model is available. §9's offline claim scoped accordingly.
- Brief packing: the token budget governs; section caps are maxima within it (§12).
- Ladder policy objects must guard against correlated errors: minimum time span and source diversity per cell (§10.3); `never_automate` seeded with effect requests and tombstones.
- Concurrent machine writes serialized through one writer queue (§7.7); auto-defer gains a terminal `parked` state (§8.2); `edit_accept` write path defined (§8.2).
- M1 gains a recall floor; contradiction-detection recall reported explicitly (§17).
- Partial model migration: per-chunk pin versions; search never mixes pins (§7.5).
- gbrain import: representation-lossless, with the semantic asymmetry named (§15).
- Glossary defines **Composer**; vendored-dira licensing note added (§7.3).

### Changelog (v2 → v2.1, council-driven)

- Schedule re-stated as **scope-gated, not date-gated**; week numbers removed (6/6 seats).
- Claim store becomes **two-tier**: fences for human-scale claims, git-tracked append-only JSONL shards for high-volume families (6/6 seats). §7.2a.
- Earned-automation ladder thresholds become a **configurable policy object** measured on false acceptance, with grouped dispositions, queue SLOs, a calibration milestone, and escape hatches (6/6 seats). §10.3.
- New **epistemic layers** contract: Source / Observation / Claim / Precept (chatgpt). §7.6.
- **Security & Identity** expanded to a threat model with daemon auth, key storage, redaction before cloud calls, and an adversarial-evaluation gate (5/6 seats). §14.
- DISPOSITION expiry changed from auto-decline to **auto-defer**; idempotency, cross-client, and transport semantics specified (oxalpha, minimax). §8.2.
- **Rebuild identity pinned to model versions** (oxalpha). §7.5, §9.
- dira dependency **vendored-and-pinned**; upstreaming is parallel, not critical-path (oxalpha, minimax, grok). §7.3.
- Evals de-circularized: held-out sets, second labeler, P/R/F1 per predicate family (oxalpha, minimax, grok). §16.
- **Flutter app deferred to v1.1** (founder decision, 2026-08-27); v1 ships CLI-only, and the CLI is the first conformant client of all three protocols. §13.
- New **git operations** section: concurrent-edit merge policy, compaction, durability floor (4/6 seats). §7.7.
- M1 connector scope cut to IMAP + one certified provider; Graph deferred (minimax, chatgpt, grok).
- Rejected (reasons in the council's synthesis, internal review artifact): dropping DISPOSITION/DIRECTION from v1; SQLite-primary storage; web UI instead of Flutter; loosening public-protocols-only.

---

## 1. One-sentence summary

Serenity is a claim-based personal memory and direction system — a protocol-compatible successor to gbrain — that ingests everything one person produces, reconciles what is true in a git-canonical brain repository, serves that truth plus the person's standing judgments to AI agents over open protocols, and gates every consequential change through human disposition.

## 2. Positioning: the relationship to gbrain

Serenity is not a fork of gbrain and not a competitor to its thesis. It is the project gbrain's own v0 design document deferred and named — the "intelligence compiler": *treat every fact as a first-class claim with source span, entity links, validity window, confidence, and contradiction status.* Serenity builds that, keeps every substrate decision gbrain proved right, and stays wire-compatible so gbrain users can switch — or run both — without stranding anything.

| Dimension | gbrain (proven — keep) | Serenity (the successor bet) |
|---|---|---|
| System of record | Markdown repo canonical; DB is a rebuildable cache | **Same contract, kept verbatim** — CI-gated, invariant-tested |
| Atomic unit | Page (compiled truth + timeline) with facts/takes fences | **Claim**: subject–predicate–object with provenance, confidence, validity window, supersession chain |
| Staleness | Recency decay and source-tier boosts at *ranking* time | Confidence decay in the *data* — per-predicate-family half-lives |
| Contradictions | Probe measures; operator applies paste-ready fixes | Structural detection on `(subject, predicate)`; **approval-gated resolution with an earned-automation ladder** (§10.3) |
| Direction | Takes (calibrated opinions, not enforced) | **Precepts** (dira schema): immutable decisions/constraints with rejected alternatives, machine-checked against agent plans |
| Wire protocol | MEMORY_VERBS v1 (frozen, conformance-certifiable) | **Conformant MEMORY_VERBS v1 server**, plus two new protocols (§8) |
| Retrieval | Hybrid FTS + vector + RRF, typed zero-LLM graph edges, cited synthesis with gap analysis | **Ported pattern-for-pattern, not respecced** (§11) |
| Stack | Bun + TypeScript | Go, single static binary |

Credit is explicit: the README and docs cite gbrain as the lineage, and `serenity import --from-gbrain` (§15) migrates a gbrain brain losslessly. Passing `gbrain protocol conformance` against a Serenity endpoint is a release gate.

## 3. Strategy (why this is open source)

Serenity ships as a standalone Apache-2.0 project that is fully useful alone: one binary, one brain repo, no accounts, no hosted service. Its interoperability surfaces are published open protocols, not private integrations:

- Any HITL client can implement **DISPOSITION v1** (§8.2) to become the approval/capture surface. **Blink is the reference mobile client** — the best way to answer Serenity's approval queue from a phone.
- Any agent runtime can implement **DIRECTION v1** (§8.3) to consume briefs and plan-check verdicts. **Sire is the reference agent-runtime consumer** — the best way to run governed agents against a Serenity brain.

The funnel thesis: Serenity's OSS adoption creates demand for best-in-class protocol clients, which Blink and Sire are. The protocols must therefore be genuinely open — the CLI is the first conformant client of all three protocols in v1 (the Flutter app follows in v1.1), proving the project works with zero external products.

**The wedge is DIRECTION.** Memory is the substrate; "agents that know what you've decided — and are stopped before violating it" is the differentiated product behavior. Positioning, docs, and the first demo lead with plan-check, not with ingestion breadth.

**Protocol governance:** each protocol's document states who arbitrates changes (the maintainer, via a public RFC process in-repo), where versioned schemas and conformance fixtures live (`docs/protocol/`, `testdata/conformance/`), and the additive-forever rule. Conformance alone is not governance; the change process is part of the spec. **Kill criterion:** if real-world trials show users do not repeatedly exercise plan-check or conflict review, the protocol surface stops expanding until they do.

Serenity's roadmap is set by its standalone user, never by Blink/Sire needs; any affordance those products want must arrive as a public protocol change through the same RFC process.

## 4. Motivation

Human working memory holds ~7 items; an LLM holds ~a million tokens; neither holds a life. Personal-AI products either rent memory from a vendor (it resets on someone else's schedule) or pile up markdown with full-text search (stale facts surface confidently; contradictions accumulate silently). gbrain proved the substrate answer: your knowledge in your repo, compiled by agents, queried over a frozen protocol. What it measures but does not yet model is *belief*: which statements are currently held true, at what confidence, superseded by what, and decaying at what rate.

Serenity's three convictions:

1. **Memory must be atomic and revocable.** Claims, not pages: statements with provenance, confidence, and validity windows, superseded independently of the document they arrived in.
2. **Judgment must be immutable and enforced.** Decisions and constraints are recorded with the alternatives rejected and why — and agents' plans are checked against them before execution, not after.
3. **Capture must cost zero and hygiene must earn automation.** Ideas enter by rambling; machines file, reconcile, and resurface. Every automatic resolution right is *earned* by measured precision on human dispositions (§10.3) — never assumed.

## 5. Glossary

| Term | Meaning |
|---|---|
| **Brain repo** | The git repository holding all canonical knowledge (entities, claims, precepts, sources metadata) |
| **Source** | Raw imported material. Immutable, content-addressed |
| **Observation** | A machine-extracted assertion tied to one source span — not yet believed (§7.6) |
| **Claim** | Atomic reconciled statement with provenance, confidence, validity window, supersession chain |
| **Precept** | Immutable record of judgment (decision, constraint, intent, question) in the dira entry schema — authored once, only superseded |
| **Entity** | Person, company, account, project, topic, or health item — the join key; one markdown page per entity |
| **Reconciliation** | Resolving conflicting claims |
| **Disposition** | A human decision on a proposed change: accept, edit-accept, reject, defer |
| **Brief** | Attention-budgeted context packet served to an agent session |
| **Composer** | The synthesis engine behind `ask`: retrieval → cited answer + gap statement (§11) |
| **Index** | The derived, rebuildable database (search, embeddings, graph). Never canonical |

## 6. Non-goals (v1)

- No multi-user support (single principal; auth exists so the daemon isn't open). The file formats must not *preclude* multi-principal later: per-claim visibility fields exist in v1, are ignored by the daemon, and the format is locked so v2 can enforce them.
- No hosted service; nothing phones home. Egress is only the model APIs you configure. All metrics are local; sharing a report card is a manual export.
- No model training or fine-tuning.
- No autonomous side effects (sending, spending, deleting outside the brain). Draft generation yes; effects require disposition through the approval queue.
- No plugin ecosystem in v1. Connectors are in-tree; the connector interface is public API from day one so the ecosystem can come later.
- Not a note-taking app. Humans author Sources and dispose of proposals; the claim fences are machine-written, human-*repairable*.
- No GUI in v1 (founder decision, 2026-08-27): the Flutter desktop app is the v1.1 headline (§13.2); v1 is daemon + CLI.

## 7. System of record and file formats (the load-bearing section)

**The brain repo is canonical. The index is a derived cache. There are no database backups — the index rebuilds from the repo** (`serenity sync && serenity extract all`). This is gbrain's contract, kept verbatim, with the same enforcement posture: a CI gate that fails any code path writing user knowledge to the index without a file-first write, and an invariant E2E test that wipes derived tables and proves byte-identical rebuild. Rebuild identity is asserted **within a pinned model set** (§7.5) — re-extraction under a different model version is an upgrade, not a rebuild.

What this buys, same as it bought gbrain: disaster recovery is `git clone` + rebuild; multi-machine sync is `git push/pull`; audit history is `git log`; privacy granularity is gitignore; concurrent writers merge through git; and when extraction is wrong, a human opens the file and fixes it.

### 7.1 Repo layout

```
brain/
  entities/<type>/<slug>.md        # one page per entity (see 7.2)
  claims/<slug>/<family>.jsonl     # high-volume claim shards (see 7.2a)
  sources/<sha256[0:2]>/<sha256>/  # raw bytes + meta.yaml sidecar (see 7.4)
  .dira/entries/dec-*.md           # precepts — the dira ledger, unchanged (see 7.3)
  serenity.yml                     # config: connectors, models, pinned model set, index engine, ignore rules
.serenity/                         # derived: index db, embeddings, caches (gitignored)
```

### 7.2 Entity pages — canonical claims live with their subject

Each entity page is markdown with clearly-owned sections, gbrain-fence style:

```markdown
---
type: person
slug: alice-tan
aliases: [Alice, A. Tan]
---
# Alice Tan

<!-- serenity:summary:begin (DERIVED — regenerated by consolidate; do not hand-edit) -->
Runs engineering at Acme (series-B fintech). Last contact 2026-04-22…
<!-- serenity:summary:end -->

<!-- serenity:claims:begin -->
| id | predicate | object | conf | valid | src | state |
|----|-----------|--------|------|-------|-----|-------|
| c7f3a | works_at | acme | 0.92 | 2025-06-.. | e42#3 | active |
| c81b0 | committed_to | security review by 2026-05-01 | 0.85 | 2026-04-22.. | e57#1 | active |
| ~~c55d2~~ | works_at | initech | 0.88 | 2023-..2025-06 | e12#7 | superseded→c7f3a |
<!-- serenity:claims:end -->

<!-- serenity:timeline:begin -->
- 2026-04-22: pricing chat (e57)
<!-- serenity:timeline:end -->
```

Rules (each mirrors a gbrain rule that held up in production):

- The **claims fence is machine-written by a deterministic writer** that round-trips byte-identically (parse → edit → render). Humans may repair a row; the next extract cycle preserves human edits — for **fence-tier** families, the writer treats the file as truth. (Shard-tier head rows are derived; their authority rule is §7.2a.)
- **Claim ids are derived, not random:** `id = shorthash(subject_slug, predicate, normalized_object_key, valid_from, source_ref)` — the normalizer (lowercase, collapse whitespace, canonical date/number forms) is part of the spec and covered by property tests. Two machines extracting the same claim from the same source derive the same id (merges collapse cleanly); the same logical claim from *different* sources gets different ids **by design** — that is corroboration, and **dedup is semantic, performed at reconcile on `(subject, predicate, normalized object)` plus embedding-similarity neighbors — never by id equality**. Hash width is sized for the per-entity claim population; a detected collision (same id, different normalized content) is a hard error that widens the hash via migration, never a silent overwrite.
- **Supersession keeps the row** — strikethrough + pointer, gbrain's forget/supersede encoding. Nothing hard-deletes except the right-to-forget path (§14) and shard compaction (§7.7).
- Rows are append-mostly, so concurrent git merges are usually clean; the writer sorts and normalizes so merges are deterministic.
- Long objects (> 120 chars) move to a `claims-detail` block keyed by id; the table cell holds the head.
- `predicate` comes from a controlled vocabulary seeded at install (`works_at`, `has_role`, `owns_account`, `has_balance`, `has_condition`, `takes_medication`, `prefers`, `committed_to`, `deadline_on`, `relates_to`, `belongs_to_project`, `said`, `costs`), extensible only via `serenity.yml` + migration, never ad hoc by workers.
- Claim ids are content-derived short hashes — stable across rebuilds, usable in protocol responses.

### 7.2a Two-tier claim storage (council: 6/6 — fences alone do not scale)

Fence tables are the right home for **human-scale** claims: the ones a person reads, repairs, and reasons about (roles, preferences, commitments, conditions). They are the wrong home for **high-volume** predicate families — balances, transactions, recurring machine observations — where thousands of rows per subject per year produce git bloat, hostile diffs, and merge pain. Tiering, all of it git-canonical:

- Each predicate family is declared `tier: fence` or `tier: shard` in `serenity.yml` (seeded: `has_balance`, `costs`, and transaction-shaped families are `shard`).
- Shard-tier claims live in `brain/claims/<entity-slug>/<family>.jsonl` — **append-only JSONL, one claim per line**, same fields as a fence row, same provenance, human-readable and hand-repairable. Supersession appends a superseding line; nothing rewrites history in place.
- The entity page's claims fence holds only the **current resolved head** of each shard family (one row per (predicate, object-key), marked `src: shard`), so the page stays the human-readable current view.
- **Authority rule (which artifact wins):** for shard-tier families, **the shard is canonical and the fence head is derived** — regenerated by consolidate exactly like the summary fence, and marked `DERIVED` the same way. Conflict detection (§10.2) reads shards, never fence heads. A human edit to a shard-head fence row is not lost: the next cycle detects the divergence and raises a disposition item whose accept **appends a human-provenance line to the shard** (then regenerates the head) — the human's intent lands in the canonical artifact instead of being clobbered or silently trusted. Direct hand-repair of a shard line itself is always allowed (it's JSONL in git).
- The file-first CI gate, the wipe-and-rebuild invariant test, and the deterministic-writer round-trip rule apply to shards exactly as to fences. The invariant test includes a **fence/shard disagreement fixture** and asserts the rebuilt state derives from the shard.
- **Property test (M0):** 10,000 claims into one shard family — append, resolve, rebuild, merge two divergent copies — with bounded file size, deterministic merges, and byte-identical rebuild.

Rejected alternative (recorded): making SQLite primary for high-churn claims with periodic git rollups — it breaks the file-first invariant four council seats independently named the project's strongest part.

### 7.3 Precepts — the dira schema, natively

The precept store **is** a dira ledger: `.dira/entries/` files in the kazi-org/dira entry schema (decision, why, rejected alternatives with `why_not`/`revisit_if`, provenance, supersedes). **Dependency posture (council): vendor-and-pin now, upstream in parallel.** Serenity vendors dira's schema types and check engine at a pinned commit from day one; promoting them to an importable public package upstream is pursued concurrently (the PR is M3 work) but is **not on the critical path** — the vendored copy ships if the upstream PR is still open. The dira CLI operating correctly on a Serenity brain repo is a conformance test, and every existing dira ledger imports natively. A dira breaking change upstream cannot break Serenity: the pin moves only by deliberate migration. dira is Apache-2.0, so the vendored copy is license-compatible; the vendored directory carries dira's LICENSE and NOTICE per Apache-2.0 §4.

Precept rules carried from v1 unchanged: active precepts are never edited, only superseded; constraints carry machine-detectable `applies_when` clauses over a small closed action set (`start_project`, `deploy_to_prod`, `spend_over`, `contact_new_party`, `schedule_outside_hours`); `revisit_if` conditions are evaluated weekly and surface as review cards; question-precepts mark their targets execution-blocked. **Precepts are creatable only through human disposition** — no ingest, extraction, or model path may mint or alter one (this is a stated security invariant, tested adversarially in §16).

### 7.4 Sources

Content-addressed under `brain/sources/`, immutable, with a `meta.yaml` sidecar (kind, uri, occurred_at, connector state). Large or sensitive originals can be marked `index_only` in `serenity.yml` — bytes stay on disk, out of git (gbrain's `db_only` pattern). Deleting a source is a tombstone operation that cascades retraction proposals to its claims.

### 7.5 Derived index (never canonical)

Default engine: embedded SQLite (pure-Go driver; FTS5 for lexical search) plus an in-process vector store — exact cosine scan over memory-mapped embeddings, which at gbrain-production scale (~22k chunks) is single-digit milliseconds; an ANN index is an optimization gated on measured need. A `BrainIndex` Go interface keeps the engine pluggable; **Postgres + pgvector is the documented scale profile**, not the default. Runtime-only state (job queue, approval queue, spend ledger, caches) is DB-only by design, enumerated in an allowlist exactly as gbrain's system-of-record doc does.

**Pinned model set (council: rebuild identity vs model drift).** `serenity.yml` records the exact embedding and extraction model identifiers (provider + model + version) in use; every observation and claim carries `model@version` in its provenance. The byte-identical rebuild invariant is asserted only within an unchanged pinned set. Changing a pin is a migration (`serenity migrate --models`): re-extraction runs as a new observation pass whose diffs flow through reconciliation like any other source — never a silent full rewrite. **During a partial migration, pins never mix in search:** every stored vector carries its `model@version`, cosine distances across models are incomparable, so vector search uses only the active pin's embeddings and not-yet-re-embedded chunks are served by FTS until their re-embed lands; the rebuild-identity invariant is asserted only once migration is complete.

### 7.6 Epistemic layers (council: model output must not silently become user truth)

Four layers, each with distinct authority, distinguishable in the data:

| Layer | Authored by | Mutability | Where |
|---|---|---|---|
| **Source** | The world / the user | Immutable | `brain/sources/` |
| **Observation** | Extraction (a model) | Immutable, tied to one source span + `model@version` | provenance refs inside claims/shards |
| **Claim** | Reconciliation (machine proposes, ladder/human accepts) | Supersedable | fences + shards |
| **Precept** | The human, via disposition only | Immutable, supersede-only | `.dira/entries/` |

Confidence is a property of observations and claims — it never applies to precepts, and it is not the universal epistemic scalar: a high-confidence extraction of a false source is still false, which is why provenance (what was observed, what was inferred, what was asserted by the user, what was decided) is queryable alongside confidence in every protocol response.

### 7.7 Git operations (council: 4/6 — the unwritten half of "git-canonical")

- **Concurrent edits.** All machine writes — ingest workers, reconciliation, entity-resolution merges, consolidate — are **serialized through one writer queue with per-file ordering**; two workers never race on the same file, so machine-vs-machine write contention cannot occur by construction (a property test asserts it). The daemon writes only through the deterministic writer and commits its own changes with a `serenity:` prefix. Before writing, it checks the working tree: a dirty human edit in a target file pauses that file's writes and raises a disposition item ("your edit vs pending machine write"); the human's file state is truth (§7.2). Merge conflicts across machines resolve deterministically for fences and shards (sorted, normalized, append-only) and are covered by the M0 property test.
- **Compaction.** Shard files roll over at a configured size; superseded lines older than a configured horizon compact into an archive shard (`<family>.archive.jsonl`) by an explicit, disposition-approved `serenity compact` run — never silently. `git gc`/repack guidance ships in the operator's manual; repo growth per month is a reported metric.
- **Durability floor.** "No DB backups" assumes a healthy remote. `serenity init` configures a post-commit push (or timer push) to the user's remote and warns loudly when the repo has no remote or push is failing; `serenity doctor` checks last-push age. A corrupt or force-pushed remote plus a lost laptop is the data-loss scenario the docs name honestly.

## 8. Protocols (the interoperability standards)

All three live in `docs/protocol/`, versioned under the MEMORY_VERBS policy: field names and semantics frozen forever, additive-optional changes allowed, `protocol_version` on every response, breaking change = new document (expected never). Each ships with a machine-readable schema (`serenity protocol --json`), conformance fixtures in-repo, and a conformance command. Changes go through the public RFC process (§3).

### 8.1 MEMORY_VERBS v1 (adopted, not invented)

Serenity is a conformant MEMORY_VERBS v1 server: `recall`, `remember`, `entity`, `synthesize`, `forget` over MCP, with the envelope fields (evidence, provenance, budget meta, enumerated errors) as specified by gbrain. `gbrain protocol conformance --target <serenity>` passing is a release gate. Serenity-specific additions ride as optional fields (per-fact `confidence`, `claim_id`, `superseded_by`) — legal under additive-forever.

### 8.2 DISPOSITION v1 (new)

The human-in-the-loop wire protocol. Operations:

- `list_pending(kinds?, expiring_before?, group?)` — approval items: reconciliation pairs (A/B with provenance), precept drafts, effect requests, distill items, tombstones. With `group`, similar items (same connector, predicate family, and proposed resolution shape) return as **grouped items** — one disposition covers all members, each recorded individually for the ladder.
- `dispose(item_id | group_id, verdict, edited_payload?, note?, idempotency_key)` — `accept | edit_accept | reject | defer`. Reject requires a note. `idempotency_key` makes retries safe.
- `capture(text | audio_ref, hint?)` — zero-friction ingress; returns a staged distill item id.
- `subscribe(cursor)` — server-sent events over HTTP (long-poll fallback), at-least-once with monotonic cursors; clients resume from their cursor after disconnect.

Semantics (council-hardened):

- **Expiry is auto-DEFER, never auto-decline.** An expired item (default 14 days, configurable per kind) drops to a low-priority aged state with visible aging and periodic resurfacing; it never silently converts machine ambiguity into a claim-state change. Silent destructive defaults are forbidden by the protocol spec. **Deferral terminates:** after N defer cycles (default 3, configurable) an item moves to `parked` — a terminal-but-reversible state, visible in its own view (`serenity inbox --parked`), excluded from queue-depth/age SLO metrics, and resurfaced only by explicit filter or by new evidence arriving on the same `(subject, predicate)`. Nothing resurfaces forever.
- **`edit_accept` writes through the deterministic writer** like any accept — the edited payload lands in the fence or shard with `human` -tier provenance and the disposition recorded; it is not a client-side file patch. Hand-repair directly in the file (§7.2) remains the other, equally legitimate path.
- **Cross-client conflict:** first successful `dispose` wins; a second disposition of the same item returns `already_disposed` with the recorded verdict so the other client reconciles its view. A property test runs two clients against one queue and proves the human's recorded intent is single and durable.
- Every disposition is recorded with actor, client, and timestamp; disposition history is the training signal for the earned-automation ladder (§10.3).

**Blink is the reference mobile client.** Serenity's own CLI (`serenity inbox`, gmail-style J/K/space keys) is the first conformant client — the daemon has no privileged internal path. The v1.1 Flutter app implements the same wire.

### 8.3 DIRECTION v1 (new)

The agent-governance wire protocol — the wedge (§3):

- `brief(task_hint?, token_budget)` — the attention-budgeted context packet (§12): active precepts, current intents, relevant entities, open blocking questions. Server-side packing; whole-section drop-not-truncate; omission counts always stated.
- `check_plan(plan_text, actions?)` — the **schema verdict is primary**: per applicable active constraint, `pass | violated(precept_id, why_not, revisit_if)`, plus warnings for unanswered blocking questions, plus a `not_applicable` result naming how many active constraints matched nothing — **a plan matching zero constraints returns `no_applicable_constraints`, never a bare pass**, so callers can distinguish "checked and clean" from "nothing checked."
  **Matching mechanism (two stages):** (1) callers may pass a structured `actions[]` list (the closed action set of §7.3, with parameters — e.g. `{action: spend_over, amount: 500}`); matching structured actions against `applies_when` clauses is **deterministic and fully offline**. (2) When only free text is supplied, the local-cheap tier classifies the plan into the closed action set before deterministic matching; classifier output rides in the verdict (`matched_actions`, with spans) so the caller can audit the mapping. **With no model available, free-text checking returns `unverified` — an explicit verdict, never a silent pass** — and the CLI exits 1 (error), not 0. Conformance fixtures cover both stages, with the classifier stage run against the pinned model set (§7.5). Well-behaved agent runtimes (Sire, Claude Code hooks) send structured actions; free text is the fallback, not the design center. Verdicts carry enough context to be actionable (the constraint text, the matched action, the stored reasoning) — an agent needs an explanation it can act on, not only a flag. CLI exit codes (0 pass, 2 violation, 1 error) are a transport convenience of `serenity check`, not the semantics.
- `propose(kind, payload)` — agents propose precept drafts and effects; everything lands in the DISPOSITION queue. No model call ever mutates a precept.

**Sire is the reference consumer**: a governed agent runtime calls `brief` at session start and `check_plan` before executing any plan. Claude Code integrates via hook + MCP in one command (`serenity connect claude`).

## 9. Model routing

One chokepoint: `Router.Complete(taskClass, prompt, budget)`. Commodity-first defaults:

| Tier | Default | Task classes |
|---|---|---|
| **Local-cheap** | BYO cloud key, cheapest tier (Haiku-class) — or local models (Ollama-class) as a config choice | embedding, transcription, classification, extraction candidates, summarization, consolidation |
| **Judgment** | BYO cloud key, frontier tier | decomposition proposals, hard-conflict reconciliation judgment, plan-vs-precept analysis, Composer synthesis |

A 128GB-class unified-memory box running 70B-class local models is a documented **scale profile** (`serenity.yml: profile: local-heavy`), not the baseline. Every judgment-tier call writes to the spend ledger; a configurable monthly ceiling (default $50 — commodity users, not lab budgets) queues further judgment calls behind an approval, with projected-spend shown in `serenity status` and the briefing before the ceiling trips. Offline: ingestion, retrieval, entity pages, briefings, and plan-check over **structured actions** all function with zero internet; free-text plan classification needs a model (local models qualify) and returns `unverified` without one (§8.3); judgment-tier tasks degrade.

Confidence bounds (council: arithmetic fixed): machine-assigned confidence is capped at 0.90 (local-cheap) / 0.95 (judgment). **Only human confirmation can set confidence above 0.95.** Model identifiers and versions ride in provenance per §7.5.

## 10. Subsystems

### 10.1 Ingest

Connector interface (public Go API from day one):

```go
type Connector interface {
    Name() string
    Poll(ctx context.Context, cursor Cursor) (items []RawItem, next Cursor, err error)
    ToSource(item RawItem) (Source, error)
}
```

Idempotent by sha256 dedup; every run is a job row; each connector ships a test-corpus fixture and golden-output tests. **v1 connectors (P0, council-cut):** file watcher (drops/exports/PDFs/screenshots), email — **IMAP with one certified provider** (OQ2 picks it; Microsoft Graph is deferred post-launch as its own project), git-repo crawler (docs/READMEs; flags dira-pattern files as precept-draft candidates), voice note (transcription via router). **P1:** Graph email, finance CSV (column-mapping wizard), health export, WhatsApp/iMessage export files. Connector docs state the support matrix and the auth-refresh UX ("your token expired" is the first failure users hit; re-auth is one command with a deep link).

Pipeline (layered per §7.6): chunk → extract candidate **observations** (controlled predicates, confidence, `model@version`) → embed → reconcile into **claims** via the deterministic writer (below threshold 0.6 the item goes to the distill queue instead) → index update → `claims.created` event.

### 10.2 Reconcile

Triggered on `claims.created`. New claim vs. active claims sharing `(subject, predicate)` plus high-similarity neighbors:

- **Agree** → corroborate; weighted confidence bump.
- **Neutral additive** → both stay.
- **Conflict (facts)** → a reconciliation item in the DISPOSITION queue, A/B with provenance. *No automatic supersession at trust level 0* (see 10.3).
- **Conflict (touches a precept)** → never auto-resolved; a precept-draft/question item showing the constraint verbatim with `why_not` and `revisit_if`.

Temporal and scoped conflicts ("was true then", "true for project X") are first-class: the detector compares validity windows and scope qualifiers before declaring contradiction, and proposes window-close (temporal supersession) as distinct from refutation — mirroring gbrain's six-verdict temporal enum.

Weekly sweep: alias-merge candidates, confidence decay (half-life per predicate family: balances 1 day, roles 90 days, preferences 365 days — decay affects *ranking and staleness banners*; it never by itself authorizes supersession), low-confidence claims to the distill queue.

### 10.3 The earned-automation ladder (fixes v1's unsafe auto-supersede; council-recalibrated)

v1 allowed auto-supersession on a confidence delta — which, combined with decay, let any fresh noisy extraction silently overwrite an aged-but-true claim. gbrain holds the opposite invariant (auto-supersession NEVER applies). Serenity's position: gbrain's invariant is the *starting* state, and automation is *earned per (connector, predicate-family)* — but the thresholds are a **policy object, not constants** (council: 6/6 — flat 50/98% is unreachable for sparse cells and unsafe as sole evidence for dense ones):

```yaml
ladder:
  default: {min_dispositions: 50, min_accept: 0.98, sample_rate: 0.05}
  per_cell:                      # frequency-aware overrides
    email/has_balance:  {min_dispositions: 50,  min_accept: 0.99, sample_rate: 0.05}
    email/works_at:     {min_dispositions: 20,  min_accept: 0.95, sample_rate: 0.10}
  correlation_guards:            # promotion also requires…
    min_span_days: 14            # …dispositions spread over time, and
    min_distinct_sources: 5      # …across sources — one bad connector day
                                 # or one labeling mood cannot promote a cell
  never_automate: [precept-touching, effect-requests, tombstones, contact_new_party]
```

- **Trust 0 (default):** every conflict is a disposition item. This is gbrain's posture.
- **Trust 1:** unlocks per cell when the cell's policy is met — with the promotion metric being **false-acceptance rate on sampled audits, not raw accept rate** (a 98% accept streak is not evidence against distribution shift; the spot-sample is). Counts and rates alone are also insufficient for sparse cells — at 50/0.98 a single mislabeled disposition decides promotion — so the correlation guards (time span, source diversity) are mandatory fields of every policy, not options. Every automatic action is still logged as a disposition item marked `auto` and sampled into the queue at the cell's rate.
- **Demotion is automatic:** a human reversal of an auto action drops the cell back to Trust 0.
- **Calibration is a milestone, not a guess (M2):** simulated disposition streams from labeled fixtures sweep the thresholds; shipped defaults are published with the sweep data. The numbers above are priors, replaced by evidence before launch.
- **Queue hygiene is structural:** queue depth and age carry SLOs (alert in the briefing when p50 age > 3 days or depth > 50); grouped dispositions (§8.2) let one decision clear N similar items; a **stuck-queue escape hatch** (`serenity inbox --bulk-defer <filter>`, per-family pause) exists so the queue can never hold the user hostage.

The disposition history is the measurement; the ladder is the policy; the queue is the audit. "Hygiene must be automatic" becomes true over time *because it is proven*, not assumed at install.

### 10.4 Consolidate

Nightly + on-demand: regenerate entity summary fences from active claims (freshness banners: "nothing on X since March"); refresh shard heads in fences (§7.2a); re-embed changed chunks; render the daily briefing — fixed sections *Blocked / Needs you / Moved forward / Watched / Drift*, hard cap 800 words, whole-section drop-not-truncate with stated omissions.

### 10.5 Entity resolution

Staged: exact/alias match → embedding similarity within type (auto-merge with undoable merge event + audit trail) → ambiguous cases as low-priority disposition items. Splits supported from the client.

### 10.6 Direct

First-run **interview wizard** (~30 questions → draft precepts, each confirmed by one disposition) seeds the direction layer — archaeology of old documents explicitly is not the seeding mechanism. **Decompose** (judgment tier proposes child intents with `derives_from`; one-keystroke confirmations). **Orphan detector** (weekly: activity with no derivation edge to an active intent → briefing Drift section). **Distill queue** (staged capture items age and resurface; dispositions: claim-batch / precept draft / decaying note / trash).

## 11. Retrieval and synthesis (ported, not respecced)

Serenity adopts gbrain's proven retrieval architecture pattern-for-pattern: hybrid lexical + vector search with RRF fusion; multi-query expansion on the cheap tier; 4-layer dedup (source, cosine > 0.85, type cap, per-page max); typed graph edges extracted zero-LLM from entity links and claim `object_entity` references, traversable to answer relational queries embedding search can't reach; synthesis that returns a cited answer plus an explicit **gap statement** (what the brain doesn't know and how stale its newest evidence is). Claims give synthesis one new power: answers can state confidence and cite the supersession chain ("believed X until June, now Y because Z").

Benchmark posture: BrainBench (gbrain's public eval corpus) runs **continuously in CI from M1** — parity is tracked as a trend and published at launch, not held as a single launch gate; the claims-specific eval set (contradiction resolution, validity-window questions, plan-check true/false rates) is the differentiating benchmark.

## 12. Brief format

Identical in spirit to v1, now a DIRECTION v1 wire object: standing precepts (cap 12) → current intents chained to ambitions (cap 8) → relevant entities with claim heads + provenance (cap 8) → open blocking questions (cap 5) → `omitted: N` line. Server-side packing against the caller's token budget. Drop whole sections by priority, never truncate mid-item. **Precedence: the caller's token budget always governs** — the per-section caps are maxima *within* the budget, so the effective caps are conditional; when even the highest-priority section alone exceeds the budget, its items are dropped whole from the tail (item-level drop-not-truncate) with the omission counted. Packing quality is measured, not assumed: the Ava corpus (§16) includes brief-relevance judgments per task hint.

## 13. Client surfaces

### 13.1 CLI (P0 — the v1 surface, and the first conformant protocol client)

The daemon (`serenityd`) and CLI ship in one binary. Verbs mirror the protocols: `init`, `sync`, `extract`, `search`, `ask`, `inbox` (J/K/space disposition review, `--bulk-defer`, grouped items), `brief`, `check` (exit-code plan-check), `capture`, `import`, `compact`, `serve` (MCP stdio + HTTP), `doctor`, `status` (includes spend-to-date, queue depth/age, ladder cell states, connector health, last-push age). CLI and protocol surfaces are thin wrappers over one engine; drift tests assert identical results. The CLI consumes DISPOSITION and DIRECTION over the same wire the protocols specify — the daemon has no privileged internal path (v1's enforcement of the rule the Flutter app inherits).

### 13.2 Flutter app (v1.1 — deferred from v1 by founder decision, 2026-08-27)

Council consensus (6/6) flagged the desktop app as the schedule bomb; v1 ships CLI-only. The Flutter app is the **v1.1 headline deliverable** on its own track, unchanged in design: desktop first (replacing v1's web-console idea; the daemon serves APIs only — no HTML surface), same codebase to iOS/Android after. Screens: **Today/Briefing** (cards, deep links) · **Inbox** (approvals: reconciliation A/B with provenance, precept drafts with `why_not` always visible) · **Distill** (swipeable staged items: claim / precept / note / trash, one-gesture confirm) · **Ask** (Composer with citation chips into the source viewer) · **Entities** (browser + page viewer + claims explorer with state/confidence filters) · **Direction** (precept tree + supersession timelines, intent tree with orphan flags) · **Ops** (workers, spend, connector health) · first-run interview wizard.

Binding design principles: provenance ≤ 1 click from any fact; claims never editable in place (repair happens in the file or through reconciliation); the UI states what it omitted; information density over decoration. The app talks to the daemon through the public protocols only — no private paths, enforced by acceptance criterion.

Mobile capture and approvals before the Flutter builds ship: any DISPOSITION v1 client — Blink first.

## 14. Security and privacy (council: 5/6 — expanded from a paragraph to a contract)

A single binary that ingests a person's email, files, and repos and serves them to agents is a large attack surface; this section is P0 spec, landed before M0 code.

**Threat model (documented in-repo, kept current):** malicious or instruction-injected source content (email, files, repo docs) attempting to steer extraction or agents; a malicious or compromised MCP client exfiltrating via `recall`/`synthesize`; model-provider data handling; secrets accidentally present in ingested repos; local attackers reading the index or key material.

- **Keys and tokens:** model API keys and connector OAuth tokens live in the OS keychain, never in files, the brain repo, or the index. Connector token rotation/refresh is automatic with a one-command re-auth path.
- **Daemon exposure:** binds localhost with a bearer token required by default (loopback is authenticated too — any local process is not automatically trusted); LAN/Tailscale exposure is explicit config with token + optional mTLS. Every protocol endpoint authenticates; DISPOSITION and DIRECTION are never anonymous.
- **Prompt-injection posture:** ingested content is data, never instructions — extraction prompts are structured to prevent source text from steering predicates or confidence; **no ingest path can create or modify a precept** (§7.3), and the adversarial eval (§16) attacks exactly this ("can a malicious email make an agent believe a precept exists?").
- **Redaction before cloud calls:** prompts to cloud models carry only composed briefs/chunks (data-flow diagram required in docs); a configurable redaction pass (patterns + entity-type rules, e.g. account numbers) runs before any cloud egress; `index_only` sources never leave the machine.
- **Right-to-forget:** source tombstone → claim retraction proposals → fences/shards rewritten, index rebuilt, derived pages regenerated; git history is the operator's to rewrite or accept (documented honestly — a git-canonical brain remembers unless you rewrite history). Deletion semantics are contractual: source deleted → its observations invalidated → claims with sole provenance demoted or retracted → rebuild.
- **Durability:** backups are `git push` (configured and monitored per §7.7) plus an optional sources snapshot.

## 15. gbrain migration (adoption lever, P0 at launch)

`serenity import --from-gbrain <repo>`: pages → entity pages; facts fences → claims (provenance preserved, confidence seeded by source tier); takes → claims with `said`/`prefers` predicates flagged for review; timelines → timeline fences; links → graph edges. **"Lossless" is defined operationally, and its asymmetry is named** (council: counts prove little): every gbrain fence row maps to exactly one Serenity claim with predicate, validity window, visibility, and provenance fields asserted equal by a field-level round-trip test on a public fixture brain — plus counts and spot-check evals on top. This proves **representation-level** losslessness; semantic translations (take-calibration data seeding confidences, source-tier boosts becoming initial confidence) are lossy interpretations by nature — each is a documented mapping, and every claim produced by one is flagged for review rather than presented as settled. Import is a resumable, incremental runbook (not a one-shot script) so multi-year brains migrate without downtime. A gbrain user's brain works in Serenity in one command; their connected agents keep working because the verbs are conformant.

## 16. Observability and evals

Structured logs with job correlation ids; metrics — all local, shareable only by manual export (`serenity report --export`): ingest lag, reconcile backlog, disposition queue depth/age and time-to-dispose, spend/day and projected month, auto-action sampled false-acceptance rate, repo growth/month, rebuild timing, connector health/MTTR, search p95.

**Eval methodology (council: de-circularized).** Golden sets are held-out: labeled by a second labeler with adjudication, versioned, never trained-on or tuned-against in the same milestone that builds the extractor. Reported per predicate family: precision, recall, F1 — plus contradiction-detection F1 and plan-check true/false rates. Gates run in CI on cached model outputs so the budget isn't burned per-push.

Corpora: the "Ava Standardo" synthetic-persona corpus (50 Q&A for Composer correctness; per-connector extraction P/R; reconcile confusion matrix; plan-check rates; brief-relevance judgments) · BrainBench parity trend (§11) · **the adversarial corpus**: injected instructions in email/files/repos, contradictory and stale sources, deliberate false claims, entity collisions, poisoned documents, and precept-fabrication attempts — the release gate asserts zero precept mutations and zero unauthorized effect proposals from adversarial sources.

Weekly report card: claims by state, corrections per 100 extractions (must trend down), trust-ladder cell promotions/demotions with sampled false-acceptance, spend. First-value walkthrough (§17 M1 AC) is re-run on every release.

## 17. Milestones and acceptance criteria (scope-gated — council: 6/6 against date-gating)

**Milestones are ordered scope gates, not calendar weeks.** Launch = M5's ACs green, whenever that is; nothing ships early by weakening an AC. The v1 critical path deliberately excludes the Flutter app (v1.1) and the Graph connector (post-launch).

**M0 — Skeleton + substrate invariants.** Go monorepo; `serenity init` scaffolds a brain repo (with remote-push durability check); fence AND shard writers with byte-identical round-trip property tests; the 10K-claim shard property test (append/resolve/rebuild/merge, §7.2a); SQLite index + `sync`/`extract all` rebuild; pinned-model-set config; secrets in OS keychain; daemon auth token on by default; goreleaser + brew tap CI. ✅ AC: wipe `.serenity/`, rebuild from repo, byte-identical within the pinned model set; single binary runs on macOS arm64 + Linux amd64/arm64; shard property test green.

**M1 — Ingest spine + honest evals.** Watcher, IMAP email (one certified provider — OQ2), repo crawler; extraction to observations → claims; hybrid search + `ask` with citations; the eval harness built properly (held-out, second labeler, P/R/F1 per family). ✅ AC: import 30 days of real email + 5 repos on a laptop; extraction P/R/F1 published per predicate family on the held-out set — **floors on both axes: ≥ 90% precision AND ≥ 80% recall per family** (low-recall extraction that misses contradictions entirely is the failure mode this architecture exists to catch; a precision-only gate would pass it invisibly); contradiction-detection recall reported explicitly; double-import produces zero duplicates. **First-value walkthrough AC:** a new user installs, ingests one connector corpus, corrects five proposed claims, creates three precepts via the wizard, gets one cited answer, and sees a plan rejected against one precept — end to end from docs alone.

**M2 — Reconcile + entities + ladder calibration.** Conflict detection (incl. temporal/scoped), DISPOSITION queue + CLI inbox with grouped items and bulk-defer, queue SLOs wired to the briefing, entity resolution, consolidate, decay sweep; **ladder calibration**: simulated disposition streams from labeled fixtures sweep thresholds; shipped policy defaults published with the sweep data. ✅ AC: injected contradicting-balance fixture produces a correct A/B disposition item and, on accept, a correct supersession line in the shard + updated fence head (visible in `git diff`); entity merge round-trips; two-client dispose race resolves to one durable verdict; calibration report exists.

**M3 — Direction.** dira vendored-and-pinned (upstream PR opened in parallel, non-blocking); interview wizard; `check` + DIRECTION `check_plan` with schema-primary verdicts incl. `no_applicable_constraints`; orphan detector. ✅ AC: interview seeds ≥ 10 active precepts; a plan containing a constrained action exits 2 with the stored `why_not` verbatim; a plan matching nothing returns `no_applicable_constraints`, not pass; the dira CLI reads the same ledger unmodified; a question-precept blocks its target.

**M4 — Serve + protocols.** MCP + HTTP with auth; MEMORY_VERBS conformance; DISPOSITION and DIRECTION v1 with `--json` schemas and in-repo conformance fixtures; spend ledger + ceiling + projection. ✅ AC: `gbrain protocol conformance` passes against Serenity; an external Claude Code session answers "what did we decide about X?" citing two sources on the Ava corpus; ceiling trip forces the approval path; all endpoints reject unauthenticated calls.

**M5 — Migration + launch.** `import --from-gbrain` (lossless per §15's operational definition; resumable); docs site (install, operator's manual, threat model, protocol specs + RFC process, connector guide); BrainBench trend published; adversarial corpus gate green. ✅ AC: public fixture gbrain brain migrates with the field-level round-trip test green; fresh-machine install-to-first-answer ≤ 15 minutes on an **empty brain** from docs alone, and a large-brain import (≥ 10K messages) completes within a stated budget (≤ 4h, resumable) — split ACs per council; **the name decision (working title vs rename) is resolved and executed before the repo is public** — renames after launch forfeit early SEO.

**M6 — Hardening.** 7 consecutive days unattended on both profiles (laptop, 128GB box); chaos test (kill any worker, no lost jobs — job durability semantics documented: WAL-backed queue, idempotent workers); trust-ladder promotion demonstrated on real dispositions with sampled false-acceptance reported; p95 warm search < 400ms; repo-growth and rebuild-time metrics collected over the soak. ✅ AC: all of the above measured, not asserted.

**v1.1 track (post-launch, own sequence):** Flutter desktop app (§13.2, all screens, public protocols only; AC: end-to-end ramble → distill → dispose in app → next morning's briefing references it; A/B disposition from the app writes the correct fence/shard row) → Graph email connector → Flutter mobile (iOS/Android) → finance/health/messaging connectors.

## 18. Risks and mandated mitigations

1. **Extraction noise → garbage brain.** Controlled predicates; provenance-or-it-doesn't-exist; epistemic layering (§7.6) keeps observations from silently becoming truth; trust ladder starts at gbrain's never-auto invariant; per-connector eval gates; human repair path always open in the files.
2. **Disposition fatigue** (the failure mode v1's "zero clerical work" hid). Queue-depth/age SLOs with briefing alerts; grouped dispositions and bulk-defer; frequency-aware ladder policy calibrated on data (M2); distill confirmations are one gesture; time-to-dispose and abandonment tracked locally.
3. **Security failure** (new, council). Threat model is P0 spec; precepts unmintable by any machine path; adversarial corpus is a release gate; every endpoint authenticated.
4. **Protocol stagnation or capture.** Additive-forever policy; public RFC process; conformance suites runnable by third parties; Blink/Sire integrate through the public wire only — no private daemon paths (CLI enforces in v1, app AC enforces in v1.1); kill criterion on protocol expansion (§3).
5. **Retrieval regression vs gbrain.** BrainBench runs continuously from M1; the trend is published at launch.
6. **Substrate scaling.** Two-tier storage with the 10K-claim property test in M0; compaction and growth metrics; the tier split is config, so a family can move tiers by migration, not redesign.
7. **Scope gravity.** §6 non-goals are contractual; a PR adding an in-place claim-edit path or a private integration surface requires RFC amendment sign-off.

## 19. Open questions

- OQ1 — **resolved as process (2026-08-27):** "Serenity" is a working title; the rename decision gates M5 (blocking AC). Candidate shortlist happens before M5, checked against existing OSS projects and trademarks.
- OQ2 (before M1): which single email provider is certified for the P0 IMAP connector.
- OQ3 (before M3): exact surface of the promoted dira public package (schema types only vs schema + check engine vs schema + full ledger I/O) — negotiated as the upstream PR; **vendoring means any answer works** (§7.3).
- OQ4 (before v1.1): Flutter desktop distribution/signing per platform (notarization, MSIX).
- OQ5 (post-launch): multi-principal enforcement (visibility fields exist per-claim in v1, ignored by the daemon; v2 enforces — format locked accordingly, §6).

---

*Appendix available on request: protocol field-level specs, fence + shard grammar (EBNF), Ava Standardo corpus spec, adversarial corpus spec, gbrain migration field mapping.*

This document is deliberately opinionated where ambiguity kills schedules (formats, policy shapes, protocol envelopes, epistemic layering) and explicitly open where taste matters. Build in M-order; don't trade M-order for cleverness.
