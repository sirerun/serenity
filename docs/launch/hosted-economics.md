# Hosted Serenity economics: cost model and load harness (T23.60)

Generated 2026-09-18, repaired 2026-09-19 at source
`810349ba7c1ed2c05fe34e3892de764a26a4633c` by the T23.60 worker. This
document, `scripts/hosted/cost_model.py` and `scripts/hosted/load.py` are a
reviewable cost/capacity estimate, not a spend authorization, a measured
capacity ceiling, or a plan-limit change. The historical USD 60/month figure
remains a ceiling, never a target. Full machine-readable output:
`docs/launch/evidence/T23.60/cost.json` and
`docs/launch/evidence/T23.60/load-fixture.json`.

## Status and what this evidence does and does not prove

- **Cost rates are now verified with live, dated citations.** Every entry in
  `RATE_TABLE` carries `"kind": "published"` (or `"constant"` for the fixed
  730 hr/month figure) and a `source` URL fetched via WebSearch/WebFetch on
  2026-09-19 — see `cost.json`'s `rate_table` and top-level `citation_note`.
  This replaces the task's earlier PARTIAL receipt, which used unverified
  training-data recollections tagged `assumption_recollection`. Two rates
  changed materially on verification: the T4g Unlimited-mode burst-credit
  price is **USD 0.04/vCPU-hour** (the earlier receipt's USD 0.05 was
  wrong), and S3 Standard storage in `us-west-2` is **USD 0.0265/GB-month**
  (the earlier receipt used the unlabeled `us-east-1` rate of USD 0.023).
  One rate remains an honest gap: this session could not extract a
  `us-west-2`-specific EC2 `t4g.small` on-demand hourly rate from any page
  fetched (JS-rendered pricing tables); the cited figure is `us-east-1`'s,
  used because T-family on-demand pricing has historically matched between
  those two regions — a probable, not confirmed, number (see
  `rate_table.ec2_t4g_small_usd_per_hour.note` in `cost.json`).
- **The backup-retention mechanism was fixed, not just the rates.** The
  earlier receipt modeled hourly backups as overwriting the *same* S3 object
  key, so a review correctly flagged it as testing the wrong mechanism —
  `deploy/hosted/backup.sh` writes a **new, unique timestamped key prefix**
  on every run (`snapshots/$stamp/`, `stamp=$(date -u
  +%Y%m%dT%H%M%SZ)`) and never overwrites a key. See "Critical finding"
  below for the corrected math, which is *worse* (higher cost), not better.
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
- **`--live` now implements the real MCP Streamable HTTP protocol and is
  tested against a real local server, but has still never run against a
  real deployed host.** The earlier receipt's `--live` path had material
  defects found by review: it sent a literal `"REDACTED:<secret_ref>"`
  string as the bearer credential instead of a real one, built a `"query"`
  argument for every verb regardless of whether the verb was
  recall/remember/forget, never performed the MCP session handshake
  (`initialize` → `notifications/initialized`), only checked the HTTP
  status code and never inspected the JSON-RPC envelope's `error` field or
  a tool result's `isError` flag, ran one sequential pass ignoring the
  workload's schedule/concurrency/repetitions, checked budget caps against
  already-sent totals instead of the next request's own cost, never
  enforced a dollar cap at all, used a fixed 30s timeout regardless of
  remaining budget, and could follow an HTTP redirect to an arbitrary host
  while sending the real bearer credential. All of these are fixed in the
  current `scripts/hosted/load.py` (see "Load harness" below for exactly
  what changed and how it's tested). No hosted candidate is deployed yet at
  T23.60 (that is T23.64's job), so this path has been exercised only
  against a real local `http.server` fixture
  (`evals/hosted-load/test_load_cli.py`, 90/90 tests passing including a
  full `main()`-level end-to-end run), never a real host. T23.68 depends on
  this file and will exercise `--live` and `cost_model.py --measurements`
  against a real deployed candidate.
- **Task41's threshold freeze has not happened.** The numeric thresholds in
  `evals/hosted-load/workload.json` are transcribed verbatim from this
  task's contract and `interfaces.md`'s "Threshold review" section as
  *proposed* targets; `workload.json.reviewer` and `.review_date` are `null`.
  This worker cannot self-approve that freeze.

Given the above, this task's own acceptance status remains **PARTIAL**: the
cost math, retention mechanics, rate citations, and the `--live` protocol
implementation are now correct and unit-tested (90/90 passing in
`evals/hosted-load`, see `docs/launch/evidence/T23.60/result.json`), but
task41's threshold freeze still depends on a human reviewer this task cannot
self-authorize, and `--live` has still never run against a real host because
none is deployed yet.

## Cost scenarios

Four scenarios per the task contract: 0 accounts (idle), an assumed
light-usage 10- and 100-account mix (documented assumption, not a product
decision), and the ratified full-limit mix (1 Scale + 3 Builder + 10 Free at
100% of `internal/hosted/plans/plans.go` V1 allowances — 90,000 total facts,
matching `docs/launch/hosted-completion/evidence.md`).

| Scenario | Accounts | Known-category subtotal (USD/mo) | Within USD 60 ceiling |
|---|---:|---:|---|
| `idle_0_accounts` | 0 | 22.53 | yes |
| `10_accounts_light` (assumed 8 free/2 builder, 10% usage) | 10 | 33.26 | yes |
| `100_accounts_light` (assumed 90 free/8 builder/2 scale, 10% usage) | 100 | 126.15 | **no** |
| `full_limit_mix` (ratified 1 Scale + 3 Builder + 10 Free, 100% usage) | 14 | 367.43 | **no** |

Every figure above is the sum of **known-priced categories only**. Unknown
categories (embeddings, CloudWatch Logs, CloudWatch custom metrics, S3
GET/List/restore) are excluded from these subtotals and reported separately
in `cost.json` — they are never silently zeroed, and adding them can only
raise these numbers further.

Fixed monthly components (all scenarios), each cited in `RATE_TABLE`:

| Category | USD/mo | Basis |
|---|---:|---|
| EC2 `t4g.small` baseline | 12.264 | `$0.0168/hr × 730 hr/mo`, `deploy/hosted/stack.json` instance type (region caveat: see Status above) |
| EBS gp3 (8 GB root + 30 GB data) | 3.04 | `deploy/hosted/stack.json` volume sizes |
| Elastic IP | 3.65 | AWS charges hourly for every public IPv4 address since Feb 2024, attached or not |
| KMS key | 1.00 | one customer-managed key, `deploy/hosted/stack.json` |
| Secrets Manager storage | 1.60 | 4 secrets (Resend, embeddings, Stripe secret, Stripe webhook) |
| CloudWatch alarms | 0.20 | 2 alarms currently in `stack.json` (CPU, instance health) |

**Fixed cost floor is USD 21.754/month before any backup storage.** T4g
burst credits, Secrets Manager API calls, S3 request costs, KMS requests,
data transfer, and email are usage-scaling categories reported separately
per scenario (most net to USD 0.00 at these volumes once AWS's free tiers —
20,000 KMS requests/month, 100 GB/month data transfer out — are applied; see
`cost.json`).

## Critical finding: backup-set retention, not same-key versioning, drives S3 cost — and it's worse than the earlier estimate

The earlier PARTIAL receipt modeled hourly backups as overwriting the *same*
S3 object key, so S3 versioning's `NoncurrentVersionExpiration` would
accumulate roughly 721 simultaneous noncurrent versions of that one key.
That is not what `deploy/hosted/backup.sh` does:

```sh
stamp=$(date -u +%Y%m%dT%H%M%SZ)
aws s3 cp "$work/snapshot/" "s3://$SERENITY_BACKUP_BUCKET/snapshots/$stamp/" --recursive
aws s3 cp "$work/COMPLETE" "s3://$SERENITY_BACKUP_BUCKET/snapshots/$stamp/COMPLETE"
```

Every backup run writes to a **unique timestamped key prefix**
(`snapshots/<stamp>/...`) and never overwrites a previous run's keys. Each
object therefore gets its own, independent lifecycle under
`deploy/hosted/stack.json`'s `LifecycleConfiguration`:
`ExpirationInDays: 30` inserts a delete marker and makes the object
noncurrent 30 days after it was written, then `NoncurrentVersionExpiration.
NoncurrentDays: 30` permanently deletes it 30 days after *that* — roughly a
**60-day total object lifetime** (`S3_OBJECT_LIFETIME_DAYS = 60` in
`cost_model.py`), not a 30-day one and not a same-key noncurrent-version
pileup. At hourly backups, steady state retains
`RETAINED_BACKUP_SETS = 60 × 24 + 1 = 1441` distinct backup-sets — roughly
**double** the earlier (wrong-mechanism) estimate of ~721.

Each backup-set's own object count is `manifest.json` + `control.db` +
`COMPLETE` (3 fixed objects, `internal/hosted/backup/backup.go`) plus one
`.bundle` per brain (one per account under the current 1-brain-per-account
assumption) — `objects_per_backup = 3 + accounts`.

| Scenario | Accounts | Customer+control-DB data size | Retained backup-sets | S3 storage/mo | Share of scenario's known subtotal |
|---|---:|---:|---:|---:|---:|
| idle | 0 | ~0.02 GB | 1441 | 0.76 | 3.4% |
| 10 accounts light | 10 | ~0.30 GB | 1441 | 11.46 | 34.4% |
| 100 accounts light | 100 | ~2.72 GB | 1441 | 103.87 | 82.3% |
| full-limit mix | 14 | ~9.02 GB | 1441 | **344.44** | **93.7%** |

**This single line item is why `100_accounts_light` and `full_limit_mix`
both exceed the USD 60 ceiling on known costs alone**, by a wider margin
than the earlier estimate showed, and it now dominates every other category
combined even more completely than before. This is not a new problem this
task introduces — it is the same gap `docs/launch/hosted-plan.md` and
`docs/tasks/hosted-completion/T23.49.md` (backup integrity, owner of
`R-hosted-backup`) already name — but the corrected number should raise, not
lower, T23.49's redesign priority and T23.41's storage-envelope review.

**Not authorized here:** changing snapshot frequency, retention window, or
backup format is an architecture change per `docs/launch/hosted-plan.md`
("A different hosting topology is a costed proposal reviewed through the
existing architecture path, never an individual worker choice"). For
illustration only, `cost_model.py`'s own math implies that moving from
hourly to daily full snapshots (1441 → ~61 retained sets) would cut the
full-limit-mix S3 storage line from ~344 to ~15 USD/month — a **proposal for
T23.49/task41 review**, not a change made by this task.

## Per-plan cost contribution (full-limit mix)

Fixed shared-node infrastructure (EC2, EBS, EIP, KMS key, Secrets Manager
storage, CloudWatch alarms — USD 21.754/mo) is not allocated per-plan: one
node serves every tenant. Only usage-driven categories (S3 backup storage
and requests, KMS requests, data transfer, Secrets Manager API calls — USD
344.50/mo total at full-limit mix) are apportioned by each plan's share of
total customer storage bytes:

| Plan | Accounts | Storage share | Variable cost/mo (USD) |
|---|---:|---:|---:|
| Free | 10 | 11.1% | 38.28 |
| Builder | 3 | 33.3% | 114.83 |
| Scale | 1 | 55.6% | 191.39 |

A single Scale account's backup storage alone costs more than the entire
Free-tier cohort's, and the Builder cohort's share exceeds Free's by 3x —
storage-proportional backup cost, not seat count, is the real driver of
per-plan economics here.

## Unknown-priced categories (reported, never zeroed)

| Category | Why unknown | Owner/trigger |
|---|---|---|
| Embeddings | `perplexity/pplx-embed-v1-0.6b` via OpenRouter has no qualified price/terms yet | T23.42 pins provider/model/price |
| CloudWatch Logs ingest/storage | Rate is priced (`$0.50/GB` ingest, `$0.03/GB-mo` storage) but no log shipping is wired; ops go through SSM Run Command per ADR 014 | T23.53 telemetry, OPS gate |
| CloudWatch custom metrics | Rate is priced (`$0.30/metric/mo`) but T23.53's metric cardinality is not yet implemented | T23.53 |
| S3 GET/List/restore | No routine integrity check or restore drill exists in the current design | T23.50 recovery testing |

`cost.json`'s `unknown_categories.embeddings` now reports the full token
accounting, not just write volume: `write_tokens` (input allowance),
`query_tokens` (recall queries, `AVG_QUERY_TOKENS=74`/call),
`rebuild_reembed_tokens` (an assumed 1% of stored facts re-embedded per cold
reopen, 30 reopens/account/month), and `readiness_tokens` (a bounded ~10-token
embedding call every 5 minutes, matching the CloudWatch alarm evaluation
period) — `total_tokens` sums all four. At full-limit mix this is
**72,692,871 tokens/month**, not just the 4,000,000-token input allowance a
narrower model would report. A reviewer only needs to supply
`usd_per_1k_tokens` once task42 pins a rate; the formula is
`total_tokens / 1000 * usd_per_1k_tokens`.

## Load harness

`evals/hosted-load/workload.json` freezes the task's exact required
parameters: 10 min warmup + 30 min steady + 5 min 2x burst, 8 concurrent
clients, baseline 4 req/s, 80/15/5 recall/remember/forget mix, 20% cold-brain
traffic, 50% traffic to one hot tenant, 20–128 token queries, 32–512 token
facts, the paid-profile full-limit cardinalities (90,000 facts across 14
accounts), 3 repetitions, and the proposed quality/capacity thresholds
verbatim from the task contract.

`evals/hosted-load/harness.py` is a chronological discrete-event simulator
(`--fixtures` mode): each admitted request holds its pool slot from arrival
to its computed completion time. It models:

- Pool admission (`MaxOpen=8`, `MaxInFlight=16`, immediate rejection, no
  queueing) matching `internal/hosted/pool/pool.go` exactly.
- Per-account rate limiting (120/min) matching
  `internal/hosted/gateway/admission.go`.
- Per-brain writer serialization for `remember`/`forget` (mutations queue
  behind each other on the same brain; `recall` does not).
- Cold-open cost, bounded provider retries with injected failure, and a
  separate quota-boundary saturation run (tightened to `MaxOpen=1`,
  `MaxInFlight=2`) kept apart from the main completion-rate statistic.

### `--live`: the real MCP Streamable HTTP protocol, protocol-tested but never run against a real host

`scripts/hosted/load.py --live` speaks the actual wire protocol implemented
by `internal/server/mcp/http.go` (read in full this session, not assumed):

- **Session handshake**: `initialize` (no `Mcp-Session-Id` header on the
  first request) → server returns `Mcp-Session-Id`/`MCP-Protocol-Version`
  response headers → client sends `notifications/initialized` (a JSON-RPC
  notification, no `id`, expects `202 Accepted` with an empty body) before
  any `tools/call`. Every subsequent request carries both headers; the
  server's `MCP-Protocol-Version` value must match exactly
  (`2025-11-25`) or it returns 400.
- **Per-verb argument shapes**, matching each tool's real JSON Schema
  (`internal/server/memory/{recall,remember,forget}.go`): `recall` sends
  `{"query": ...}`; `remember` sends the required `{"fact", "provenance"}`;
  `forget` sends `{"id": ...}` sourced **only** from a fact id a `remember`
  call actually returned earlier in the same run — a `forget` arrival for an
  account with no remembered id yet is skipped and counted separately
  (`skipped_forgets`), never sent with a synthesized or empty id.
  `remember`'s real response is a **JSON string embedded in
  `result.content[0].text`**, not a `structuredContent` field — the client
  parses that string to recover the fact id for later `forget` calls.
- **Dual failure-mode inspection**: a JSON-RPC top-level `error` (protocol
  failure — bad params, unknown tool, too many in-flight calls) and a
  successful envelope's `result.isError == true` (tool-level failure, e.g.
  `remember` called without `provenance`) are both checked; neither is
  mistaken for the other, and an HTTP 200 is never treated as automatic
  success.
- **Real concurrency and pacing**: a `ThreadPoolExecutor` sized to
  `workload.concurrency.clients` dispatches arrivals from
  `harness.generate_arrivals` at their real scheduled wall-clock time (a
  single dispatch thread paces submissions; work itself runs concurrently),
  across all `workload.repetitions`, not a single sequential pass.
- **Precharge budget enforcement**: `calls_used`, `tokens_used`, a
  worst-case dollar cost (`budget.worst_case_usd_per_call` × calls, since
  the real embeddings price is still unknown — see above), and elapsed time
  are all checked against the **next** request's own projected cost
  *before* it is sent, not against already-sent totals after the fact. The
  elapsed check also runs before committing to a scheduled wait, so a
  distant arrival late in a long phase can't force the harness to sleep
  past its own budget before noticing it's exhausted.
- **Per-request timeouts bounded by remaining elapsed budget**:
  `min(10s default, remaining_elapsed_seconds)`, never a fixed timeout that
  could itself overrun the cap.
- **Transport security**: `manifest.environment.origin`'s host must be in
  `allowed_hosts`, `app.serenity.sire.run`/`serenity.sire.run` are refused
  outright even if allowlisted, plaintext `http://` is refused for any
  non-loopback host, and **any** 3xx response (including from the readiness
  probe) raises immediately instead of being followed — a custom
  `HTTPRedirectHandler` raises `HTTPError` from `redirect_request` rather
  than relying on urllib's undocumented-to-callers "return None to skip
  following" behavior, so a redirect to a credential-harvesting host is
  never dialed.
- **Private per-account credentials**: loaded from
  `<manifest.live.credential_dir>/<account_id>.token`, each required to be a
  regular file (never a symlink), mode exactly `0600`, non-empty — checked
  for **every** account in the workload's account mix before any network
  call. A missing or wrong-permission file for even one account blocks the
  entire run before it starts.
- **Fail-closed ordering**: every offline guard (environment, budget shape,
  credential files, workload validity) runs before this process opens a
  single socket. The first live network call is a read-only `GET /readyz`
  probe against the target, and only after every offline guard has passed.

**Tested against a real local `http.server` fixture, not an injected mock
transport.** `evals/hosted-load/test_load_cli.py` runs a genuine
`ThreadingHTTPServer` on `127.0.0.1` that speaks the same session/header/
JSON-RPC rules as the real Go handler (verified by two tests that confirm
the *fixture itself* enforces the protocol-version-header and Origin-header
rules, not just that the client happens to pass). 90/90 tests pass,
including: the full initialize → notified → tools/call handshake; JSON-RPC
top-level error handling; tool-level `isError` handling; a redirect that the
client refuses to follow (asserted via a "was the redirect target ever
actually hit" flag, not just an exception message); calls/tokens/dollar/
elapsed cap enforcement using the real precharge logic (four separate red
tests, one per cap); credential file permission and presence validation;
and a `main()`-level end-to-end CLI run against the fake server. **No
deployed hosted target exists yet as of T23.60** (T23.64's job), so none of
this has run against a real host — see `docs/launch/evidence/T23.60/
result.json`'s `limitations` for the exact scope of "tested" here.

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
   before T23.68's live run.** The corrected math (1441 retained backup-sets,
   not 721) makes this more urgent than the earlier estimate: at minimum,
   decide before deploying whether backup frequency, retention window, or a
   delta/incremental format changes — the current design is not
   cost-viable past a light 10-account mix.
2. **Task41 reviewer: freeze or amend `evals/hosted-load/workload.json`'s
   thresholds** (fill `reviewer`/`review_date`) before any `--live` run is
   treated as authoritative, per `interfaces.md`.
3. **Confirm the `us-west-2` EC2 `t4g.small` on-demand rate.** Every other
   rate in `RATE_TABLE` is region-pinned and cited; this is the one
   remaining probable-not-confirmed figure (see Status above).
4. **Task42 should supply the embeddings price** so `unknown_categories.
   embeddings` in `cost.json` can be filled in without re-deriving token
   volume — the token accounting (write + query + rebuild/re-embed +
   readiness) is already complete.
5. **Before any `--live` run against a real deployed candidate (T23.68)**:
   provision a private `live.credential_dir` with one 0600 token file per
   workload account, and set `budget.worst_case_usd_per_call` from a real,
   conservative per-call cost ceiling once task42 pins the embeddings
   price — the current committed manifest deliberately leaves both `null`/
   unset-for-live so `--live` stays refused by construction.
