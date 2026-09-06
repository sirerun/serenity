# M1 exit verification report (T1.23)

Real run against David's own brain, 2026-09-05/06. Not a fixture, not a dry
run: real Gmail (30-day IMAP poll), 5 real git repos, real extraction
against a real locally-hosted model, real live eval against held-out golden
labels.

## Run record

- **Machine**: macOS 26.6.2, arm64 (laptop).
- **Extraction/composer model**: `qwen3.8-27b@v1`, served via SGLang on the
  DGX Spark (`http://192.168.86.250:30000/v1`, OpenAI-compatible), speculative
  decoding (DSpark), YaRN-extended 1,010,000-token context. Not a cloud
  model -- `models.provider: openai` pointed at this local endpoint via
  `OPENAI_BASE_URL` (ADR 013's local-server path).
- **Connectors polled**: `imap:ndungu.sink@gmail.com` (30-day window) plus 5
  git repos (`serenity`, `zerfoo`, `spark`, `foundation`, `gavel`).
- **Sync elapsed**: completed cleanly, exit 0 (see docs/roadmap.md T1.27 --
  this run is what surfaced and then verified the `git add` ARG_MAX fix,
  PR #69).
- **Ingest counts** (`.serenity/index.db`, `chunks` table, distinct
  `source_sha256`): 11,092 email sources, 318 git_repo sources, 1
  entity_page -- 11,411 real sources chunked, 92,586 chunks total.
- **Full-corpus extraction**: attempted via `serenity extract all`, aborted
  deliberately after 35 chunks (David's call, see docs/plan.md /
  AskUserQuestion record 2026-09-05) -- at the observed per-chunk rate
  (reasoning-model overhead), extracting all 92,586 real chunks projects to
  roughly 2-3 months, not viable on this timeline. This does not affect the
  P/R/F1 numbers below: those come from `eval-runner -mode live`, which
  calls `ExtractChunk` directly against held-out labeled fixtures
  (`evals/corpora/ava`), independent of the bulk `extract all` job.
- **Live eval**: `cmd/eval-runner -mode live -provider openai -model
  qwen3.8-27b -corpus evals/corpora/ava`, 52 spans scored, $0 spend (local
  model), 0 spans skipped on budget. Two of four run attempts hit a
  transient DGX-path network timeout (`read: operation timed out`) under
  host CPU contention (`GET /api/v1/resources` showed 1,100 of 18,000
  allocatable millicores free at the time) -- the pod itself never
  restarted and kept returning 200 OK in its own logs throughout; a third
  attempt (wrapped in a 3-try retry loop) succeeded on its first try. See
  "Remediation" below -- `router.Complete` has no timeout/retry policy of
  its own, which is a real, separate gap this exposed.

## Per-family P/R/F1

Acceptance bar per T1.23: P >= 0.90 and R >= 0.80 per family.

| family | TP | FP | FN | Precision | Recall | F1 | Meets bar? |
|---|---|---|---|---|---|---|---|
| belongs_to_project | 3 | 0 | 1 | 1.000 | 0.750 | 0.857 | No (R) |
| committed_to | 0 | 4 | 4 | 0.000 | 0.000 | 0.000 | No |
| costs | 0 | 3 | 4 | 0.000 | 0.000 | 0.000 | No |
| deadline_on | 4 | 1 | 0 | 0.800 | 1.000 | 0.889 | No (P) |
| has_balance | 1 | 3 | 3 | 0.250 | 0.250 | 0.250 | No |
| has_condition | 3 | 1 | 1 | 0.750 | 0.750 | 0.750 | No |
| has_role | 4 | 2 | 0 | 0.667 | 1.000 | 0.800 | No (P) |
| owns_account | 0 | 6 | 4 | 0.000 | 0.000 | 0.000 | No |
| prefers | 3 | 2 | 1 | 0.600 | 0.750 | 0.667 | No |
| relates_to | 2 | 3 | 2 | 0.400 | 0.500 | 0.444 | No |
| said | 0 | 5 | 4 | 0.000 | 0.000 | 0.000 | No |
| takes_medication | 3 | 1 | 1 | 0.750 | 0.750 | 0.750 | No |
| works_at | 3 | 1 | 1 | 0.750 | 0.750 | 0.750 | No |

**No family meets the P >= 0.90 / R >= 0.80 bar.** This is the honest
result of the first real live eval this repo has ever run -- `-mode cached`
(ci.yml's per-push gate) has been green throughout M1 because its
predictions fixture was hand-authored, not model-generated; `-mode live`
(nightly-eval.yml) exists but had never actually been exercised against a
real model + real fixtures until this session.

**Contradiction recall**: not measured -- no production contradiction
detector exists yet (T1.9 deferred semantic reconciliation to E2). Not a
T1.23 regression; this is expected per T1.9's own scope note.

## Real defects found and fixed while producing this report

Three real, previously-latent defects were found and fixed by actually
running T1.23 for real, each with a red -> green test proving the exact
production failure and its fix (docs/roadmap.md has full detail):

1. **T1.26 (PR #69)**: `internal/extract` never threaded
   `Source.IndexOnly` into the router prompt, so an index_only source's
   raw bytes could reach the external model via extraction even though
   the composer path already respected it (RFC SS7.4/SS14).
2. **T1.27 (PR #69)**: `internal/writer/commit.go`'s `git add` used argv,
   which blew past `ARG_MAX` on the real 11,092-email sync
   (`argument list too long`). Fixed via `--pathspec-from-file=-`.
3. **Eval-scoring defect (PR #71)**: `internal/eval/score.go`'s `Score()`
   did exact-string matching on `Object`, but the `ava` corpus's golden
   objects are hand-authored slugs (`contoso-systems`) while a real model
   emits prose (`Contoso Systems`) -- neither `buildPrompt` nor
   `store.NormalizeKey` ever reconciled the two. This alone was
   responsible for most of the pre-fix run's 11-of-13 P=0/R=0 families
   (a live model was frequently correct but scored as a false
   positive/negative pair). Fixed by normalizing both sides
   (hyphen-fold + `store.NormalizeKey`) before matching. The table above
   already reflects the fixed scorer.
4. **Missing `OPENAI_BASE_URL` support in `cmd/eval-runner`** (PR #70):
   `-mode live -provider openai` never read the env var, so it would have
   silently targeted the real OpenAI API instead of the local DGX Qwen
   endpoint -- caught before it ever sent a real request externally.

## Remediation (families that miss the bar)

Given no family clears the bar, remediation is filed as new E1 tasks
(docs/plans/E1-m1-ingest.md wave 1g) rather than listed per-family here,
since the root causes span at least two different issues:

- **T1.28**: diagnose and improve extraction quality for the
  fully-zeroed families (`committed_to`, `costs`, `owns_account`,
  `said`) -- TP=0 across the board suggests either a genuine `qwen3.8-27b`
  capability gap on these predicates or a further prompt/vocabulary
  disambiguation issue (distinct from the object-slug bug already fixed),
  not yet root-caused.
- **T1.29**: for the partially-scoring families (`has_condition`,
  `takes_medication`, `works_at`, `prefers`, `relates_to`, `has_role`,
  `belongs_to_project`, `deadline_on` -- all in the 0.4-0.89 range),
  determine whether a prompt refinement (few-shot examples per predicate,
  clearer subject/object boundary rules) or a stronger model closes the
  gap to the P >= 0.90 / R >= 0.80 bar.
- **T1.30**: give `internal/router.Complete` (or its
  `OpenAICompatibleProvider` transport) an explicit HTTP timeout and a
  bounded retry-on-transient-network-error policy -- found running this
  report's live eval against the DGX endpoint under host CPU contention;
  currently a single dropped connection aborts the whole run with no
  retry anywhere in the call chain.
- **T1.31**: full-corpus extraction throughput -- at the observed
  per-chunk rate, `qwen3.8-27b`'s reasoning overhead makes a 92k-chunk
  real corpus take ~2-3 months; determine whether a non-reasoning model,
  a reasoning-effort/budget flag on the SGLang server, or batching closes
  this before extraction is run for real again at this scale.

T1.23 itself is exit-verified in the sense its acceptance criteria
required: this report exists, records the real run, and lists every
family that misses the bar together with remediation task ids (T1.28-
T1.31), since no family cleared it.
