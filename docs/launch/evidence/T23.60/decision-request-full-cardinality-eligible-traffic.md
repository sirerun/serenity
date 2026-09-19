# Decision request: full advertised cardinality versus eligible steady traffic

Status: **open. T23.60 changed no number.** This request needs a named answer from the task41 (T23.41) reviewer before any seeded fixture is treated as a steady-state qualification. It changes no workload value, entitlement, threshold or acceptance criterion.

## The finding

The frozen paid profile starts every account at its advertised memory count (90,000 in all). The frozen mix offers 15% remembers and 5% forgets, so it adds memories faster than it removes them. At the cap the gateway refuses every new remember, so part of the steady sample stops being eligible traffic.

| Quantity | Value | Source |
|---|---|---|
| Starting inventory in the frozen profile | 90,000 memories: 1 Scale (50,000), 3 Builder (10,000 each), 10 Free (1,000 each), spread over 29 brains | `evals/hosted-load/workload.json` `cardinalities.paid`, `internal/hosted/plans/plans.go` |
| Memory limit | account-wide, not per brain: 29 brains share the 90,000 | `internal/hosted/gateway/gateway.go` `inventory` |
| Rule | a nonreplay remember is refused with `limit_exceeded` when live memories are at or above `Plan.Memories` (the same check refuses when storage bytes reach `Plan.StorageBytes`) | `gateway.go` |
| Mix | 80% recall, 15% remember, 5% forget: net +0.10 memory per offered request | `workload.json` `traffic_mix` |

A deterministic replay of the frozen first repetition (`BASE_SEED`, the same arrivals in every repetition) with every account starting at its cap. It models the memory-count rule only. It does not model the rate limiter, the storage rule, the monthly write cap, provider faults, cold opens, races or latency, and it is not a measured runtime failure.

| Phase | Offered | Remembers | Forgets | Remembers refused at the cap | Share of all offered |
|---|---:|---:|---:|---:|---:|
| warmup | 2,378 | 352 | 117 | 246 | 10.34% |
| **steady** | **7,379** | **1,130** | **396** | **726** | **9.84%** |
| burst (2× rate) | 2,430 | 380 | 128 | 256 | 10.53% |

This is the coordinator's replay and it reproduces exactly (`coordinator-full-cardinality-schedule.json`, 726 of 7,379, 89,993 memories at the end). It gives each forget the most favorable reading: a forget may remove a preseeded fact.

The current load client is less favorable. It forgets only an id it remembered (`scripts/hosted/load.py`, `SKIPPED_FORGET`). At the cap it can remember nothing, so it also has nothing to forget:

| Steady phase, starting at the cap | Forget removes any fact (coordinator replay) | Current client |
|---|---:|---:|
| Remembers refused (`limit_exceeded`) | 726 | 1,130 |
| Forgets skipped (no id remembered by this run) | 0 | 396 |
| Most the phase can complete if every other request succeeds | 90.16% | 79.32% |
| Frozen `min_offered_completion_pct` | 99% | 99% |

Both readings miss the completion threshold on this rule alone, before any server fault.

## What this is not

- **It is not the unexpected-admission statistic.** The client decodes `limit_exceeded` into the fixed class `quota` (`TOOL_ERROR_CLASSES`), records it as outcome `tool_error`, and keeps it in the offered denominator. It is not `rejected_admission`, so it does not count toward `unexpected_admission_rejection_max_pct`. Its actual class and its place in the total offered stay unchanged. Do not relabel it.
- **It is not a reason to drop offered rows.** The skipped forgets and refused remembers stay in the denominator. The client's forget seam (it cannot forget a seeded fact) is unfinished work, not a reason to exclude requests.
- **It is not a defect in the server.** Refusing a remember at the plan ceiling is the intended behavior. The question is what the eligible sample is.

## How much headroom the frozen mix needs

Each account needs, below its cap, the peak net growth of the scope it runs, so that no remember is refused. The table starts every account at its cap minus that peak (per account, so the total below is the sum of peaks). `cardinality-headroom.json` holds the per-account figures and `evals/hosted-load/cardinality_headroom.py` reproduces them.

| Scope | Headroom (forget removes any fact) | Start at cap minus headroom | Ends at | Headroom (current client) |
|---|---:|---:|---:|---:|
| Steady phase only | 737 | 89,263 | 89,997 | 739 |
| Warmup and steady | 972 | 89,028 | 89,997 | 977 |
| One repetition (warmup, steady, burst) | 1,228 | 88,772 | 89,993 | 1,233 |
| Three repetitions, state not reset | 3,670 | 86,330 | 89,993 | 3,675 |

For one repetition the headroom splits by plan as Scale 626 (the hot tenant, `scale-0`, 1.25% of 50,000), Builder 154 (0.51% of 30,000) and Free 448 (4.48% of 10,000). One repetition adds 1,862 remembers and removes 641 forgets, a net +1,221 memories.

## The storage and history gap

Filling the memory count does not fill the storage quota, and the two need separate labels.

- The gateway refuses when `inventory.StorageBytes` reaches `Plan.StorageBytes`, and that count is every file under the brain directories: canonical facts, Git history and the derived index.
- A fact is at most 4,096 bytes (`gateway.go`). At that size the fact text of a full plan is 4.1 MB for Free, 41 MB for Builder and 205 MB for Scale, which is 4.096% of each plan's 100 MB, 1 GB and 5 GB. The record envelope, provenance, Git history and derived vectors add to that. Their size is unmeasured, so the arithmetic is a floor on the gap and not a measurement.
- A fixture at exactly 90,000 memories therefore represents the memory-count boundary and not the storage boundary. Reaching the storage quota needs history or derived-index growth that the frozen mix does not offer. A storage-saturation sample is a separate design question, and this request leaves it open.

## What T23.60 did and did not do

- Did: reproduce the coordinator's replay, add the headroom scopes and the current-client reading, and pin them with tests (`test_cardinality_headroom.py`).
- Did: keep `workload.json`, the 90,000 facts, the 29 brains, the 15%/5% mix, every advertised entitlement and every threshold unchanged. It does not reuse idempotency keys as new remembers, and it raises no ceiling.
- Did not: decide the sample, lower the starting count, or label any seeded fixture a steady-state qualification. `--live` stays unconditionally blocked.
- The offline fixture preparation that follows this request can represent exactly 90,000 memories over 29 brains and show the inventory by an independent count. Its receipt says so and says it is not eligible-steady qualification. It reports the measured storage bytes against the quotas as the storage and history gap, separately from the memory count.

## Decisions needed

1. **task41 reviewer (workload owner).** What is the eligible steady sample, and what is the boundary sample?
   - **A (recommended).** Two named samples with exact cardinalities. The eligible sample starts each account at its cap minus the headroom for the scope you choose (for one repetition, 88,772 memories, ending at 89,993) and runs the frozen mix unchanged. The boundary sample starts at exactly 90,000, runs after it as the workload's separate quota-saturation run (`quota_boundary_saturation.separate_run`), and expects `limit_exceeded` as class `quota`. This reads 90,000 as the ceiling of the eligible sample and the start of the boundary sample. It changes the frozen starting inventory of the eligible sample by 1.4%, so it needs your signature in `workload.json`.
   - **B.** Keep 90,000 as the eligible sample's start and change the mix so it no longer adds memories (equal remember and forget rates or more forgets). That changes the frozen 15%/5% mix.
   - **C.** Run the frozen mix only from 90,000 and accept that it cannot qualify eligible steady traffic. Then the acceptance criteria state that the completion threshold cannot be met by construction, and full cardinality is a boundary sample only.
   - **Not offered:** raising plan entitlements for the fixture, reusing idempotency keys as new remembers, or raising ceilings. Each hides the finding instead of answering it.
2. **task41 reviewer.** Does a repetition restore the seeded state before it starts? If it does, size the headroom on one repetition (1,228). If state carries across the three repetitions, it needs 3,670, because every repetition replays the same arrivals and adds +1,221.
3. **task41 reviewer.** Which phases enter the eligible statistic? The client evaluates the steady phase only, so warmup and burst still change the inventory that steady starts from. Warmup and steady need 972 headroom, and all three phases need 1,228.
4. **Load client owner.** The client forgets only ids it remembered, so a seeded fixture's forgets are skipped. Either the fixture supplies the seeded ids to the client (an integration change outside this task's write scope), or the skipped forgets stay in the accounting as `skipped_forget_no_fact`. Say which.
5. **task41 reviewer.** How is the storage and history gap labeled in the acceptance criteria, and does a storage-saturation sample belong in scope?

## Recommendation

Choose A for the sample, and answer 2 and 3 in the same signature, because they fix the exact starting and ending cardinalities. A keeps every advertised entitlement, the 29 brains and the frozen mix, and it keeps refusals at the ceiling in their own class in a run built to observe them. It is the smallest change that makes the eligible sample eligible. Answer 4 decides whether the fixture must carry seeded ids.

None of this is a T23.60 worker's decision. T23.68 and every live run stay blocked on it.

## Reproduce

```sh
python3 evals/hosted-load/cardinality_headroom.py --output /tmp/cardinality-headroom.json
python3 -m unittest evals/hosted-load/test_cardinality_headroom.py
```

The output is byte-identical to [cardinality-headroom.json](cardinality-headroom.json). The coordinator's replay is `coordinator-full-cardinality-schedule.py` in the run directory (not committed).
