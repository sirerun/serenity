# T23.43 — Semantic retrieval qualification corpus and harness

Owner: T23.43 (embeddings lane). This file is T23.43's own deliverable
(exclusive write scope per its task contract); it is not the task41
interface freeze itself, but the proposed freeze content task41's
reviewer signs off on before any `--live` run (docs/launch/hosted-plan.md
"Threshold review").

## Corpus

`evals/hosted/lib/corpus_data.py` is the literal, reviewable source of
every case and fact; `python3 evals/hosted/corpus_gen.py` renders it to
the committed `evals/hosted/corpus.json` and `evals/hosted/facts.json`,
and `python3 evals/hosted/corpus_gen.py --check` fails if either file
drifts from what the generator would produce today. No case or fact in
this corpus refers to a real person, account, or company; every subject
is invented for this corpus (`test_corpus.py::test_no_customer_data_markers`
guards against an accidental real-world name).

Composition (exactly matches T23.43.md step 1):

| Category | Count | Notes |
|---|---:|---|
| paraphrase | 40 | >=20 have zero content-word overlap with their target fact (computed, not hand-flagged — see `lib/textutil.content_overlap`); this corpus revision has 21 |
| name_entity | 20 | confusable-name/company triads and pairs, testing entity binding under near-identical names |
| preference | 20 | negation and near-duplicate preference pairs/triads |
| multilingual | 10 | English-authored facts, 10 distinct non-English query languages |
| temporal | 5 | facts with an explicit year/date |
| empty | 5 | expected-empty forgotten/expired cases; frozen queries unchanged. Live: each is backed by a synthetic target that is remembered and then removed, three by `forget` and two by expiring past a fixed absolute TTL (see "Expected-empty cases"). Local lexical control: isolated single-fact disposable brains |

**Frozen corpus hash** (`evals/hosted/corpus.json`'s `meta.corpus_hash_sha256`,
sha256 of the canonical-JSON case list): recorded at generation time in
the committed file itself. Any change to `lib/corpus_data.py` changes this
hash; a `--live` manifest's `corpus_sha256` must match it exactly or the
harness refuses to run (see "Manifest extensions" below).

Distractor design: paraphrase/name_entity/preference cases are built as
topic triads — three subjects sharing one topic template, each becoming
its own case with the other two triad members as its scored distractors.
A model that clusters on topic alone (ignoring the correct subject
binding) ranks the wrong triad member equally well, so Hit@5 genuinely
measures subject-specific retrieval, not topic classification.

## Frozen acceptance thresholds

Copied verbatim from `docs/tasks/hosted-completion/T23.43.md`'s acceptance
section; this file does not introduce new numbers, only implements them
(`evals/hosted/lib/scoring.py`):

- Hit@5 overall >= 0.90 over the 95 positive queries.
- Category floors: paraphrase >= 36/40, name_entity >= 19/20,
  preference >= 19/20, multilingual >= 9/10, temporal = 5/5.
- All 5 expected-empty cases return no current fact, scored strictly: any
  returned search result or any fact in recall's facts arm fails the case.
  There is no allowed set (an earlier revision let an account's own
  unrelated filler pass; that made the criterion vacuous and is removed).
  Cross-account/forgotten leakage = 0, with the cross-account sentinel
  scored separately.
- The contract asks for at least 18 of 20 lexical-negative paraphrases (a
  90% ratio) to hit semantically (expected fact in top 5) **and** miss in the
  local lexical control (expected fact absent from the lexical-only top 5).
  This corpus flags **21** zero-content-word-overlap paraphrase cases, not
  20, and the draft implementation is a **raw count**: it passes at 18 hits
  when at least 20 cases are flagged (`LEXICAL_NEGATIVE_MIN_HITS = 18`,
  `LEXICAL_NEGATIVE_MIN_DENOM = 20` in `evals/hosted/lib/scoring.py`). Over 21
  flagged cases that is 18/21 = 85.7%, which is **weaker than** the
  contract's 18/20 = 90%. It is not a proportional reading of the contract.
  It is an unapproved interpretation, kept only so the harness has a number to
  report, and it is not the freeze. **Before acceptance the named task41
  reviewer must choose** either an exact subset of 20 of the 21 flagged
  cases, named by id, scored 18 of 20; or an exact ratio (for example
  at least 90%, which is 19 of 21). Nothing in this document, the harness or
  the corpus changes until that choice is made and recorded as a reviewed
  amendment.
- A fixed-vector or lexical-only stub must fail the quality predicate;
  fixture mode may pass harness mechanics but must never report quality
  PASS. `scripts/hosted/eval_embeddings.py --fixtures` raises immediately
  if its own fixed-vector negative control ever clears the floor — that
  would mean the scoring pipeline is broken, not that a stub is good.
- Budget exhaustion or provider unavailability yields BLOCKED with
  partial results and no further calls, never a silent PASS/skip.

## Freeze receipt (task41 reviewer fills this in)

The corpus/threshold choice and the supplemental forgotten/expired plan are
separate approvals. Record the exact source revisions and reviewer for each;
neither approval may be inferred from the other.

### A. Corpus, thresholds and seeding protocol

- Status: **PARTIALLY FROZEN — one field still open, see below**. Per this
  document's own rule, nothing in the corpus, harness or thresholds changes
  until every field below is recorded; the executable guard will correctly
  keep blocking `--seed`/`--live` until the open field is resolved.
- Reviewer / date (UTC): David Ndungu, 2026-09-21.
- Reviewed T23.41 revision / reviewed T23.43 revision:
  `c61ab91bb48099dfbfcfec2ee86ea2cd590439c4` /
  `b1049482a1f1ffbd42f960a2b68341bb51691116`.
- Corpus hash reviewed and accepted:
  `f2593f81a5935a0e763672a057195f57c3b81475f21f78132689903ce56b56b6`
  (`evals/hosted/corpus.json`'s `meta.corpus_hash_sha256`, matches the
  committed file). Expected IDs (95 positive + 5 empty = 100 cases,
  `expected_fact_id` per case) reviewed and accepted as committed in
  `evals/hosted/corpus.json`/`facts.json`, `facts_hash_sha256`
  `35f54860e48070db927b5ad072fee65386829695998022eb0274c80fc1cdb1bc`.
  Numeric thresholds (Hit@5 >= 0.90/95; paraphrase >= 36/40; name_entity >=
  19/20; preference >= 19/20; multilingual >= 9/10; temporal = 5/5;
  expected-empty and cross-account leakage = 0) reviewed and accepted as
  specified in `T23.43.md` and implemented in `evals/hosted/lib/scoring.py`.
- **Model/provider configuration: cannot be frozen — BLOCKED, not a review
  decision.** T23.42's real OpenRouter-compatible provider adapter is merged
  at `639a1abd6a38fdb34d528562470f0e85726ab5f9`, but no live-provider
  measurement has been taken and the qualification manifest has not yet been
  provisioned with its credential reference. The manifest's `provider` object
  (`model`, `version_pin`, `dimensions`, `serving_provider`,
  `privacy_review_ref`) still needs a reviewed concrete pin; freezing it now
  would fabricate a measurement-backed configuration that does not exist.
- Lexical-negative criterion: **`exact_ratio_21`** — score all 21 flagged
  cases, require 19 hits (~90.5%), replacing the draft's unapproved
  raw-count 18-over-21 interpretation described above. The 21 flagged case
  IDs are: `para-02a`, `para-02b`, `para-02c`, `para-04a`, `para-04b`,
  `para-04c`, `para-05a`, `para-05b`, `para-05c`, `para-06a`, `para-06b`,
  `para-06c`, `para-07a`, `para-07b`, `para-07c`, `para-10a`, `para-10b`,
  `para-10c`, `para-12a`, `para-12b`, `para-12c`.
  **Provisional, not yet the reviewer's own confirmation**: this document's
  named reviewer (David Ndungu) was asked to choose between this and
  `exact_subset_20` (2026-09-21) and did not respond within the session's
  standing wait window; per the fleet's standing policy that an unanswered
  question defaults to the recommended option with the default explicitly
  flagged, `exact_ratio_21` (the recommended option — no case exclusion
  needed, marginally stricter than the contract's literal 90%) is entered
  here as a default, not a personally confirmed choice. **Treat this field
  as open until David Ndungu explicitly confirms or overrides it** via a
  reviewed amendment; do not treat the presence of a value here as proof a
  human reviewed and chose it.
- Live-phase seeding protocol (see "Seeding and the real-run recipe")
  confirmed: yes, David Ndungu, 2026-09-21 — the documented recipe (fresh
  empty-case account, absolute TTL, ledger discipline, rate-limit pacing)
  is the protocol to follow; no change requested.
- Decision and rationale: corpus, expected IDs, corpus/facts hashes and
  numeric thresholds accepted as committed — they match what T23.41's
  acceptance and T23.43's merged PR (#237) already establish, with no
  proposed change. Model/provider configuration deferred, not frozen,
  because the adapter has landed but no live measurement-backed provider pin
  exists yet. Lexical-negative criterion entered as the recommended default
  (`exact_ratio_21`) after the
  named reviewer did not respond to an explicit choice within the standing
  wait window, per fleet policy — provisional, not confirmed; treat as open
  until David Ndungu explicitly confirms or overrides via a reviewed
  amendment. **No `--seed` or `--live` run starts until a human reviewer has
  actually confirmed this field, not merely until it holds a value.**

### B. Supplemental forgotten/expired plan

- Status: **ACCEPTED**.
- Reviewer / date (UTC): David Ndungu, 2026-09-21.
- Reviewed T23.41 revision / reviewed T23.43 revision:
  `c61ab91bb48099dfbfcfec2ee86ea2cd590439c4` /
  `b1049482a1f1ffbd42f960a2b68341bb51691116`.
- Exact `evals/hosted/lib/forgotten_targets.py` hash accepted:
  `4a1b4822937f57c947ab2ef4c8d44462e3c7879d2280b32de5a07198827aa193`. That
  hash covers the five synthetic target texts, which removal mode each uses
  (`empty-01`, `empty-02`, `empty-05` forgotten; `empty-03`, `empty-04`
  expired by TTL), the expiry timing (TTL 60 s, margin 5 s, tail 30 s), and
  the cross-account sentinel.
- Decision and rationale: accepted as committed, no proposed amendment. The
  removal modes, timing constants and cross-account sentinel are unchanged
  from the merged T23.43 PR.

The frozen queries, 95 positive cases and every threshold are unchanged
(pinned by test: corpus hash
`f2593f81a5935a0e763672a057195f57c3b81475f21f78132689903ce56b56b6`). Both
approvals must be complete before any `--seed` or `--live` run.

The executable guard requires two separate manifest objects,
`threshold_freeze` and `supplemental_freeze`, each with `status: "accepted"`,
a named `reviewer`, an ISO-8601 `reviewed_at_utc`, and full `t23_41_sha` /
`t23_43_sha` values. The T23.43 SHA must equal the clean source revision being
run. The threshold object also pins the current `corpus_sha256`, `facts_sha256`,
sets `seeding_protocol_confirmed: true`, and records the reviewer's exact
lexical choice as either
`{"mode":"exact_subset_20","case_ids":[...20 sorted IDs...],"min_hits":18}`
or `{"mode":"exact_ratio_21","case_ids":[...all 21 sorted IDs...],"min_hits":19}`.
The supplemental object pins `forgotten_targets_sha256`. Missing, pending,
malformed or stale fields block preflight, seed and live execution before a
request is sent. Preflight validates both freezes for the selected phase, so it
cannot report a run as ready when the same manifest would block at seed or live.
Offline fixture runs remain available while review is pending; they do not grant
permission to seed or measure a live endpoint. The example qualification
manifest intentionally carries no accepted review receipt.

Per `docs/launch/hosted-completion/interfaces.md`, a failed live result
never authorizes a worker to edit these thresholds; only a reviewed
amendment can.

## Expected-empty cases

`T23.43.md` defines the 5 expected-empty cases as forgotten/expired and
requires each to return no current fact. A query for content that was never
authored proves only that an empty index returns nothing, so the live protocol
makes the removal real, both ways a fact can stop being current. The frozen
empty queries are unchanged. For each one, the seed run:

1. remembers a synthetic target fact (`lib/forgotten_targets.py`; every
   subject invented) into the empty-case account, requiring `status=inserted`
   and `search_state=semantic` (a vector was stored). The two **expire-mode**
   targets (`empty-03`, `empty-04`) are remembered with one fixed absolute
   `ttl`: an RFC 3339 UTC timestamp, computed once per run as the local clock
   rounded up to a whole second plus 60 s. The service refuses a relative TTL
   on a keyed `remember`, so the instant must be absolute. The response must
   report the same `valid_until` back; a service that ignores the TTL, or does
   not report it, BLOCKS the seed, because a fact that may never expire cannot
   be shown expired;
2. proves presence, before the expiry: the account inventory equals the 5
   targets, and each case's own query retrieves its target in the top 5. A
   target that cannot be found makes the later absence meaningless, so this
   BLOCKS the seed. If the expiry passes first, the reason says the TTL is too
   short for the endpoint's latency;
3. waits, for real, until the local clock is 5 s past the expiry instant. The
   wait sleeps in 5 s chunks, so the elapsed cap is observed during it, and it
   counts against `max_elapsed_seconds`;
4. proves the expiry: the two expire-mode targets are absent from the inventory,
   from search results and from the facts arm, while the three forget-mode
   targets are still present. That control shows the index still works and the
   expiry was specific; a forget-mode target that vanished, or an unknown fact
   that appeared, BLOCKS the seed. Nothing was called to remove the expired
   facts: the service's own clock removed them;
5. forgets the three **forget-mode** targets (`empty-01`, `empty-02`,
   `empty-05`) through the ordinary `forget` API and requires `expired: true`;
6. proves the final state: the inventory has zero facts, and every one of the
   five empty queries returns zero `results` and zero `facts`;
7. queries the cross-account sentinel (a fact stored only in the positive
   account) against the empty-case account and requires nothing back.

The account therefore ends with **zero current facts**, and the live run
re-checks this before scoring (a non-empty inventory BLOCKS the run) and scores
every expected-empty query strictly. The receipt (schema version 2) records the
returned ids, the sent and reported `valid_until`, the local clock reading taken
after the wait, and each proof. Receipt verification rejects a forged or
truncated expiry proof, including a probe taken less than a margin after the
expiry, a bool or string where a number belongs, and a receipt from an older
plan.

If the hosted service cannot do any step (no `forget`, a `forget` that reports
success but removes nothing, a search index that keeps a forgotten or expired
fact, a TTL it ignores, a clock more than the margin behind the harness's) the
seed stops BLOCKED. Nothing is reinterpreted to pass.

The expiry is judged on the **service's** clock while the wait runs on the
harness's. A service clock up to the 5 s margin behind the harness's is
tolerated; further behind, the fact is still alive after the wait and the seed
BLOCKS with the same "still visible after its expiry" reason, which cannot tell
a broken TTL from a skewed clock. Either way it is never a pass.

## Harness

`scripts/hosted/eval_embeddings.py` takes one of `--fixtures`, `--preflight`,
`--init-ledger`, `--seed` or `--live`, plus `--manifest PATH` and `--output PATH`.
Code lives in `evals/hosted/lib/`:

- `cosine.py`: cosine similarity, rejects empty/nonfinite/mismatched/zero
  vectors; `test_cosine.py` proves scale invariance.
- `fixture_embedder.py`: two local network-free stubs. `HashBagEmbedder` is a
  weak real embedding used only to prove the pipeline runs;
  `FixedVectorEmbedder` is the required negative control that must fail the
  quality bar.
- `lexical_control.py`: the real lexical-only control. It builds a disposable
  brain with `serenity init`, a small seeding helper
  (`evals/hosted/fixtures/seedbrain`) and `serenity sync`, then queries it with
  the unmodified `serenity search`. No public debug bypass.
- `manifest.py`: validates a manifest per phase (`seed` or `live`). A missing,
  null, non-finite, boolean or non-positive cap, a missing provider pin, an
  endpoint host outside `environment.allowed_hosts`, a credential reference that
  is not an environment-variable name (never echoed), or two targets sharing one
  credential reference blocks the run.
- `mcp_client.py`: stdlib `http.client` MCP-over-HTTP client. See "Transport
  guarantees".
- `budget.py`: one `BudgetGuard` shared by `--seed` and `--live`. See "Budget
  units".
- `ledger.py`: the durable, cumulative budget every request is reserved in before
  it is sent. See "Cumulative budget ledger".
- `seeding.py`: seed plan, empty-account proof, the forgotten/expired protocol
  (forget, TTL expiry and the wait), receipt writing and strict receipt
  verification.
- `forgotten_targets.py`: the synthetic targets, their removal modes, the
  expiry timing and the sentinel: the supplemental plan whose hash the freeze
  review confirms.
- `scoring.py`: Hit@5, category floors, strict expected-empty scoring and the
  lexical-negative check, as pure functions over already-ranked id lists.
- `scripts/hosted/eval_embeddings.py` also verifies, on every load, that
  `corpus.json` and `facts.json` hash to the `meta` hashes they claim (the
  hashes sit in the same file as the data they pin, so reading `meta` alone
  would let an edited case keep a stale hash).

### Transport guarantees

- **Wire contract.** Real `tools/call` results are
  `{"content":[{"type":"text","text":"<json>"}]}`; the client decodes that (or
  `structuredContent`) and refuses anything else. A recall result names a fact
  `source-<sha256>` (the id `remember` returned), never the corpus id, so a live
  run needs the seed receipt's id map to attribute results. Facts are seeded
  without an entity so the slug keeps that shape.
- **Session handshake.** The service's MCP transport requires the session id and
  `MCP-Protocol-Version` on every request after `initialize`, and refuses
  `tools/call` until `notifications/initialized` has been sent. `initialize()`
  sends both and takes the version from the response; the notification is a
  charged request like any other. The earlier client sent neither, and the fake
  server accepted that, so the gap appeared only against the actual service.
  `fake_hosted_mcp.py` now enforces the same rules.
- **Redirects and proxies.** Any 3xx is an error; no header is forwarded
  anywhere. `http.client` reads no proxy environment variable.
- **Wall-clock deadline.** A socket timeout bounds one `recv`, not the
  exchange: a server that sends one byte per 0.05 s never trips it. Every
  request therefore runs under a wall-clock deadline (the smaller of 30 s and
  the time left in `max_elapsed_seconds`, on a monotonic clock) enforced by a
  watchdog that shuts the socket down, which interrupts a blocked read in the
  headers or the body. `test_mcp_client.py::TestWallClockDeadline` reproduces the
  drip and asserts the cut-off. The deadline is absolute, so bytes arriving do
  not restart it: a `Connection: close` response whose first body byte arrives
  at half the budget and whose remainder never does is cut off at the deadline
  (`TestResponseClosureAndStallDeadline`, the coordinator's load-close-body
  case; the earlier urllib transport ran 1.105 s against a 0.6 s budget).
  **Not covered:** DNS resolution (`getaddrinfo` cannot be interrupted), so
  there is no end-to-end time guarantee for a manifest that names a hostname
  whose lookup stalls; name literal IPs or already-resolvable hosts.
- **Closure on every path.** Every response and socket is closed explicitly
  before `MCPClient` returns or raises: success, HTTP error, redirect, non-JSON
  body, JSON-RPC error, oversize body and deadline. On a `Connection: close`
  exchange `http.client` hands the socket to the response and `conn.close()` no
  longer reaches it, and a body read with no length returns short data at EOF
  without closing the response, so closing was left to the garbage collector.
  The test records every socket and response the client opens and asserts each is
  closed; it fails if the explicit close is removed.
- **No echoed upstream text.** Errors carry fixed text, exception class names,
  an HTTP status or a validated integer JSON-RPC code, never a server message or
  a `URLError` reason, because a server can reflect the `Authorization` header
  into any field it controls. Server-controlled strings (`status`,
  `search_state`, `search_degraded`, slugs) are reduced to fixed values, hashes,
  booleans or counts before they reach a reason, receipt or result.
  `TestNoCredentialOrUpstreamTextEverEscapes` reflects a sentinel credential
  through each channel and scans every result and receipt file.
- **Bounded reads.** A response over 4 MiB is refused.

### Budget units

`BudgetGuard` runs inside `MCPClient._post`, on the exact serialized request,
before any counter moves or any socket opens, so a request that cannot fit is
rejected whole.

- `max_calls`: every HTTP request, including each `initialize` and its
  `notifications/initialized`.
- `max_input_tokens`: charged at **one token per serialized request byte**. No
  tokenizer emits more tokens than bytes, so this is an upper bound (the earlier
  chars/4 estimate under-counted non-English text and JSON punctuation, and was
  checked only against bytes already spent). It runs about four times stricter
  than chars/4. Size it from `--preflight`'s `plan.input_token_upper_bound`.
- `max_cost_per_call_usd`: an operator ceiling per request, not an invoice.
  The guard reserves `calls * max_cost_per_call_usd` against
  `approved_max_usd`. A result reports that reservation as
  `cost.operator_ceiling_projection_usd`. `cost.actual_usd` is **null**: no
  billing measurement exists, and a projection reported as spend would be a
  fabricated figure.
- `max_elapsed_seconds`: monotonic deadline for one invocation (a wall clock
  cannot be carried across processes honestly, so it is not part of the ledger).
  `--seed`
  waits out a real TTL, so its cap must exceed the whole run: positive-account
  seeding, the empty-account calls before the wait, the wait itself (60 s TTL +
  5 s margin, plus up to 1 s of rounding) and 30 s for the probes after it. The
  floor `TTL + margin + tail` = 95 s is **necessary, not sufficient**: a cap
  below it is BLOCKED before the first call (in `--preflight --phase seed` too),
  and a wait that no longer fits the remaining cap BLOCKS before it starts.
- The four spend caps bind the whole qualification through the ledger below, not
  each invocation.

### Cumulative budget ledger

The manifest's caps bind the whole qualification: the seed, every retry of it and
every live run, in any process and in any order. A per-invocation counter cannot
enforce that. Three `--live` runs under `max_calls=237` sent 447 requests, and two
seed retries under `max_calls=50` sent 100. Every request is now reserved in a
ledger before its socket opens.

- **Where.** `budget.ledger_path` names an append-only JSON-lines file (a relative
  path resolves against the manifest's directory) with a sibling `<name>.lock`.
  Each record carries the SHA-256 of the record before it. The first record binds
  the ledger to the manifest's `budget.authorization_ref`, its four spend caps
  (`max_calls`, `max_input_tokens`, `approved_max_usd`, `max_cost_per_call_usd`)
  and an identity hash over the corpus, the supplemental plan, the declared
  provider pin, the environment and both endpoints. `max_elapsed_seconds` and the
  credentials are not part of the binding, so a seed retry against fresh accounts
  still draws on the same budget.
- **Reserve, then send.** A reservation takes an inter-process lock, reads and
  validates the whole ledger, checks the cumulative totals, appends its record and
  flushes it to the device before the request goes out. Concurrent processes
  serialize on the lock, so they cannot each spend the remaining cap.
- **No refunds.** A process that dies after reserving leaves an uncertain call
  that stays counted, because the request may have left.
- **Fail closed.** A corrupt, truncated, reordered, empty or unreadable ledger, a
  lock that cannot be taken, and a manifest with a different authorization
  reference, cap or identity all block before any request. Changing a cap or a
  reference never resets a ledger.
- **Created on purpose.** `--init-ledger` is the only step that creates a ledger.
  It runs the seed-phase preflight first, because the caps it binds are the
  authorization's caps from then on, and it refuses a path where a ledger, an
  empty file or a lock file already exists. `--seed` and `--live` never create
  one, and a missing ledger is never re-created. A new authorization takes a new
  `budget.ledger_path`, chosen by whoever holds the EMBEDDINGS authority; nothing
  renews a budget automatically, and no approval step exists beyond that
  authority's `authorization_ref`.
- **Receipts do not establish a budget.** A seed receipt carries the ledger's own
  figures for its invocation. `--live` blocks a receipt with no ledger provenance,
  provenance from another ledger, or counters that differ from the ledger, even
  when the edited receipt is re-hashed into the manifest.
- **Run deadline.** The absolute `max_elapsed_seconds` deadline goes into the lock
  wait, so the 30 s lock timeout cannot outlast a shorter cap, and it is checked
  again after the ledger is read and again after the record is durable, before the
  request is sent. A reservation that lands past the cap stays counted, nothing is
  sent, and the result reports it as `usage.reserved_not_sent`. Only this
  process's monotonic clock and the request's socket deadline are controlled.

**What the ledger is not.** It is an operator-side accident and crash guard. It is
not tamper-proof against its owner, who can delete every file and start over,
rewrite the ledger with fresh hashes or restore an older copy, and it cannot see
spend that did not go through this harness. The hash chain detects an edited
middle record, a removed or reordered record and a torn final line. It cannot
detect the removal of a whole suffix that leaves a previously valid prefix, or an
edit to the final record, which no later record hashes.
`test_ledger.py::TestStatedLimits` pins both gaps. Owner tampering and rollback
are outside this guard, and nothing here claims every possible truncation is
caught.

### Fixture and live-provider evidence

The harness cannot tell a loopback oracle from a provider, so it classifies the
endpoints it was given and never lets a local run stand for the qualification.

- A loopback or private-network endpoint (a literal address, or `localhost`), or
  `environment.kind: local-fixture`, makes the run a **fixture**: `evidence_level`
  is `fixture` and the status cannot exceed `PARTIAL`, however the 95 rankings
  score. The Hit@5 row is `NOT_RUN` when a fixture run meets the thresholds and
  `FAIL` when it misses them.
- `environment.kind` must be `local-fixture`, `disposable`, `staging` or
  `production`; any other value blocks the run, and `local-fixture` may name only
  local endpoints.
- A remote endpoint does not prove the selected provider either: the recall
  response reports no model or dimensions. `provider.declared` records the
  manifest's pin, `provider.observed` lists only the endpoints called and their
  class (`provider_identity_verified` is `false`), and
  `provider.qualification_prerequisites` lists what still has to hold: a remote
  run against the provisioned test accounts, deployment evidence showing which
  provider served it, the EMBEDDINGS authority's approval of the caps, and the
  task41 freeze confirmation.
- Host names are not resolved, so a name pointed at a local address is not
  detected as local.
- The budget-boundary acceptance row is `PASS` only when the run was stopped by a
  cap or an unavailable provider. Any other outcome, a full run or an inventory
  drift block included, leaves it `NOT_RUN`; the unit tests carry that evidence.
- A lexical control that fails or times out is a sanitized `BLOCKED` result and
  exit 2 in `--fixtures` and `--live`; only the exit code or the timeout reaches
  the result, never the binary's output.

### Manifest extensions

The shared `qualification.example.json` does not carry the fields the hosted
phases need; this task does not own that file, so they are documented here.
Beyond the shared template, a T23.43 `--seed`/`--live` manifest needs:

```json
"provider": {
  "model": "...", "version_pin": "...", "dimensions": 1024,
  "serving_provider": "...", "privacy_review_ref": "..."
},
"environment": {
  "kind": "disposable",
  "allowed_hosts": ["<hosted-host>"],
  "production_target_allowed": false
},
"hosted_mcp": {
  "endpoint_url": "https://<hosted-host>/mcp",
  "credential_secret_ref": "NAME_OF_ENV_VAR_FOR_THE_POSITIVE_ACCOUNT",
  "allowed_origins": ["https://<hosted-host>"],
  "seed_receipt_path": "receipts/T23.43-seed-positive.json",
  "corpus_seeded_confirmation": "sha256:<printed by --seed>"
},
"empty_case_hosted_mcp": {
  "endpoint_url": "https://<hosted-host>/mcp",
  "credential_secret_ref": "NAME_OF_A_DIFFERENT_ENV_VAR_FOR_A_SEPARATE_ACCOUNT",
  "allowed_origins": ["https://<hosted-host>"],
  "seed_receipt_path": "receipts/T23.43-seed-empty_case.json",
  "corpus_seeded_confirmation": "sha256:<printed by --seed>"
},
"seeding": { "authorized": true, "authorization_ref": "<separate approval>" },
"budget": {
  "max_cost_per_call_usd": 0.0,
  "ledger_path": "qualification-ledger.jsonl"
}
```

- `budget.ledger_path` is required for `--seed` and `--live`; see "Cumulative
  budget ledger". The template `qualification.example.json` is not edited.
- `environment.kind` is one of `local-fixture`, `disposable`, `staging`,
  `production`.
- `seed_receipt_path` and `corpus_seeded_confirmation` are needed only for
  `--live`. The confirmation must be exactly `sha256:` plus the receipt file's
  hash; a hand-typed id or timestamp is rejected. `seeding` is needed only for
  `--seed`, and `authorized` must be the boolean `true`.
- The hosted service is multi-tenant by bearer token, so the two accounts can
  share one URL; the credential is the account discriminator, and the two
  environment variables must hold different values.
- Every endpoint host must appear in `environment.allowed_hosts`, and its
  origin in the target's `allowed_origins`.

### Scope of the account-isolation evidence

Distinct credentials do not prove distinct accounts, and the `recall`/
`remember` contract exposes no account identifier. The evidence is behavioural:

- the empty-case account is proven empty **after** the positive account is
  fully seeded, so two credentials that resolve to one account fail that proof
  (`preceded_by_seeded_facts` in the receipt, required by verification);
- each live phase re-inventories its account before scoring: the positive
  account must hold exactly the receipt's ids, and the empty-case account must
  hold nothing;
- the sentinel exists only in the positive account and must be unreachable from
  the empty-case account, at seed time and again at live time.

A receipt hash is **integrity, not approval**: it shows the receipt was not
edited after the manifest recorded it. It does not show anyone reviewed it, and
it cannot show the harness produced it. Approval is `seeding.authorization_ref`
plus the task41 reviewer's freeze receipt; the live inventory check exists so
the run does not have to trust the receipt's claim about account state.

## Seeding and the real-run recipe

Everything below is separately authorized work; nothing here runs on its own.
`--preflight` makes no network call. Keep manifests, receipts and results on a
private path: they carry the endpoint URL and account-state details, and only a
sanitized copy of a result belongs in committed evidence.

```sh
# 0. Build the lexical-control binaries once, under the R-build-lease protocol.
export SERENITY_BIN=/abs/path/serenity SEEDBRAIN_BIN=/abs/path/seedbrain
export POSITIVE_ACCOUNT_KEY=...   # names only in the manifest; values stay in the environment
export EMPTY_ACCOUNT_KEY=...      # a different account's credential

# 1. Offline readiness: manifest shape, pins, caps vs the exact plan, clean tree.
python3 scripts/hosted/eval_embeddings.py --preflight --phase seed \
  --manifest "$MANIFEST" --output "$PRIVATE/preflight-seed.json"
#    plan.total_calls, plan.input_token_upper_bound and plan.per_role show what the
#    caps must cover for seed AND live together; a smaller cap is BLOCKED here.
#    plan.expiry names the expire/forget cases, the timing and the supplemental
#    hash; plan.expiry.min_elapsed_seconds is the necessary floor for
#    budget.max_elapsed_seconds (size it well above: it must also cover seeding
#    the positive account first).

# 1b. Begin the cumulative ledger (offline, once per authorization). It binds the
#    caps to budget.authorization_ref, so run step 1 first and size the caps from it.
#    It refuses a path where a ledger, an empty file or a lock file exists.
python3 scripts/hosted/eval_embeddings.py --init-ledger \
  --manifest "$MANIFEST" --output "$PRIVATE/init-ledger.json"

# 2. Seed both accounts (paid; needs seeding.authorized and the ledger). Writes two
#    receipts and prints each confirmation. Refuses non-empty accounts and existing
#    receipts.
python3 scripts/hosted/eval_embeddings.py --seed --manifest "$MANIFEST" \
  --receipt-dir "$PRIVATE/receipts" --output "$PRIVATE/seed-result.json"
#    Copy each receipt's seed_receipts[<role>].confirmation into the live manifest.

# 3. Offline check that the receipts verify against the live manifest.
python3 scripts/hosted/eval_embeddings.py --preflight --phase live \
  --manifest "$LIVE_MANIFEST" --output "$PRIVATE/preflight-live.json"

# 4. Score (paid). Exit 0 = PASS, 1 = tested failure, 2 = BLOCKED, 3 = invalid input.
python3 scripts/hosted/eval_embeddings.py --live --manifest "$LIVE_MANIFEST" \
  --output "$PRIVATE/live-result.json"
```

Keep the ledger and its `.lock` file with the manifest. A seed retry and every
live run draw on the same ledger, so a retry never gets a fresh budget; more
budget means a new authorization and a new `budget.ledger_path`. If the ledger is
corrupt, empty or missing, every step blocks until whoever holds the authority
decides what to do; the harness never repairs or re-creates it.

The hosted gateway allows 120 requests a minute per account (a fixed one-minute
window, answered with HTTP 429 and `Retry-After: 60`) and 600 a minute per client
IP. The positive account's seed and its live scoring each send about 100 to 110
requests at full speed, so leave a minute between phases against one account.
The harness does not pace requests: a 429 stops the run as `BLOCKED` with "live
call failed: HTTP 429".

Before step 2 the empty-case account must be a fresh account. A partial seed
leaves an incomplete receipt and a non-empty account; reset the account
out-of-band before another attempt, because seeding never resumes or overwrites.
A reused account also fails for a second reason: expired and forgotten facts
leave the recall view but keep their `operation_key`, so a repeat `remember`
recovers the old fact (`status=duplicate`) or conflicts on its changed expiry.

Step 2 takes roughly `TTL + margin` seconds longer than a plain seed, about
65 s of that in a sleep, and it is the only step that waits. Expect it to print
nothing while it waits. A `BLOCKED` seed names its reason: an expiry the service
did not report, a fact still visible after its expiry plus margin, a TTL too
short for the endpoint's latency, an elapsed cap too small for the wait, or a
forget-mode target that vanished while the expiry was pending. For a service on
a slow network the TTL is a reviewed constant, not a knob; if presence keeps
failing before the expiry, that is a finding for the freeze review, not
something to tune per run.
A live PASS also needs the local lexical control (step 0), which runs first so a
missing binary blocks before any paid call.

`--seed` and `--live` are status `PARTIAL` or `BLOCKED` until the live run
scores; seeding measures no retrieval quality and never reports PASS.

### `--fixtures` output shape

Every `--fixtures` run produces three arms, all clearly non-qualifying, plus a
mechanical status:

1. **harness mechanics** (`HashBagEmbedder`): proves the full pipeline executes
   over all 95 positive cases without error.
2. **quality-gate negative control** (`FixedVectorEmbedder`): must score 0.0;
   the harness raises `AssertionError` if this control ever clears the floor.
3. **real lexical-only control** (`serenity search`, no embedder).

`result.json`'s top-level `status` is `PARTIAL` for every `--fixtures` run; the
Hit@5 acceptance row is always `BLOCKED` (a live measurement) and the
fixed-vector-must-fail row is `PASS` once the assertion above holds. The result
also carries `per_query` rows (rank, hit and top 5 for every case in every arm),
`usage`, `elapsed_seconds` and the manifest's `configuration_sha256`. A dirty
working tree under `evals/hosted` or `scripts/hosted` is recorded as a
limitation in fixtures mode and blocks `--seed`/`--live`, because a recorded
`source_sha` must name the code that ran.

## Verification against the actual service (local)

`evals/hosted/local_service_fixture.py` runs the pre-built `serenity hosted serve`
binary on `127.0.0.1` in development mode behind a small synthetic embedding
provider it starts itself, then runs this harness's own `--init-ledger`, `--seed`
and `--live` against it. It is opt-in and starts a real process, so no default
test run does it:

```sh
T2343_LOCAL_SERVICE=1 SERENITY_BIN=/abs/path/serenity \
  python3 evals/hosted/local_service_fixture.py --output "$PRIVATE/local-service.json"
# the same scenario as a test, from evals/hosted:
T2343_LOCAL_SERVICE=1 SERENITY_BIN=/abs/path/serenity python3 -m unittest test_local_service
```

With the production constants (60 s TTL, 5 s margin) a run takes about three and a
half minutes: the seed's real wait, plus a 61 s pause before each of two later
phases so the gateway's per-account limit (see the recipe) does not stop the
fixture. `--smoke-ttl-seconds` and `--smoke-margin-seconds` shorten the wait for a
quick look; the report is then marked `SMOKE-NOT-QUALIFYING`.

**What is real:** the service binary, its dashboard signup (development mode
prints the login link to the service log), its scoped credential issuance, its MCP
endpoint, its memory store on disk and its own clock. **What is synthetic:** the
embedding provider, a cached-vector oracle that gives each query its answer's
vector plus a hash-bag fallback at 1536 dimensions, and the two throwaway
accounts. `fake_hosted_mcp.py` is a protocol stand-in for unit tests and is never
called the service.

What it checks, each through an ordinary interface:

- The seed receipt, the dashboard's live-memory count and the brain export
  (`facts.json`, current facts only) agree for the positive account, and both the
  dashboard and the export show **zero** current facts for the empty-case account.
- The canonical records on disk (read-only): all five expected-empty targets still
  exist as `memory_fact` records, the two expire-mode targets carry the fixed
  `valid_until` instant and have no expiry record, and exactly the three
  forget-mode targets have `memory_expiry` records. The two TTL targets left the
  current view by lapsing, not by being removed.
- The expiry was crossed on the service's clock: the probe was past `valid_until`
  by more than the measured skew plus the one-second resolution of the `Date`
  header.
- A second `--seed` is refused after the empty-account proof, writes nothing and
  spends four ledger calls; a repeat into the same receipt directory spends none.
- A `--live` run over the real service with the oracle ranking all 95 positive
  queries correctly is still `PARTIAL`, `evidence_level: fixture`, with the Hit@5
  row `NOT_RUN`.
- Isolation: a marker written to one account never reaches the other, the
  sentinel is unreachable from the empty-case account, and each export holds only
  its own account's facts.
- The exact PID the fixture started was stopped and confirmed gone, and its owned
  directory was removed.

**What it proves:** plumbing, the TTL and forget seed protocol, the actual
service clock, account isolation and the ordinary APIs, on this machine. **What it
does not prove:** retrieval quality (the oracle is the answer key), any real
provider's latency, dimensions, model pin, availability or cost, deployment,
billing, mail, or a remote network's latency against the TTL. Provider and
semantic qualification stay BLOCKED, and the lexical-negative criterion is
unchanged.

The fixture refuses an existing path, a non-loopback bind, origin or embedding
base URL, and any provider but its own. The child gets a minimal environment (no
proxy variable, no credential, its own `HOME` and `TMPDIR`). Every synthetic
credential is registered for redaction, and a report that still contains one is
refused. It signals only the PID it spawned, after `ps` shows the command line
still carries its marker. The sanitized report is
`docs/launch/evidence/T23.43/local-service-verification.json`; the tests that
need no binary are in `evals/hosted/test_local_service.py`.

## Known limits

- **No live-provider measurement has ever been taken.** The qualification
  manifest has no provisioned credential reference, no hosted account is
  seeded, and no live request has been made. Every number here is a
  fixtures-only mechanical rehearsal, a loopback test against
  `evals/hosted/fake_hosted_mcp.py` (a protocol stand-in
  that embeds nothing), or the local run against the actual service described
  above, behind a synthetic provider. Provider and semantic qualification stay
  BLOCKED.
- **Both removal paths are exercised against the actual service, locally.** The
  fake evaluates `now >= valid_until` at query time, and the local run shows the
  actual service does the same on its own clock with the frozen 60 s TTL and 5 s
  margin (see "Verification against the actual service (local)"). That run used
  a loopback provider and a throwaway account on this machine. It says nothing
  about a remote deployment: network latency against the TTL, a real provider's
  indexing delay and a service clock on another host are still untested.
- **Expiry is judged on the service's clock.** A service clock more than the 5 s
  margin behind the harness's blocks the seed as "still visible after its
  expiry"; the harness cannot tell that from a service that ignores the TTL.
- **A live PASS depends on real retrieval of the seeded targets.** If the
  provider cannot retrieve a target before it is forgotten, the seed BLOCKS.
  That is deliberate: absence of a fact the index never held proves nothing.
- **The supplemental plan (targets, removal modes, expiry timing, sentinel) is a
  proposed addition** outside `corpus.json`, pending the task41 reviewer's
  confirmation of hash `4a1b4822937f57c947ab2ef4c8d44462e3c7879d2280b32de5a07198827aa193`.
- **The production lexical fallback is an implicit FTS5 AND across every query
  term, including stopwords** (`internal/index.LiteralFTSQuery`). A natural
  question such as "How does Amara get to the office?" requires every word,
  including "how", "does" and "to", in one short fact, which essentially never
  happens; the local lexical control therefore measures about 0/95. That
  satisfies the lexical-negative criterion's intent, but the discriminating
  signal is the semantic arm's hits, not a lexical hit/miss contrast. Whoever
  next tunes `internal/search`'s lexical fallback (outside this task's file
  scope) should consider stopword-stripped or OR-of-content-words queries.
- **The local lexical control still uses one unrelated filler fact per empty
  case**, because `seedbrain` refuses an empty brain. Its empty cases are scored
  strictly too (returning the filler fails), and it is a control, not the
  qualification.
- **`cost.actual_usd` is null.** There is no verified pricing source for the
  hosted recall/embedding pipeline (T23.42's concern) and nothing reads an
  invoice. The worst-case reservation is `cost.operator_ceiling_projection_usd`.
- **Provider fields are the manifest's declared pin.** The hosted recall
  response does not report a model or dimensions.
- **The harness does not pace requests.** The gateway's 120-requests-a-minute
  account limit answers a burst with HTTP 429, which the harness reports as
  `BLOCKED` ("live call failed: HTTP 429") and does not retry. Leave a minute
  between phases against one account.
- **DNS resolution is not covered by the wall-clock deadline.** No claim is made
  of a total end-to-end time guarantee against a hostname.
- **The lexical-negative criterion is an unapproved raw-count interpretation.**
  The draft passes at 18 hits over the 21 flagged paraphrases (85.7%), which is
  weaker than the contract's 18 of 20 (90%). It is not proportional. The named
  task41 reviewer must choose an exact subset of 20 flagged cases or an exact
  ratio before acceptance; no scoring, threshold or corpus change is made here.

## Running it

```sh
# Unit and loopback tests (no binaries needed; the lexical-control test skips
# with a stated reason if SERENITY_BIN/SEEDBRAIN_BIN are unset):
python3 -m unittest discover -s evals/hosted -p "test_*.py"

# Corpus drift check:
python3 evals/hosted/corpus_gen.py --check

# The cumulative-ledger tests (real second processes, a real lock, independently
# counted loopback requests) run in the same command; test_ledger.py is theirs.

# The actual hosted service on loopback, opt-in (see "Verification against the
# actual service (local)"):
T2343_LOCAL_SERVICE=1 SERENITY_BIN=/path/to/built/serenity \
  python3 evals/hosted/local_service_fixture.py --output /private/path/local-service.json

# Fixtures run including the real lexical-only control (build once under the
# R-build-lease protocol; the harness and its tests never build automatically):
export SERENITY_BIN=/path/to/built/serenity
export SEEDBRAIN_BIN=/path/to/built/seedbrain   # go build ./evals/hosted/fixtures/seedbrain
python3 scripts/hosted/eval_embeddings.py --fixtures \
  --manifest /absolute/private/path/to/fixtures-manifest.json \
  --output docs/launch/evidence/T23.43/eval-fixture.json
```
