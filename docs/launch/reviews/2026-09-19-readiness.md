# Hosted launch: measured constraints and decisions still required

2026-09-19 UTC. **Review input, not launch acceptance or an approved amendment.** The paid scope, ratified plan allowances, 34-task dependency graph and external gates remain unchanged. [PR236](https://github.com/sirerun/serenity/pull/236) at `218d7234d9abea5964f9d4640d1bfffe5c9f8087` contains the four proposed architecture decisions; none has a named ruling. Its 13 CI checks pass, which does not accept those decisions.

## Measured runtime behavior

A headless audit exercised the real hosted service and ordinary remember/recall handlers with a loopback three-dimensional embedding fixture, synthetic text, one account and one brain. The product binary was built at `7b8854945f94a5ffab28169f6cc6cf3df972d8c2`; its `internal/**` and `cmd/**` code matches main `810349ba7c1ed2c05fe34e3892de764a26a4633c`. Environment: macOS arm64, Go 1.27.1, Python 3.14, Apple Git 2.54.0, load1 roughly 2–4.5. The coordinator independently recomputed the latency medians and failure counts from saved output.

- 1,500 paced remembers completed with HTTP 200 and no reported operation errors. Median latency in the first 100 was 117 ms; in the last 100 it was 621 ms. This establishes a scaling concern, not performance at the 50,000-memory Scale limit or on the target Linux VM. Profiling is pending.
- At about 1,540 facts, single-connection recall took roughly 0.84–0.90 s. In controlled concurrent traffic, seven calls drained in about 5.63 s without backup and 5.71 s with backup. The backup's own idle-start wall time was 0.15–0.18 s; starting beside an in-flight call extended backup wall time to about 1.7 s. Waiting for the maintenance lock and holding it are distinct. The 80 controlled probes had no HTTP or tool errors.
- The source/allocated-size distinction matters: at 1,500 facts the brain was about 10.6 MB logical while open and 26.7 MB allocated. Source files alone used 12.3 MB allocation for 0.76 MB logical bytes. Git packing introduced multi-megabyte temporary jumps in the separate 5,000-commit layout experiments. A mean or percentile growth multiplier cannot establish a physical hard bound.
- The full control database grew about 1.0 KB per write in this fixture. Committed reservations and audit rows have no time-based pruning in the inspected code; only old released reservations are removed. Recall-row growth is estimated from row sizes, not measured as a separate slope. This database lies outside per-brain storage quotas and is copied into every snapshot, so a fixed 20 MB allowance is not a lifetime upper bound.

The entity-page and emulated-layout experiments are separate from ordinary memory writes. Two transient Git failures occurred in those harness-driven commits; they recovered on retry, but do not establish a production failure rate. A first contention attempt shared an HTTP connection between threads and was invalid; its results are excluded. The successful contention measurements use separate pre-initialized connections and interleaved no-backup controls. Three-dimensional fixture vectors do not qualify semantic quality, production vector-size cost, or provider latency.

## Workload conflict requiring review

The frozen workload gives one tenant 50% of 4 requests/s. The current gateway limits every account to 120 requests/minute. Replaying the frozen deterministic Poisson schedule through the current first-request-anchored limiter predicts 180 rejected steady requests out of 7,379 per repetition (2.44%), before setup overhead. This conflicts with the frozen <1% admission-rejection and >=99% completion targets even when no server defect is injected.

**Requested decision:** task 41's reviewer and the task 45 admission owner must reconcile the existing service limit with the frozen workload. Do not silently lower the workload, raise the rejection threshold, or move steady refusals into the separate saturation sample. The simulator also needs the actual window semantics rather than wall-minute buckets. Client classification repairs belong to task 60: the gateway returns capacity/quota errors inside HTTP 200 tool results, so counting only HTTP 429 falsely reports zero admission refusal; a refused forget is not observed durability loss.

## Backup alternative for architecture review

Current hourly full snapshots with 30-day current plus 30-day noncurrent lifecycle retention yield a modeled known subtotal near $322/month at the full-limit mix. Task 52's active purge near 29 days reduces that modeled subtotal to about $168, still above the historical $60 ceiling. These figures are assumptions-based exposure, not incurred charges.

**Unapproved proposal:** retain one self-contained full capture every hour, keep hourly recovery points for 24 hours, keep one daily full for up to 29 days, and retain task 52's scheduled all-version purge and success verification. Classify the daily hour's capture as daily rather than duplicating it. Restore remains independent of incremental chains. The older recovery-point granularity changes and requires an explicit ruling.

An illustration allowing 55 simultaneous full sets (24 hourly + 29 daily + 2 boundary sets) at the previously retrieved Oregon Standard rate of $0.023/GB-month gives:

| Scenario | Assumed full snapshot GB | Backup storage/month | Known subtotal/month |
|---|---:|---:|---:|
|10 light accounts|0.30|$0.38|$22.18|
|100 light accounts|2.72|$3.44|$25.73|
|1Scale+3Builder+10Free full limit|9.02|$11.41|$34.39|

This swaps only the backup-storage category in task 60's model. It does not bound delayed purge, lifetime control-DB growth, provider charges or telemetry; unknown categories still block cost qualification. Thinning retained sets also leaves hourly capture CPU and upload work unchanged. No increase to the budget, reduction of customer allowances, or provisioning is requested here. Regional source: [AWS Oregon S3 price catalog](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonS3/current/us-west-2/index.json).

### Peak exposure omitted by the illustrative subtotal

The current model assigns only $1.168/month to surplus CPU credits; that is a usage assumption, not a peak bound. The template does not set `CreditSpecification`, so the account's default must be verified. AWS normally defaults T4g to Unlimited. With depleted credit balance, two vCPUs held at 100% against the 20%-per-vCPU baseline imply `2 × (1 − 0.20) × 730 × $0.04 = $46.72/month` of surplus credits. At 70% average CPU the analogous figure is $29.20. These are steady-state sensitivities, not measured CPU use. [AWS credit definitions](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-credits-baseline-concepts.html).

Replacing $1.168 with $46.72 makes the 55-set full-limit illustration about **$79.94/month before other unknowns**. A cheaper retention policy alone therefore does not establish compliance with the $60 ceiling. Standard credit mode would trade that charge for throttling and still need performance qualification; no credit-mode or topology change is made here.

The template also enables KMS key rotation. [AWS charges another $1/month for each of the first two rotations](https://aws.amazon.com/kms/pricing/), so the key-storage component can rise from $1 to $3/month. Account-wide free KMS requests and transfer allowances must not be assumed unused by other workloads. Control-DB growth, provider work and telemetry remain additional unqualified costs.

The coordinator has now verified regional EC2, gp3, CPU-credit, KMS, Secrets Manager, IPv4 and CloudWatch rates directly from AWS data. `t4g.small` remains $0.0168/hour; the gp3 storage rate remains $0.08/GB-month. The numeric rates are less uncertain; the current workload and lifetime assumptions are still insufficient for a conservative maximum. Saved extracts: `ec2-oregon-rate-receipt.json`, `ec2-ebs-oregon-rate-extract.json`, and `aws-regional-rates-selected.json`. The full regional EC2 source was streamed and hashed at 474,950,676 bytes, source version `20260918212757`, SHA256 `ec1e0d93973aa692ae4c350df1f52688b0c5b76abc2523a2d536e374dca8e904`.

Lifecycle-only expiry is not an alternative guarantee: [AWS documents asynchronous removal](https://docs.aws.amazon.com/AmazonS3/latest/userguide/lifecycle-expire-general-considerations.html). Keep the scheduled source-age cutoff, all-version/delete-marker inventory, owned-prefix checks, failure status and post-purge verification. Exclude the independent deletion journal from snapshot purge. A failed purge does not support a 30-day erasure claim.

**Requested decision:** approve this thinning policy with active purge, amend the windows, or require another independently costed design. Also assign control-DB retention/compaction ownership with explicit retry, billing and audit semantics; do not delete accounting history merely to meet a cost estimate. Task 49 owns verified manifests, 52 owns backup/purge schedules, 54 owns scoped infrastructure, and 66/68 own live recovery and cost qualification after their gates.

## Additional source preparation

[Compute options](2026-09-19-compute-options.md) compares current Oregon on-demand base charges for the existing burstable instance and fixed-compute candidates. It does not select a new machine or establish capacity. [Provider sources](2026-09-19-provider-sources.md) and the [selected public catalog rows](2026-09-19-provider-catalog-extract.json) now identify the candidate Perplexity embedding endpoint in OpenRouter's endpoint-specific ZDR list. This improves the privacy source trail; actual account policy, route enforcement, model identity and quality remain unqualified. All provider requests in this source check were public metadata reads without credentials or inference.

## Evidence and remaining work

Raw local artifacts are retained in the coordinator's launch run directory; no credentials or customer data are included. This document records their hashes so later qualification cannot quietly substitute new measurements. These are audit artifacts, not task-completion receipts.

| Artifact | SHA256 |
|---|---|
|`launch-audit-real-writer-e4-writes.json`|`7f751bad33eb98177f428ec16ba6db329135fe127b743e4a82b642674c0b0ca8`|
|`launch-audit-real-writer-e4.out.json`|`69265f5b132032a83a39f98280bdcb2bb063b031422f331d8a7d482f59a5a299`|
|`launch-audit-real-writer-e4b.out.json`|`ef4c1c60667b4efe576e9844bfb2ada8d9a31e9456ca03faad7fdb9cdf0ad717`|
|`T23.41-independent-load-review-repro-schedule.py`|`2b63de03410f45913863582358c6008c0738c5b18c04e44e022144e6c802b02e`|

Local baseline reproductions additionally confirm embedding error-body reflection, unverified response model identity, and redirect replay of synthetic input/auth to another loopback port. Task 42 owns the provider repair once task 41 is accepted. The current website privacy page covers adoption chat; task 42/55/56 must provide a qualified hosted-memory disclosure rather than reuse that scope.

Next work remains source-pinned implementation review, the named task 41 decisions, the rest of the dependency graph, and authorized real-provider/infrastructure/browser qualification. Founder account/secret references and controlled-test prerequisites remain on the requested founder surface. No provider spend, cloud mutation, public activation or merge occurred in this audit.
