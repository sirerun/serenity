#!/usr/bin/env python3
"""Bounded capacity/load harness for hosted Serenity (T23.60).

--fixtures runs the deterministic in-process simulator in evals/hosted-load
(no network, no real brain, no provider call) to prove harness mechanics:
replay determinism, cap enforcement and threshold evaluation shape. It is
never semantic-quality or real-capacity evidence.

--live is deliberately disabled at the CLI before reading a manifest or
credentials or opening any socket, and no code path from main() reaches the
candidate client below. The candidate client (prepare_live/run_live) is
exercised against a real loopback HTTP server by the local tests. It can
never report better than PARTIAL: seeded account/brain/cardinality/storage
state, a real cold-open workload, host telemetry and the reviewer freeze are
not verifiable from the client side. See
docs/launch/evidence/T23.60/live-enable-requirements.md.
"""
from __future__ import annotations

import argparse
import errno
import http.client
import ipaddress
import json
import math
import random
import re
import socket
import ssl
import stat
import sys
import threading
import time
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from typing import NamedTuple
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
# Allowance for watchdog-timer wake-up and thread scheduling when comparing the
# measured run against max_elapsed_seconds; not slack for slow servers.
DEADLINE_JITTER_S = 0.25
MAX_BODY_BYTES = 4 * 1024 * 1024
REDACTED = "[REDACTED]"
BASE_SEED = 20260918  # every repetition replays the same arrivals ("replay captures exact arrivals; repeat 3 times").
FROZEN_REPETITIONS = 3
STEADY_PHASE = "steady"
FORGET_ID_BYTES_ALLOWANCE = 256  # preflight bound for a server-issued fact id; dispatch charges the real id.

LIVE_GATE_REASON = "Live load execution is disabled pending outcome, deadline, budget and workload qualification; see live-enable-requirements.md"

# Outcome taxonomy: every offered request ends in exactly one of these.
OK = "ok"
TOOL_ERROR = "tool_error"
PROTOCOL_ERROR = "protocol_error"
REJECTED_ADMISSION = "rejected_admission"  # HTTP 429 only.
UNEXPECTED_5XX = "unexpected_5xx"  # every 5xx, including 503 capacity refusals: the stricter bucket.
NETWORK_ERROR = "network_error"
CLIENT_ERROR = "client_error"
SKIPPED_FORGET = "skipped_forget_no_fact"
NOT_DISPATCHED = "not_dispatched"
ALL_OUTCOMES = (OK, TOOL_ERROR, PROTOCOL_ERROR, REJECTED_ADMISSION, UNEXPECTED_5XX, NETWORK_ERROR, CLIENT_ERROR, SKIPPED_FORGET, NOT_DISPATCHED)

# Every socket operation the client can perform. Every authenticated one may make
# the gateway open (cold-open) a brain, which can trigger provider work server-side.
OPERATIONS = ("readiness", "initialize", "notification", "tools_call", "close")

TRANSPORT_ERRORS = (OSError, http.client.HTTPException)


def die(payload: dict, code: int) -> None:
    print(json.dumps(payload, indent=2))
    raise SystemExit(code)


def load_manifest(path: Path) -> dict:
    return json.loads(path.read_text())


def error_class(exc: BaseException) -> str:
    """A fixed, bounded description of a failure: exception class (and errno
    name). Never message text, which can carry upstream-controlled bytes."""
    name = type(exc).__name__
    code = getattr(exc, "errno", None)
    if isinstance(code, int) and code in errno.errorcode:
        return f"{name}:{errno.errorcode[code]}"
    return name


# ---------------------------------------------------------------------------
# Canonical origins and the environment guard.
# ---------------------------------------------------------------------------

_DNS_LABEL = re.compile(r"[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?")
_NUMERIC_TLD = re.compile(r"[0-9]+|0x[0-9a-f]*")
_CREDENTIAL_VALUE = re.compile(r"[\x21-\x7e]+")
_SESSION_ID = re.compile(r"[\x21-\x7e]{1,256}")
_DEFAULT_PORTS = {"http": 80, "https": 443}


def _is_loopback_host(host: str) -> bool:
    if host in LOOPBACK_HOSTNAMES:
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def _normalized_host(host: str) -> str:
    return host.strip().lower().rstrip(".")


def canonical_host_error(host: object) -> str | None:
    """None only for an exact lowercase DNS name or a canonical IP literal.

    Rejects case variants, trailing dots, empty labels, non-ASCII, and
    numeric-looking names such as 127.1 or 0x7f.1 that a resolver would
    silently reinterpret as an IP address.
    """
    if not isinstance(host, str) or not host:
        return "host is empty or not a string"
    try:
        ip = ipaddress.ip_address(host)
    except ValueError:
        ip = None
    if ip is not None:
        return None if host == ip.compressed else f"IP literal {host!r} is not in canonical form {ip.compressed!r}"
    if len(host) > 253:
        return "host is longer than 253 characters"
    labels = host.split(".")
    if not all(_DNS_LABEL.fullmatch(label) for label in labels):
        return "host must be lowercase ASCII DNS labels with no trailing dot or empty label"
    if _NUMERIC_TLD.fullmatch(labels[-1]):
        return "host looks like a non-canonical numeric address"
    return None


def canonical_origin(origin: object) -> tuple[tuple[str, str, int | None] | None, str | None]:
    """Exact-origin parse: only `scheme://host[:port]`, byte-for-byte canonical.

    Returns ((scheme, host, port), None) or (None, reason). urlsplit lowercases
    the scheme and host, so canonicality is enforced by rebuilding the origin
    and requiring string equality with the input.
    """
    if not isinstance(origin, str) or not origin:
        return None, "origin is missing"
    if any(ord(c) <= 0x20 or ord(c) >= 0x7F or c == "\\" for c in origin):
        return None, "origin contains whitespace, control, non-ASCII or backslash characters"
    try:
        parts = urlsplit(origin)
        port = parts.port
    except ValueError as e:
        return None, f"origin is malformed: {e}"
    if parts.scheme not in ("http", "https"):
        return None, f"unsupported origin scheme {parts.scheme!r}: use http (loopback only) or https"
    host = parts.hostname
    host_error = canonical_host_error(host)
    if host_error:
        return None, f"origin host is not canonical: {host_error}"
    if port is not None and (port == 0 or port == _DEFAULT_PORTS[parts.scheme]):
        return None, f"origin port {port} is invalid or an explicit scheme default: omit it"
    expected = f"{parts.scheme}://{'[' + host + ']' if ':' in host else host}" + (f":{port}" if port is not None else "")
    if origin != expected:
        return None, f"origin {origin!r} is not canonical: expected exactly {expected!r} (lowercase, no userinfo, path, query, fragment or trailing dot)"
    return (parts.scheme, host, port), None


def check_environment_guard(manifest: dict) -> str | None:
    env = manifest.get("environment")
    if not isinstance(env, dict):
        return "manifest.environment must be an object"
    if env.get("production_target_allowed") is not False:
        return "manifest.environment.production_target_allowed must be exactly false: this harness never targets production"
    origin = env.get("origin")
    if origin in (None, ""):
        return "manifest.environment.origin is required for --live"
    allowed = env.get("allowed_hosts")
    if not isinstance(allowed, list) or not allowed or not all(isinstance(h, str) for h in allowed):
        return "manifest.environment.allowed_hosts must be a non-empty list of host strings"
    # Production is refused on the normalized form first, so a case or trailing-dot
    # variant is reported as production rather than as a mere formatting error.
    try:
        origin_host = urlsplit(origin).hostname if isinstance(origin, str) else None
    except ValueError:
        origin_host = None
    for host in [origin_host, *allowed]:
        if isinstance(host, str) and _normalized_host(host) in PRODUCTION_HOSTS:
            return f"refusing production host {host!r}"
    for host in allowed:
        host_error = canonical_host_error(host)
        if host_error:
            return f"allowed_hosts entry {host!r} is not canonical: {host_error}"
    parsed, origin_error = canonical_origin(origin)
    if origin_error:
        return origin_error
    scheme, host, _port = parsed
    if host not in allowed:
        return f"origin host {host!r} is not in manifest.environment.allowed_hosts {allowed!r}"
    if scheme == "http" and not _is_loopback_host(host):
        return f"refusing plaintext http to non-loopback host {host!r}: use https, or http only against 127.0.0.1/::1/localhost"
    return None


# ---------------------------------------------------------------------------
# Budget: strict parse, and the single reservation point for every socket op.
# ---------------------------------------------------------------------------

REQUIRED_BUDGET_FIELDS = (
    "approved_max_usd",
    "max_calls",
    "max_input_tokens",
    "max_elapsed_seconds",
    "authorization_ref",
    "worst_case_usd_per_call",
)


class Budget(NamedTuple):
    approved_max_usd: float
    max_calls: int
    max_input_tokens: int
    max_elapsed_seconds: float
    worst_case_usd_per_call: float
    authorization_ref: str
    readiness_tokens: int  # declared upper bound of provider work one readiness probe can trigger.
    cold_open_tokens: int  # declared upper bound of provider work one authenticated request can trigger via a cold open.
    provider_work_basis: str  # where the two declared bounds come from; the client cannot observe server-side provider work.


def _is_count(value: object) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def _is_finite_number(value: object) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)


def _budget_value_error(b: Budget) -> str | None:
    """Value rules shared by parse_budget and RunBudget, so a Budget built by hand
    is held to the same numeric rules as one parsed from a manifest."""
    for name in ("max_calls", "max_input_tokens"):
        if not _is_count(getattr(b, name)) or getattr(b, name) <= 0:
            return f"budget.{name} must be a positive integer, got {getattr(b, name)!r}"
    for name in ("readiness_tokens", "cold_open_tokens"):
        if not _is_count(getattr(b, name)) or getattr(b, name) < 0:
            return f"budget.provider_work_bound.{name} must be an integer >= 0, got {getattr(b, name)!r}"
    if not _is_finite_number(b.max_elapsed_seconds) or b.max_elapsed_seconds <= 0:
        return f"budget.max_elapsed_seconds must be a positive finite number, got {b.max_elapsed_seconds!r}"
    if not _is_finite_number(b.approved_max_usd) or b.approved_max_usd < 0:
        return f"budget.approved_max_usd must be a finite number >= 0, got {b.approved_max_usd!r}"
    if not _is_finite_number(b.worst_case_usd_per_call) or b.worst_case_usd_per_call <= 0:
        return f"budget.worst_case_usd_per_call must be a positive finite number, got {b.worst_case_usd_per_call!r}"
    for name in ("authorization_ref", "provider_work_basis"):
        if not isinstance(getattr(b, name), str) or not getattr(b, name).strip():
            return f"budget.{name} must be a non-empty string"
    return None


def parse_budget(manifest: dict) -> tuple[Budget | None, str | None]:
    """Strict numeric parse: rejects missing, boolean, non-finite (NaN/Infinity,
    which json.loads accepts), negative, zero-where-meaningless and malformed
    values, and blocks when server-side provider work is left undeclared."""
    budget = manifest.get("budget")
    if not isinstance(budget, dict):
        return None, "manifest.budget must be an object"
    for field in REQUIRED_BUDGET_FIELDS:
        if budget.get(field) in (None, ""):
            return None, f"manifest.budget.{field} must be set (non-null) before --live"
    bound = budget.get("provider_work_bound")
    if not isinstance(bound, dict) or any(bound.get(f) in (None, "") for f in ("readiness_tokens", "cold_open_tokens", "basis")):
        return None, (
            "manifest.budget.provider_work_bound must declare readiness_tokens, cold_open_tokens and basis: provider work "
            "triggered server-side by readiness probes and cold opens is not observable from the client, so an undeclared bound blocks the run"
        )
    for field in ("automatic_reset", "auto_top_up"):
        if field in budget and budget[field] is not False:
            return None, f"manifest.budget.{field} must be exactly false when present"
    parsed = Budget(
        approved_max_usd=budget["approved_max_usd"],
        max_calls=budget["max_calls"],
        max_input_tokens=budget["max_input_tokens"],
        max_elapsed_seconds=budget["max_elapsed_seconds"],
        worst_case_usd_per_call=budget["worst_case_usd_per_call"],
        authorization_ref=budget["authorization_ref"],
        readiness_tokens=bound["readiness_tokens"],
        cold_open_tokens=bound["cold_open_tokens"],
        provider_work_basis=bound["basis"],
    )
    error = _budget_value_error(parsed)
    if error:
        return None, f"manifest.{error}"
    return parsed, None


def check_budget(manifest: dict) -> str | None:
    return parse_budget(manifest)[1]


class BudgetExhausted(Exception):
    """Raised instead of opening a socket when a cap or deadline forbids it."""

    def __init__(self, reason: str):
        super().__init__(reason)
        self.reason = reason


class RunBudget:
    """One run-wide ledger. reserve() is called immediately before every socket
    operation (readiness, initialize, notification, tools/call, session close),
    so no request is ever launched past a cap or deadline, however long it sat
    in a queue. It returns the wall-clock allowance for that one exchange: the
    remaining elapsed budget, never a floor above it. The exchange enforces it
    as a hard deadline (see _http_exchange), not as a socket-inactivity timeout.
    """

    def __init__(self, budget: Budget, clock=time.monotonic):
        error = _budget_value_error(budget)
        if error:
            raise ValueError(error)
        self.budget = budget
        self._clock = clock
        self._start = clock()
        self._lock = threading.Lock()
        self.stop_event = threading.Event()
        self._stopped_reason: str | None = None
        self.calls = 0
        self.tokens = 0
        self.usd = 0.0
        # attempted: reserved against the caps and about to open a socket.
        # sent: the request was fully written. completed: a whole response was read inside its deadline.
        self.attempted_by_operation: Counter = Counter()
        self.sent_by_operation: Counter = Counter()
        self.completed_by_operation: Counter = Counter()

    def elapsed(self) -> float:
        return self._clock() - self._start

    def remaining_elapsed(self) -> float:
        return self.budget.max_elapsed_seconds - self.elapsed()

    @property
    def stopped_reason(self) -> str | None:
        return self._stopped_reason

    def stop(self, reason: str) -> None:
        """Sticky: the first reason wins. Cancels queued work; does not itself
        forbid reserve(), so session cleanup stays possible after a non-cap stop."""
        with self._lock:
            if self._stopped_reason is None:
                self._stopped_reason = reason
        self.stop_event.set()

    def reserve(self, operation: str, tokens: int = 0, default_timeout_s: float = DEFAULT_CALL_TIMEOUT_S) -> float:
        # Argument checks come first and change no state: a negative token count
        # would otherwise credit the budget, and a non-finite timeout would
        # disable the deadline.
        if operation not in OPERATIONS:
            raise ValueError(f"unknown operation {operation!r}")
        if not _is_count(tokens) or tokens < 0:
            raise ValueError(f"tokens must be an integer >= 0, got {tokens!r}")
        if not _is_finite_number(default_timeout_s) or default_timeout_s <= 0:
            raise ValueError(f"default_timeout_s must be a positive finite number, got {default_timeout_s!r}")
        b = self.budget
        with self._lock:
            reason = None
            remaining = self.remaining_elapsed()
            if remaining <= 0:
                reason = "max_elapsed_seconds budget exhausted"
            elif self.calls + 1 > b.max_calls:
                reason = "max_calls budget exhausted"
            elif self.tokens + tokens > b.max_input_tokens:
                reason = "max_input_tokens budget exhausted"
            elif self.usd + b.worst_case_usd_per_call > b.approved_max_usd:
                reason = "approved_max_usd budget exhausted (worst-case precharge)"
            if reason is None:
                self.calls += 1
                self.tokens += tokens
                self.usd += b.worst_case_usd_per_call
                self.attempted_by_operation[operation] += 1
                return min(default_timeout_s, remaining)
        self.stop(reason)
        raise BudgetExhausted(reason)

    def mark_sent(self, operation: str) -> None:
        with self._lock:
            self.sent_by_operation[operation] += 1

    def mark_completed(self, operation: str) -> None:
        with self._lock:
            self.completed_by_operation[operation] += 1


def check_credentials(credential_dir: Path, accounts: list[dict]) -> tuple[dict[str, str] | None, str | None]:
    """Loads one private, per-account bearer credential per account.

    Each file must be `<credential_dir>/<account_id>.token`, a regular file
    (never a symlink -- that could point outside the private directory),
    mode exactly 0600, non-empty, one printable-ASCII token with no
    whitespace (anything else could inject header lines). Fails closed on
    the first problem found, before any network call: --live must never send
    even one real request for a workload it cannot fully authenticate.
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
        if not _CREDENTIAL_VALUE.fullmatch(value):
            return None, f"credential file for account {account['id']!r} must hold one printable ASCII token with no whitespace: {path}"
        creds[account["id"]] = value
    return creds, None


# ---------------------------------------------------------------------------
# Tool arguments and the conservative input-token precharge.
# ---------------------------------------------------------------------------

def tool_arguments(verb: str, account_id: str, rep: int, seq: int, tokens: int, fact_id: str | None = None) -> dict:
    if verb == "recall":
        return {"query": harness.render_text(f"{account_id}:recall:{rep}:{seq}", tokens)}
    if verb == "remember":
        return {"fact": harness.render_text(f"{account_id}:remember:{rep}:{seq}", tokens), "provenance": "hosted-load-harness T23.60"}
    if verb == "forget":
        return {"id": fact_id}
    raise ValueError(f"unknown verb {verb!r}")


def argument_bytes(arguments: dict) -> int:
    """Precharge for one tools/call: the encoded bytes of the arguments actually
    sent. A tokenizer emits at most one token per byte, so bytes upper-bound the
    embedded input tokens; the synthetic per-arrival `tokens` count is words and
    understates them several-fold. No tokenizer is available to verify a tighter
    figure, and provider-added special tokens are covered only by the JSON framing."""
    return len(json.dumps(arguments).encode("utf-8"))


def argument_bytes_upper_bound(verb: str, tokens: int) -> int:
    """Seed-independent upper bound of argument_bytes(tool_arguments(...)), for preflight."""
    filler = "x" * (harness.max_rendered_bytes(tokens) if verb in ("recall", "remember") else FORGET_ID_BYTES_ALLOWANCE)
    args = {"recall": {"query": filler}, "remember": {"fact": filler, "provenance": "hosted-load-harness T23.60"}, "forget": {"id": filler}}[verb]
    return argument_bytes(args)


# ---------------------------------------------------------------------------
# Offline preparation: authority inputs, guards and full-workload budget coverage.
# ---------------------------------------------------------------------------

class LivePlan(NamedTuple):
    accounts: list
    credentials: dict
    budget: Budget
    arrivals: list


def _nonempty_str(value: object) -> bool:
    return isinstance(value, str) and bool(value.strip())


def check_authority_inputs(manifest: dict, workload: dict) -> str | None:
    """The reviewer/provider/source inputs that no worker can self-supply."""
    if not workload.get("frozen") or not _nonempty_str(workload.get("reviewer")) or not _nonempty_str(workload.get("review_date")):
        return "workload is not reviewer-frozen: workload.frozen, reviewer and review_date must all be set by the task41 reviewer"
    if workload.get("repetitions") != FROZEN_REPETITIONS:
        return f"workload.repetitions must be the frozen {FROZEN_REPETITIONS}, got {workload.get('repetitions')!r}"
    provider = manifest.get("provider")
    if not isinstance(provider, dict):
        return "manifest.provider must be an object"
    for field in ("version_pin", "serving_provider", "privacy_review_ref"):
        if not _nonempty_str(provider.get(field)):
            return f"manifest.provider.{field} must be set: exact provider authority is required before --live"
    if not _is_count(provider.get("dimensions")) or provider["dimensions"] <= 0:
        return "manifest.provider.dimensions must be a positive integer"
    if not re.fullmatch(r"[0-9a-f]{40}", str(manifest.get("source_sha") or "")):
        return "manifest.source_sha must be the 40-hex source commit deployed under test"
    if not re.fullmatch(r"[0-9a-f]{64}", str(manifest.get("binary_sha256") or "")):
        return "manifest.binary_sha256 must be the 64-hex sha256 of the deployed binary under test"
    return None


def preflight_budget(budget: Budget, workload: dict, accounts: list[dict], arrivals: list[dict]) -> str | None:
    """Refuse before spending when the caps cannot cover the whole frozen workload.

    Lower bounds on need, upper bounds on cost: one readiness probe, three
    authenticated operations per account (initialize, notification, close),
    every offered request at its byte-based token bound, the declared
    provider-work bound on every authenticated operation, and every
    repetition's full schedule span. A run that could only ever be cut short
    by its own budget must not start.
    """
    reps = workload["repetitions"]
    offered = reps * len(arrivals)
    authenticated = 3 * len(accounts) + offered
    calls = 1 + authenticated
    tokens = (
        budget.readiness_tokens
        + budget.cold_open_tokens * authenticated
        + reps * sum(argument_bytes_upper_bound(a["verb"], a["tokens"]) for a in arrivals)
    )
    usd = calls * budget.worst_case_usd_per_call
    elapsed = reps * sum(p["minutes"] * 60 for p in workload["phases"])
    shortfalls = []
    if calls > budget.max_calls:
        shortfalls.append(f"max_calls {budget.max_calls} < required {calls}")
    if tokens > budget.max_input_tokens:
        shortfalls.append(f"max_input_tokens {budget.max_input_tokens} < byte-based required {tokens}")
    if usd > budget.approved_max_usd:
        shortfalls.append(f"approved_max_usd {budget.approved_max_usd} < worst-case required {round(usd, 6)}")
    if elapsed > budget.max_elapsed_seconds:
        shortfalls.append(f"max_elapsed_seconds {budget.max_elapsed_seconds} < required schedule {elapsed}")
    if shortfalls:
        return "budget cannot cover the full frozen workload (" + "; ".join(shortfalls) + ")"
    return None


def prepare_live(manifest: dict, workload: dict) -> tuple[LivePlan | None, str | None]:
    """Every offline guard, in order, with zero sockets. Not called by main()."""
    error = check_authority_inputs(manifest, workload)
    if error:
        return None, error
    error = check_environment_guard(manifest)
    if error:
        return None, error
    budget, error = parse_budget(manifest)
    if error:
        return None, error
    try:
        accounts = harness.build_accounts(workload["cardinalities"], manifest.get("profile", "paid"))
    except ValueError as e:
        return None, str(e)
    credential_dir = (manifest.get("live") or {}).get("credential_dir")
    if not credential_dir:
        return None, "manifest.live.credential_dir is required for --live"
    arrivals = harness.generate_arrivals(workload, accounts, random.Random(BASE_SEED))
    error = preflight_budget(budget, workload, accounts, arrivals)
    if error:
        return None, error
    credentials, error = check_credentials(Path(credential_dir), accounts)
    if error:
        return None, error
    return LivePlan(accounts=accounts, credentials=credentials, budget=budget, arrivals=arrivals), None


def run_fixtures(manifest: dict, workload: dict) -> dict:
    accounts = harness.build_accounts(workload["cardinalities"], manifest.get("profile", "paid"))
    reps = []
    base_seed = BASE_SEED
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


# ---------------------------------------------------------------------------
# The candidate MCP-over-HTTP client.
# ---------------------------------------------------------------------------

class McpProtocolError(Exception):
    """A wire-level violation. Messages are fixed strings: they never carry
    upstream-controlled text, so they are safe to record."""


class McpHttpStatusError(McpProtocolError):
    """An HTTP status the client will not treat as a reply; carries it for classification."""

    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status


class ExchangeDeadlineExceeded(TimeoutError):
    """The exchange hit its wall-clock deadline (an inactivity timeout never raises this)."""


def _connect(scheme: str, host: str, port: int | None, deadline: float, hold) -> socket.socket:
    """Connect (and, for https, complete the TLS handshake) without ever leaving
    a socket the watchdog cannot see: every socket created is passed to hold()
    before it blocks. Each connect attempt is bounded by the time left, so a
    multi-address host cannot spend the deadline once per address."""
    port = _DEFAULT_PORTS[scheme] if port is None else port
    last_error: OSError | None = None
    sock = None
    for family, socktype, proto, _canon, sockaddr in socket.getaddrinfo(host, port, type=socket.SOCK_STREAM):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ExchangeDeadlineExceeded("exchange deadline reached")
        candidate = socket.socket(family, socktype, proto)
        hold(candidate)
        candidate.settimeout(remaining)
        try:
            candidate.connect(sockaddr)
        except OSError as e:
            last_error = e
            candidate.close()
            continue
        sock = candidate
        break
    if sock is None:
        raise last_error or OSError("name resolution returned no address")
    if scheme == "https":
        # do_handshake_on_connect=False: keep a reference to the TLS socket, which
        # takes over the plain socket's descriptor, before the handshake can block.
        sock = ssl.create_default_context().wrap_socket(sock, server_hostname=host, do_handshake_on_connect=False)
        hold(sock)
        sock.settimeout(max(deadline - time.monotonic(), 0.001))
        sock.do_handshake()
    sock.settimeout(max(deadline - time.monotonic(), 0.001))
    return sock


def _http_exchange(method: str, origin: tuple[str, str, int | None], path: str, headers: dict, body: bytes | None, timeout_s: float, on_sent=None) -> tuple[int, http.client.HTTPMessage, bytes]:
    """One HTTP exchange bounded by a hard wall-clock deadline of `timeout_s`.

    http.client's socket timeout is an inactivity timeout: a peer that drips
    one byte per interval, or sends a byte and stalls, keeps a read alive
    beyond it. So the exchange owns its sockets: it connects itself, registers
    every socket it creates (the plain one and the TLS one) with a watchdog,
    and the watchdog shuts them all down at the deadline, waking any blocked
    recv. It never relies on conn.sock: getresponse() clears conn.sock when the
    response is `Connection: close` (every response to this client), after
    which only the response's file object still references the live socket.
    Everything after the deadline is reported as ExchangeDeadlineExceeded, and
    the response, the connection and every owned socket are closed on every
    exit path. The client never follows a redirect (a 3xx is just a status
    returned to the caller) and http.client honors no environment proxy, so
    the exchange dials exactly the origin given.

    Not bounded: name resolution. getaddrinfo() cannot be interrupted, so for a
    hostname origin the deadline covers everything after resolution only; an
    IP-literal origin resolves instantly, so its deadline is end to end.
    """
    scheme, host, port = origin
    deadline = time.monotonic() + timeout_s
    expired = threading.Event()
    held: list[socket.socket] = []
    held_lock = threading.Lock()

    def hold(sock: socket.socket) -> None:
        with held_lock:
            held.append(sock)

    def abort() -> None:
        expired.set()
        with held_lock:
            owned = list(held)
        for sock in owned:
            try:
                sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass  # already closed, detached into a TLS wrapper, or never connected

    watchdog = threading.Timer(timeout_s, abort)
    watchdog.daemon = True
    watchdog.start()
    conn = resp = None
    try:
        try:
            sock = _connect(scheme, host, port, deadline, hold)
            conn = (http.client.HTTPSConnection if scheme == "https" else http.client.HTTPConnection)(host, port, timeout=timeout_s)
            conn.sock = sock
            conn.request(method, path, body=body, headers={**headers, "Connection": "close"})
            if on_sent is not None:
                on_sent()
            resp = conn.getresponse()
            status, resp_headers = resp.status, resp.headers
            chunks: list[bytes] = []
            total = 0
            while True:
                chunk = resp.read1(65536)  # read(n) on a buffered file would loop until n bytes or EOF
                if not chunk:
                    break
                total += len(chunk)
                if total > MAX_BODY_BYTES:
                    raise McpProtocolError("response body exceeds the size bound")
                if time.monotonic() >= deadline:
                    raise ExchangeDeadlineExceeded("exchange deadline reached")
                chunks.append(chunk)
        except (OSError, http.client.HTTPException, ValueError):  # ValueError: an SSL read after the watchdog unwrapped the socket
            if expired.is_set() or time.monotonic() >= deadline:
                raise ExchangeDeadlineExceeded("exchange deadline reached") from None
            raise
        if expired.is_set() or time.monotonic() > deadline:
            raise ExchangeDeadlineExceeded("exchange deadline reached")
        return status, resp_headers, b"".join(chunks)
    finally:
        watchdog.cancel()
        if resp is not None:
            resp.close()
        if conn is not None:
            conn.close()
        for sock in held:
            try:
                sock.close()
            except OSError:
                pass


class McpSession:
    """One real MCP-over-HTTP session: initialize -> notifications/initialized -> tools/call.

    Every request goes over real HTTP against origin + MCP_PATH; there is no
    injected transport function here on purpose -- tests point origin at a real
    local http.server fixture instead, so the client's actual request/response
    (de)serialization is what gets exercised. Every socket operation, including
    the notification and the session close, reserves against the RunBudget
    immediately before it is opened. Nothing upstream-controlled is echoed into
    an error: failures carry a status number or a fixed class only.
    """

    def __init__(self, origin: str, credential: str, run: RunBudget):
        parsed, error = canonical_origin(origin)
        if error:
            raise ValueError(error)
        if not isinstance(credential, str) or not _CREDENTIAL_VALUE.fullmatch(credential):
            raise ValueError("credential must be one printable ASCII token with no whitespace")
        self._origin = parsed
        self._credential = credential
        self._run = run
        self._session_id: str | None = None
        self._initialized = False
        self.closed = False
        self._next_id = 1
        self._id_lock = threading.Lock()

    @property
    def has_server_session(self) -> bool:
        return self._session_id is not None

    @property
    def initialized(self) -> bool:
        return self._initialized

    def _next_request_id(self) -> int:
        with self._id_lock:
            rid = self._next_id
            self._next_id += 1
            return rid

    def _request(self, method: str, operation: str, body: bytes | None, arg_bytes: int):
        headers = {"Accept": "application/json", "Authorization": f"Bearer {self._credential}"}
        if body is not None:
            headers["Content-Type"] = "application/json"
        if self._session_id is not None:
            headers["Mcp-Session-Id"] = self._session_id
            headers["MCP-Protocol-Version"] = PROTOCOL_VERSION
        # Any authenticated request can make the gateway open a brain, which the
        # client cannot observe: charge the declared cold-open bound on each.
        timeout_s = self._run.reserve(operation, arg_bytes + self._run.budget.cold_open_tokens)  # immediately before the socket
        start = time.monotonic()
        status, resp_headers, raw = _http_exchange(method, self._origin, MCP_PATH, headers, body, timeout_s, on_sent=lambda: self._run.mark_sent(operation))
        latency_s = time.monotonic() - start
        self._run.mark_completed(operation)
        return status, resp_headers, raw, latency_s

    def _post(self, payload: dict, operation: str, arg_bytes: int, expect_body: bool):
        status, resp_headers, raw, latency_s = self._request("POST", operation, json.dumps(payload).encode("utf-8"), arg_bytes)
        if 300 <= status < 400:
            raise McpHttpStatusError(status, f"refused HTTP {status} redirect")
        if not expect_body:
            if status != 202:
                raise McpHttpStatusError(status, f"expected 202 Accepted for a notification, got {status}")
            return None, resp_headers, latency_s
        if status != 200:
            raise McpHttpStatusError(status, f"unexpected HTTP status {status}")
        try:
            parsed = json.loads(raw)
        except (json.JSONDecodeError, UnicodeDecodeError):
            raise McpProtocolError("non-JSON response body") from None
        if not isinstance(parsed, dict):
            raise McpProtocolError("response body is not a JSON-RPC object")
        return parsed, resp_headers, latency_s

    @staticmethod
    def _jsonrpc_code(error: object) -> int | None:
        code = error.get("code") if isinstance(error, dict) else None
        return code if _is_count(code) and -32768 <= code <= 32767 else None

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
        parsed, headers, latency_s = self._post(payload, "initialize", 0, expect_body=True)
        if parsed.get("id") != rid:
            raise McpProtocolError("initialize response id does not match the request id")
        if parsed.get("error") is not None:
            raise McpProtocolError(f"initialize failed with JSON-RPC error code {self._jsonrpc_code(parsed['error'])}")
        session_id = headers.get("Mcp-Session-Id")
        if not session_id:
            raise McpProtocolError("initialize succeeded but the server did not return Mcp-Session-Id")
        if not _SESSION_ID.fullmatch(session_id):
            raise McpProtocolError("server returned a malformed Mcp-Session-Id")
        self._session_id = session_id
        self._post({"jsonrpc": "2.0", "method": "notifications/initialized"}, "notification", 0, expect_body=False)
        self._initialized = True
        return latency_s

    def call_tool(self, name: str, arguments: dict) -> dict:
        if not self._initialized:
            raise McpProtocolError("call_tool before initialize")
        rid = self._next_request_id()
        payload = {"jsonrpc": "2.0", "id": rid, "method": "tools/call", "params": {"name": name, "arguments": arguments}}
        parsed, _headers, latency_s = self._post(payload, "tools_call", argument_bytes(arguments), expect_body=True)
        if parsed.get("id") != rid:
            raise McpProtocolError("tools/call response id does not match the request id")
        if parsed.get("error") is not None:
            return {"ok": False, "level": "protocol", "jsonrpc_code": self._jsonrpc_code(parsed["error"]), "latency_s": latency_s}
        result = parsed.get("result")
        if not isinstance(result, dict):
            raise McpProtocolError("tools/call reply has neither error nor a result object")
        is_error = bool(result.get("isError"))
        content = result.get("content")
        first = content[0] if isinstance(content, list) and content and isinstance(content[0], dict) else {}
        text = first.get("text") if first.get("type") == "text" and isinstance(first.get("text"), str) else None
        parsed_text = None
        if text is not None:
            try:
                parsed_text = json.loads(text)
            except json.JSONDecodeError:
                parsed_text = None
        return {"ok": not is_error, "level": "tool", "is_error": is_error, "text": parsed_text, "latency_s": latency_s}

    def close(self) -> tuple[str, str | None]:
        """(status, detail). status is "closed" only when the server confirmed the
        DELETE with 200/204; otherwise "failed" (detail: a class or HTTP status)
        or "not_attempted" (a cap or deadline forbade the socket)."""
        if self._session_id is None:
            return "no_session", None
        if self.closed:
            return "closed", None
        try:
            status, _headers, _raw, _latency = self._request("DELETE", "close", None, 0)
        except BudgetExhausted:
            return "not_attempted", "budget"
        except (*TRANSPORT_ERRORS, McpProtocolError) as e:
            return "failed", error_class(e)
        if status in (200, 204):
            self.closed = True
            return "closed", None
        return "failed", f"http_{status}"


def check_readiness(origin: str, run: RunBudget) -> str | None:
    """The first socket operation of a run: a read-only, unauthenticated GET of
    /readyz. It reserves against the same RunBudget as every other operation,
    charging the declared readiness provider-work bound."""
    parsed, origin_error = canonical_origin(origin)
    if origin_error:
        return origin_error
    try:
        timeout_s = run.reserve("readiness", run.budget.readiness_tokens, READINESS_TIMEOUT_S)
    except BudgetExhausted as e:
        return f"readiness probe not sent: {e.reason}"
    try:
        status, _headers, _body = _http_exchange("GET", parsed, "/readyz", {}, None, timeout_s, on_sent=lambda: run.mark_sent("readiness"))
    except (*TRANSPORT_ERRORS, McpProtocolError) as e:
        return f"readiness probe failed: {error_class(e)}"
    run.mark_completed("readiness")
    if 300 <= status < 400:
        return f"refused HTTP {status} redirect from the readiness probe"
    if status != 200:
        return f"readiness probe returned HTTP {status}"
    return None


# ---------------------------------------------------------------------------
# The candidate run: schedule, dispatch, classify, evaluate.
# ---------------------------------------------------------------------------

def _classify_status(status: int) -> str:
    if status == 429:
        return REJECTED_ADMISSION
    if 500 <= status < 600:
        return UNEXPECTED_5XX
    return PROTOCOL_ERROR


def evaluate_live_thresholds(workload: dict, records: list[dict]) -> dict:
    """Published thresholds over one repetition's offered requests.

    Completion, 5xx and admission rates are over ALL offered steady-state
    requests: skipped, cancelled and rejected requests stay in the
    denominator. Anything this client cannot measure is pass=None, never
    True: unmeasured is not passed.
    """
    t = workload["thresholds"]
    checks: dict = {}
    steady = [r for r in records if r["phase"] == STEADY_PHASE]

    def unmeasured(limit, reason):
        return {"limit": limit, "observed": None, "pass": None, "reason": reason}

    def rate(limit, count, maximum=True):
        observed = count / len(steady) * 100.0
        return {"limit": limit, "observed": observed, "pass": observed <= limit if maximum else observed >= limit}

    if not steady:
        why = f"no offered requests in a phase named {STEADY_PHASE!r}"
        for name in ("min_offered_completion_pct", "unexpected_5xx_max_pct", "unexpected_admission_rejection_max_pct", "recall_p95_max_s", "remember_p95_max_s"):
            checks[name] = unmeasured(t[name], why)
    else:
        n_ok = sum(1 for r in steady if r["outcome"] == OK)
        checks["min_offered_completion_pct"] = rate(t["min_offered_completion_pct"], n_ok, maximum=False)
        checks["unexpected_5xx_max_pct"] = rate(t["unexpected_5xx_max_pct"], sum(1 for r in steady if r["outcome"] == UNEXPECTED_5XX))
        checks["unexpected_admission_rejection_max_pct"] = rate(t["unexpected_admission_rejection_max_pct"], sum(1 for r in steady if r["outcome"] == REJECTED_ADMISSION))
        for verb, name in (("recall", "recall_p95_max_s"), ("remember", "remember_p95_max_s")):
            latencies = [r["latency_s"] for r in steady if r["verb"] == verb and r["outcome"] == OK and r.get("latency_s") is not None]
            if latencies:
                p95 = harness.percentile(latencies, 95)
                checks[name] = {"limit": t[name], "observed": p95, "pass": p95 <= t[name], "samples": len(latencies)}
            else:
                checks[name] = unmeasured(t[name], f"no successful steady-state {verb} latency samples")
    checks["cold_ready_max_s"] = unmeasured(t["cold_ready_max_s"], "the cold flag is a schedule label; this client opens no cold brain and measures no cold-ready time")
    for name in ("cpu_max_pct", "rss_max_pct_of_ram", "disk_max_pct"):
        checks[name] = unmeasured(t[name], "host telemetry is not collected by this client")
    durability = sum(1 for r in records if r.get("durability_error"))
    checks["isolation_durability_errors_max"] = {
        "limit": t["isolation_durability_errors_max"],
        "observed": durability,
        "pass": False if durability > t["isolation_durability_errors_max"] else None,
        "reason": "durability counts forget failures on ids this run remembered; no cross-tenant isolation probe exists, so a zero is not a pass",
    }
    return checks


def _scrub(value, secrets: list[str]):
    """Defense in depth: no field carries upstream text any more, but a
    credential must never reach the output by any route."""
    secrets = [x for x in secrets if x]  # str.replace("", ...) would corrupt every string
    if isinstance(value, str):
        for secret in secrets:
            value = value.replace(secret, REDACTED)
        return value
    if isinstance(value, list):
        return [_scrub(v, secrets) for v in value]
    if isinstance(value, dict):
        return {_scrub(k, secrets): _scrub(v, secrets) for k, v in value.items()}
    return value


def _execute(origin: str, workload: dict, accounts: list[dict], credentials: dict[str, str], arrivals: list[dict], run: RunBudget, records: list) -> list[McpSession]:
    """Readiness, sessions, then the schedule. Fills `records` by flat index
    (rep * len(arrivals) + seq); an index left None was never dispatched."""
    sessions: dict[str, McpSession] = {}
    created: list[McpSession] = []
    readiness_error = check_readiness(origin, run)
    if readiness_error:
        run.stop(readiness_error)
        return created
    for account in accounts:
        try:
            session = McpSession(origin, credentials[account["id"]], run)
        except (ValueError, KeyError) as e:
            run.stop(f"cannot create a session for account {account['id']!r}: {type(e).__name__}")
            return created
        created.append(session)
        try:
            session.initialize()
        except BudgetExhausted:
            return created
        except (*TRANSPORT_ERRORS, McpProtocolError, ValueError) as e:
            run.stop(f"session initialize failed for account {account['id']!r}: {error_class(e)}")
            return created
        sessions[account["id"]] = session

    remembered: dict[str, dict[str, None]] = {a["id"]: {} for a in accounts}
    remembered_lock = threading.Lock()
    n = len(arrivals)

    def dispatch(rep: int, seq: int, arrival: dict, scheduled_at: float) -> None:
        try:
            dispatch_inner(rep, seq, arrival, scheduled_at)
        except Exception as e:  # noqa: BLE001 - a worker bug must surface as a failed outcome, never a lost record
            records[rep * n + seq] = {
                "rep": rep, "seq": seq, "phase": arrival["phase"], "account": arrival["account"], "verb": arrival["verb"],
                "tokens": arrival["tokens"], "cold_flagged": arrival["cold"], "outcome": CLIENT_ERROR, "error_class": error_class(e),
            }

    def dispatch_inner(rep: int, seq: int, arrival: dict, scheduled_at: float) -> None:
        idx = rep * n + seq
        account_id, verb, tokens = arrival["account"], arrival["verb"], arrival["tokens"]
        base = {
            "rep": rep, "seq": seq, "phase": arrival["phase"], "account": account_id, "verb": verb,
            "tokens": tokens, "cold_flagged": arrival["cold"], "queue_delay_s": round(time.monotonic() - scheduled_at, 6),
        }
        if run.stop_event.is_set():
            records[idx] = {**base, "outcome": NOT_DISPATCHED, "reason": "run stopped before this queued request started"}
            return
        session = sessions[account_id]
        fact_id = None
        if verb == "forget":
            with remembered_lock:
                ids = remembered[account_id]
                fact_id = ids.popitem()[0] if ids else None
            if fact_id is None:
                records[idx] = {**base, "outcome": SKIPPED_FORGET, "reason": "no fact id remembered by this run for the account: seeded state is not implemented, so the offered forget was not executed"}
                return
        args = tool_arguments(verb, account_id, rep, seq, tokens, fact_id)

        def restore_id():
            if fact_id is not None:
                with remembered_lock:
                    remembered[account_id][fact_id] = None

        try:
            outcome = session.call_tool(verb, args)
        except BudgetExhausted as e:
            restore_id()
            records[idx] = {**base, "outcome": NOT_DISPATCHED, "reason": e.reason}
            return
        except McpHttpStatusError as e:
            records[idx] = {**base, "outcome": _classify_status(e.status), "http_status": e.status}
            return
        except McpProtocolError as e:
            records[idx] = {**base, "outcome": PROTOCOL_ERROR, "error": str(e)}
            return
        except TRANSPORT_ERRORS as e:
            records[idx] = {**base, "outcome": NETWORK_ERROR, "error_class": error_class(e)}
            return

        record = {**base, "latency_s": outcome["latency_s"], "level": outcome["level"]}
        if outcome["level"] == "protocol":
            records[idx] = {**record, "outcome": PROTOCOL_ERROR, "jsonrpc_code": outcome["jsonrpc_code"]}
        elif outcome["is_error"]:
            records[idx] = {**record, "outcome": TOOL_ERROR, "durability_error": verb == "forget"}
        elif verb == "remember":
            text = outcome.get("text")
            new_id = text.get("id") if isinstance(text, dict) else None
            if isinstance(new_id, str) and new_id:
                with remembered_lock:
                    remembered[account_id][new_id] = None
                records[idx] = {**record, "outcome": OK}
            else:
                records[idx] = {**record, "outcome": PROTOCOL_ERROR, "error": "remember reported success without a fact id"}
        else:
            records[idx] = {**record, "outcome": OK}

    span_s = sum(p["minutes"] * 60 for p in workload["phases"])
    origin_t = time.monotonic()
    executor = ThreadPoolExecutor(max_workers=workload["concurrency"]["clients"])
    try:
        for rep in range(workload["repetitions"]):
            for seq, arrival in enumerate(arrivals):
                if run.stop_event.is_set():
                    break
                scheduled_at = origin_t + rep * span_s + arrival["t"]
                wait_s = scheduled_at - time.monotonic()
                # Decide before committing to the wait: a distant arrival must not
                # hold the loop asleep past a deadline that is already known.
                if wait_s > run.remaining_elapsed():
                    run.stop("max_elapsed_seconds budget exhausted before the next scheduled arrival")
                    break
                if wait_s > 0 and run.stop_event.wait(wait_s):
                    break
                executor.submit(dispatch, rep, seq, arrival, scheduled_at)
            if run.stop_event.is_set():
                break
    finally:
        # Each in-flight exchange carries its own hard deadline, so this wait is bounded.
        executor.shutdown(wait=True, cancel_futures=run.stop_event.is_set())
    return created


def run_live(manifest: dict, workload: dict, accounts: list[dict], credentials: dict[str, str]) -> dict:
    """Candidate run. Re-validates the environment and budget itself (never
    trusts the caller), spends nothing unless both pass, and never reports
    better than PARTIAL."""
    arrivals = harness.generate_arrivals(workload, accounts, random.Random(BASE_SEED))
    reps = workload["repetitions"]
    records: list = [None] * (reps * len(arrivals))
    run = None
    created: list[McpSession] = []
    reason = check_environment_guard(manifest)
    budget = None
    if reason is None:
        budget, reason = parse_budget(manifest)
    if reason is None:
        run = RunBudget(budget)
        created = _execute(manifest["environment"]["origin"], workload, accounts, credentials, arrivals, run, records)
        reason = run.stopped_reason  # captured before cleanup, so a cleanup refusal cannot rewrite it
        elapsed_s = run.elapsed()
    else:
        elapsed_s = 0.0
    cleanup = {"accounts_initialized": sum(1 for s in created if s.initialized), "server_sessions_created": 0, "closed_confirmed": 0, "close_failed": {}, "close_not_attempted": 0}
    for session in created:
        if not session.has_server_session:
            continue
        cleanup["server_sessions_created"] += 1
        status, detail = session.close()
        if status == "closed":
            cleanup["closed_confirmed"] += 1
        elif status == "failed":
            cleanup["close_failed"][detail] = cleanup["close_failed"].get(detail, 0) + 1
        else:
            cleanup["close_not_attempted"] += 1
    cleanup["sessions_left_open"] = cleanup["server_sessions_created"] - cleanup["closed_confirmed"]
    total_elapsed_s = run.elapsed() if run else 0.0
    return _build_result(workload, arrivals, records, run, budget, reason, elapsed_s, total_elapsed_s, cleanup, credentials, (manifest.get("environment") or {}).get("origin"))


def _deadline_scope(origin: object) -> str:
    parsed, _error = canonical_origin(origin)
    if parsed is None:
        return "not applicable: no exchange was attempted"
    try:
        ipaddress.ip_address(parsed[1])
    except ValueError:
        return "excludes name resolution: getaddrinfo() cannot be interrupted, so a hostname origin has no hard end-to-end deadline guarantee"
    return "end to end: an IP-literal origin needs no name resolution"


def _build_result(workload, arrivals, records, run, budget, reason, elapsed_s, total_elapsed_s, cleanup, credentials, origin) -> dict:
    reps = workload["repetitions"]
    n = len(arrivals)
    full = []
    for idx, rec in enumerate(records):
        if rec is None:
            rep, seq = divmod(idx, n)
            a = arrivals[seq]
            rec = {
                "rep": rep, "seq": seq, "phase": a["phase"], "account": a["account"], "verb": a["verb"], "tokens": a["tokens"],
                "cold_flagged": a["cold"], "outcome": NOT_DISPATCHED, "reason": reason or "cancelled before dispatch",
            }
        full.append(rec)
    by_repetition = []
    for rep in range(reps):
        rep_records = full[rep * n:(rep + 1) * n]
        counts = Counter(r["outcome"] for r in rep_records)
        by_repetition.append({
            "rep": rep,
            "offered": len(rep_records),
            "outcome_counts": {o: counts.get(o, 0) for o in ALL_OUTCOMES},
            "threshold_evaluation": evaluate_live_thresholds(workload, rep_records),
        })
    total = Counter(r["outcome"] for r in full)
    not_dispatched = total.get(NOT_DISPATCHED, 0)
    over_cap = max(0.0, elapsed_s - budget.max_elapsed_seconds) if budget else 0.0
    overran = over_cap > DEADLINE_JITTER_S
    failed_checks = [
        f"rep {rep['rep']}: {name}" for rep in by_repetition for name, c in rep["threshold_evaluation"].items() if c["pass"] is False
    ]
    if reason or not_dispatched:
        status = "BLOCKED"
        reason = reason or f"{not_dispatched} offered requests were not dispatched"
    elif failed_checks or overran:
        status = "FAIL"
        reason = "; ".join(failed_checks + ([f"elapsed cap overrun by {round(over_cap, 3)}s"] if overran else []))
    else:
        status = "PARTIAL"
        reason = "no published threshold failed and no offered request was left undispatched; qualification gaps remain"
    skipped = total.get(SKIPPED_FORGET, 0)
    non_steady_failures = sum(1 for r in full if r["phase"] != STEADY_PHASE and r["outcome"] not in (OK, NOT_DISPATCHED))
    gaps = [
        "seeded account/brain/cardinality/storage state is not verified by this client",
        "the cold flag is a schedule label only: no cold-open workload was executed and no cold-ready time was measured",
        "provider work triggered by readiness and cold opens is charged at the operator-declared provider_work_bound and is not observed by this client",
        "input tokens are precharged as encoded argument bytes, an upper bound rather than a provider-billed count",
        "no cross-tenant isolation probe exists; durability covers only forget of ids this run remembered",
        "the quota-boundary saturation run is separate and not implemented in this client",
        "hostname origins: the per-exchange wall deadline is unqualified because getaddrinfo() cannot be interrupted by the standard library and no bounded resolver is implemented; only IP-literal origins have an end-to-end deadline",
    ]
    unevaluated = sorted({name for rep in by_repetition for name, c in rep["threshold_evaluation"].items() if c["pass"] is None})
    if unevaluated:
        gaps.append(f"thresholds with no measured pass/fail (unmeasured is not passed): {unevaluated}")
    if not workload.get("reviewer"):
        gaps.append("thresholds are not reviewer-frozen (workload.reviewer is unset)")
    if skipped:
        gaps.append(f"{skipped} forget requests were offered with no fact id to forget: seeding is not implemented")
    if non_steady_failures:
        gaps.append(f"{non_steady_failures} non-ok outcomes fall outside the steady phase, where no published threshold applies: reviewer disposition required")
    if cleanup["sessions_left_open"]:
        gaps.append(f"{cleanup['sessions_left_open']} server sessions were not confirmed closed: they remain until the server's idle timeout")
    result = {
        "mode": "live",
        "status": status,
        "reason": reason,
        "qualified": False,
        "qualification_gaps": gaps,
        "repetitions_planned": reps,
        "repetitions_fully_dispatched": sum(1 for r in by_repetition if r["outcome_counts"][NOT_DISPATCHED] == 0),
        "offered_total": len(full),
        "outcome_counts": {o: total.get(o, 0) for o in ALL_OUTCOMES},
        "by_repetition": by_repetition,
        "cold_requests_flagged": sum(1 for r in full if r["cold_flagged"]),
        "cold_workload_verified": False,
        "seeded_state_verified": False,
        # attempted = reserved and a socket about to open; sent = request fully written; completed = whole response read inside its deadline.
        "calls_attempted": run.calls if run else 0,
        "calls_sent": sum(run.sent_by_operation.values()) if run else 0,
        "calls_completed": sum(run.completed_by_operation.values()) if run else 0,
        "calls_attempted_by_operation": dict(run.attempted_by_operation) if run else {},
        "calls_sent_by_operation": dict(run.sent_by_operation) if run else {},
        "calls_completed_by_operation": dict(run.completed_by_operation) if run else {},
        "input_tokens_precharged_upper_bound": run.tokens if run else 0,
        "usd_worst_case_reserved": round(run.usd, 6) if run else 0.0,
        "elapsed_s": round(elapsed_s, 3),
        "elapsed_over_cap_s": round(over_cap, 3),
        "exchange_deadline_scope": _deadline_scope(origin),
        "elapsed_including_cleanup_s": round(total_elapsed_s, 3),
        "cleanup": cleanup,
        "results": full,
    }
    return _scrub(result, list(credentials.values()))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--fixtures", action="store_true")
    mode.add_argument("--live", action="store_true")
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    if args.live:
        # Unconditional: nothing below this branch, and nothing in prepare_live
        # or run_live, is reachable from the CLI.
        result = {
            "mode": "live", "status": "BLOCKED", "calls_used": 0,
            "tokens_used": 0, "usd_used_worst_case": 0,
            "reason": LIVE_GATE_REASON,
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

    result = run_fixtures(manifest, workload)
    result["source_workload_sha256"] = harness.sha256_file(workload_path)
    result["source_manifest_sha256"] = harness.sha256_file(args.manifest)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"status": "PASS", "output": str(args.output), "replay_determinism_verified": result["replay_determinism_verified"]}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
