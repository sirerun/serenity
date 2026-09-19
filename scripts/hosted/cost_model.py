#!/usr/bin/env python3
"""Hosted Serenity monthly cost model (T23.60).

Prices four fixed scenarios (0 accounts idle, an assumed light-usage 10- and
100-account mix, and the ratified 1 Scale + 3 Builder + 10 Free full-limit
mix) against deploy/hosted/stack.json's actual resources (t4g.small, 8 GB
root + 30 GB data gp3 EBS, one Elastic IP, one KMS key, four Secrets Manager
secrets, two CloudWatch alarms, hourly S3 backups with 30-day
current/noncurrent versioned retention).

Every rate in RATE_TABLE is tagged "assumption_recollection": this worker's
training-data recollection of long-stable AWS list prices, NOT fetched live
in this session (WebSearch/WebFetch require an interactive permission grant
this headless run does not have -- see docs/launch/evidence/T23.60/result.json
limitations). Treat every dollar figure below as a reviewable estimate, not a
dated citation, until a reviewer re-verifies against
https://aws.amazon.com/ec2/pricing/on-demand/ (and the other AWS pricing
pages named per rate) and updates the "kind" field. Unknown-priced categories
(embeddings, CloudWatch Logs/custom metrics not yet wired) are reported as
explicit unknowns, never silently zeroed.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path

RATE_TABLE = {
    "ec2_t4g_small_usd_per_hour": {"value": 0.0168, "kind": "assumption_recollection", "note": "AWS EC2 On-Demand Linux us-west-2 t4g.small (2 vCPU burstable, 2 GiB)."},
    "ec2_t4g_burst_credit_usd_per_vcpu_hour": {"value": 0.05, "kind": "assumption_recollection", "note": "T4g Unlimited-mode surplus CPU credit surcharge, us-west-2."},
    "ebs_gp3_usd_per_gb_month": {"value": 0.08, "kind": "assumption_recollection", "note": "EBS gp3 storage, us-west-2 (baseline 3,000 IOPS / 125 MB/s included)."},
    "s3_standard_usd_per_gb_month": {"value": 0.023, "kind": "assumption_recollection", "note": "S3 Standard storage, us-west-2, first 50 TB tier."},
    "s3_put_usd_per_1000_requests": {"value": 0.005, "kind": "assumption_recollection", "note": "S3 PUT/COPY/POST/LIST requests, us-west-2."},
    "s3_get_usd_per_1000_requests": {"value": 0.0004, "kind": "assumption_recollection", "note": "S3 GET/SELECT requests, us-west-2."},
    "kms_key_usd_per_month": {"value": 1.00, "kind": "assumption_recollection", "note": "One customer-managed KMS key."},
    "kms_usd_per_10000_requests": {"value": 0.03, "kind": "assumption_recollection", "note": "KMS API requests; SSE-KMS S3 puts/gets each incur roughly one KMS request."},
    "secrets_manager_usd_per_secret_month": {"value": 0.40, "kind": "assumption_recollection", "note": "Per-secret monthly charge."},
    "secrets_manager_usd_per_10000_calls": {"value": 0.05, "kind": "assumption_recollection", "note": "Secrets Manager API calls."},
    "cloudwatch_alarm_usd_per_month": {"value": 0.10, "kind": "assumption_recollection", "note": "Standard-resolution alarm, first 10 alarms tier."},
    "eip_usd_per_hour": {"value": 0.005, "kind": "assumption_recollection", "note": "AWS charges hourly for every public IPv4 address, attached or not, since the Feb 2024 pricing change; matches ADR 014's own ~USD 4/month estimate."},
    "data_transfer_out_usd_per_gb": {"value": 0.09, "kind": "assumption_recollection", "note": "Internet egress, first tier."},
    "hours_per_month": {"value": 730, "kind": "constant", "note": "365*24/12."},
}
UNKNOWN_RATES = {
    "embeddings_usd_per_1k_tokens": "pending task42 provider/serving-provider pin (perplexity/pplx-embed-v1-0.6b terms are unqualified per docs/launch/hosted-plan.md)",
    "cloudwatch_logs_ingest_usd_per_gb": "no CloudWatch Logs shipping is wired yet (ops go through SSM Run Command per ADR 014); revisit with T23.53 telemetry",
    "cloudwatch_custom_metric_usd_per_month": "T23.53 telemetry metric cardinality not yet implemented",
    "resend_usd_beyond_free_tier": "email volume assumed within Resend's free tier at these account counts; pin a rate if MAIL gate volume ever exceeds it",
}

# deploy/hosted/stack.json.
ROOT_EBS_GB = 8
DATA_EBS_GB = 30
DATA_VOLUME_OPERATOR_HEADROOM_FRACTION = 0.20  # interfaces.md "Storage admission" seam requires reserved operator headroom.
KMS_SECRETS_COUNT = 4
CLOUDWATCH_ALARM_COUNT = 2
BACKUPS_PER_MONTH = 24 * 30  # hourly, per ADR 014.
S3_NONCURRENT_RETENTION_DAYS = 30
CONTROL_DB_ASSUMED_GB = 0.02

# internal/hosted/plans/plans.go V1; kept in sync by hand, no cross-language import.
PLAN_ALLOWANCES = {
    "free": {"recalls": 10000, "writes": 500, "input_tokens": 100000, "storage_bytes": 100_000_000},
    "builder": {"recalls": 100000, "writes": 5000, "input_tokens": 1_000_000, "storage_bytes": 1_000_000_000},
    "scale": {"recalls": 300000, "writes": 20000, "input_tokens": 4_000_000, "storage_bytes": 5_000_000_000},
}

SCENARIOS = {
    "idle_0_accounts": {"mix": {}, "usage_fraction": 0.0, "assumption": "no signups yet; fixed infrastructure cost floor"},
    "10_accounts_light": {"mix": {"free": 8, "builder": 2, "scale": 0}, "usage_fraction": 0.10, "assumption": "early-adoption mix at 10% of each plan's allowance; product/growth input not yet recorded"},
    "100_accounts_light": {"mix": {"free": 90, "builder": 8, "scale": 2}, "usage_fraction": 0.10, "assumption": "at ADR 014's 100 free-account cap, light usage; product/growth input not yet recorded"},
    "full_limit_mix": {"mix": {"free": 10, "builder": 3, "scale": 1}, "usage_fraction": 1.00, "assumption": "the ratified 1 Scale + 3 Builder + 10 Free mix at full advertised allowance (90,000 total facts) per docs/launch/hosted-completion/evidence.md"},
}


def rate(name: str) -> float:
    return RATE_TABLE[name]["value"]


def account_totals(mix: dict, usage_fraction: float) -> dict:
    totals = {"accounts": 0, "recalls": 0, "writes": 0, "input_tokens": 0, "storage_bytes": 0.0}
    for plan_id, count in mix.items():
        allowance = PLAN_ALLOWANCES[plan_id]
        totals["accounts"] += count
        totals["recalls"] += count * allowance["recalls"] * usage_fraction
        totals["writes"] += count * allowance["writes"] * usage_fraction
        totals["input_tokens"] += count * allowance["input_tokens"] * usage_fraction
        totals["storage_bytes"] += count * allowance["storage_bytes"] * usage_fraction
    return totals


def price_scenario(name: str, spec: dict, measurements: dict | None) -> dict:
    totals = account_totals(spec["mix"], spec["usage_fraction"])
    measurements = measurements or {}
    known = {}

    known["ec2_baseline"] = round(rate("ec2_t4g_small_usd_per_hour") * rate("hours_per_month"), 4)
    burst_vcpu_hours = measurements.get("ec2_burst_vcpu_hours")
    burst_kind = "measured" if burst_vcpu_hours is not None else "assumption_recollection"
    if burst_vcpu_hours is None:
        # t4g.small baseline is 20% of 2 vCPU continuous; assume the full-limit mix's steady 4 req/s
        # workload exceeds baseline for the burst phase only (5 of 45 workload minutes), light scenarios do not.
        burst_vcpu_hours = 2 * rate("hours_per_month") * 0.02 if spec["usage_fraction"] >= 1.0 else 0.0
    known["ec2_burst_credits"] = {"value": round(burst_vcpu_hours * rate("ec2_t4g_burst_credit_usd_per_vcpu_hour"), 4), "kind": burst_kind}

    known["ebs_fixed"] = round((ROOT_EBS_GB + DATA_EBS_GB) * rate("ebs_gp3_usd_per_gb_month"), 4)
    usable_data_bytes = DATA_EBS_GB * 1_000_000_000 * (1 - DATA_VOLUME_OPERATOR_HEADROOM_FRACTION)
    storage_risk = totals["storage_bytes"] > usable_data_bytes

    known["eip"] = round(rate("eip_usd_per_hour") * rate("hours_per_month"), 4)
    known["kms_key"] = round(rate("kms_key_usd_per_month"), 4)
    known["secrets_manager"] = round(KMS_SECRETS_COUNT * rate("secrets_manager_usd_per_secret_month"), 4)
    known["cloudwatch_alarms"] = round(CLOUDWATCH_ALARM_COUNT * rate("cloudwatch_alarm_usd_per_month"), 4)

    snapshot_size_bytes = measurements.get("snapshot_size_bytes")
    snapshot_kind = "measured" if snapshot_size_bytes is not None else "assumption_recollection"
    if snapshot_size_bytes is None:
        snapshot_size_bytes = totals["storage_bytes"] + CONTROL_DB_ASSUMED_GB * 1_000_000_000
    snapshot_size_gb = snapshot_size_bytes / 1_000_000_000
    stored_version_count = S3_NONCURRENT_RETENTION_DAYS * 24 + 1  # hourly full snapshot; see module docstring gap #4.
    s3_storage_gb_month = snapshot_size_gb * stored_version_count
    known["s3_storage_hourly_full_snapshot"] = {
        "value": round(s3_storage_gb_month * rate("s3_standard_usd_per_gb_month"), 4),
        "kind": snapshot_kind,
        "snapshot_size_gb": round(snapshot_size_gb, 4),
        "stored_version_count": stored_version_count,
        "note": "S3 NoncurrentVersionExpiration(30d) keeps ~30 days of hourly full snapshots live simultaneously (720 noncurrent + 1 current); this is not a 1x-of-working-set cost. See docs/launch/hosted-economics.md risk section.",
    }
    objects_per_backup = 1 + max(totals["accounts"], 0)  # one control-db object + one bundle per account's primary brain.
    s3_requests = measurements.get("s3_requests_per_month")
    s3_requests_kind = "measured" if s3_requests is not None else "assumption_recollection"
    if s3_requests is None:
        s3_requests = BACKUPS_PER_MONTH * objects_per_backup
    known["s3_requests"] = {"value": round(s3_requests / 1000 * rate("s3_put_usd_per_1000_requests"), 4), "kind": s3_requests_kind}
    known["kms_requests"] = {"value": round(s3_requests / 10000 * rate("kms_usd_per_10000_requests"), 4), "kind": s3_requests_kind, "note": "SSE-KMS attaches ~1 KMS request per S3 backup PUT."}

    avg_recall_response_bytes, avg_write_request_bytes = 4096, 1024
    transfer_gb = (totals["recalls"] * avg_recall_response_bytes + totals["writes"] * avg_write_request_bytes) / 1_000_000_000
    known["data_transfer_out"] = {"value": round(transfer_gb * rate("data_transfer_out_usd_per_gb"), 4), "kind": "assumption_recollection", "note": f"{avg_recall_response_bytes}B/recall response, {avg_write_request_bytes}B/write request assumed."}

    known_subtotal = sum(v["value"] if isinstance(v, dict) else v for v in known.values())

    unknown = {
        "embeddings": {"tokens_estimate": round(totals["input_tokens"]), "usd_per_1k_tokens": None, "monthly_usd": None, "reason": UNKNOWN_RATES["embeddings_usd_per_1k_tokens"]},
        "cloudwatch_logs": {"reason": UNKNOWN_RATES["cloudwatch_logs_ingest_usd_per_gb"]},
        "cloudwatch_custom_metrics": {"reason": UNKNOWN_RATES["cloudwatch_custom_metric_usd_per_month"]},
    }

    return {
        "scenario": name,
        "assumption": spec["assumption"],
        "account_totals": {k: (round(v) if k != "storage_bytes" else round(v)) for k, v in totals.items()},
        "storage_risk": {"exceeds_usable_data_volume": storage_risk, "usable_data_volume_bytes": round(usable_data_bytes)},
        "known_categories_usd": known,
        "known_subtotal_usd": round(known_subtotal, 2),
        "unknown_categories": unknown,
        "within_60usd_ceiling_known_subtotal": known_subtotal <= 60.0,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--measurements", type=Path, default=None, help="optional load.py --live output; overrides assumption-tagged usage figures with measured ones")
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    if not args.manifest.exists():
        print(json.dumps({"status": "BLOCKED", "reason": f"manifest not found: {args.manifest}"}, indent=2))
        return 2
    manifest = json.loads(args.manifest.read_text())

    measurements = None
    if args.measurements is not None:
        if not args.measurements.exists():
            print(json.dumps({"status": "BLOCKED", "reason": f"measurements file not found: {args.measurements}"}, indent=2))
            return 2
        measurements = json.loads(args.measurements.read_text())

    scenarios = {name: price_scenario(name, spec, measurements) for name, spec in SCENARIOS.items()}
    result = {
        "task_id": "T23.60",
        "profile": manifest.get("profile", "paid"),
        "rate_table": RATE_TABLE,
        "unknown_rates": UNKNOWN_RATES,
        "measurements_applied": measurements is not None,
        "scenarios": scenarios,
        "historical_ceiling_usd": 60.0,
        "ceiling_note": "The historical USD 60/month figure is a ceiling, not a budget target or new spend authorization (docs/launch/hosted-plan.md).",
        "citation_limitation": "Rates are this worker's training-data recollection, not a live-fetched dated citation: WebSearch/WebFetch tool permission was not granted in this headless session. A reviewer must re-verify RATE_TABLE against current AWS/Resend pricing pages before this cost model is treated as authoritative.",
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")

    worst = max(scenarios.values(), key=lambda s: s["known_subtotal_usd"])
    print(json.dumps({
        "status": "PASS",
        "output": str(args.output),
        "idle_usd": scenarios["idle_0_accounts"]["known_subtotal_usd"],
        "full_limit_mix_known_subtotal_usd": scenarios["full_limit_mix"]["known_subtotal_usd"],
        "worst_known_scenario": worst["scenario"],
        "worst_known_subtotal_usd": worst["known_subtotal_usd"],
        "any_scenario_exceeds_60usd_ceiling_on_known_costs_alone": any(not s["within_60usd_ceiling_known_subtotal"] for s in scenarios.values()),
    }, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
