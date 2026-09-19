#!/usr/bin/env python3
"""Bounded capacity/load harness for hosted Serenity (T23.60).

--fixtures runs the deterministic in-process simulator in evals/hosted-load
(no network, no real brain, no provider call) to prove harness mechanics:
replay determinism, cap enforcement and threshold evaluation shape. It is
never semantic-quality or real-capacity evidence.

--live speaks the real Rakazo/MCP HTTP protocol (Authorization: Bearer
<credential>, JSON-RPC tools/call) against manifest.environment.origin under
an explicit allowlist and budget caps, and refuses a production origin. No
deployed hosted target exists yet as of T23.60; this path is implemented and
unit-tested against an injected fake transport, but not exercised against a
real host in this session -- see docs/launch/evidence/T23.60/result.json
limitations. SSE-streamed tool responses are not parsed; only a single JSON
response body is supported today.
"""
from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "evals" / "hosted-load"))
import harness  # noqa: E402


def die(payload: dict, code: int) -> None:
    print(json.dumps(payload, indent=2))
    raise SystemExit(code)


def load_manifest(path: Path) -> dict:
    return json.loads(path.read_text())


def check_environment_guard(manifest: dict) -> str | None:
    env = manifest.get("environment") or {}
    if env.get("production_target_allowed"):
        return "manifest.environment.production_target_allowed must be false: this harness never targets production"
    origin = env.get("origin")
    allowed = env.get("allowed_hosts") or []
    if not origin:
        return "manifest.environment.origin is required for --live"
    from urllib.parse import urlsplit
    host = urlsplit(origin).hostname
    if host not in allowed:
        return f"origin host {host!r} is not in manifest.environment.allowed_hosts {allowed!r}"
    if host in ("app.serenity.sire.run", "serenity.sire.run"):
        return f"refusing production host {host!r}"
    return None


def check_budget(manifest: dict) -> str | None:
    budget = manifest.get("budget") or {}
    for field in ("approved_max_usd", "max_calls", "max_input_tokens", "max_elapsed_seconds", "authorization_ref"):
        if budget.get(field) in (None, ""):
            return f"manifest.budget.{field} must be set (non-null) before --live"
    return None


def run_fixtures(manifest: dict, workload: dict) -> dict:
    accounts = harness.build_accounts(workload["cardinalities"], manifest.get("profile", "paid"))
    reps = []
    base_seed = 20260918
    for i in range(workload["repetitions"]):
        reps.append(harness.run_repetition(workload, accounts, seed=base_seed, saturation=False))
    replay_identical = all(r["total_offered"] == reps[0]["total_offered"] and r["outcomes"] == reps[0]["outcomes"] for r in reps)
    saturation = harness.run_repetition(workload, accounts, seed=base_seed + 1000, saturation=True)
    main_run = reps[0]
    thresholds = harness.evaluate_thresholds(workload, main_run)
    return {
        "mode": "fixtures",
        "profile": manifest.get("profile", "paid"),
        "accounts": len(accounts),
        "repetitions": reps,
        "replay_determinism_verified": replay_identical,
        "saturation_run": saturation,
        "threshold_evaluation": thresholds,
        "thresholds_frozen": bool(workload.get("reviewer")),
        "resource_usage": {"available": False, "reason": "fixtures mode does not exercise a real host or provider"},
    }


def http_call(origin: str, credential: str, verb: str, tokens: int, timeout_s: float) -> tuple[bool, float, int | None]:
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": verb, "arguments": {"query": "x" * tokens}}}).encode()
    req = urllib.request.Request(
        origin.rstrip("/") + "/mcp",
        data=body,
        method="POST",
        headers={
            "Authorization": f"Bearer {credential}",
            "Accept": "application/json, text/event-stream",
            "Content-Type": "application/json",
        },
    )
    start = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout_s) as resp:
            resp.read()
            return True, time.monotonic() - start, resp.status
    except urllib.error.HTTPError as e:
        return False, time.monotonic() - start, e.code
    except (urllib.error.URLError, TimeoutError):
        return False, time.monotonic() - start, None


def run_live(manifest: dict, workload: dict, transport=http_call) -> dict:
    origin = manifest["environment"]["origin"]
    credential_ref = manifest.get("provider", {}).get("secret_ref") or "UNSET"
    budget = manifest["budget"]
    accounts = harness.build_accounts(workload["cardinalities"], manifest.get("profile", "paid"))
    import random
    arrivals = harness.generate_arrivals(workload, accounts, random.Random(20260918))
    calls_used, tokens_used, results = 0, 0, []
    start = time.monotonic()
    for arrival in arrivals:
        if calls_used >= budget["max_calls"]:
            return {"mode": "live", "status": "BLOCKED", "reason": "max_calls budget exhausted", "calls_used": calls_used, "tokens_used": tokens_used, "results": results}
        if tokens_used >= budget["max_input_tokens"]:
            return {"mode": "live", "status": "BLOCKED", "reason": "max_input_tokens budget exhausted", "calls_used": calls_used, "tokens_used": tokens_used, "results": results}
        if time.monotonic() - start >= budget["max_elapsed_seconds"]:
            return {"mode": "live", "status": "BLOCKED", "reason": "max_elapsed_seconds budget exhausted", "calls_used": calls_used, "tokens_used": tokens_used, "results": results}
        ok, latency_s, status = transport(origin, "REDACTED:" + credential_ref, arrival["verb"], arrival["tokens"], 30.0)
        calls_used += 1
        tokens_used += arrival["tokens"]
        results.append({"verb": arrival["verb"], "ok": ok, "latency_s": latency_s, "status": status})
    return {"mode": "live", "status": "COMPLETE", "calls_used": calls_used, "tokens_used": tokens_used, "results": results}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--fixtures", action="store_true")
    mode.add_argument("--live", action="store_true")
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    if not args.manifest.exists():
        die({"status": "BLOCKED", "reason": f"manifest not found: {args.manifest}"}, 2)
    manifest = load_manifest(args.manifest)
    workload_path = Path(__file__).resolve().parents[2] / "evals" / "hosted-load" / "workload.json"
    try:
        workload = harness.load_workload(workload_path)
    except ValueError as e:
        die({"status": "BLOCKED", "reason": str(e)}, 2)

    if args.fixtures:
        result = run_fixtures(manifest, workload)
        result["source_workload_sha256"] = harness.sha256_file(workload_path)
        result["source_manifest_sha256"] = harness.sha256_file(args.manifest)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps({"status": "PASS", "output": str(args.output), "replay_determinism_verified": result["replay_determinism_verified"]}, indent=2))
        return 0

    guard_error = check_environment_guard(manifest)
    if guard_error:
        die({"status": "BLOCKED", "reason": guard_error}, 2)
    budget_error = check_budget(manifest)
    if budget_error:
        die({"status": "BLOCKED", "reason": budget_error}, 2)
    result = run_live(manifest, workload)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"status": result["status"], "output": str(args.output)}, indent=2))
    return 0 if result["status"] == "COMPLETE" else 2


if __name__ == "__main__":
    raise SystemExit(main())
