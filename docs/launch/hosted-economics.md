# Hosted Serenity economics and load tooling — T23.60

Status: PARTIAL. Source revision and test evidence are recorded in
[evidence/T23.60/result.json](evidence/T23.60/result.json). These are scenario
estimates and local fixture results, not measured capacity or authority to spend.

## Available now

- `scripts/hosted/cost_model.py --manifest PATH --output PATH` calculates idle,
  10-account, 100-account and full-limit paid scenarios, including per-plan contribution.
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

| Scenario | Known subtotal | S3 full-snapshot storage portion |
|---|---:|---:|
| idle_0_accounts | $22.43 | $0.66 |
| 10_accounts_light | $31.74 | $9.94 |
| 100_accounts_light | $112.44 | $90.15 |
| full_limit_mix | $321.93 | $298.95 |

Unknown provider charges, log volume, custom-metric cardinality, restore/list/GET
activity and account-wide free-tier consumption can increase these figures. The
10/100-account mixes and usage fractions are assumptions, not product decisions.
The historical $60/month ceiling remains a ceiling, not new spending permission.

## Backup amplification blocks the full-limit proposal

`backup.sh` uploads a new timestamped prefix hourly. `stack.json` currently expires
current objects after 30 days and their noncurrent versions after another 30 days.
Modeling those phases in series yields approximately 1,441 retained hourly sets,
not 721 versions of one overwritten key. A full paid mix at the advertised storage
allowances plus the assumed control database is 9.02 GB per full backup. At the
verified Oregon first-tier S3 rate, that is about $299/month for stored backup bytes
alone. This is a modeled steady-state exposure, not a measured bill; lifecycle
rounding/asynchronous cleanup, history size and compression require qualification.

The backup/durability and architecture owners must prepare a costed alternative
(frequency, format, retention implementation) that preserves recovery and deletion
requirements. This document does not approve reducing plan limits or retention.

## Rates, units and assumptions

The coordinator retrieved the [AWS regional S3 catalog](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonS3/current/us-west-2/index.json)
and stored its relevant entries and retrieval metadata in [s3-rate.json](evidence/T23.60/s3-rate.json).
Oregon Standard first 50 TB is $0.023/GB-month; the worker's earlier $0.0265 regional
claim was incorrect. T4g surplus credits use the [published $0.04/vCPU-hour](https://aws.amazon.com/ec2/instance-types/t4/).
Other table entries retain their cited sources/caveats and require region/account
verification before provisioning. In particular, t4g.small hourly pricing remains
an explicit regional-parity assumption; no complete spend-ready quote is claimed.

The model includes EC2 baseline/burst credits, root/data gp3 storage, IPv4, KMS,
Secrets Manager storage/calls, alarms, full backups plus manifest/COMPLETE objects,
email and data transfer. It separately reports write/query/rebuild/readiness
embedding tokens; provider rate remains unknown pending the provider pin. Log and
custom-metric rates exist but volumes/cardinality remain unknown. Free allowances
must be confirmed available in the actual shared AWS account.

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
