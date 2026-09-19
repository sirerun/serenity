"""Qualification manifest loading and validation
(docs/launch/hosted-completion/evidence.md, qualification.example.json).

The shared template only carries the fields common to every hosted
completion task's live phase (provider/mail/stripe/budget/environment).
T23.43 additionally requires a "hosted_mcp" object naming the real hosted
MCP endpoint and the client credential to authenticate with -- documented
in docs/launch/hosted-completion/embedding-eval.md rather than added to the
shared qualification.example.json template, which is not this task's file
to edit (docs/launch/hosted-completion/** belongs to the integrator except
this task's own embedding-eval.md).

validate_live_manifest returns a list of problems; a non-empty list means
BLOCKED (exit 2), never a silent default. This is the concrete
implementation of evidence.md's "Workers implement validation, not
defaults that turn missing fields into unlimited access."
"""

from __future__ import annotations

import ipaddress
import json
import math
import re
from pathlib import Path
from urllib.parse import urlsplit

from .mcp_client import validate_endpoint_url


PHASE_SEED = "seed"
PHASE_LIVE = "live"

# The environment kinds this harness understands. An arbitrary non-empty string
# is not a classification: it was accepted before, and "disposable" with a
# loopback endpoint produced a live-provider PASS. `local-fixture` may only
# name endpoints on this machine; the others may name remote hosts, but a
# remote host still does not prove which provider is behind it.
SUPPORTED_ENVIRONMENT_KINDS = ("local-fixture", "disposable", "staging", "production")

ENDPOINT_CLASS_LOOPBACK = "loopback"
ENDPOINT_CLASS_PRIVATE = "private-network"
ENDPOINT_CLASS_REMOTE = "remote-unverified"


def endpoint_class(url: str) -> str:
    """Where an endpoint is, judged from its literal host only (no DNS is
    resolved, so this stays offline). A loopback or private literal address, or
    `localhost`, is local. Any other host name is `remote-unverified`: the
    harness does not resolve it, so a name an operator points at a local
    address is not detected. Neither class proves which provider serves it."""
    host = urlsplit(url).hostname or ""
    if host == "localhost" or host.endswith(".localhost"):
        return ENDPOINT_CLASS_LOOPBACK
    try:
        ip = ipaddress.ip_address(host)
    except ValueError:
        return ENDPOINT_CLASS_REMOTE
    if ip.is_loopback:
        return ENDPOINT_CLASS_LOOPBACK
    if ip.is_private or ip.is_link_local or ip.is_unspecified:
        return ENDPOINT_CLASS_PRIVATE
    return ENDPOINT_CLASS_REMOTE


def is_local_endpoint(url: str) -> bool:
    return endpoint_class(url) != ENDPOINT_CLASS_REMOTE


def load_manifest(path: str) -> dict:
    text = Path(path).read_text(encoding="utf-8")
    manifest = json.loads(text)
    if not isinstance(manifest, dict):
        raise ValueError(f"manifest at {path} is not a JSON object")
    return manifest


def _positive_finite_number(value, name: str, problems: list[str]) -> None:
    """Rejects None, bool (a bool is an int subclass in Python -- True/1
    must never be silently accepted as a valid numeric cap), NaN, +-Inf,
    non-numeric, and non-positive values.
    """
    if value is None:
        problems.append(f"{name} is null; a live run must have an explicit finite positive cap")
        return
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        problems.append(f"{name} must be a number, got {value!r}")
        return
    if not math.isfinite(value):
        problems.append(f"{name} must be finite, got {value!r}")
        return
    if value <= 0:
        problems.append(f"{name} must be positive, got {value!r}")


_CONFIRMATION = re.compile(r"^sha256:[0-9a-f]{64}$")
_ENV_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]{0,127}$")


def _validate_hosted_mcp_target(prefix: str, target: dict, problems: list[str], phase: str) -> None:
    if not target.get("endpoint_url"):
        problems.append(f"{prefix}.endpoint_url is missing; no real hosted MCP endpoint to target")
    ref = target.get("credential_secret_ref")
    if not ref:
        problems.append(f"{prefix}.credential_secret_ref is missing; no client credential reference")
    elif not (isinstance(ref, str) and _ENV_NAME.match(ref)):
        # Not echoed: a value pasted here by mistake would be a credential.
        problems.append(f"{prefix}.credential_secret_ref must be an environment variable NAME, never a credential value")
    if phase == PHASE_LIVE:
        # A live run scores against ids the seed run assigned. The receipt
        # that maps them is bound by hash, so a hand-typed confirmation
        # string or an edited receipt is rejected before any network call.
        if not target.get("seed_receipt_path"):
            problems.append(
                f"{prefix}.seed_receipt_path is missing; run --seed first and point this at its receipt "
                "(the live run cannot attribute recall results to corpus facts without it)"
            )
        if not _CONFIRMATION.match(str(target.get("corpus_seeded_confirmation") or "")):
            problems.append(
                f"{prefix}.corpus_seeded_confirmation must be 'sha256:<64 hex>' of the seed receipt file "
                "(printed by --seed); a free-form receipt id or timestamp is not accepted"
            )
    allowed_origins = target.get("allowed_origins")
    if not allowed_origins or not isinstance(allowed_origins, list):
        problems.append(f"{prefix}.allowed_origins is missing or empty; the endpoint origin must be explicitly allowlisted")
    elif target.get("endpoint_url"):
        try:
            validate_endpoint_url(target["endpoint_url"], allowed_origins)
        except ValueError as e:
            problems.append(f"{prefix}.endpoint_url: {e}")


def _validate_provider_pin(provider: dict, problems: list[str]) -> None:
    """T23.43.md step 2: record model pin, provider and dimensions. Evidence
    that omits them cannot be tied to an embedding space, so a live or seed
    run without them is blocked rather than recorded as null."""
    for field in ("model", "version_pin", "serving_provider", "privacy_review_ref"):
        if not provider.get(field):
            problems.append(f"provider.{field} is missing; the qualified embedding pin must be recorded, not implied")
    dims = provider.get("dimensions")
    if isinstance(dims, bool) or not isinstance(dims, int) or dims <= 0:
        problems.append(f"provider.dimensions must be a positive integer, got {dims!r}")


def _validate_environment(manifest: dict, problems: list[str]) -> None:
    environment = manifest.get("environment") or {}
    kind = environment.get("kind")
    if not kind:
        problems.append("environment.kind is missing")
    elif kind not in SUPPORTED_ENVIRONMENT_KINDS:
        problems.append(f"environment.kind must be one of {list(SUPPORTED_ENVIRONMENT_KINDS)}, got {str(kind)[:40]!r}")
    if kind == "local-fixture":
        for prefix in ("hosted_mcp", "empty_case_hosted_mcp"):
            url = (manifest.get(prefix) or {}).get("endpoint_url")
            if url and not is_local_endpoint(url):
                problems.append(f"environment.kind is 'local-fixture' but {prefix}.endpoint_url is not a local address")
    if environment.get("production_target_allowed") and kind != "production":
        problems.append("environment.production_target_allowed is true but environment.kind is not 'production'")
    if kind == "production" and not environment.get("production_target_allowed"):
        problems.append("environment.kind is 'production' but production_target_allowed is not true; production targets are refused")
    hosts = environment.get("allowed_hosts")
    if not hosts or not isinstance(hosts, list):
        problems.append("environment.allowed_hosts is missing or empty; the explicit host allowlist is required")
        return
    for prefix in ("hosted_mcp", "empty_case_hosted_mcp"):
        url = (manifest.get(prefix) or {}).get("endpoint_url")
        if url and urlsplit(url).hostname not in hosts:
            problems.append(f"{prefix}.endpoint_url host {urlsplit(url).hostname!r} is not in environment.allowed_hosts")


def validate_live_manifest(manifest: dict, phase: str = PHASE_LIVE) -> list[str]:
    """Returns a list of human-readable problems. Empty means the manifest
    carries enough concrete authority to attempt a live run (still subject
    to the live call itself failing on bad credentials/network -- this only
    validates presence and shape of required fields, never their real-world
    correctness).
    """
    problems: list[str] = []

    budget = manifest.get("budget") or {}
    for field in ("max_calls", "max_input_tokens", "max_elapsed_seconds", "approved_max_usd"):
        _positive_finite_number(budget.get(field), f"budget.{field}", problems)
    # This harness has no verified real per-call $ price for the hosted
    # recall/embedding pipeline (provider pricing is T23.42's concern, not
    # this task's) -- rather than invent one, the operator who authorizes
    # this manifest supplies a conservative ceiling; approved_max_usd is
    # then enforced as calls_made * max_cost_per_call_usd, mechanically,
    # never assumed unbounded just because a $/call figure is unknown.
    _positive_finite_number(budget.get("max_cost_per_call_usd"), "budget.max_cost_per_call_usd", problems)
    if not budget.get("authorization_ref"):
        problems.append("budget.authorization_ref is missing; no recorded authorization for this spend")
    ledger_path = budget.get("ledger_path")
    if not (isinstance(ledger_path, str) and ledger_path.strip()):
        problems.append(
            "budget.ledger_path is missing; every request is reserved in a durable cumulative ledger bound to this "
            "authorization, so the caps hold across retries and separate runs"
        )
    if budget.get("automatic_reset"):
        problems.append("budget.automatic_reset must be false; no automatic top-up permitted")
    if budget.get("auto_top_up"):
        problems.append("budget.auto_top_up must be false; no automatic top-up permitted")

    provider = manifest.get("provider") or {}
    if not provider.get("secret_ref"):
        problems.append("provider.secret_ref is missing; no credential reference to resolve")
    if provider.get("allow_fallback"):
        problems.append("provider.allow_fallback must be false; no silent alternative-model routing")

    _validate_provider_pin(provider, problems)

    if phase == PHASE_SEED:
        seeding = manifest.get("seeding") or {}
        if seeding.get("authorized") is not True:
            problems.append(
                "seeding.authorized must be exactly true; writing the corpus into a hosted account "
                "needs its own explicit authorization, separate from the recall budget"
            )
        if not seeding.get("authorization_ref"):
            problems.append("seeding.authorization_ref is missing; no recorded authorization for the seed writes")

    hosted_mcp = manifest.get("hosted_mcp") or {}
    _validate_hosted_mcp_target("hosted_mcp", hosted_mcp, problems, phase)

    # T23.43.md's own acceptance criterion requires cross-account leakage
    # to be checked -- impossible to test honestly with a single shared
    # credential for both the 95 positive cases and the 5 empty cases
    # (a near-miss embedding hit against an unrelated positive fact would
    # be indistinguishable from genuine leakage). A second, separately
    # seeded isolated target is required for the empty-case phase; its
    # absence fails this manifest closed rather than silently reusing
    # hosted_mcp for both phases.
    empty_case_mcp = manifest.get("empty_case_hosted_mcp")
    if not empty_case_mcp:
        problems.append(
            "empty_case_hosted_mcp is missing; the 5 expected-empty cases require a separately "
            "seeded, isolated account/credential distinct from hosted_mcp so a leakage finding is "
            "unambiguous -- reusing hosted_mcp's single account cannot test cross-account leakage=0"
        )
    else:
        _validate_hosted_mcp_target("empty_case_hosted_mcp", empty_case_mcp, problems, phase)
        if empty_case_mcp.get("credential_secret_ref") and empty_case_mcp.get("credential_secret_ref") == hosted_mcp.get(
            "credential_secret_ref"
        ):
            problems.append(
                "empty_case_hosted_mcp.credential_secret_ref equals hosted_mcp.credential_secret_ref; "
                "one credential is one account, so cross-account leakage could not be tested"
            )

    _validate_environment(manifest, problems)

    corpus_sha256 = manifest.get("corpus_sha256")
    if not corpus_sha256:
        problems.append("corpus_sha256 is missing; the frozen corpus this run scores against must be pinned")

    return problems
