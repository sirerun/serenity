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
| empty | 5 | isolated single-fact disposable brains; query targets content that was never authored |

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
- All 5 expected-empty cases return no current fact; cross-account/
  forgotten leakage = 0 (operationalized as: an isolated case's result
  set contains nothing outside its own isolated brain's legitimate
  content).
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
- Live-phase seeding mechanism (see "Known limits") resolved: **not yet
  executed**.

Per `docs/launch/hosted-completion/interfaces.md`, a failed live result
never authorizes a worker to edit these thresholds; only a reviewed
amendment can.

## Harness

`scripts/hosted/eval_embeddings.py --fixtures|--live --manifest PATH
--output PATH`. Implementation lives in `evals/hosted/lib/`:

- `cosine.py` — cosine similarity, rejects empty/nonfinite/mismatched/
  zero vectors; `test_cosine.py` proves scale invariance (scaling either
  operand by 1e-4..1e6 never changes the similarity value or a ranking).
- `fixture_embedder.py` — two local, network-free stubs: `HashBagEmbedder`
  (a real-but-weak bag-of-tokens hash embedding, used only to prove the
  corpus/scoring/reporting pipeline runs end-to-end) and
  `FixedVectorEmbedder` (returns the same vector for every input — the
  required negative control that must fail the quality bar).
- `lexical_control.py` — the real lexical-only control: builds a
  disposable brain via `serenity init` + a small new seeding helper
  (`evals/hosted/fixtures/seedbrain`, using the exact
  `store.NewEntityPage`/`writer.Fence` primitives
  `internal/cli/search_test.go` already proves safe for a synchronous,
  zero-LLM disposable brain) + `serenity sync`, then queries it with the
  real, unmodified `serenity search` command. No public debug bypass; no
  modification to any shared/production path.
- `manifest.py` — validates a `--live` manifest; a missing/null/non-finite/
  boolean/non-positive budget field, credential, endpoint, `allowed_origins`
  entry, corpus hash, or seeding confirmation blocks the run rather than
  defaulting to unlimited access. `test_manifest.py` covers the
  bool-is-an-int-subclass and NaN/Infinity footguns explicitly (Python
  silently accepts `True` as a valid positive number and `nan <= 0` is
  `False`, both of which would otherwise slip an unbounded cap past
  validation).
- `mcp_client.py` — a stdlib-only MCP-over-HTTP JSON-RPC client
  implementing exactly the wire protocol `internal/server/mcp` serves
  (`initialize` bootstrap, `Mcp-Session-Id` header, `tools/call`). Refuses
  every HTTP redirect outright (`urllib`'s default opener follows 3xx
  responses and forwards the `Authorization` header to whatever origin the
  redirect names, even cross-origin — confirmed empirically against two
  real loopback servers in `test_mcp_client.py`, including that the
  redirect target's handler is never invoked at all); refuses any
  `endpoint_url` whose scheme/host/port is not exactly one of the
  manifest's `allowed_origins`, plus any URL carrying userinfo or a query
  string; treats an MCP tool result's `isError: true` as a hard failure
  rather than an empty/successful result; and never auto-initializes a
  session inside `call_tool`, so a caller enforcing a per-request budget
  can always charge `initialize()` against that budget before it happens.
- `scoring.py` — Hit@5, category floors, empty-case leakage, and the
  lexical-negative check, as pure functions over already-ranked id lists.

### Manifest extensions

The shared `docs/launch/hosted-completion/qualification.example.json`
template does not carry the fields T23.43's `--live` mode needs to reach
the hosted service itself (as opposed to the raw embedding provider);
this task does not own that shared file, so the extension is documented
here instead of added there. A T23.43 live manifest additionally needs:

```json
"hosted_mcp": {
  "endpoint_url": "https://<hosted-host>/mcp",
  "credential_secret_ref": "NAME_OF_ENV_VAR_HOLDING_A_CLIENT_CREDENTIAL",
  "corpus_seeded_confirmation": "<receipt id or timestamp>",
  "allowed_origins": ["https://<hosted-host>"]
},
"empty_case_hosted_mcp": {
  "endpoint_url": "https://<a-genuinely-separate-hosted-host-or-account>/mcp",
  "credential_secret_ref": "NAME_OF_ENV_VAR_HOLDING_A_SEPARATE_CLIENT_CREDENTIAL",
  "corpus_seeded_confirmation": "<receipt id or timestamp, seeded with only the isolated filler facts>",
  "allowed_origins": ["https://<a-genuinely-separate-hosted-host-or-account>"]
},
"budget": {
  "max_cost_per_call_usd": 0.0
}
```

`corpus_seeded_confirmation` exists because T23.43 does not own the
hosted write path (identity/provisioning is T23.46, the accounting/
storage layer T23.44, remember/forget T23.42-adjacent) — it never seeds a
live account itself. The coordinator/operator seeds the frozen corpus
into the target account/brain out-of-band (mechanism to be decided by
whoever owns that write path once T23.42's real embedder is merged) and
records an explicit confirmation before invoking `--live`; its absence
blocks the run.

`allowed_origins` pins the exact scheme+host(+port) `mcp_client.py` will
ever send a credential to; `endpoint_url` must resolve to one of its
entries or the manifest is rejected before any network call.

`empty_case_hosted_mcp` is a second, independently required target with
its own credential and `allowed_origins`, distinct from `hosted_mcp`.
T23.43.md's own acceptance criterion requires cross-account/forgotten
leakage to be checked, which one shared credential cannot test honestly —
a near-miss embedding hit against one of the 95 positive facts sitting in
the *same* account as the 5 empty-case queries would be indistinguishable
from genuine leakage. Its absence blocks the manifest closed
(`manifest.py::test_missing_empty_case_target_blocks`).

`budget.max_cost_per_call_usd` is a conservative operator-supplied ceiling
on the hosted MCP call's real per-call cost. This harness has no verified
pricing source for the hosted recall/embedding pipeline (that is T23.42's
concern, not this task's), so rather than assume the spend is unbounded
when a $/call figure is unknown, the manifest requires an explicit,
finite, positive figure and the harness enforces
`approved_max_usd >= (calls_made + 1) * max_cost_per_call_usd` before
every real HTTP call, including `initialize()` — if that projection would
exceed `approved_max_usd`, the run blocks before the network call happens,
never after.

**Posture on `--live` today: fail closed, PARTIAL, pending implementable
integration input.** No hosted MCP endpoint, host, or account currently
exists to put in either `hosted_mcp` or `empty_case_hosted_mcp` — this is
not a gap in `manifest.py`'s validation, it is that T23.46 (hosted
identity/provisioning) and a seeded qualification account have not been
built yet. `--live` is fully implemented and covered by real-wire tests
(`test_mcp_client.py`'s loopback-server redirect/isError tests,
`test_eval_embeddings.py::TestLiveModeBudgetEnforcement`'s budget-exhaustion/
max_calls=1/token-cap/dollar-cap/separate-credential tests), but has never
been run against a real hosted target and this document does not claim
otherwise. A worker or reviewer must not fabricate a manifest against a
placeholder/test endpoint and report a live PASS from it.

### `--fixtures` output shape

Every `--fixtures` run produces three arms, all clearly non-qualifying,
plus a mechanical status:

1. **harness mechanics** (`HashBagEmbedder`): proves the full pipeline
   executes over all 95 positive cases without error. Observed overall
   Hit@5 in this repo's own dry run: ~0.58 — real but weak, expected,
   never a quality claim.
2. **quality-gate negative control** (`FixedVectorEmbedder`): must score
   0.0 (degenerate ranking); the harness raises `AssertionError` if this
   control ever clears the floor.
3. **real lexical-only control** (`serenity search`, no embedder): see
   "Known limits" below for a load-bearing finding about this arm.

`result.json`'s top-level `status` is `PARTIAL` for every `--fixtures`
run; the `acceptance[]` row for the Hit@5 criterion is always `BLOCKED`
(a live measurement, not a fixture one) and the fixed-vector-must-fail
row is `PASS` once the assertion above holds.

## Known limits

- **The production lexical fallback is an implicit FTS5 AND across every
  query term, including stopwords** (`internal/index.LiteralFTSQuery`
  quotes each token and joins with spaces, and FTS5's default syntax ANDs
  space-separated terms). A natural-language question like "How does
  Amara get to the office?" requires "how", "does", "get", "to", and
  "office" to ALL appear in the same short fact statement — which
  essentially never happens. Verified directly against the real built
  `serenity` binary: `serenity search "golden retriever"` (a bare content
  phrase) finds its target; `serenity search "How does Amara get to the
  office?"` (a natural question) returns "no results" even though
  "office" is a literal word in the target fact. Practical effect: this
  corpus's local lexical-only control measures ~0/95 Hit@5 regardless of
  a paraphrase's designed overlap category — which still correctly
  satisfies the lexical-negative acceptance criterion's intent (semantic
  buys real value lexical cannot), but the discriminating signal comes
  entirely from whether the *semantic* arm hits, not from a lexical
  hit/miss contrast between "easy" and "hard" paraphrases. Flagging this
  for whoever next tunes `internal/search`'s lexical fallback (T23.44/45
  territory, not this task's file scope): an OR-of-content-words or
  stopword-stripped query would degrade far more gracefully for
  real end-user natural-language questions.
- **Empty-case fixture arm approximates "forgotten/expired" as "never
  authored."** T23.43 does not own the remember/forget/expiry pipeline
  (T23.42/44/48); a real "forgotten fact must not resurface" proof needs
  those tasks' write paths and can only be exercised live, against a real
  account, once they and the EMBEDDINGS gate are available.
- **No live semantic measurement has ever been taken.** `EMBEDDINGS_API_KEY`
  is not provisioned, T23.42's real provider adapter is not yet merged,
  and no hosted test account has been seeded. Every number in this
  document's "Harness" section is a fixtures-only mechanical rehearsal,
  never mistaken in `result.json` for a quality PASS.
- **`--live` mode's per-call cost is an operator-supplied ceiling, not a
  provider-reported figure.** No verified real pricing source exists for
  the hosted recall/embedding pipeline (T23.42's concern), so
  `cost.actual_usd` is computed as `calls_made * budget.max_cost_per_call_usd`
  — a conservative bound the manifest's authorizer commits to in advance,
  not a true invoice reconciliation. The harness enforces
  `max_calls`/`max_input_tokens` (char/4 estimate)/`max_elapsed_seconds`/
  the projected-cost-vs-`approved_max_usd` check directly, all of which
  stop the run before the next call rather than after.

## Running it

```sh
# Composition/hash/manifest/cosine/scoring unit tests (no binaries needed):
python3 -m unittest discover -s evals/hosted -p "test_*.py"

# Full run including the real lexical-only control (build once under the
# R-build-lease protocol; never built automatically by the harness or its tests):
export SERENITY_BIN=/path/to/built/serenity
export SEEDBRAIN_BIN=/path/to/built/seedbrain   # go build ./evals/hosted/fixtures/seedbrain
python3 scripts/hosted/eval_embeddings.py --fixtures \
  --manifest /absolute/private/path/to/fixtures-manifest.json \
  --output docs/launch/evidence/T23.43/eval-fixture.json
```
