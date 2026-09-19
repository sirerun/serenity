#!/usr/bin/env python3
"""Bounded capacity/load harness for hosted Serenity (T23.60).

--fixtures runs the deterministic in-process simulator in evals/hosted-load
(no network, no real brain, no provider call) to prove harness mechanics:
replay determinism, cap enforcement and threshold evaluation shape. It is
never semantic-quality or real-capacity evidence.

--live is deliberately disabled at the CLI before reading credentials or
opening any socket. The candidate client below is exercised by local tests,
but outcome, queued-deadline, complete budget and cold-workload proofs remain
unfinished. See docs/launch/evidence/T23.60/live-enable-requirements.md.
"""
from __future__ import annotations

import argparse
import ipaddress
import json
import random
import stat
import sys
import threading
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from urllib.parse import urlsplit

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "evals" / "hosted-load"))
import harness  # noqa: E402

PROTOCOL_VERSION = "2025-11-25"
CLIENT_NAME = "serenity-hosted-load"
CLIENT_VERSION = "T23.60"
MCP_PATH = "/mcp"
LOOPBACK_HOSTNAMES = {"localhost"}
PRODUCTION_HOSTS = {"app.serenity.sire.run", "serenity.sire.run"}
DEFAULT_CALL_TIMEOUT_S = 10.0
READINESS_TIMEOUT_S = 5.0


def die(payload: dict, code: int) -> None:
    print(json.dumps(payload, indent=2))
    raise SystemExit(code)


def load_manifest(path: Path) -> dict:
    return json.loads(path.read_text())


def _is_loopback_host(host: str) -> bool:
    if host in LOOPBACK_HOSTNAMES:
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def check_environment_guard(manifest: dict) -> str | None:
    env = manifest.get("environment") or {}
    if env.get("production_target_allowed"):
        return "manifest.environment.production_target_allowed must be false: this harness never targets production"
    origin = env.get("origin")
    if not origin:
        return "manifest.environment.origin is required for --live"
    allowed = env.get("allowed_hosts") or []
    parts = urlsplit(origin)
    host = parts.hostname
    if host is None:
        return f"origin {origin!r} has no parseable host"
    if host not in allowed:
        return f"origin host {host!r} is not in manifest.environment.allowed_hosts {allowed!r}"
    if host in PRODUCTION_HOSTS:
        return f"refusing production host {host!r}"
    if parts.scheme not in ("http", "https"):
        return f"unsupported origin scheme {parts.scheme!r}: use http (loopback only) or https"
    if parts.scheme == "http" and not _is_loopback_host(host):
        return f"refusing plaintext http to non-loopback host {host!r}: use https, or http only against 127.0.0.1/::1/localhost"
    return None


REQUIRED_BUDGET_FIELDS = (
    "approved_max_usd",
    "max_calls",
    "max_input_tokens",
    "max_elapsed_seconds",
    "authorization_ref",
    "worst_case_usd_per_call",
)


def check_budget(manifest: dict) -> str | None:
    budget = manifest.get("budget") or {}
    for field in REQUIRED_BUDGET_FIELDS:
        if budget.get(field) in (None, ""):
            return f"manifest.budget.{field} must be set (non-null) before --live"
    return None


def check_credentials(credential_dir: Path, accounts: list[dict]) -> tuple[dict[str, str] | None, str | None]:
    """Loads one private, per-account bearer credential per account.

    Each file must be `<credential_dir>/<account_id>.token`, a regular file
    (never a symlink -- that could point outside the private directory),
    mode exactly 0600, non-empty. Fails closed on the first problem found,
    before any network call: --live must never send even one real request
    for a workload it cannot fully authenticate.
    """
    if not credential_dir.is_dir():
        return None, f"live.credential_dir {credential_dir} is not a directory"
    creds: dict[str, str] = {}
    for account in accounts:
        path = credential_dir / f"{account['id']}.token"
        if path.is_symlink():
            return None, f"credential file for account {account['id']!r} must not be a symlink: {path}"
        if not path.is_file():
            return None, f"missing credential file for account {account['id']!r}: {path}"
        mode = stat.S_IMODE(path.stat().st_mode)
        if mode != 0o600:
            return None, f"credential file for account {account['id']!r} must be mode 0600, got {oct(mode)}: {path}"
        value = path.read_text().strip()
        if not value:
            return None, f"credential file for account {account['id']!r} is empty: {path}"
        creds[account["id"]] = value
    return creds, None


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


class McpProtocolError(Exception):
    """A wire-level violation: bad status, redirect, missing header, malformed body, mismatched id."""


class _RefuseRedirects(urllib.request.HTTPRedirectHandler):
    """Raises on every 3xx instead of silently dialing the redirect target.

    A client that instead returned None from redirect_request would rely on
    urllib's own (undocumented-to-callers) fallback of surfacing the raw 3xx
    response, which callers can accidentally treat as an ordinary reply.
    Raising here makes the refusal unambiguous and impossible to miss.
    """

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise urllib.error.HTTPError(req.full_url, code, f"refusing to follow redirect to {newurl!r}", headers, fp)


_OPENER = urllib.request.build_opener(_RefuseRedirects)


class McpSession:
    """One real MCP-over-HTTP session: initialize -> notifications/initialized -> tools/call.

    Every request goes over real urllib HTTP against origin + MCP_PATH; there
    is no injected transport function here on purpose -- tests point origin
    at a real local http.server fixture instead, so the client's actual
    request/response (de)serialization is what gets exercised.
    """

    def __init__(self, origin: str, credential: str, timeout_s: float):
        self._url = origin.rstrip("/") + MCP_PATH
        self._credential = credential
        self._timeout_s = timeout_s
        self._session_id: str | None = None
        self._initialized = False
        self._next_id = 1
        self._id_lock = threading.Lock()

    def _next_request_id(self) -> int:
        with self._id_lock:
            rid = self._next_id
            self._next_id += 1
            return rid

    def _post(self, payload: dict, timeout_s: float, expect_body: bool):
        body = json.dumps(payload).encode("utf-8")
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json",
            "Authorization": f"Bearer {self._credential}",
        }
        if self._session_id is not None:
            headers["Mcp-Session-Id"] = self._session_id
            headers["MCP-Protocol-Version"] = PROTOCOL_VERSION
        req = urllib.request.Request(self._url, data=body, method="POST", headers=headers)
        start = time.monotonic()
        try:
            with _OPENER.open(req, timeout=timeout_s) as resp:
                status = resp.status
                resp_headers = resp.headers
                raw = resp.read()
        except urllib.error.HTTPError as e:
            status = e.code
            resp_headers = e.headers
            try:
                raw = e.read()
            except (OSError, ValueError):
                # A redirect (3xx) response body is irrelevant -- we refuse to
                # follow it regardless -- and some servers close the
                # connection without a body right after issuing one.
                raw = b""
        latency_s = time.monotonic() - start
        if 300 <= status < 400:
            raise McpProtocolError(f"refused HTTP {status} redirect from {self._url}")
        if not expect_body:
            if status != 202:
                raise McpProtocolError(f"expected 202 Accepted for a notification, got {status}: {raw[:200]!r}")
            return None, resp_headers, latency_s
        if status != 200:
            raise McpProtocolError(f"unexpected HTTP status {status}: {raw[:200]!r}")
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError as e:
            raise McpProtocolError(f"non-JSON response body: {e}") from e
        return parsed, resp_headers, latency_s

    def initialize(self) -> float:
        if self._session_id is not None:
            raise McpProtocolError("initialize called twice on the same McpSession")
        rid = self._next_request_id()
        payload = {
            "jsonrpc": "2.0",
            "id": rid,
            "method": "initialize",
            "params": {
                "protocolVersion": PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": {"name": CLIENT_NAME, "version": CLIENT_VERSION},
            },
        }
        parsed, headers, latency_s = self._post(payload, self._timeout_s, expect_body=True)
        if parsed.get("id") != rid:
            raise McpProtocolError(f"initialize response id {parsed.get('id')!r} does not match request id {rid!r}")
        if parsed.get("error") is not None:
            raise McpProtocolError(f"initialize failed: {parsed['error']}")
        session_id = headers.get("Mcp-Session-Id")
        if not session_id:
            raise McpProtocolError("initialize succeeded but server did not return Mcp-Session-Id")
        self._session_id = session_id
        notify_payload = {"jsonrpc": "2.0", "method": "notifications/initialized"}
        self._post(notify_payload, self._timeout_s, expect_body=False)
        self._initialized = True
        return latency_s

    def call_tool(self, name: str, arguments: dict, timeout_s: float) -> dict:
        if not self._initialized:
            raise McpProtocolError("call_tool before initialize")
        rid = self._next_request_id()
        payload = {"jsonrpc": "2.0", "id": rid, "method": "tools/call", "params": {"name": name, "arguments": arguments}}
        parsed, _headers, latency_s = self._post(payload, timeout_s, expect_body=True)
        if parsed.get("id") != rid:
            raise McpProtocolError(f"tools/call response id {parsed.get('id')!r} does not match request id {rid!r}")
        if parsed.get("error") is not None:
            return {"ok": False, "level": "protocol", "error": parsed["error"], "latency_s": latency_s}
        result = parsed.get("result") or {}
        is_error = bool(result.get("isError"))
        content = result.get("content") or []
        text = content[0]["text"] if content and content[0].get("type") == "text" else None
        parsed_text = None
        if text is not None:
            try:
                parsed_text = json.loads(text)
            except json.JSONDecodeError:
                parsed_text = None
        return {"ok": not is_error, "level": "tool", "is_error": is_error, "text": parsed_text, "latency_s": latency_s}

    def close(self) -> None:
        if self._session_id is None:
            return
        req = urllib.request.Request(
            self._url,
            method="DELETE",
            headers={"Mcp-Session-Id": self._session_id, "MCP-Protocol-Version": PROTOCOL_VERSION},
        )
        with _OPENER.open(req, timeout=self._timeout_s) as resp:
            resp.read()


def check_readiness(origin: str, timeout_s: float) -> str | None:
    """The first real network call of a --live run: a read-only GET against
    the service's own liveness/readiness route, only after every offline
    guard (environment, budget, credentials, workload) has already passed."""
    url = origin.rstrip("/") + "/readyz"
    req = urllib.request.Request(url, method="GET")
    try:
        with _OPENER.open(req, timeout=timeout_s) as resp:
            if 300 <= resp.status < 400:
                return f"refused HTTP {resp.status} redirect from readiness probe {url}"
            if resp.status != 200:
                return f"readiness probe {url} returned HTTP {resp.status}"
    except urllib.error.HTTPError as e:
        return f"readiness probe {url} returned HTTP {e.code}"
    except (urllib.error.URLError, TimeoutError, OSError) as e:
        return f"readiness probe {url} failed: {e}"
    return None


def run_live(manifest: dict, workload: dict, accounts: list[dict], credentials: dict[str, str]) -> dict:
    origin = manifest["environment"]["origin"]
    budget = manifest["budget"]
    max_calls = budget["max_calls"]
    max_tokens = budget["max_input_tokens"]
    max_usd = budget["approved_max_usd"]
    max_elapsed = budget["max_elapsed_seconds"]
    worst_case_usd = budget["worst_case_usd_per_call"]

    run_start = time.monotonic()
    counters_lock = threading.Lock()
    calls_used = 0
    tokens_used = 0
    usd_used = 0.0
    results: list[dict] = []
    results_lock = threading.Lock()
    remembered_ids: dict[str, list[str]] = {a["id"]: [] for a in accounts}
    remembered_lock = threading.Lock()
    counts = {"skipped_forgets": 0, "network_errors": 0, "protocol_errors": 0, "tool_errors": 0}

    def remaining_elapsed() -> float:
        return max_elapsed - (time.monotonic() - run_start)

    def precharge(next_tokens: int) -> str | None:
        nonlocal calls_used, tokens_used, usd_used
        with counters_lock:
            if calls_used + 1 > max_calls:
                return "max_calls budget exhausted"
            if tokens_used + next_tokens > max_tokens:
                return "max_input_tokens budget exhausted"
            if usd_used + worst_case_usd > max_usd:
                return "approved_max_usd budget exhausted (worst-case precharge)"
            if remaining_elapsed() <= 0:
                return "max_elapsed_seconds budget exhausted"
            calls_used += 1
            tokens_used += next_tokens
            usd_used += worst_case_usd
            return None

    sessions: dict[str, McpSession] = {}
    blocked_reason: str | None = None

    for account in accounts:
        pre_err = precharge(0)
        if pre_err:
            blocked_reason = pre_err
            break
        timeout_s = max(0.1, min(DEFAULT_CALL_TIMEOUT_S, remaining_elapsed()))
        session = McpSession(origin, credentials[account["id"]], timeout_s)
        try:
            latency_s = session.initialize()
        except (McpProtocolError, urllib.error.URLError, TimeoutError, OSError) as e:
            blocked_reason = f"session initialize failed for account {account['id']!r}: {e}"
            break
        sessions[account["id"]] = session
        results.append({"account": account["id"], "verb": "initialize", "ok": True, "latency_s": latency_s})

    def dispatch(account_id: str, verb: str, tokens: int, cold: bool, seq: int) -> None:
        session = sessions[account_id]
        if verb == "recall":
            args = {"query": harness.render_text(f"{account_id}:recall:{seq}", tokens)}
        elif verb == "remember":
            args = {"fact": harness.render_text(f"{account_id}:remember:{seq}", tokens), "provenance": "hosted-load-harness T23.60"}
        elif verb == "forget":
            with remembered_lock:
                ids = remembered_ids[account_id]
                fact_id = ids.pop() if ids else None
            if fact_id is None:
                with results_lock:
                    counts["skipped_forgets"] += 1
                    results.append({"account": account_id, "verb": "forget", "ok": None, "skipped": True, "reason": "no remembered id available yet for this account this run"})
                return
            args = {"id": fact_id}
        else:
            raise ValueError(f"unknown verb {verb!r}")

        timeout_s = max(0.1, min(DEFAULT_CALL_TIMEOUT_S, remaining_elapsed()))
        try:
            outcome = session.call_tool(verb, args, timeout_s)
        except McpProtocolError as e:
            with results_lock:
                counts["protocol_errors"] += 1
                results.append({"account": account_id, "verb": verb, "ok": False, "level": "protocol", "error": str(e)})
            return
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            with results_lock:
                counts["network_errors"] += 1
                results.append({"account": account_id, "verb": verb, "ok": False, "level": "network", "error": str(e)})
            return

        record = {"account": account_id, "verb": verb, "cold": cold, "latency_s": outcome["latency_s"], "ok": outcome["ok"], "level": outcome["level"]}
        if outcome["level"] == "protocol":
            record["error"] = outcome["error"]
            with results_lock:
                counts["protocol_errors"] += 1
                results.append(record)
            return
        if outcome.get("is_error"):
            record["text"] = outcome.get("text")
            with results_lock:
                counts["tool_errors"] += 1
                results.append(record)
            return
        if verb == "remember" and isinstance(outcome.get("text"), dict):
            fact_id = outcome["text"].get("id")
            if fact_id:
                with remembered_lock:
                    remembered_ids[account_id].append(fact_id)
        with results_lock:
            results.append(record)

    if blocked_reason is None:
        executor = ThreadPoolExecutor(max_workers=workload["concurrency"]["clients"])
        futures = []
        for rep in range(workload["repetitions"]):
            if blocked_reason:
                break
            arrivals = harness.generate_arrivals(workload, accounts, random.Random(20260918 + rep))
            rep_start = time.monotonic()
            for seq, arrival in enumerate(arrivals):
                target_time = rep_start + arrival["t"]
                sleep_s = target_time - time.monotonic()
                # Check the elapsed cap before committing to the wait, not after:
                # otherwise a distant scheduled arrival (late in a long phase)
                # would block real wall-clock time for its full delay before this
                # loop ever notices the budget is already exhausted.
                if sleep_s > remaining_elapsed():
                    blocked_reason = "max_elapsed_seconds budget exhausted"
                    break
                if sleep_s > 0:
                    time.sleep(sleep_s)
                pre_err = precharge(arrival["tokens"])
                if pre_err:
                    blocked_reason = pre_err
                    break
                futures.append(executor.submit(dispatch, arrival["account"], arrival["verb"], arrival["tokens"], arrival["cold"], seq))
        for f in futures:
            f.result()
        executor.shutdown(wait=True)

    for session in sessions.values():
        try:
            session.close()
        except (McpProtocolError, urllib.error.URLError, TimeoutError, OSError):
            pass

    status = "BLOCKED" if blocked_reason else "COMPLETE"
    return {
        "mode": "live",
        "status": status,
        "reason": blocked_reason,
        "calls_used": calls_used,
        "tokens_used": tokens_used,
        "usd_used_worst_case": round(usd_used, 6),
        "elapsed_s": round(time.monotonic() - run_start, 3),
        "accounts_initialized": len(sessions),
        **counts,
        "results": results,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--fixtures", action="store_true")
    mode.add_argument("--live", action="store_true")
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    if args.live:
        result = {
            "mode": "live", "status": "BLOCKED", "calls_used": 0,
            "tokens_used": 0, "usd_used_worst_case": 0,
            "reason": "Live load execution is disabled pending outcome, deadline, budget and workload qualification; see live-enable-requirements.md",
        }
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps(result))
        return 2

    if not args.manifest.exists():
        die({"status": "BLOCKED", "reason": f"manifest not found: {args.manifest}"}, 2)
    try:
        manifest = load_manifest(args.manifest)
    except json.JSONDecodeError as e:
        die({"status": "BLOCKED", "reason": f"manifest is not valid JSON: {e}"}, 2)

    workload_path = Path(__file__).resolve().parents[2] / "evals" / "hosted-load" / "workload.json"
    try:
        workload = harness.load_workload(workload_path)
    except (ValueError, json.JSONDecodeError) as e:
        die({"status": "BLOCKED", "reason": str(e)}, 2)

    if args.fixtures:
        result = run_fixtures(manifest, workload)
        result["source_workload_sha256"] = harness.sha256_file(workload_path)
        result["source_manifest_sha256"] = harness.sha256_file(args.manifest)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps({"status": "PASS", "output": str(args.output), "replay_determinism_verified": result["replay_determinism_verified"]}, indent=2))
        return 0

    # --live: every offline guard runs before any socket opens.
    guard_error = check_environment_guard(manifest)
    if guard_error:
        die({"status": "BLOCKED", "reason": guard_error}, 2)
    budget_error = check_budget(manifest)
    if budget_error:
        die({"status": "BLOCKED", "reason": budget_error}, 2)
    try:
        accounts = harness.build_accounts(workload["cardinalities"], manifest.get("profile", "paid"))
    except ValueError as e:
        die({"status": "BLOCKED", "reason": str(e)}, 2)
    credential_dir = (manifest.get("live") or {}).get("credential_dir")
    if not credential_dir:
        die({"status": "BLOCKED", "reason": "manifest.live.credential_dir is required for --live"}, 2)
    credentials, cred_error = check_credentials(Path(credential_dir), accounts)
    if cred_error:
        die({"status": "BLOCKED", "reason": cred_error}, 2)

    readiness_error = check_readiness(manifest["environment"]["origin"], READINESS_TIMEOUT_S)
    if readiness_error:
        die({"status": "BLOCKED", "reason": readiness_error}, 2)

    result = run_live(manifest, workload, accounts, credentials)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"status": result["status"], "output": str(args.output), "reason": result.get("reason")}, indent=2))
    return 0 if result["status"] == "COMPLETE" else 2


if __name__ == "__main__":
    raise SystemExit(main())
