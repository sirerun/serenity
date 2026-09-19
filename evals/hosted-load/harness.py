"""Deterministic synthetic simulator for the T23.60 bounded capacity workload.

This models the real hosted admission/pool/writer shape documented in
internal/hosted/gateway/gateway.go, internal/hosted/gateway/admission.go,
internal/hosted/pool/pool.go and internal/hosted/service/service.go (the
production defaults MAX_OPEN=8, MAX_IN_FLIGHT=16, per-account rate limit
120/min, immediate rejection with no queueing) so --fixtures rejection and
contention shape are grounded in the real code, not invented. It never spawns
a real Go process, opens a real brain, or calls a real provider: latency and
resource numbers are a synthetic cost function, never a measured host or
provider metric. Keep the plan allowances in PLAN_ALLOWANCES in sync with
internal/hosted/plans/plans.go V1 by hand; there is no cross-language import.
"""
from __future__ import annotations

import hashlib
import json
import random
from dataclasses import dataclass, field
from pathlib import Path

# Real production defaults, internal/hosted/service/service.go DefaultConfig.
MAX_OPEN = 8
MAX_IN_FLIGHT = 16
IDLE_TIMEOUT_S = 600
ACCOUNT_RATE_LIMIT_PER_MIN = 120
IP_RATE_LIMIT_PER_MIN = 600

# internal/hosted/plans/plans.go V1; id -> (monthly_cents, brains, memories, writes, recalls, input_tokens, storage_bytes).
PLAN_ALLOWANCES = {
    "free": {"monthly_cents": 0, "brains": 1, "memories": 1000, "writes": 500, "recalls": 10000, "input_tokens": 100000, "storage_bytes": 100_000_000},
    "builder": {"monthly_cents": 1900, "brains": 3, "memories": 10000, "writes": 5000, "recalls": 100000, "input_tokens": 1_000_000, "storage_bytes": 1_000_000_000},
    "scale": {"monthly_cents": 4900, "brains": 10, "memories": 50000, "writes": 20000, "recalls": 300000, "input_tokens": 4_000_000, "storage_bytes": 5_000_000_000},
}


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_workload(path: Path) -> dict:
    workload = json.loads(path.read_text())
    for field_name in ("phases", "concurrency", "traffic_mix", "cardinalities", "thresholds", "repetitions"):
        if field_name not in workload:
            raise ValueError(f"workload manifest missing required field: {field_name}")
    mix_total = sum(workload["traffic_mix"].values())
    if abs(mix_total - 1.0) > 1e-9:
        raise ValueError(f"traffic_mix must sum to 1.0, got {mix_total}")
    return workload


def build_accounts(cardinalities: dict, profile: str) -> list[dict]:
    mix = cardinalities.get(profile)
    if not mix or "accounts" not in mix:
        raise ValueError(f"cardinalities has no frozen account mix for profile {profile!r}")
    accounts = []
    for plan_id, count in mix["accounts"].items():
        if plan_id not in PLAN_ALLOWANCES:
            raise ValueError(f"unknown plan id in cardinalities: {plan_id}")
        for i in range(count):
            accounts.append({"id": f"{plan_id}-{i}", "plan": plan_id, "allowance": PLAN_ALLOWANCES[plan_id]})
    if not accounts:
        raise ValueError("account mix produced zero accounts")
    return accounts


@dataclass
class RateLimiter:
    """Reproduces the fixed one-minute bucket in internal/hosted/gateway/admission.go."""
    limit: int
    buckets: dict = field(default_factory=dict)

    def allow(self, key: str, now_s: float) -> bool:
        minute = int(now_s // 60)
        window_start, count = self.buckets.get(key, (minute, 0))
        if window_start != minute:
            window_start, count = minute, 0
        if count >= self.limit:
            self.buckets[key] = (window_start, count)
            return False
        self.buckets[key] = (window_start, count + 1)
        return True


@dataclass
class Pool:
    """Reproduces the immediate-rejection admission in internal/hosted/pool/pool.go Acquire.

    Tracks a per-brain active-call count (mirrors Runtime.users), because the
    real pool only evicts a brain with zero active users -- a brain mid-call
    is never evicted out from under itself.
    """
    max_open: int
    max_in_flight: int
    idle_timeout_s: float
    open_brains: dict = field(default_factory=dict)  # id -> {"last": now_s, "active": int}
    in_flight: int = 0

    def acquire(self, brain_id: str, now_s: float) -> tuple[bool, bool]:
        """Returns (admitted, cold_open). Never blocks: capacity denial is immediate."""
        if self.in_flight >= self.max_in_flight:
            return False, False
        for bid, info in list(self.open_brains.items()):
            if bid != brain_id and info["active"] == 0 and now_s - info["last"] >= self.idle_timeout_s:
                del self.open_brains[bid]
        cold = brain_id not in self.open_brains
        if cold and len(self.open_brains) >= self.max_open:
            evictable = [b for b, info in self.open_brains.items() if b != brain_id and info["active"] == 0]
            if not evictable:
                return False, False
            oldest = min(evictable, key=lambda b: self.open_brains[b]["last"])
            del self.open_brains[oldest]
        entry = self.open_brains.setdefault(brain_id, {"last": now_s, "active": 0})
        entry["active"] += 1
        entry["last"] = now_s
        self.in_flight += 1
        return True, cold

    def release(self, brain_id: str, now_s: float) -> None:
        self.in_flight -= 1
        entry = self.open_brains.get(brain_id)
        if entry:
            entry["active"] -= 1
            entry["last"] = now_s


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    k = (len(ordered) - 1) * (pct / 100.0)
    lo, hi = int(k), min(int(k) + 1, len(ordered) - 1)
    if lo == hi:
        return ordered[lo]
    return ordered[lo] + (ordered[hi] - ordered[lo]) * (k - lo)


def generate_arrivals(workload: dict, accounts: list[dict], rng: random.Random) -> list[dict]:
    """Deterministic arrival schedule for the given RNG state. Same rng seed -> identical schedule."""
    hot = accounts[0]
    others = accounts[1:] or accounts
    mix = workload["traffic_mix"]
    verbs, weights = list(mix.keys()), list(mix.values())
    arrivals = []
    t = 0.0
    for phase in workload["phases"]:
        duration_s = phase["minutes"] * 60
        rate = workload["concurrency"]["baseline_request_rate_per_s"] * phase.get("rate_multiplier", 1)
        phase_end = t + duration_s
        while True:
            t += rng.expovariate(rate)
            if t >= phase_end:
                break
            account = hot if rng.random() < workload["hot_tenant_traffic_fraction"] else rng.choice(others)
            verb = rng.choices(verbs, weights=weights, k=1)[0]
            cold = rng.random() < workload["cold_brain_fraction"]
            qt = workload["query_tokens"]
            ft = workload["fact_tokens"]
            tokens = rng.randint(ft["min"], ft["max"]) if verb == "remember" else rng.randint(qt["min"], qt["max"])
            arrivals.append({"t": round(t, 6), "phase": phase["name"], "account": account["id"], "verb": verb, "cold": cold, "tokens": tokens})
        t = phase_end  # reset to the ideal cumulative boundary; do not carry the last draw's overshoot forward.
    return arrivals


def run_repetition(workload: dict, accounts: list[dict], seed: int, saturation: bool = False) -> dict:
    """Chronological sweep: each admitted request holds its pool slot from
    arrival until its computed completion time, so later arrivals see real
    concurrent occupancy. A synchronous acquire-then-immediately-release
    model would never let in_flight exceed 1 and would make capacity
    rejection impossible regardless of load -- this is the fix for that.
    """
    rng_arrivals = random.Random(seed)
    arrivals = generate_arrivals(workload, accounts, rng_arrivals)
    rng_exec = random.Random(seed + 1)
    max_open = 1 if saturation else MAX_OPEN
    max_in_flight = 2 if saturation else MAX_IN_FLIGHT
    pool = Pool(max_open=max_open, max_in_flight=max_in_flight, idle_timeout_s=IDLE_TIMEOUT_S)
    rate_limiter = RateLimiter(limit=ACCOUNT_RATE_LIMIT_PER_MIN)
    writer_busy_until: dict = {}
    pending: list = []  # (end_time, brain_id) still holding a pool slot.
    failure_rate = workload.get("provider_failure_injection", {}).get("rate", 0.0)
    max_retries = workload.get("provider_failure_injection", {}).get("max_retries", 0)

    results = []
    for arrival in arrivals:
        now = arrival["t"]
        still_pending = []
        for end_time, brain_id in pending:
            if end_time <= now:
                pool.release(brain_id, end_time)
            else:
                still_pending.append((end_time, brain_id))
        pending = still_pending

        if not rate_limiter.allow(arrival["account"], now):
            results.append({**arrival, "outcome": "rejected_rate_limit", "latency_s": None})
            continue
        brain_id = arrival["account"]  # one primary brain per account for this workload
        admitted, cold = pool.acquire(brain_id, now)
        if not admitted:
            results.append({**arrival, "outcome": "rejected_capacity", "latency_s": None})
            continue

        writer_wait_s = 0.0
        if arrival["verb"] in ("remember", "forget"):
            writer_wait_s = max(0.0, writer_busy_until.get(brain_id, now) - now)
        cold_open_s = rng_exec.uniform(0.8, 3.5) + (arrival["tokens"] / 128.0) * 0.05 if cold else 0.0
        provider_s, attempts, failed = 0.0, 0, False
        for attempt in range(max_retries + 1):
            attempts += 1
            call_s = rng_exec.uniform(0.05, 0.4) + (arrival["tokens"] / 512.0) * 0.15
            provider_s += call_s
            if rng_exec.random() >= failure_rate:
                break
            if attempt == max_retries:
                failed = True
        base_s = {"recall": rng_exec.uniform(0.05, 0.3), "remember": rng_exec.uniform(0.08, 0.35), "forget": rng_exec.uniform(0.03, 0.15)}[arrival["verb"]]
        latency_s = base_s + provider_s + cold_open_s + writer_wait_s
        end_time = now + latency_s
        if arrival["verb"] in ("remember", "forget"):
            writer_busy_until[brain_id] = end_time
        pending.append((end_time, brain_id))
        outcome = "unexpected_5xx" if failed else "ok"
        results.append({**arrival, "outcome": outcome, "latency_s": latency_s, "cold_open": cold, "provider_attempts": attempts, "writer_wait_s": writer_wait_s})

    by_verb: dict = {}
    for r in results:
        by_verb.setdefault(r["verb"], []).append(r)
    latency_by_verb = {
        verb: {
            "p50": percentile([r["latency_s"] for r in items if r["latency_s"] is not None], 50),
            "p95": percentile([r["latency_s"] for r in items if r["latency_s"] is not None], 95),
            "p99": percentile([r["latency_s"] for r in items if r["latency_s"] is not None], 99),
            "max": max([r["latency_s"] for r in items if r["latency_s"] is not None], default=0.0),
        }
        for verb, items in by_verb.items()
    }
    outcomes: dict = {}
    for r in results:
        outcomes[r["outcome"]] = outcomes.get(r["outcome"], 0) + 1
    cold_latencies = [r["latency_s"] for r in results if r.get("cold_open") and r["latency_s"] is not None]
    total = len(results)
    ok = outcomes.get("ok", 0)
    unexpected_5xx = outcomes.get("unexpected_5xx", 0)
    rejected = outcomes.get("rejected_capacity", 0) + outcomes.get("rejected_rate_limit", 0)
    return {
        "saturation": saturation,
        "total_offered": total,
        "outcomes": outcomes,
        "completion_pct": (ok / total * 100.0) if total else 0.0,
        "unexpected_5xx_pct": (unexpected_5xx / total * 100.0) if total else 0.0,
        "admission_rejection_pct": (rejected / total * 100.0) if total else 0.0,
        "latency_by_verb_s": latency_by_verb,
        "cold_open_p95_s": percentile(cold_latencies, 95),
        "cold_open_count": len(cold_latencies),
    }


def evaluate_thresholds(workload: dict, main_run: dict) -> dict:
    t = workload["thresholds"]
    recall = main_run["latency_by_verb_s"].get("recall", {})
    remember = main_run["latency_by_verb_s"].get("remember", {})
    checks = {
        "cold_ready_max_s": {"limit": t["cold_ready_max_s"], "observed": main_run["cold_open_p95_s"], "pass": main_run["cold_open_p95_s"] <= t["cold_ready_max_s"]},
        "recall_p95_max_s": {"limit": t["recall_p95_max_s"], "observed": recall.get("p95", 0.0), "pass": recall.get("p95", 0.0) <= t["recall_p95_max_s"]},
        "remember_p95_max_s": {"limit": t["remember_p95_max_s"], "observed": remember.get("p95", 0.0), "pass": remember.get("p95", 0.0) <= t["remember_p95_max_s"]},
        "min_offered_completion_pct": {"limit": t["min_offered_completion_pct"], "observed": main_run["completion_pct"], "pass": main_run["completion_pct"] >= t["min_offered_completion_pct"]},
        "unexpected_5xx_max_pct": {"limit": t["unexpected_5xx_max_pct"], "observed": main_run["unexpected_5xx_pct"], "pass": main_run["unexpected_5xx_pct"] <= t["unexpected_5xx_max_pct"]},
        "unexpected_admission_rejection_max_pct": {"limit": t["unexpected_admission_rejection_max_pct"], "observed": main_run["admission_rejection_pct"], "pass": main_run["admission_rejection_pct"] <= t["unexpected_admission_rejection_max_pct"]},
    }
    checks["cpu_max_pct"] = {"limit": t["cpu_max_pct"], "observed": None, "pass": None, "reason": "not measured in fixtures mode"}
    checks["rss_max_pct_of_ram"] = {"limit": t["rss_max_pct_of_ram"], "observed": None, "pass": None, "reason": "not measured in fixtures mode"}
    checks["disk_max_pct"] = {"limit": t["disk_max_pct"], "observed": None, "pass": None, "reason": "not measured in fixtures mode"}
    checks["isolation_durability_errors_max"] = {"limit": t["isolation_durability_errors_max"], "observed": 0, "pass": True, "reason": "fixtures mode has no real storage; zero by construction, not proof"}
    return checks


_TEXT_VOCAB = [
    "onboarding", "latency", "budget", "provenance", "brain", "recall", "remember",
    "forget", "account", "plan", "migration", "cutover", "incident", "runbook",
    "review", "threshold", "capacity", "gateway", "pool", "session", "credential",
    "retention", "backup", "snapshot", "embedding", "index", "queue", "worker",
    "deploy", "rollback", "signal", "metric", "alarm", "region", "instance",
    "customer", "ticket", "escalation", "vendor", "invoice", "renewal", "quota",
]


def render_text(seed_str: str, tokens: int) -> str:
    """Deterministic pseudo-natural text, roughly `tokens` whitespace-separated
    words for a given seed. Not a real tokenizer count (words != model tokens)
    -- an approximate sizing for exercising the wire protocol with real,
    non-placeholder request bodies, never a claim about actual token billing."""
    rng = random.Random(seed_str)
    n = max(int(tokens), 1)
    return " ".join(rng.choice(_TEXT_VOCAB) for _ in range(n))
