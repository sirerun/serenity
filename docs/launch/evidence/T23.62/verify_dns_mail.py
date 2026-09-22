#!/usr/bin/env python3
"""Read-only DNS/TLS gate verifier; never changes DNS or sends email."""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path


def resolve(name: str, record_type: str) -> list[str]:
    try:
        p = subprocess.run(
            ["dig", "+short", "+time=3", "+tries=1", name, record_type],
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return []
    return [line.strip().rstrip(".") for line in p.stdout.splitlines() if line.strip()]


def verify_https(url: str) -> dict:
    request = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "serenity-launch-verifier/1"})
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            return {"status": response.status, "url": response.geturl(), "ok": 200 <= response.status < 400}
    except urllib.error.HTTPError as exc:
        return {"status": exc.code, "url": exc.geturl(), "ok": False}
    except Exception as exc:  # network failure is evidence, never a pass
        return {"status": None, "url": url, "ok": False, "error": type(exc).__name__}


def verify(expected: dict, resolver=resolve, https=verify_https) -> dict:
    app = expected["app_origin"].rstrip("/")
    host = app.split("://", 1)[-1].split("/", 1)[0]
    expected_ip = expected.get("expected_a")
    a_records = resolver(host, "A")
    cname_records = resolver(host, "CNAME")
    https_result = https(app + "/healthz")
    dns_ok = bool(expected_ip and expected_ip in a_records) or bool(expected.get("expected_cname") and expected["expected_cname"] in cname_records)
    return {
        "status": "PASS" if dns_ok and https_result.get("ok") else "BLOCKED",
        "app_host": host,
        "a_records": a_records,
        "cname_records": cname_records,
        "expected_a": expected_ip,
        "expected_cname": expected.get("expected_cname"),
        "https": https_result,
        "mail": {"sender_domain": expected["sender_domain"], "controlled_recipient_ref": expected["controlled_recipient_ref"], "delivery_status": "NOT_RUN"},
        "network_mutations": 0,
        "limitations": [
            "DNS/TLS checks are read-only and do not prove authority to change the zone.",
            "Mail delivery is NOT_RUN until a scoped Resend key, verified sender and private qualification origin exist.",
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--expected", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    expected = json.loads(args.expected.read_text(encoding="utf-8"))
    result = verify(expected)
    result["recorded_at"] = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(f"{result['status']}: dns/tls read-only check; network_mutations=0")
    return 0 if result["status"] == "PASS" else 2


if __name__ == "__main__":
    raise SystemExit(main())
