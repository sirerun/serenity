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
from pathlib import Path


def load_manifest(path: str) -> dict:
    text = Path(path).read_text(encoding="utf-8")
    manifest = json.loads(text)
    if not isinstance(manifest, dict):
        raise ValueError(f"manifest at {path} is not a JSON object")
    return manifest


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
        value = budget.get(field)
        if value is None:
            problems.append(f"budget.{field} is null; a live run must have an explicit finite cap")
        elif not isinstance(value, (int, float)) or value <= 0:
            problems.append(f"budget.{field} must be a positive number, got {value!r}")
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
    if not hosted_mcp.get("endpoint_url"):
        problems.append("hosted_mcp.endpoint_url is missing; no real hosted MCP endpoint to target")
    if not hosted_mcp.get("credential_secret_ref"):
        problems.append("hosted_mcp.credential_secret_ref is missing; no client credential reference")
    if not hosted_mcp.get("corpus_seeded_confirmation"):
        problems.append(
            "hosted_mcp.corpus_seeded_confirmation is missing; T23.43 does not own the hosted "
            "write path and never seeds a live account itself -- the coordinator/operator must "
            "seed the frozen corpus into the target account/brain out-of-band and record an "
            "explicit confirmation (e.g. a timestamp or receipt id) here before a live run"
        )

    environment = manifest.get("environment") or {}
    if environment.get("production_target_allowed") and environment.get("kind") != "production":
        problems.append("environment.production_target_allowed is true but environment.kind is not 'production'")

    corpus_sha256 = manifest.get("corpus_sha256")
    if not corpus_sha256:
        problems.append("corpus_sha256 is missing; the frozen corpus this run scores against must be pinned")

    return problems
