# Hosted Serenity economics and load tooling — T23.60

Status: PARTIAL. Source revision and test evidence are recorded in
[evidence/T23.60/result.json](evidence/T23.60/result.json). These are scenario
estimates and local fixture results, not measured capacity or authority to spend.

## Available now

- `scripts/hosted/cost_model.py --manifest PATH --output PATH` calculates idle,
  10-account, 100-account and full-limit paid scenarios, including per-plan
  contribution and a peak exposure for each. A successful run prints `CALCULATED`
  and `launch_cost_qualification: NOT_QUALIFIED`. The arithmetic is not a launch
  cost qualification, a measured bill or a spend authority. See
  [Calculation is not qualification](#calculation-is-not-qualification).
- `scripts/hosted/load.py --fixtures --manifest PATH --output PATH` exercises a
  deterministic simulator: three repetitions, replay, burst/cold/skewed tenants,
  quota saturation separate from the eligible workload, explicit failed thresholds.
  Its rate limiter reproduces the gateway's fixed window, which starts at an
  account's first request.
- **The frozen hot tenant offers the gateway's per-account rate limit** (120
  requests a minute), so a healthy server fails the steady admission and completion
  thresholds. The rate-limit component is 180 of 7,379 steady requests (2.44%);
  the threshold-matching admission total is 204 of 7,379 (2.76%), including 24
  pool-capacity refusals. Fixture thresholds now use steady arrivals only, and
  burst results are reported separately. The client and simulator count these
  as failures, not as expected saturation. No threshold changed. Chief-architect's
  PR #239 ruling keeps the frozen workload and `<1%` target unchanged; T23.45's
  admission implementation and a passing rerun remain open in
  [decision-request-hot-tenant-rate-limit.md](evidence/T23.60/decision-request-hot-tenant-rate-limit.md).
- A candidate MCP client has local tests against a real loopback HTTP server:
  per-request outcomes over all offered requests, one wall-clock deadline on each
  exchange that includes name resolution (a disposable child process, killed at
  the deadline, for hostnames), a cap check before every socket operation,
  byte-based token precharge, canonical origins, redirect refusal, fixed error
  classes, gateway error codes decoded into a fixed enum (capacity as admission,
  quota and client faults kept apart, a refused forget never lost durability),
  an elapsed-budget preflight that counts setup, drain and cleanup, and
  confirmed-only session cleanup that survives a malformed reply. Word counts in
  the workload are nominal sizes, not tokens. It never reports better than
  `PARTIAL` and is not a supported live qualification runner.
- **The full-cardinality sample cannot be eligible steady traffic as frozen.** With every
  account seeded at its plan memory cap (90,000 memories), the gateway's memory-count rule alone
  refuses 726 of 7,379 steady requests (9.84%) in the frozen first repetition, because the mix
  offers more remembers than forgets. This is an inventory replay and not a measured failure, and
  the 9.84% is not the unexpected-admission statistic. The decision is open in
  [decision-request-full-cardinality-eligible-traffic.md](evidence/T23.60/decision-request-full-cardinality-eligible-traffic.md).
  No number changed.
- **`--live` is disabled unconditionally**, returning BLOCKED/exit 2 and a zero-call
  receipt before manifest/credential reads or networking. Seeded state, a real
  cold-open workload, host telemetry, isolation probes and the reviewer freeze
  remain open in
  [live-enable-requirements.md](evidence/T23.60/live-enable-requirements.md).

## Candidate monthly cost

| Scenario | Known subtotal | S3 full-snapshot storage portion | Peak, sustained 100% CPU | Peak, 70% mean CPU |
|---|---:|---:|---:|---:|
| idle_0_accounts | $22.40 | $0.62 | $71.12 | $53.60 |
| 10_accounts_light | $31.26 | $9.26 | $80.01 | $62.49 |
| 100_accounts_light | $108.05 | $83.96 | $156.91 | $139.39 |
| full_limit_mix | $305.47 | $278.42 | $353.34 | $335.82 |

The known subtotal prices the categories that have a rate and an assumed quantity. The
peak columns replace four of its lines with their worst case (see
[Peak exposure](#peak-exposure)) and add nothing else. Neither column is a full maximum:
[the unknowns](#what-stays-unknown) are outside both.

### Conditional 55-set retention sensitivity

The architect-approved working schedule's 55 retained full sets cost $10.63/month in
stored snapshots under the PR #239 assumptions. Applying that one storage-line change to
the current full-limit scenario gives a **$37.68 known subtotal**: $305.47 − $278.42 +
$10.63. It assumes hourly full uploads continue, so the modeled 824,400 S3 PUT requests
and the one-KMS-request-per-object assumption remain unchanged. Applying the same storage
line to the wider T23.60 peak calculations gives **$68.03 at 70% mean CPU** and **$85.55
at sustained 100% CPU**. These are arithmetic sensitivities, not a separate cost-model CLI
scenario, measured usage, complete monthly maximum, or spend approval. Unknown categories
remain excluded and launch cost qualification stays `NOT_QUALIFIED`.

This reconciles PR #239's **$69.61** figure: it sums only the `$12.264` instance base,
`$46.72` sustained CPU-credit exposure, and `$10.63` snapshot-storage line. It is not a
complete service subtotal. T23.60's broader 100%-CPU sensitivity is `$85.55` for the same
55-set storage assumption and still omits the unknown categories listed below.

Two corrections from the independent review change these figures against the previous
revision (full mix $322.00, peak $369.87):

- S3 storage is billed in GiB-months. Converting snapshot bytes by 2^30 instead of 10^9 lowers
  every S3 storage line by 6.87% (full mix $298.95 to $278.42). See [Units](#units).
- S3 requests are counted per object size instead of per object. The full mix was priced at 32
  requests a backup ($0.12 a month) and is now priced at 1,145 ($4.12 a month); see
  [S3 requests](#s3-requests).

Unknown provider charges, log volume, custom-metric cardinality, restore/list/GET
activity, control database growth and account-wide free-tier
consumption can increase these figures. The 10/100-account mixes and usage fractions are
assumptions, not product decisions. The historical $60/month ceiling remains a ceiling, not
new spending permission.

The full mix counts every allowed brain (10 Free, 9 Builder and 10 Scale, 29 in all) in
each backup. That gives 32 objects a backup (`manifest.json`, `control.db`, `COMPLETE` and
29 bundles). The 10- and 100-account scenarios model each account's default brain only, and
each reports its allowed-brain ceiling.

## Units

S3 bills storage in binary gigabytes: "Amazon S3 storage usage is calculated in binary
gigabytes (GB), where 1 GB is 2^30 bytes" ([S3 pricing](https://aws.amazon.com/s3/pricing/);
[receipt](evidence/T23.60/aws-rates/s3-storage-unit-receipt.json) with the page hash and the
extracted sentence). The price list's own tier boundaries agree: the next tier begins at 51,200
GB, which is 50 TB at 1,024 GB each. The catalog unit `GB-Mo` is therefore GiB-months, and the
model converts snapshot bytes to the billed quantity with 2^30.

Every quantity carries one unit, listed in `unit_definitions` in `cost.json`:

| Quantity | Unit | Source |
|---|---|---|
| Plan quotas (`storage_bytes`, memories, brains, writes, recalls) | decimal bytes and counts | `internal/hosted/plans/plans.go`; never converted for a quota comparison |
| S3 storage billed | GiB-month, 1 GB = 2^30 bytes | the receipt above |
| S3, KMS and Secrets Manager requests | requests; catalog prices are per request or per 1,000 and convert to per 10,000 | the regional receipts |
| EBS volume size | the configured size in `stack.json`, priced as written | not re-based; the check that reads the configured 30 as decimal GB is conservative for a volume configured in GiB |
| gp3 throughput | catalog GiBps-month converted to MiBps-month by 1/1,024 | the EC2 receipt |
| Data transfer | GB a month, decimal assumed | AWS's data-transfer GB is not sourced here, so a binary reading (up to 7.4% different) stays open |

Only S3 storage was re-based. The other rows keep the unit their own source gives.

## Backup amplification blocks the full-limit proposal

`backup.sh` uploads a new timestamped prefix hourly. `stack.json` currently expires
current objects after 30 days and their noncurrent versions after another 30 days.
Modeling those phases in series yields approximately 1,441 retained hourly sets,
not 721 versions of one overwritten key. A full paid mix at the advertised storage
allowances plus the assumed control database is 9.02 GB (8.40 GiB) per full backup. At the
verified Oregon first-tier S3 rate, that is about $278/month for stored backup bytes
alone (12,105 GiB-months). This is a modeled steady-state exposure, not a measured bill;
lifecycle rounding/asynchronous cleanup, history size and compression require qualification.
The correction to GiB does not make the retention affordable: the storage line alone is
still more than four times the historical $60 ceiling.

The 9.02 GB is 9 GB of customer storage at the advertised allowances plus the 20 MB control
database assumed at snapshot time, which is not a lifetime bound
([Control database growth](#control-database-growth)). The baseline is the retention the
deployed template sets.

The chief architect approved the PR #239 proposal as a **working design**: hourly full
captures, 24 hourly recovery points, one daily full retained up to 29 days, and task52's
scheduled all-version purge/verification. It remains conditional on tasks48/52 proving
idempotent purge that excludes the independent deletion journal, and on naming an owner
for control-DB retention/compaction before task52 starts. This T23.60 cost table still
models only the deployed template's current 30-plus-30-day retention; it does not price
the proposed schedule using the corrected T23.60 inputs. The proposal is not a budget,
deployed behavior, or launch-cost qualification, and it does not approve reducing plan
limits or the customer deletion-retention promise.

## Peak exposure

The master plan asks for conservative idle and peak exposure. Each scenario reports
`peak_exposure`, which replaces four lines of the known subtotal with their worst case. It is
a sensitivity, not measured usage and not a forecast.

**CPU credits.** A t4g.small has 2 vCPUs at a 20% baseline each (24 CPU credits earned an
hour; [AWS burstable credits](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-credits-baseline-concepts.html)).
In unlimited credit mode, surplus credits cost $0.04 per vCPU-hour:

| Mean CPU | Surplus vCPU-hours a month | Monthly cost |
|---:|---:|---:|
| 20% (baseline) | 0 | $0.00 |
| 70% | 2 × (0.70 − 0.20) × 730 = 730 | $29.20 |
| 100%, credits exhausted | 2 × (1 − 0.20) × 730 = 1,168 | $46.72 |

The known subtotal's $1.168 (2% of vCPU-hours, full mix only) is a mean-case assumption and
not a peak. **The credit mode is unverified.** `deploy/hosted/stack.json` sets no
`CreditSpecification`, so the instance takes the account default for T4g, and AWS's default
is unlimited. In standard mode no surplus is billed and the node throttles at its baseline,
which is a capacity limit and not a cost. An owner must confirm the mode. The model sets
neither and changes no infrastructure, threshold or retention.

**KMS key storage.** The template sets `EnableKeyRotation`. AWS adds $1 a month for each of
the first two rotations (prorated hourly) and stops after the second
([KMS pricing](https://aws.amazon.com/kms/pricing/)), so key storage rises from $1 to $3 a
month. A flat $1 is not a worst case.

**No global free allowance.** The 20,000 free KMS requests and the 100 GB of free transfer
are account-wide. The shared AWS account's other use is unknown, so the peak uses none of
either. The known subtotal still applies them and says so.

The peak of the full mix is $353.34 at sustained 100% CPU (305.47 − 1.168 − 1.00 − 0.009 +
46.72 + 3.00 + 0.069 + 0.258). The transfer term is the average recall-response model, which
nothing enforces, so it is not an egress bound ([Egress](#egress)). The 55-set backup
thinning schedule proposed in PR #239 is an architect-approved working design subject to
the purge and ownership conditions above. It is not the baseline and is not priced by this
table: the baseline is the deployed template's 30-plus-30-day retention.

## Control database growth

`control.db` is copied whole into every snapshot, and no runtime code prunes committed
reservations or `audit_log` rows. Every successful embedding call, including each `/readyz`
recompute, adds a provider audit row. The model's 20 MB is a scenario assumption at snapshot
time, not a lifetime bound, and the model does not replace it with a sampled slope as a bound.

Each scenario reports the growth as an unknown category with three named components that
do not overlap:

| Component | Rows and bytes | Notes |
|---|---|---|
| Write and recall paths | 1,060 B a write and 530 B a recall | The real-writer audit sample (one account, one brain, 1,500 writes and 11 recalls, a stub embedder) already includes the provider rows its own write and recall paths wrote, so no second provider slope is added for them. |
| Readiness probe rows | 28,800 to 43,200 rows a month at 229 B a row | The sample reports no readiness call, so these rows are separate. The row size is the sampled audit row and a real provider's row may be larger. |
| Restore re-embed rows | up to one row per stored fact for each restore event | The number of events is unknown, so this is a per-event figure (90,000 rows, 20.6 MB for the full mix) and is in no horizon. |

The horizon table sums the first two components, assumes every month runs at the scenario's
usage, nothing is pruned, and every retained backup set carries the grown size, billed at
GiB. Low and high are the two ends of the readiness cadence. Amounts are USD a month.

| Scenario | Growth a month | Added S3 cost after 1 month | Added S3 cost after 12 months | Added S3 cost after 24 months |
|---|---:|---:|---:|---:|
| idle_0_accounts | 6.6 to 9.9 MB | 0.20 to 0.31 | 2.44 to 3.66 | 4.89 to 7.33 |
| 10_accounts_light | 22.9 to 26.2 MB | 0.71 to 0.81 | 8.49 to 9.71 | 16.98 to 19.42 |
| 100_accounts_light | 141.7 to 145.0 MB | 4.38 to 4.48 | 52.50 to 53.72 | 105.01 to 107.45 |
| full_limit_mix | 420.0 to 423.3 MB | 12.96 to 13.07 | 155.57 to 156.79 | 311.13 to 313.58 |

An idle service still grows: readiness probes alone add 6.6 to 9.9 MB a month. The full mix
reaches about $156 a month of added backup storage after a year. This is a sensitivity at
sampled row sizes. It is outside every subtotal and every peak, and it is not an upper bound.

Measuring actual growth needs the allocated size of `control.db` (page bytes from `dbstat` and
the file size) at the start and end of a bounded window, bound to the scenario, the window and
the source. The provider ledger rows carry only the task class `embedding`, so write, query,
readiness and rebuild rows cannot be told apart without instrumentation, and a failed or retried
billable attempt may not be ledgered at all. The measurement schema does not accept a database
size, because no producer can attest its provenance yet. A pruning or retention change needs its
own review, and this document does not propose one.

## Embeddings and provider units

No provider rate, provider token count or provider dollar figure exists, and none is invented.
The model keeps three units apart:

- **Service counter.** The gateway counts `cl100k_base` tokens on each remembered fact against
  `Plan.InputTokens`. At full usage the entitlement is 8,000,000 tokens a month in the full mix.
  It meters remembers only. Recalls, readiness probes and restore or reopen re-embeds are not
  metered, so the account input cap does not govern them.
- **Provider calls.** Counts of embed calls: 40,000 writes and 700,000 recalls (entitlement
  times usage, one query embedding a recall, not measured), plus the readiness range below. A
  retried or failed attempt is not counted.
- **Provider tokens.** Unknown. The candidate provider's tokenizer is unqualified, so the
  8,000,000 service tokens are not a bound on provider-billable tokens, and an average is not a
  bound either. The load scenario's averages (272 tokens a fact, 74 a query, a query mean and not a
  service cap) appear as `average_proxy_tokens` (10,880,000 write, 51,800,000 query) and are never
  capped against the service counter.

Text bytes handed to the embedder are stated only where the source proves them: the readiness
probe is a 24-byte literal, and a query cannot exceed the 1 MiB request frame. A write's or a
re-embed's embedded text is the indexed chunk text, which is not shown to equal the fact, so no
byte bound is claimed for it. None of these bounds the provider's request serialization or token
count.

**Restore and reopen.** The old rebuild line divided bytes by tokens per fact, which is not a
fact count, and the event was missing from the model. The count now comes from the plan table
(memories times usage): a full restore may call the provider once for every stored eligible
fact, because the Git bundle excludes the derived index. That is 90,000 calls for one restore of
the full mix (2,800 and 27,000 for the two light scenarios). Outside a restore a vector is
missing only after a failed write-time embed, and every cold open retries it. The number of
restore events, cold reopens (`max_open` is 8 against up to 29 brains) and failed embeds is
request-driven and unmeasured, so the monthly count and the provider tokens stay unknown. A
restore's embeds run one at a time under the brain pool lock, so time to ready scales with facts
times provider latency; it is unmeasured.

## Readiness cadence

Caddy probes `/readyz` every 30 seconds (`deploy/hosted/Caddyfile`), and the service
recomputes, with one paid provider embed, only when its cached answer is older than one minute
(`internal/hosted/service/service.go`). Recomputes are therefore at least 60 seconds apart, and
with Caddy's 30-second tick the usual spacing is 90 seconds. The two EC2 alarms in `stack.json`
read `CPUUtilization` and `StatusCheckFailed` and never call `/readyz`, so the earlier
300-second cadence had no source.

| Month basis | Seconds | 90-second spacing | 60-second spacing |
|---|---:|---:|---:|
| 30-day (the model's event month) | 2,592,000 | 28,800 | 43,200 |
| 730-hour | 2,628,000 | 29,200 | 43,800 |

The range is derived from source and is not measured. It counts successful embeds. Failed
probes, the router's retries (up to 3 provider attempts an embed) and probes from other callers
are unmeasured, and only a success is ledgered. Readiness adds 288,000 to 432,000 average-proxy
tokens a month at 10 tokens a probe, a proxy and not a provider count.

## S3 requests

The AWS CLI defaults are `multipart_threshold` and `multipart_chunksize` of 8MB
([CLI S3 configuration](https://docs.aws.amazon.com/cli/latest/topic/s3-config.html);
[receipt](evidence/T23.60/aws-rates/aws-cli-s3-config-receipt.json)). The page does not say
whether the `MB` suffix is 10^6 or 2^20 bytes for a size, so the model uses 8 MiB. Eight
million bytes changes a part count by at most 4.9%. `preferred_transfer_client` defaults to
`auto`, which resolves to `classic` on an instance type outside the page's listed large types.

An object below the threshold is one request. An object at or above it is a create, one request
per part and a complete. The model sets the object sizes by assumption: `manifest.json` and
`COMPLETE` are 4,096 bytes, `control.db` is the 20 MB scenario assumption, and each brain bundle
is an even share of its account's storage quota bytes, which also stand in for the snapshot
size. The full mix is then 1,145 requests a backup (29 bundles at 14, 42 and 62 requests plus
`control.db`, `manifest.json` and `COMPLETE`), or 824,400 a month, and each object is listed in
`known_categories_usd.s3_requests.per_object_size_assumption`. The sizes are an assumption and
not an inventory.

KMS requests are not set equal to S3 requests. AWS documents the KMS operations but not a
one-request-a-part count, so the KMS line keeps an explicit assumption of one request per
object written (23,040 a month for the full mix). If every S3 request made one KMS request the
line would be $2.41 with the global free tier and $2.47 without. That is an unverified
sensitivity reported in `unknown_categories.s3_multipart_requests`, in no subtotal or peak.
Still unmeasured: the deployed CLI version, configuration and transfer client, the real object
size inventory, retries, aborted multipart uploads (the template's lifecycle aborts incomplete
uploads after one day), list or head checks, and the KMS requests a part makes. A measured
`s3_put_requests_per_month` replaces the modeled S3 count and does not change the KMS line.

## Egress

The egress line counts recall response bytes only, at an assumed 4,096-byte average a recall
(2.87 GB a month for the full mix, inside the assumed free allowance). The average is not
enforced: recall's `limit` accepts any nonnegative integer, the facts arm can return every
visible fact, and response bytes are not metered or capped. The model reports the line as
`average_recall_response_unbounded_by_current_plan`. It does not multiply `limit` by 4,096
into a bound, because JSON escaping, metadata, result-arm duplication and HTTP compression change
the bytes, and it claims no exposure beyond the average. This lane implements no runtime cap.

Inbound write request bytes are not outbound egress and are no longer counted. Outbound bytes the
model does not cover: write and other tool responses, embedding request text sent to the
provider, email API calls, dashboard responses and AWS API traffic. A future measured egress figure
must name its scope (the request mix and the `limit` values used) and its horizon (the window),
and it does not bound anything outside them. The data-transfer GB unit is assumed decimal.

## What stays unknown

The known subtotal and the peaks leave these out, and `full_maximum_computable` is false
until they are priced. Each is reported as unknown and no bound is invented for any of them:

- Embeddings: the provider pin (task42) is open, and no provider token count exists (see
  [Embeddings and provider units](#embeddings-and-provider-units)).
- Restore and reopen re-embeds: the events and the missing-vector rate are unmeasured.
- CloudWatch log volume and custom-metric cardinality: the rates are known and the volumes are
  not (T23.53).
- S3 GET, list and restore activity.
- Control database lifetime growth, above.
- S3 request inputs: the CLI configuration, object sizes, retries, aborts, list checks and the
  KMS ratio for multipart parts, above.
- The recall response size, which the plan does not bound, and the outbound bytes beyond recall
  responses ([Egress](#egress)).
- Peak email volume: the model uses average logins, and an average day is not a maximum. A day
  past Resend's free cap needs a paid tier.
- The credit mode and the global free allowances, above.

## Measurement input

`--measurements` takes a document in the owned schema
`serenity-hosted-cost-measurements` version 1. Each record names one scenario, one quantity in
an exact unit, a value, an observation time and a source:

```json
{
  "schema": "serenity-hosted-cost-measurements",
  "schema_version": 1,
  "records": [
    {
      "scenario": "full_limit_mix",
      "quantity": "snapshot_size_bytes",
      "unit": "bytes",
      "value": 123456789,
      "observed_at": "2026-09-01T00:00:00Z",
      "source": {"kind": "backup_snapshot_listing", "ref": "path/or/receipt/id", "sha256": "<64 lowercase hex of the source file>"}
    }
  ]
}
```

| Quantity | Unit | Type and range |
|---|---|---|
| `snapshot_size_bytes` | `bytes` | integer, 1 to 30,000,000,000 (the data volume) |
| `s3_put_requests_per_month` | `requests_per_month` | integer, 1 to 10,000,000 |
| `ec2_surplus_credit_vcpu_hours` | `vcpu_hours_per_month` | finite number, 0 to 1,168 |

A record changes only its own scenario and the lines that quantity feeds; a snapshot size is
`partly_measured` because the retained-set count stays modeled, and it is decimal bytes that the
model converts to GiB for the S3 bill. A measured request count replaces the modeled S3 requests
and does not change the KMS line, which stays an assumption of one request per object. Everything else is priced on assumptions. The run is `BLOCKED`, with a
sanitized reason and no output file written by that run, for any of the following:

- A boolean, string, `NaN`, `Infinity`, negative, zero-where-meaningless or oversized value, an
  integer quantity given as a float, or a wrong container.
- A missing, ambiguous or mismatched unit, an unknown scenario or quantity, a repeated scenario
  and quantity, or an extra key.
- A source without a kind, reference and SHA-256, or an observation time that is not a past
  RFC 3339 UTC timestamp.
- A file that is not this schema (a load result, `{}` or an empty `records` list), is not valid
  JSON, repeats a key, or is larger than 1 MiB.

**Not accepted today.** The schema has no field for provider embedding tokens, the allocated
size of `control.db`, or egress bytes, because no producer can attest their provenance and a
field without a validator would let an unverified number look measured. A later producer would
have to supply, at least: the provider token count reconciled against the provider's usage
export, split by call kind (write, query, readiness, restore) with `null` preserved for a kind not
measured, which the ledger cannot do without instrumentation; the `dbstat` allocated size and
file size of `control.db` at the two ends of a bounded window; and egress bytes bound to a request
mix, the `limit` values used and a window. Each would be scenario-bound and carry evidence. No
actual dollar figure is produced from any of them.

The reason names fields and limits and never repeats the input. `scripts/hosted/load.py` does
not produce this document, and T23.60 does not claim it will: see
[integration-request-cost-measurements.md](evidence/T23.60/integration-request-cost-measurements.md).

## Calculation is not qualification

A run that prints `CALCULATED` did arithmetic over listed rates, assumptions and validated
measurements. Every result also carries `launch_cost_qualification: NOT_QUALIFIED`, the
blocking unknowns and `full_maximum_computable: false`. A measurement is never evidence of
capacity acceptance, and the ceiling comparison is against the known subtotal only.

## Rates, units and assumptions

Every rate in `cost.json` carries a `verification`. **`aws_regional_catalog_primary`** means
the rate was read from AWS's own US West (Oregon) price list. The entry names a receipt file
kept in [evidence/T23.60/aws-rates/](evidence/T23.60/aws-rates/) (and
[s3-rate.json](evidence/T23.60/s3-rate.json) for S3 storage), the rate code, the catalog unit
and price, and the factor that converts the catalog unit to the model's unit. Each receipt
records the upstream source's version, publication date and SHA-256. The test suite checks
every primary rate against its receipt, so a rate cannot drift from its source.

| Rate | Value | Unit conversion |
|---|---:|---|
| EC2 t4g.small on-demand Linux | $0.0168/hour | Rate code `NTSJZ6S2KD2YFRVB.JRTCKXETXF.6YS6EN2CT7`, published 2026-09-18T20:33:44Z. It replaces the earlier third-party regional-parity assumption; the number is unchanged. |
| T4g surplus CPU credit | $0.04/vCPU-hour | none |
| gp3 storage, IOPS | $0.08/GB-month, $0.005/IOPS-month | none |
| gp3 throughput | $0.04/MiBps-month | The catalog states $40.96 per GiBps-month; divide by 1,024. |
| S3 Standard, first 50 TB | $0.023/GiB-month | The catalog unit `GB-Mo` is a binary gigabyte (2^30 bytes); the billed quantity is bytes / 2^30. |
| KMS key, requests | $1.00/key, $0.03/10,000 requests | The catalog states $0.000003 per request. |
| Secrets Manager | $0.40/secret, $0.05/10,000 calls | The catalog states $0.000005 per call. |
| CloudWatch | $0.10 alarm, $0.30 custom metric (first 10,000), $0.50/GB logs ingested, $0.03/GB-month logs stored | none |
| Public IPv4 | $0.005/address-hour | none |

The earlier $0.0265 Oregon S3 figure and $0.05 T4g credit figure were wrong and stay corrected.
The full AWS EC2 price list (474,950,676 bytes) was streamed and hashed, not stored. The
receipts hold the extracted rows and the source hash.

Still **not** regionally verified, and labeled `secondary_summary_not_regionally_verified`:
S3 PUT and GET request prices, and data transfer out with its 100 GB allowance. The Resend
prices are `vendor_page_secondary`. A receipt implies no account, credit, free-tier or spend
authority.

The model includes EC2 baseline/burst credits, root/data gp3 storage, IPv4, KMS, Secrets
Manager storage/calls, alarms, full backups plus manifest/COMPLETE objects, email and recall
egress. It reports embedding work in separate units (service counter, provider calls, average
proxies; provider tokens stay unknown), and the provider rate remains unknown pending the
provider pin. Log and custom-metric rates exist but volumes/cardinality remain unknown. Global allowances (the 20,000 KMS requests and the 100 GB
of transfer) are account-wide, so the model does not assume them unused: the known subtotal
applies them, and the peak exposure does not.

`evals/hosted-load/workload.json` preserves the proposed 10-minute warmup,
30-minute steady and 5-minute burst, 8 clients, 4 requests/s baseline, traffic mix,
full advertised cardinalities and three repetitions. Reviewer/date remain unset.
Simulator failures do not measure production capacity, and no thresholds were
changed to turn a simulated failure into a pass.

## Gates and follow-up

T23.41's reviewer must freeze the workload/thresholds, review the backup
exposure and answer the open
[eligible-traffic versus saturation decision](evidence/T23.60/decision-request-full-cardinality-eligible-traffic.md)
before any fixture is treated as a steady-state qualification. T23.60 must complete the live-enable requirements before T23.68 can use
the runner. T23.64 supplies an authorized isolated deployment; T23.42 supplies a
provider/cost pin. No provider calls, resource changes, billing or rollout occurred.

T23.68's verification command passes the load client's `load.json` to `--measurements`. That
file is not the measurement schema, so `cost_model.py` returns `BLOCKED` for it. The registry
and T23.68 are outside this task's write scope, so the request is in
[integration-request-cost-measurements.md](evidence/T23.60/integration-request-cost-measurements.md).
