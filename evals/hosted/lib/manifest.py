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

import json
import math
from pathlib import Path

from .mcp_client import validate_endpoint_url


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


def _validate_hosted_mcp_target(prefix: str, target: dict, problems: list[str]) -> None:
    if not target.get("endpoint_url"):
        problems.append(f"{prefix}.endpoint_url is missing; no real hosted MCP endpoint to target")
    if not target.get("credential_secret_ref"):
        problems.append(f"{prefix}.credential_secret_ref is missing; no client credential reference")
    if not target.get("corpus_seeded_confirmation"):
        problems.append(
            f"{prefix}.corpus_seeded_confirmation is missing; T23.43 does not own the hosted "
            "write path and never seeds a live account itself -- the coordinator/operator must "
            "seed the frozen corpus into the target account/brain out-of-band and record an "
            "explicit confirmation (e.g. a timestamp or receipt id) here before a live run"
        )
    allowed_origins = target.get("allowed_origins")
    if not allowed_origins or not isinstance(allowed_origins, list):
        problems.append(f"{prefix}.allowed_origins is missing or empty; the endpoint origin must be explicitly allowlisted")
    elif target.get("endpoint_url"):
        try:
            validate_endpoint_url(target["endpoint_url"], allowed_origins)
        except ValueError as e:
            problems.append(f"{prefix}.endpoint_url: {e}")


def validate_live_manifest(manifest: dict) -> list[str]:
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
    if budget.get("automatic_reset"):
        problems.append("budget.automatic_reset must be false; no automatic top-up permitted")
    if budget.get("auto_top_up"):
        problems.append("budget.auto_top_up must be false; no automatic top-up permitted")

    provider = manifest.get("provider") or {}
    if not provider.get("secret_ref"):
        problems.append("provider.secret_ref is missing; no credential reference to resolve")
    if provider.get("allow_fallback"):
        problems.append("provider.allow_fallback must be false; no silent alternative-model routing")

    hosted_mcp = manifest.get("hosted_mcp") or {}
    _validate_hosted_mcp_target("hosted_mcp", hosted_mcp, problems)

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
        _validate_hosted_mcp_target("empty_case_hosted_mcp", empty_case_mcp, problems)

    environment = manifest.get("environment") or {}
    if environment.get("production_target_allowed") and environment.get("kind") != "production":
        problems.append("environment.production_target_allowed is true but environment.kind is not 'production'")

    corpus_sha256 = manifest.get("corpus_sha256")
    if not corpus_sha256:
        problems.append("corpus_sha256 is missing; the frozen corpus this run scores against must be pinned")

    return problems
