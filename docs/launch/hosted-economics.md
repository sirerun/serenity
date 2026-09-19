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
  requests a minute), so a healthy server fails the admission and completion
  thresholds: 180 of 7,379 steady requests, 2.44%. The client and the simulator
  count these as failures, not as expected saturation. No number changed. A named
  decision is open in
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
- **`--live` is disabled unconditionally**, returning BLOCKED/exit 2 and a zero-call
  receipt before manifest/credential reads or networking. Seeded state, a real
  cold-open workload, host telemetry, isolation probes and the reviewer freeze
  remain open in
  [live-enable-requirements.md](evidence/T23.60/live-enable-requirements.md).

## Candidate monthly cost

| Scenario | Known subtotal | S3 full-snapshot storage portion | Peak, sustained 100% CPU | Peak, 70% mean CPU |
|---|---:|---:|---:|---:|
| idle_0_accounts | $22.43 | $0.66 | $71.15 | $53.63 |
| 10_accounts_light | $31.74 | $9.94 | $80.50 | $62.98 |
| 100_accounts_light | $112.44 | $90.15 | $161.30 | $143.78 |
| full_limit_mix | $322.00 | $298.95 | $369.87 | $352.35 |

The known subtotal prices the categories that have a rate and an assumed quantity. The
peak columns replace four of its lines with their worst case (see
[Peak exposure](#peak-exposure)) and add nothing else. Neither column is a full maximum:
[the unknowns](#what-stays-unknown) are outside both.

Unknown provider charges, log volume, custom-metric cardinality, restore/list/GET
activity, multipart requests, control database growth and account-wide free-tier
consumption can increase these figures. The 10/100-account mixes and usage fractions are
assumptions, not product decisions. The historical $60/month ceiling remains a ceiling, not
new spending permission.

The full mix counts every allowed brain (10 Free, 9 Builder and 10 Scale, 29 in all) in
each backup. That gives 32 objects a backup (`manifest.json`, `control.db`, `COMPLETE` and
29 bundles) and 23,040 PUT requests a month. The 10- and 100-account scenarios model each
account's default brain only, and each reports its allowed-brain ceiling.

## Backup amplification blocks the full-limit proposal

`backup.sh` uploads a new timestamped prefix hourly. `stack.json` currently expires
current objects after 30 days and their noncurrent versions after another 30 days.
Modeling those phases in series yields approximately 1,441 retained hourly sets,
not 721 versions of one overwritten key. A full paid mix at the advertised storage
allowances plus the assumed control database is 9.02 GB per full backup. At the
verified Oregon first-tier S3 rate, that is about $299/month for stored backup bytes
alone. This is a modeled steady-state exposure, not a measured bill; lifecycle
rounding/asynchronous cleanup, history size and compression require qualification.

The 9.02 GB is 9 GB of customer storage at the advertised allowances plus the 20 MB control
database assumed at snapshot time, which is not a lifetime bound
([Control database growth](#control-database-growth)). The baseline is the retention the
deployed template sets.

The backup/durability and architecture owners must prepare a costed alternative
(frequency, format, retention implementation) that preserves recovery and deletion
requirements. This document does not approve reducing plan limits or retention.

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

The peak of the full mix is $369.87 at sustained 100% CPU (322.00 − 1.168 − 1.00 − 0.009 +
46.72 + 3.00 + 0.069 + 0.262). The 55-set backup thinning illustration in PR 239 is an
unapproved sensitivity. It is not the baseline and is not modeled here: the baseline is the
deployed template's 30-plus-30-day retention.

## Control database growth

`control.db` is copied whole into every snapshot, and no runtime code prunes committed
reservations or `audit_log` rows. The real-writer audit measured about 1.0 KB per lifetime
write, and each recall adds a reservation row and an audit row. The model's 20 MB is a
scenario assumption at snapshot time, not a lifetime bound, and the model does not replace it
with the sampled slope as a bound.

Each scenario reports the growth as an unknown category with a sensitivity, using sampled row
sizes (301 B a reservation row, 229 B an audit row; 1,060 B a write, 530 B a recall) from a
sample of one account, one brain, 1,500 writes and 11 recalls with a stub embedder. A real
provider's audit rows may be larger. The table assumes every month runs at the scenario's
usage, nothing is pruned, and every retained backup set carries the grown size:

| Scenario | Growth a month | Added S3 cost after 1 month | Database after 12 months | Added S3 cost after 12 months | Added S3 cost after 24 months |
|---|---:|---:|---:|---:|---:|
| idle_0_accounts | 0.0 MB | 0.00 | 0.02 GB | 0.00 | 0.00 |
| 10_accounts_light | 16.3 MB | 0.54 | 0.22 GB | 6.49 | 12.98 |
| 100_accounts_light | 135.2 MB | 4.48 | 1.64 GB | 53.75 | 107.50 |
| full_limit_mix | 413.4 MB | 13.70 | 4.98 GB | 164.42 | 328.83 |

Amounts are USD a month. The full mix reaches about $164 a month of added backup storage
after a year. It is outside every subtotal and every peak. A pruning or retention change needs
its own review, and this document does not propose one.

## What stays unknown

The known subtotal and the peaks leave these out, and `full_maximum_computable` is false
until they are priced:

- Embeddings: the provider pin (task42) is open.
- CloudWatch log volume and custom-metric cardinality: the rates are known and the volumes are
  not (T23.53).
- S3 GET, list and restore activity.
- Control database lifetime growth, above.
- S3 multipart requests: `aws s3 cp` uploads a large object in parts and each part is a
  request. At the CLI default 8 MiB chunk the full mix would add about $3.76 a month. The
  deployed CLI's setting is not in the template, and KMS requests per part are unverified.
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
`partly_measured` because the retained-set count stays modeled, and the KMS request line stays
an assumption. Everything else is priced on assumptions. The run is `BLOCKED`, with a
sanitized reason and no output file written by that run, for any of the following:

- A boolean, string, `NaN`, `Infinity`, negative, zero-where-meaningless or oversized value, an
  integer quantity given as a float, or a wrong container.
- A missing, ambiguous or mismatched unit, an unknown scenario or quantity, a repeated scenario
  and quantity, or an extra key.
- A source without a kind, reference and SHA-256, or an observation time that is not a past
  RFC 3339 UTC timestamp.
- A file that is not this schema (a load result, `{}` or an empty `records` list), is not valid
  JSON, repeats a key, or is larger than 1 MiB.

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
| S3 Standard, first 50 TB | $0.023/GB-month | none |
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
Manager storage/calls, alarms, full backups plus manifest/COMPLETE objects, email and data
transfer. It separately reports write/query/rebuild/readiness embedding tokens; provider rate
remains unknown pending the provider pin. Log and custom-metric rates exist but
volumes/cardinality remain unknown. Global allowances (the 20,000 KMS requests and the 100 GB
of transfer) are account-wide, so the model does not assume them unused: the known subtotal
applies them, and the peak exposure does not.

`evals/hosted-load/workload.json` preserves the proposed 10-minute warmup,
30-minute steady and 5-minute burst, 8 clients, 4 requests/s baseline, traffic mix,
full advertised cardinalities and three repetitions. Reviewer/date remain unset.
Simulator failures do not measure production capacity, and no thresholds were
changed to turn a simulated failure into a pass.

## Gates and follow-up

T23.41's reviewer must freeze the workload/thresholds and review the backup
exposure. T23.60 must complete the live-enable requirements before T23.68 can use
the runner. T23.64 supplies an authorized isolated deployment; T23.42 supplies a
provider/cost pin. No provider calls, resource changes, billing or rollout occurred.

T23.68's verification command passes the load client's `load.json` to `--measurements`. That
file is not the measurement schema, so `cost_model.py` returns `BLOCKED` for it. The registry
and T23.68 are outside this task's write scope, so the request is in
[integration-request-cost-measurements.md](evidence/T23.60/integration-request-cost-measurements.md).
