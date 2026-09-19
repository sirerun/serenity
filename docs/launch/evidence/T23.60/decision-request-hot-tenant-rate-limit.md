# Decision request: hot-tenant load versus the per-account rate limit

Status: **open. T23.60 changed no number.** This request needs a named answer from the task41 (T23.41) reviewer and from the owner of the gateway admission code (T23.45, whose exclusive write scope includes `internal/hosted/gateway/gateway.go` and `admission.go`) before any live run or capacity claim.

## The finding

The frozen workload offers one hot tenant exactly the gateway's per-account request limit, so a healthy server fails two published thresholds.

| Quantity | Value | Source |
|---|---|---|
| Hot tenant offered rate, steady phase | 4 requests/s × 0.50 = 2 requests/s = 120 requests/min | `evals/hosted-load/workload.json` |
| Server limit per account | 120 requests/min, a fixed window that starts at the account's first request | `internal/hosted/gateway/gateway.go` (`allow("account:"+…, 120, …)`), `admission.go` |
| Limit depends on plan | No. Scale, Builder and Free share it | same |
| What counts against it | Every `/mcp` request: `initialize`, the notification, each `tools/call` and `DELETE` | `gateway.go` `ServeHTTP` |
| Per-address limit | 600 requests/min, checked first. The frozen workload stays under it | same |

With the limiter's real window, the frozen schedule (`BASE_SEED`, identical in all three repetitions) produces, per repetition:

| Phase | Offered | Refused by the account limit | Share |
|---|---:|---:|---:|
| warmup | 2,378 | 28 | 1.18% |
| **steady** | **7,379** | **180** | **2.44%** |
| burst (2× rate) | 2,430 | 544 | 22.39% |

Every refusal is the hot tenant (`scale-0`). The steady-phase figure alone is above `unexpected_admission_rejection_max_pct` (1%) and, with the rest of the failures, breaks `min_offered_completion_pct` (99%). The independent load review reached the same 180 of 7,379 with its own code, and `test_the_frozen_hot_tenant_exceeds_the_account_limit_in_the_steady_phase` pins the figures.

The share is not an accident of the seed. Poisson arrivals with a mean equal to the limit exceed it in roughly half of the 60 s windows. Seeds 1 to 5 and the frozen seed give 1.89% to 2.79% of the steady phase, and every one is above the 1% limit.

## What T23.60 did and did not do

- Did: match the simulator's window to `admission.go` (it reset on the wall-clock minute before), add the per-address limit, break outcomes down by phase and account, and record this exposure in `load-fixture.json` (`hot_tenant_rate_limit_exposure`) and in the `qualification_gaps` of every live result.
- Did: keep counting every 429 as a failure. Nothing is excluded or labeled expected saturation. The separate quota-saturation run is for plan-ceiling refusals, and this refusal is the steady rate of one tenant.
- Did not: change any threshold, rate, cap, phase length or tenant fraction. A failed result never authorizes a worker to edit the frozen workload (`workload.json`, `status_note`).
- Limit of the simulator: it counts offered arrivals only. The server also counts setup and cleanup requests, which start each account's window earlier than the first arrival, so live counts can differ by a few requests per window.

## Decisions needed

1. **task41 reviewer (workload owner).** What does the hot tenant represent?
   - **A.** A tenant at its ceiling, on purpose. Then define what the run should observe (for example, refusals kept apart from failures for that tenant), which is a threshold change only you can make.
   - **B.** A heavy tenant under its ceiling. Then lower `hot_tenant_traffic_fraction` or the baseline rate so the hot tenant stays below 120 requests/min with margin for Poisson bursts. That changes the frozen workload and needs your signature in `workload.json`.
   - **C.** Keep the workload and accept that a healthy server fails it. Then the frozen thresholds cannot qualify capacity, and the request should say so in the acceptance criteria.
2. **T23.45 owner (admission code).** Is a plan-independent 120 requests/min per account intended for paid plans? Scale allows 300,000 recalls and 20,000 writes a month against Free's 10,000 and 500, but shares the same request rate. If a Scale tenant at 2 requests/s is legitimate use, the limiter is the defect, not the workload. Any change needs your acceptance evidence and a new run.

## Recommendation

Answer 1 first, because only the reviewer can change the workload, and it determines whether answer 2 is needed at all. If the reviewer picks B, no runtime change is required. If the reviewer picks A or C, the T23.45 owner must state whether the limit is intended, because that decides whether the fix belongs in the workload or the limiter.

Neither answer is a T23.60 worker's to give. T23.68 and every live run stay blocked on both.

## Reproduce

```sh
python3 -m unittest discover -s evals/hosted-load -p "test_harness.py" -k hot_tenant
python3 scripts/hosted/load.py --fixtures --manifest docs/launch/evidence/T23.60/manifest.json --output /tmp/load-fixture.json
```

`hot_tenant_rate_limit_exposure` in the output holds the steady and burst shares.
