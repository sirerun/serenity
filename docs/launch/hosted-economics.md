# Hosted Serenity economics: cost model and load harness (T23.60)

Generated 2026-09-18 at source `810349ba7c1ed2c05fe34e3892de764a26a4633c` by the
T23.60 worker. This document, `scripts/hosted/cost_model.py` and
`scripts/hosted/load.py` are a reviewable cost/capacity estimate, not a
spend authorization, a measured capacity ceiling, or a plan-limit change.
The historical USD 60/month figure remains a ceiling, never a target.
Full machine-readable output: `docs/launch/evidence/T23.60/cost.json` and
`docs/launch/evidence/T23.60/load-fixture.json`.

## Status and what this evidence does and does not prove

- **Cost rates are unverified this session.** `RATE_TABLE` in
  `cost_model.py` is this worker's training-data recollection of long-stable
  AWS list prices (EC2, EBS, S3, KMS, Secrets Manager, CloudWatch, Elastic
  IP, data transfer). WebSearch/WebFetch tool permission was not granted in
  this headless run, so no rate below carries a live, dated citation. Every
  entry is tagged `"kind": "assumption_recollection"`. **A reviewer must
  re-verify each rate against the current AWS pricing pages (and Resend's)
  before this cost model is treated as authoritative**, per
  `docs/launch/hosted-completion/evidence.md`'s "distinguish assumptions from
  measurements" rule.
- **The load harness's `--fixtures` mode is a mechanics proof, not a
  capacity claim.** It runs a deterministic, in-process, discrete-event
  simulator (`evals/hosted-load/harness.py`) shaped by the real production
  admission defaults in `internal/hosted/service/service.go`
  (`MaxOpen=8`, `MaxInFlight=16`, 10-minute idle timeout) and
  `internal/hosted/gateway/admission.go` (120 req/min per account). It never
  spawns the real Go binary, opens a real brain, or calls a real embeddings
  provider. Latency numbers are a synthetic cost function calibrated by
  hand, not a measured host metric. CPU/RSS/disk are explicitly reported as
  unmeasured in fixtures mode.
- **`--live` is implemented but has never run.** No deployed hosted
  candidate exists yet at T23.60 (that is T23.64's job), and this task has
  no external gates. `load.py --live` refuses to run without an explicit
  `environment.origin` in the qualification manifest, refuses any host not
  in `environment.allowed_hosts`, and refuses `app.serenity.sire.run` /
  `serenity.sire.run` outright even if allowlisted. T23.68 depends on this
  file and will exercise `--live` and `cost_model.py --measurements` against
  a real deployed candidate; the code paths are unit-tested against an
  injected fake transport (`evals/hosted-load/test_load_cli.py`) but not
  against a real host.
- **Task41's threshold freeze has not happened.** The numeric thresholds in
  `evals/hosted-load/workload.json` are transcribed verbatim from this
  task's contract and `interfaces.md`'s "Threshold review" section as
  *proposed* targets; `workload.json.reviewer` and `.review_date` are `null`.
  Per `interfaces.md`: "The proposed targets are deliberately explicit so
  reviewers can adjust them once with rationale before execution." This
  worker cannot self-approve that freeze.

Given the above, this task's own acceptance status is **PARTIAL**: the cost
math, harness mechanics and every required command run and are unit-tested
(49/49 passing, see `docs/launch/evidence/T23.60/result.json`), but two of
T23.60's three acceptance items depend on inputs only a human reviewer or a
live-fetch-capable session can supply (rate citation, task41 freeze).

## Cost scenarios

Four scenarios per the task contract: 0 accounts (idle), an assumed
light-usage 10- and 100-account mix (documented assumption, not a product
decision), and the ratified full-limit mix (1 Scale + 3 Builder + 10 Free at
100% of `internal/hosted/plans/plans.go` V1 allowances — 90,000 total facts,
matching `docs/launch/hosted-completion/evidence.md`).

| Scenario | Accounts | Known-category subtotal (USD/mo) | Within USD 60 ceiling |
|---|---:|---:|---|
| `idle_0_accounts` | 0 | 22.09 | yes |
| `10_accounts_light` (assumed 8 free/2 builder, 10% usage) | 10 | 26.80 | yes |
| `100_accounts_light` (assumed 90 free/8 builder/2 scale, 10% usage) | 100 | 67.53 | **no** |
| `full_limit_mix` (ratified 1 Scale + 3 Builder + 10 Free, 100% usage) | 14 | 173.14 | **no** |

Every figure above is the sum of **known-priced categories only**. Unknown
categories (embeddings, CloudWatch Logs, CloudWatch custom metrics) are
excluded from these subtotals and reported separately in `cost.json` — they
are never silently zeroed, and adding them can only raise these numbers
further.

Fixed monthly components (all scenarios), each dated in `RATE_TABLE`'s
`note` field:

| Category | USD/mo | Basis |
|---|---:|---|
| EC2 `t4g.small` baseline | 12.264 | `deploy/hosted/stack.json` instance type, 730 hr/mo |
| EBS gp3 (8 GB root + 30 GB data) | 3.04 | `deploy/hosted/stack.json` volume sizes |
| Elastic IP | 3.65 | AWS charges hourly for every public IPv4 address since Feb 2024, attached or not — matches ADR 014's own ~USD 4 estimate |
| KMS key | 1.00 | one customer-managed key, `deploy/hosted/stack.json` |
| Secrets Manager | 1.60 | 4 secrets (Resend, embeddings, Stripe secret, Stripe webhook) |
| CloudWatch alarms | 0.20 | 2 alarms currently in `stack.json` (CPU, instance health) |

**Fixed cost floor is ~USD 21.75/month before any backup storage**, close to
ADR 014's own "under USD 25" provisional estimate — those two independent
estimates agreeing is a useful sanity check on the fixed-cost side. The
variable component (backup storage) is where the two diverge sharply; see
below.

## Critical finding: hourly full-snapshot S3 amplification exceeds the ceiling well before full-limit scale

ADR 014 describes hourly backups: "a `VACUUM INTO` snapshot of the control
DB and one git bundle per brain," retained via S3 versioning with
`ExpirationInDays: 30` and `NoncurrentVersionExpiration.NoncurrentDays: 30`
(`deploy/hosted/stack.json`). This is a **full** snapshot every hour, not an
incremental one — the task contract itself names this risk as "hourly full
snapshot amplification," and `docs/launch/hosted-plan.md`'s gap list
separately flags that "current 30 days plus noncurrent 30 days can exceed
the promised deletion window."

Writing the same object key every hour under S3 versioning does not wait 30
days to accumulate: each new hourly PUT immediately makes the previous
version noncurrent, and `NoncurrentVersionExpiration` deletes a noncurrent
version 30 days *after* it became noncurrent — not 30 days after it was
first current. At steady state this means roughly **721 simultaneously
stored full-snapshot versions** (30 days × 24/day + 1 current), not the "one
copy of the working set" a reader might assume from "30-day retention."

`cost_model.py` models this explicitly (`stored_version_count = 30*24 + 1`)
rather than assuming the lifecycle behaves as the promised single-copy
window:

| Scenario | Snapshot size (customer data + control DB) | S3 storage/mo at 721x | Share of scenario's known subtotal |
|---|---:|---:|---:|
| idle | ~0.02 GB | 0.33 | 1.5% |
| 10 accounts light | ~0.31 GB | 4.97 | 18.6% |
| 100 accounts light | ~2.83 GB | 45.11 | 66.8% |
| full-limit mix | ~9.02 GB | **149.58** | **86.4%** |

**This single line item is why `100_accounts_light` and `full_limit_mix`
both exceed the USD 60 ceiling on known costs alone**, and it dominates
every other category combined once the account base grows past a handful of
light users. This is not a new problem this task introduces — it is the same
gap `docs/launch/hosted-plan.md` and `docs/tasks/hosted-completion/T23.49.md`
(backup integrity, owner of `R-hosted-backup`) already name — but T23.60 is
the first evidence quantifying it in dollars, and the number is large enough
that it should inform T23.49's redesign priority and T23.41's storage-envelope
review, not just its correctness fix.

**Not authorized here:** changing snapshot frequency, retention window, or
backup format is an architecture change per `docs/launch/hosted-plan.md`
("A different hosting topology is a costed proposal reviewed through the
existing architecture path, never an individual worker choice"). For
illustration only, `cost_model.py`'s own math implies that moving from
hourly to daily full snapshots (721x → 31x) would cut the full-limit-mix S3
line from ~150 to ~6 USD/month — a **proposal for T23.49/task41 review**, not
a change made by this task.

## Unknown-priced categories (reported, never zeroed)

| Category | Why unknown | Owner/trigger |
|---|---|---|
| Embeddings | `perplexity/pplx-embed-v1-0.6b` via OpenRouter has no qualified price/terms yet | T23.42 pins provider/model/price |
| CloudWatch Logs ingest/storage | No log shipping is wired; ops go through SSM Run Command per ADR 014 | T23.53 telemetry, OPS gate |
| CloudWatch custom metrics | T23.53's metric cardinality is not yet implemented | T23.53 |

`cost.json`'s `unknown_categories.embeddings.tokens_estimate` reports the
product-unit (`input_tokens` allowance) token volume per scenario so a
reviewer only needs to supply `usd_per_1k_tokens` once task42 pins a rate;
the formula is `tokens_estimate / 1000 * usd_per_1k_tokens`.

## Load harness

`evals/hosted-load/workload.json` freezes the task's exact required
parameters: 10 min warmup + 30 min steady + 5 min 2x burst, 8 concurrent
clients, baseline 4 req/s, 80/15/5 recall/remember/forget mix, 20% cold-brain
traffic, 50% traffic to one hot tenant, 20–128 token queries, 32–512 token
facts, the paid-profile full-limit cardinalities (90,000 facts across 14
accounts), 3 repetitions, and the proposed quality/capacity thresholds
verbatim from the task contract.

`evals/hosted-load/harness.py` is a chronological discrete-event simulator:
each admitted request holds its pool slot from arrival to its computed
completion time (an earlier synchronous acquire-then-release draft made
`MaxInFlight` unreachable and is fixed in the committed version — see the
harness's own comment and `test_saturation_run_has_materially_higher_rejection_than_main_run`
for the regression test). It models:

- Pool admission (`MaxOpen=8`, `MaxInFlight=16`, immediate rejection, no
  queueing) matching `internal/hosted/pool/pool.go` exactly.
- Per-account rate limiting (120/min) matching
  `internal/hosted/gateway/admission.go`.
- Per-brain writer serialization for `remember`/`forget` (mutations queue
  behind each other on the same brain; `recall` does not).
- Cold-open cost, bounded provider retries with injected failure, and a
  separate quota-boundary saturation run (tightened to `MaxOpen=1`,
  `MaxInFlight=2`) kept apart from the main completion-rate statistic, per
  the evidence-contract requirement that "expected 429s cannot hide
  failures."

### Fixtures run results (mechanics proof, not a capacity claim)

The committed run (`docs/launch/evidence/T23.60/load-fixture.json`, 14
accounts, 12,187 offered requests in the main run):

| Threshold | Limit | Observed | Pass |
|---|---:|---:|---|
| cold-ready p95 | ≤60s | 3.87s | yes |
| recall p95 | ≤2s | 3.43s | **no** |
| remember p95 | ≤3s | 3.65s | **no** |
| offered completion | ≥99% | 92.95% | **no** |
| unexpected 5xx | ≤0.1% | 0.0% | yes |
| unexpected admission rejection | ≤1% | 7.05% | **no** |
| CPU/RSS/disk | ≤70/80/80% | not measured | n/a (fixtures) |

The saturation run (tightened pool) shows 80.06% rejection versus the main
run's 7.05% — a large, real gap that would not exist if the model always
returned a canned answer regardless of capacity; this is the sensitivity
check `test_saturation_run_has_materially_higher_rejection_than_main_run`
pins down.

These synthetic numbers **are not evidence that the real service fails its
targets** — they are evidence that the harness responds to load rather than
hard-coding success, and that hand-picked latency constants in a first draft
of a synthetic model land close to (and in three cases past) the proposed
targets. Real capacity conclusions require T23.68's `--live` run against a
deployed candidate.

## Recommendations (proposals, not authorized changes)

1. **Route the S3 amplification finding to T23.49 and the task41 reviewer
   before T23.68's live run.** At minimum, decide before deploying whether
   backup frequency, retention window, or a delta/incremental format changes
   — the current design is not cost-viable past a light 10-account mix.
2. **Task41 reviewer: freeze or amend `evals/hosted-load/workload.json`'s
   thresholds** (fill `reviewer`/`review_date`) before any `--live` run is
   treated as authoritative, per `interfaces.md`.
3. **Re-verify `RATE_TABLE` against live AWS/Resend pricing pages** in a
   session with WebSearch/WebFetch permission before treating `cost.json` as
   a dated citation rather than a reviewable estimate.
4. **Task42 should supply the embeddings price** so `unknown_categories.embeddings`
   in `cost.json` can be filled in without re-deriving token volume.
