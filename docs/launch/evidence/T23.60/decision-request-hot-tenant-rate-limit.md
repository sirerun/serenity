# Decision request: hot-tenant load versus the per-account rate limit

Status: **workload and threshold ruling received; T23.45 admission work remains open.** The chief architect ruled on PR #239 that the frozen workload and `<1%` unexpected-admission threshold stand unchanged. No live run or capacity claim is qualified until T23.45 reconciles the account limiter and the frozen workload is rerun.

## Ruling received

The [chief-architect ruling on PR #239](https://github.com/sirerun/serenity/pull/239#issuecomment-5746461468) keeps the 50%-hot-tenant workload and `<1%` threshold exactly as frozen. Do not lower the offered rate or hot-tenant fraction, and do not raise the rejection threshold. The T23.45 admission owner must reconcile the plan-independent account limit with this valid Scale workload before T23.68; the reviewed options include a higher qualification-window account ceiling or excluding the hot tenant from that per-account bucket. T23.45 owns the implementation and acceptance evidence.

## The finding

The frozen workload offers one hot tenant exactly the gateway's per-account request limit. The account rate limit alone refuses 180 of 7,379 steady requests (2.44%); the threshold-matching admission total is 204 of 7,379 (2.76%) after adding 24 pool-capacity refusals. Both exceed the 1% steady admission threshold, and the steady completion rate also remains below its 99% minimum.

| Quantity | Value | Source |
|---|---|---|
| Hot tenant offered rate, steady phase | 4 requests/s × 0.50 = 2 requests/s = 120 requests/min | `evals/hosted-load/workload.json` |
| Server limit per account | 120 requests/min, a fixed window that starts at the account's first request | `internal/hosted/gateway/gateway.go` (`allow("account:"+…, 120, …)`), `admission.go` |
| Limit depends on plan | No. Scale, Builder and Free share it | same |
| What counts against it | Every `/mcp` request: `initialize`, the notification, each `tools/call` and `DELETE` | `gateway.go` `ServeHTTP` |
| Per-address limit | 600 requests/min, checked first. The frozen workload stays under it | same |

With the limiter's real window, the frozen schedule (`BASE_SEED`, identical in all three repetitions) produces, per repetition:

| Phase | Offered | Account rate-limit refusals | Pool-capacity refusals | All admission refusals | Threshold-matching share |
|---|---:|---:|---:|---:|---:|
| warmup | 2,378 | 28 | 3 | 31 | 1.30% |
| **steady** | **7,379** | **180** | **24** | **204** | **2.76%** |
| burst (2× rate) | 2,430 | 544 | 67 | 611 | 25.14% |

Every account rate-limit refusal is the hot tenant. The threshold-matching admission total also includes pool-capacity refusals; the steady 204/7,379 figure is above `unexpected_admission_rejection_max_pct` (1%) and completion remains below `min_offered_completion_pct` (99%). The independent load review reached the same 180 account-limit refusals, and the tests now pin both the account-limit component and the 204 total.

The share is not an accident of the seed. Poisson arrivals with a mean equal to the limit exceed it in roughly half of the 60 s windows. Seeds 1 to 5 and the frozen seed give 1.89% to 2.79% of the steady phase, and every one is above the 1% limit.

## What T23.60 did and did not do

- Did: match the simulator's window to `admission.go` (it reset on the wall-clock minute before), add the per-address limit, break outcomes down by phase and account, scope threshold calculations to steady arrivals, and record both account-rate-limit and all-admission totals in `load-fixture.json` (`hot_tenant_rate_limit_exposure`) and in the `qualification_gaps` of every live result.
- Did: keep counting every 429 as a failure. Nothing is excluded or labeled expected saturation. The separate quota-saturation run is for plan-ceiling refusals, and this refusal is the steady rate of one tenant.
- Did not: change any threshold, rate, cap, phase length or tenant fraction. A failed result never authorizes a worker to edit the frozen workload (`workload.json`, `status_note`).
- Limit of the simulator: it counts offered arrivals only. The server also counts setup and cleanup requests, which start each account's window earlier than the first arrival, so live counts can differ by a few requests per window.

## Remaining owner action

**T23.45 owner (admission code):** implement and qualify the account admission behavior against the frozen workload. The current plan-independent 120 requests/min bucket is shared by Free, Builder and Scale despite their different monthly allowances. Choose the scoped runtime behavior within T23.45, retain the workload/threshold unchanged, and attach a passing run before T23.68. The architecture ruling does not qualify a live run or authorize a broader production limit change by itself.

## Recommendation

The workload owner decision is settled: keep the frozen arrivals, tenant mix and `<1%` target. The remaining action belongs to T23.45. T23.68 and every live run stay blocked until that admission implementation is qualified.

## Reproduce

```sh
python3 -m unittest discover -s evals/hosted-load -p "test_harness.py" -k hot_tenant
python3 scripts/hosted/load.py --fixtures --manifest docs/launch/evidence/T23.60/manifest.json --output /tmp/load-fixture.json
```

`hot_tenant_rate_limit_exposure` in the output holds the account-rate-limit component and the threshold-matching steady admission share. Threshold evaluation excludes warmup and burst phases.
