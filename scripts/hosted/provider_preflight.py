#!/usr/bin/env python3
"""Validate hosted qualification metadata without contacting providers."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

MODEL = "perplexity/pplx-embed-v1-0.6b"
BASE_URL = "https://openrouter.ai/api/v1"
SECRET_RE = re.compile(r"^[A-Z][A-Z0-9_]{1,63}$")
PLACEHOLDERS = {"", "UNCONFIGURED", "REPLACE", "CHANGEME", "TODO"}


def read_env_file(path: Path | None) -> dict[str, str]:
    if path is None:
        return {}
    values: dict[str, str] = {}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeError):
        return values
    for line in lines:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        if SECRET_RE.fullmatch(key):
            values[key] = value.strip().strip("\"").strip("\'")
    return values


def status_for_secret(ref: str, env_file_values: dict[str, str] | None = None) -> str:
    if not SECRET_RE.fullmatch(ref):
        return "invalid_ref"
    value = os.environ.get(ref)
    if value is None and env_file_values is not None:
        value = env_file_values.get(ref)
    if value is not None:
        return "placeholder" if value.strip().upper() in PLACEHOLDERS else "present"
    secrets_dir = os.environ.get("SERENITY_SECRETS_DIR")
    if secrets_dir:
        path = Path(secrets_dir) / ref
        try:
            value = path.read_text(encoding="utf-8").strip()
        except (FileNotFoundError, PermissionError, IsADirectoryError):
            return "missing"
        return "placeholder" if value.upper() in PLACEHOLDERS else "present"
    return "missing"


def load_manifest(path: Path) -> tuple[dict | None, list[str]]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return None, [f"manifest_unreadable:{type(exc).__name__}"]
    errors: list[str] = []
    if not isinstance(data, dict):
        return None, ["manifest_not_object"]
    provider = data.get("provider")
    if not isinstance(provider, dict):
        errors.append("provider_missing")
        provider = {}
    if provider.get("base_url") != BASE_URL:
        errors.append("provider_base_url_mismatch")
    if provider.get("model") != MODEL:
        errors.append("provider_model_mismatch")
    if provider.get("allow_fallback") is not False:
        errors.append("provider_fallback_must_be_disabled")
    ref = provider.get("secret_ref")
    if not isinstance(ref, str) or not SECRET_RE.fullmatch(ref):
        errors.append("provider_secret_ref_invalid")
    budget = data.get("budget")
    if not isinstance(budget, dict):
        errors.append("budget_missing")
        budget = {}
    approved = budget.get("approved_max_usd")
    if approved is None or not isinstance(approved, (int, float)) or approved <= 0 or approved > 1:
        errors.append("qualification_cap_must_be_positive_and_at_most_1_usd")
    for key in ("max_calls", "max_input_tokens", "max_elapsed_seconds"):
        if not isinstance(budget.get(key), int) or budget[key] <= 0:
            errors.append(f"budget_{key}_must_be_positive")
    if budget.get("automatic_reset") is not False:
        errors.append("automatic_reset_must_be_disabled")
    if budget.get("auto_top_up") is not False:
        errors.append("auto_top_up_must_be_disabled")
    return data, errors


def preflight(manifest_path: Path, env_file: Path | None = None) -> dict:
    data, errors = load_manifest(manifest_path)
    env_file_values = read_env_file(env_file)
    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    result = {
        "schema_version": 1,
        "tool": "serenity-hosted-provider-preflight",
        "recorded_at": now,
        "mode": "metadata-only",
        "manifest_sha256": hashlib.sha256(manifest_path.read_bytes()).hexdigest() if manifest_path.exists() else None,
        "status": "BLOCKED",
        "network_calls": 0,
        "secrets": {},
        "errors": errors,
        "limitations": [
            "Metadata-only mode never contacts OpenRouter, Resend, Stripe, AWS, or DNS.",
            "Presence means a non-placeholder value is available to this process; it does not validate permissions or provider terms.",
        ],
    }
    if data is not None:
        provider = data.get("provider", {})
        mail = data.get("mail", {})
        stripe = data.get("stripe", {})
        refs = [provider.get("secret_ref"), mail.get("secret_ref"), stripe.get("secret_ref"), stripe.get("webhook_secret_ref")]
        for ref in refs:
            if isinstance(ref, str) and SECRET_RE.fullmatch(ref):
                result["secrets"][ref] = status_for_secret(ref, env_file_values)
        if any(v in {"missing", "placeholder", "invalid_ref"} for v in result["secrets"].values()):
            result["errors"].append("one_or_more_required_secret_refs_unavailable")
        if not result["errors"]:
            result["status"] = "READY_FOR_REVIEW"
        else:
            result["status"] = "BLOCKED"
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--metadata-only", action="store_true")
    parser.add_argument("--env-file", type=Path, help="optional private dotenv file; values are never printed")
    args = parser.parse_args()
    if not args.metadata_only:
        result = {"status": "BLOCKED", "network_calls": 0, "errors": ["metadata-only flag is required; provider validation is a separately authorized gate"]}
        args.output.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print("BLOCKED: rerun with --metadata-only; no network calls made")
        return 2
    result = preflight(args.manifest, args.env_file)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(f"{result['status']}: {len(result['errors'])} metadata errors; network_calls=0")
    return 0 if result["status"] == "READY_FOR_REVIEW" else 2


if __name__ == "__main__":
    raise SystemExit(main())
