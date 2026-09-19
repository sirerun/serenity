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
| empty | 5 | expected-empty forgotten/expired cases; frozen queries unchanged. Live: each is backed by a synthetic target that is remembered, then forgotten (see "Expected-empty cases"). Local lexical control: isolated single-fact disposable brains |

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
- At least 18 of the corpus's zero-content-word-overlap paraphrase cases
  (21 in this revision, so the check is proportional — "at least 18 of
  however many are flagged," not a literal 18-of-a-fixed-20 subset) must
  hit semantically (expected fact in top 5) **and** miss in the local
  lexical control (expected fact absent from the lexical-only top 5).
  This proportional reading is this document's own interpretation,
  offered for task41's reviewer to confirm or adjust — it is not itself
  the freeze.
- A fixed-vector or lexical-only stub must fail the quality predicate;
  fixture mode may pass harness mechanics but must never report quality
  PASS. `scripts/hosted/eval_embeddings.py --fixtures` raises immediately
  if its own fixed-vector negative control ever clears the floor — that
  would mean the scoring pipeline is broken, not that a stub is good.
- Budget exhaustion or provider unavailability yields BLOCKED with
  partial results and no further calls, never a silent PASS/skip.

## Freeze receipt (task41 reviewer fills this in)

- Reviewer / date: **not yet executed**.
- Corpus hash reviewed and accepted: **not yet executed** (current value:
  see `evals/hosted/corpus.json`'s `meta.corpus_hash_sha256`).
- Threshold interpretation (the proportional 18-of-21 reading above)
  confirmed or amended: **not yet executed**.
- Live-phase seeding protocol (see "Seeding and the real-run recipe")
  confirmed: **not yet executed**.
- Forgotten-target and sentinel texts (`evals/hosted/lib/forgotten_targets.py`,
  sha256 `7f5e08628f903686403eb8a48c7841a4ff9bfa3cb3cd2d36d54de0f196251714`)
  confirmed as a proposed addition outside `corpus.json`: **not yet
  executed**. The frozen queries, the 95 positive cases and every
  threshold are unchanged.

Per `docs/launch/hosted-completion/interfaces.md`, a failed live result
never authorizes a worker to edit these thresholds; only a reviewed
amendment can.

## Expected-empty cases

`T23.43.md` defines the 5 expected-empty cases as forgotten/expired and
requires each to return no current fact. A query for content that was never
authored proves only that an empty index returns nothing, so the live protocol
makes the forgetting real. The frozen empty queries are unchanged. For each
one, the seed run:

1. remembers a synthetic target fact (`lib/forgotten_targets.py`; every
   subject invented) into the empty-case account, requiring `status=inserted`
   and `search_state=semantic` (a vector was stored);
2. proves presence: the account inventory equals the 5 targets, and the case's
   own query retrieves its target in the top 5. A target that cannot be found
   makes the later absence meaningless, so this BLOCKS the seed;
3. forgets each target through the ordinary `forget` API and requires
   `expired: true`;
4. proves absence: the inventory has zero facts, and every empty query returns
   zero `results` and zero `facts`;
5. queries the cross-account sentinel (a fact stored only in the positive
   account) against the empty-case account and requires nothing back.

The account therefore ends with **zero current facts**, and the live run
re-checks this before scoring (a non-empty inventory BLOCKS the run) and scores
every expected-empty query strictly. If the hosted service cannot do any step
(no `forget`, a `forget` that reports success but removes nothing, a search
index that keeps a forgotten fact) the seed stops BLOCKED. Nothing is
reinterpreted to pass. Only `forget` is exercised; TTL expiry is not.

## Harness

`scripts/hosted/eval_embeddings.py` takes one of `--fixtures`, `--preflight`,
`--seed` or `--live`, plus `--manifest PATH` and `--output PATH`. Code lives in
`evals/hosted/lib/`:

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
- `seeding.py`: seed plan, empty-account proof, forgotten-fact protocol,
  receipt writing and strict receipt verification.
- `forgotten_targets.py`: the synthetic targets and the sentinel.
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
- **Redirects and proxies.** Any 3xx is an error; no header is forwarded
  anywhere. `http.client` reads no proxy environment variable.
- **Wall-clock deadline.** A socket timeout bounds one `recv`, not the
  exchange: a server that sends one byte per 0.05 s never trips it. Every
  request therefore runs under a wall-clock deadline (the smaller of 30 s and
  the time left in `max_elapsed_seconds`, on a monotonic clock) enforced by a
  watchdog that shuts the socket down, which interrupts a blocked read in the
  headers or the body. `test_mcp_client.py::TestWallClockDeadline` reproduces the
  drip and asserts the cut-off. **Not covered:** DNS resolution
  (`getaddrinfo` cannot be interrupted); name literal IPs or already-resolvable
  hosts in the manifest.
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

- `max_calls`: every HTTP request, including each `initialize`.
- `max_input_tokens`: charged at **one token per serialized request byte**. No
  tokenizer emits more tokens than bytes, so this is an upper bound (the earlier
  chars/4 estimate under-counted non-English text and JSON punctuation, and was
  checked only against bytes already spent). It runs about four times stricter
  than chars/4. Size it from `--preflight`'s `plan.input_token_upper_bound`.
- `max_cost_per_call_usd`: an operator ceiling per request, not an invoice.
  `cost.actual_usd` is `calls * max_cost_per_call_usd`.
- `max_elapsed_seconds`: monotonic deadline for one invocation.
- Spend recorded in the seed receipts counts against the live run's calls and
  tokens, so seed then live cannot together exceed a cap each would satisfy.

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
"budget": { "max_cost_per_call_usd": 0.0 }
```

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

# 2. Seed both accounts (paid; needs seeding.authorized). Writes two receipts and
#    prints each confirmation. Refuses non-empty accounts and existing receipts.
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

Before step 2 the empty-case account must be a fresh account. A partial seed
leaves an incomplete receipt and a non-empty account; reset the account
out-of-band before another attempt, because seeding never resumes or overwrites.
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

## Known limits

- **No live measurement has ever been taken.** `EMBEDDINGS_API_KEY` is not
  provisioned, T23.42's real provider adapter is not merged, and no hosted
  account is seeded. Every number here is a fixtures-only mechanical rehearsal
  or a loopback test against `evals/hosted/fake_hosted_mcp.py`, which mirrors
  the real wire shapes but embeds nothing. `--seed` and `--live` are
  implemented and tested against it; this document claims no more.
- **Only `forget` is exercised for the expected-empty cases.** TTL expiry (a
  fact that expires by time) is not tested; the corpus category is
  "forgotten/expired" and this covers the forgotten half.
- **A live PASS depends on real retrieval of the seeded targets.** If the
  provider cannot retrieve a target before it is forgotten, the seed BLOCKS.
  That is deliberate: absence of a fact the index never held proves nothing.
- **The forgotten targets and sentinel are a proposed addition** outside
  `corpus.json`, pending the task41 reviewer's confirmation.
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
- **`cost.actual_usd` is an operator ceiling, not an invoice.** There is no
  verified pricing source for the hosted recall/embedding pipeline (T23.42's
  concern).
- **Provider fields are the manifest's declared pin.** The hosted recall
  response does not report a model or dimensions.
- **DNS resolution is not covered by the wall-clock deadline.**
- **The lexical-negative reading is this document's own.** "At least 18 of the
  flagged paraphrases" (21 in this revision) is proportional; task41's reviewer
  confirms or amends it.

## Running it

```sh
# Unit and loopback tests (no binaries needed; the lexical-control test skips
# with a stated reason if SERENITY_BIN/SEEDBRAIN_BIN are unset):
python3 -m unittest discover -s evals/hosted -p "test_*.py"

# Corpus drift check:
python3 evals/hosted/corpus_gen.py --check

# Fixtures run including the real lexical-only control (build once under the
# R-build-lease protocol; the harness and its tests never build automatically):
export SERENITY_BIN=/path/to/built/serenity
export SEEDBRAIN_BIN=/path/to/built/seedbrain   # go build ./evals/hosted/fixtures/seedbrain
python3 scripts/hosted/eval_embeddings.py --fixtures \
  --manifest /absolute/private/path/to/fixtures-manifest.json \
  --output docs/launch/evidence/T23.43/eval-fixture.json
```
