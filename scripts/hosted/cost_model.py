#!/usr/bin/env python3
"""Hosted Serenity monthly cost model (T23.60).

Prices four fixed scenarios (0 accounts idle, an assumed light-usage 10- and
100-account mix, and the ratified 1 Scale + 3 Builder + 10 Free full-limit
mix) against deploy/hosted/stack.json's actual resources (t4g.small, 8 GB
root + 30 GB data gp3 EBS, one Elastic IP, one KMS key, four Secrets Manager
secrets, two CloudWatch alarms, hourly S3 backups written by
deploy/hosted/backup.sh to a unique timestamped key prefix per run, with
30-day Expiration + 30-day NoncurrentVersionExpiration).

Every rate in RATE_TABLE carries a "kind": "published" rates were fetched
live via WebSearch/WebFetch this session and are cited with a source URL and
the date fetched (2026-09-19); nothing here is a blind recollection anymore.
A "published" tag still means "this worker's search-result summary of a
public page," not an AWS invoice -- a reviewer should spot-check the cited
URL, especially for the one rate (EC2 hourly) this session could not
regionally reconfirm for us-west-2 (see its own note). Unpriced categories
(embeddings $/token, CloudWatch Logs/custom-metrics volume) are explicit
unknowns, never zeroed.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path

RATE_TABLE = {
    "ec2_t4g_small_usd_per_hour": {
        "value": 0.0168, "kind": "published", "region_caveat": "us-east-1 confirmed",
        "source": "https://www.economize.cloud/resources/aws/pricing/ec2/t4g.small/ (fetched 2026-09-19), which states the figure applies to us-east-1",
        "note": "This session could not extract a us-west-2-specific on-demand rate from the AWS or third-party pricing pages fetched (JS-rendered tables). T-family/general-purpose On-Demand pricing has historically matched between us-east-1 and us-west-2 (unlike S3, which this session confirmed differs), but that parity is not independently reverified here. Treat as a probable, not confirmed, us-west-2 figure.",
    },
    "ec2_t4g_burst_credit_usd_per_vcpu_hour": {
        "value": 0.04, "kind": "published",
        "source": "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-performance-instances-unlimited-mode-concepts.html (via WebSearch, fetched 2026-09-19)",
        "note": "T4g Unlimited-mode surplus CPU credit price, Linux/RHEL/SLES, uniform across instance size, purchase option and region per AWS's own docs. Corrects this task's earlier PARTIAL receipt, which used an unverified $0.05 recollection.",
    },
    "ebs_gp3_usd_per_gb_month": {
        "value": 0.08, "kind": "published", "source": "AWS EBS pricing summary via WebSearch (fetched 2026-09-19), stated for US West (Oregon)",
        "note": "Baseline 3,000 IOPS / 125 MB/s included free; this deployment's 8 GB root + 30 GB data volumes stay within baseline (unused overage rates recorded below for completeness).",
    },
    "ebs_gp3_extra_iops_usd_per_iops_month": {"value": 0.005, "kind": "published", "source": "same as ebs_gp3_usd_per_gb_month", "note": "Beyond the 3,000 free baseline IOPS. Not applied: this deployment has no evidence of exceeding baseline."},
    "ebs_gp3_extra_throughput_usd_per_mbps_month": {"value": 0.04, "kind": "published", "source": "same as ebs_gp3_usd_per_gb_month", "note": "Beyond the 125 MB/s free baseline throughput. Not applied, same reason."},
    "s3_standard_usd_per_gb_month": {
        "value": 0.023, "kind": "published", "region_caveat": "us-west-2 confirmed",
        "source": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonS3/current/us-west-2/index.json (regional catalog; see s3-rate.json)",
        "note": "Confirmed directly against the AWS regional offer catalog; see s3-rate.json. The earlier claim of $0.0265 for Oregon was incorrect.",
    },
    "s3_put_usd_per_1000_requests": {"value": 0.005, "kind": "published", "source": "https://aws.amazon.com/s3/pricing/ (fetched 2026-09-19)"},
    "s3_get_usd_per_1000_requests": {"value": 0.0004, "kind": "published", "source": "https://aws.amazon.com/s3/pricing/ (fetched 2026-09-19)"},
    "kms_key_usd_per_month": {"value": 1.00, "kind": "published", "source": "AWS KMS pricing summary via WebSearch (fetched 2026-09-19); AWS states this is region-uniform"},
    "kms_usd_per_10000_requests": {"value": 0.03, "kind": "published", "source": "same as kms_key_usd_per_month", "note": "First 20,000 requests/month free across all regions -- modeled explicitly, not omitted."},
    "kms_free_requests_per_month": {"value": 20000, "kind": "published", "source": "same as kms_key_usd_per_month"},
    "secrets_manager_usd_per_secret_month": {"value": 0.40, "kind": "published", "source": "AWS Secrets Manager pricing summary via WebSearch (fetched 2026-09-19); AWS states this is uniform across regions"},
    "secrets_manager_usd_per_10000_calls": {"value": 0.05, "kind": "published", "source": "same as secrets_manager_usd_per_secret_month", "note": "No published free tier for API calls, unlike KMS."},
    "cloudwatch_alarm_usd_per_month": {"value": 0.10, "kind": "published", "source": "AWS CloudWatch pricing summary via WebSearch (fetched 2026-09-19), US East (N. Virginia); AWS states standard-alarm pricing is not region-differentiated in the fetched summary", "region_caveat": "us-east-1 source, applied as region-uniform per the summary"},
    "cloudwatch_custom_metric_usd_per_month": {"value": 0.30, "kind": "published", "source": "same as cloudwatch_alarm_usd_per_month", "note": "First 10,000 metrics/month tier."},
    "cloudwatch_logs_ingest_usd_per_gb": {"value": 0.50, "kind": "published", "source": "same as cloudwatch_alarm_usd_per_month"},
    "cloudwatch_logs_storage_usd_per_gb_month": {"value": 0.03, "kind": "published", "source": "same as cloudwatch_alarm_usd_per_month"},
    "eip_usd_per_hour": {
        "value": 0.005, "kind": "published",
        "source": "https://aws.amazon.com/blogs/aws/new-aws-public-ipv4-address-charge-public-ip-insights (via WebSearch, fetched 2026-09-19)",
        "note": "Every public IPv4 address, attached or not, since Feb 1 2024 -- matches ADR 014's own ~USD 4/month estimate.",
    },
    "data_transfer_out_usd_per_gb": {"value": 0.09, "kind": "published", "source": "AWS data transfer pricing summary via WebSearch (fetched 2026-09-19), first 10 TB tier, US regions including us-west-2"},
    "data_transfer_out_free_gb_per_month": {"value": 100, "kind": "published", "source": "same as data_transfer_out_usd_per_gb"},
    "resend_free_emails_per_month": {"value": 3000, "kind": "published", "source": "https://resend.com/pricing (via WebSearch, fetched 2026-09-19)"},
    "resend_free_emails_per_day_cap": {"value": 100, "kind": "published", "source": "same as resend_free_emails_per_month"},
    "resend_pro_usd_per_month_low": {"value": 20, "kind": "published", "source": "same as resend_free_emails_per_month", "note": "Pro tier range (20-35 USD/mo) if free-tier volume/day-cap is exceeded; not applied in these scenarios (see email category)."},
    "hours_per_month": {"value": 730, "kind": "constant", "note": "365*24/12."},
}
UNKNOWN_RATES = {
    "embeddings_usd_per_1k_tokens": "pending task42 provider/serving-provider pin (perplexity/pplx-embed-v1-0.6b terms are unqualified per docs/launch/hosted-plan.md)",
    "cloudwatch_logs_volume_gb_per_month": "rate is priced (cloudwatch_logs_ingest_usd_per_gb) but no log shipping is wired yet (ops go through SSM Run Command per ADR 014); volume unknown pending T23.53 telemetry",
    "cloudwatch_custom_metrics_count": "rate is priced (cloudwatch_custom_metric_usd_per_month) but T23.53's metric cardinality is not yet implemented",
    "s3_get_list_restore": "no routine GET/List/restore is modeled (no scheduled integrity check or restore drill in the current design); a restore drill or T23.50 recovery testing adds GET/List cost on top of this baseline, not included here",
}

# deploy/hosted/stack.json.
ROOT_EBS_GB = 8
DATA_EBS_GB = 30
DATA_VOLUME_OPERATOR_HEADROOM_FRACTION = 0.20  # interfaces.md "Storage admission" seam requires reserved operator headroom.
SECRETS_COUNT = 4
CLOUDWATCH_ALARM_COUNT = 2

# deploy/hosted/backup.sh writes to s3://bucket/snapshots/<UTC timestamp>/ -- a
# unique key prefix every run, never overwritten. internal/hosted/backup/backup.go
# Create() writes exactly manifest.json + control.db + one <brain.ID>.bundle per
# brain into that prefix; backup.sh uploads those then COMPLETE separately.
BACKUPS_PER_MONTH = 24 * 30  # hourly, per ADR 014.
OBJECTS_PER_BACKUP_FIXED = 3  # manifest.json, control.db, COMPLETE.
CONTROL_DB_ASSUMED_GB = 0.02

# Because every backup's key is unique (not an overwrite), S3 versioning's
# noncurrent-version mechanism does not apply per-key the way it would for a
# repeatedly-overwritten object. deploy/hosted/stack.json's lifecycle instead
# runs Expiration(30d) -- which, on a versioned bucket, inserts a delete
# marker 30 days after creation, turning the object noncurrent -- followed by
# NoncurrentVersionExpiration(30d), which permanently removes it 30 days
# after THAT. An object's real lifetime in the bucket is therefore ~60 days
# (30+30 in series), not the single 30-day window ADR 014 promises
# customers, and not the ~720x same-key-version amplification an earlier
# revision of this file modeled (that mechanism requires overwriting one
# key repeatedly; backup.sh never does). At steady state (system running
# past 60 days) the bucket retains ~60 days worth of hourly backup-sets.
S3_OBJECT_LIFETIME_DAYS = 30 + 30
RETAINED_BACKUP_SETS = (S3_OBJECT_LIFETIME_DAYS * 24 + 1)  # +1 for the in-progress current hour.

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

AVG_QUERY_TOKENS = 74  # midpoint of workload.json query_tokens 20-128.
AVG_FACT_TOKENS = 272  # midpoint of workload.json fact_tokens 32-512.
ASSUMED_COLD_REOPENS_PER_ACCOUNT_PER_MONTH = 30  # one per day; documented assumption, not measured.
ASSUMED_REBUILD_REEMBED_FRACTION = 0.01  # fraction of an account's stored facts assumed to lack a vector on a cold reopen.
ASSUMED_READINESS_PROBES_PER_MONTH = 30 * 24 * 12  # every 5 minutes (matches the CloudWatch alarm 300s period), 30-day month.
ASSUMED_READINESS_PROBE_TOKENS = 10  # a fixed small probe string, not a customer query.
ASSUMED_LOGINS_PER_ACCOUNT_PER_MONTH = 10  # magic-link signup/login emails; documented assumption.
ASSUMED_RESTARTS_PER_MONTH = 5  # deploys/rehearsals; drives Secrets Manager API call volume.


def rate(name: str) -> float:
    return RATE_TABLE[name]["value"]


def account_totals(mix: dict, usage_fraction: float) -> dict:
    totals = {"accounts": 0, "recalls": 0.0, "writes": 0.0, "input_tokens": 0.0, "storage_bytes": 0.0}
    per_plan = {}
    for plan_id, count in mix.items():
        allowance = PLAN_ALLOWANCES[plan_id]
        plan_totals = {
            "accounts": count,
            "recalls": count * allowance["recalls"] * usage_fraction,
            "writes": count * allowance["writes"] * usage_fraction,
            "input_tokens": count * allowance["input_tokens"] * usage_fraction,
            "storage_bytes": count * allowance["storage_bytes"] * usage_fraction,
        }
        per_plan[plan_id] = plan_totals
        for k in totals:
            totals[k] += plan_totals[k]
    return totals, per_plan


def embeddings_tokens(totals: dict) -> dict:
    write_tokens = totals["writes"] * AVG_FACT_TOKENS
    query_tokens = totals["recalls"] * AVG_QUERY_TOKENS
    rebuild_tokens = totals["accounts"] * ASSUMED_COLD_REOPENS_PER_ACCOUNT_PER_MONTH * (
        (totals["storage_bytes"] / totals["accounts"] / AVG_FACT_TOKENS * ASSUMED_REBUILD_REEMBED_FRACTION) if totals["accounts"] else 0.0
    )
    readiness_tokens = ASSUMED_READINESS_PROBES_PER_MONTH * ASSUMED_READINESS_PROBE_TOKENS
    return {
        "write_tokens": {"value": round(write_tokens), "formula": "writes * AVG_FACT_TOKENS", "kind": "assumption"},
        "query_tokens": {"value": round(query_tokens), "formula": "recalls * AVG_QUERY_TOKENS", "kind": "assumption"},
        "rebuild_reembed_tokens": {"value": round(rebuild_tokens), "formula": "accounts * cold_reopens/mo * (stored_facts_per_account * 1%)", "kind": "assumption", "note": f"assumes {ASSUMED_COLD_REOPENS_PER_ACCOUNT_PER_MONTH} cold reopens/account/month and {ASSUMED_REBUILD_REEMBED_FRACTION:.0%} of stored facts missing a vector per reopen; index.RecoverMemorySearch (ADR 014) only re-embeds facts actually missing a vector, so this is a conservative upper-bound guess, not a measurement"},
        "readiness_tokens": {"value": round(readiness_tokens), "formula": "probes/mo * probe_tokens", "kind": "assumption", "note": f"assumes a /readyz probe every 5 minutes (matches the CloudWatch alarm 300s evaluation period) with a fixed ~{ASSUMED_READINESS_PROBE_TOKENS}-token bounded embedding call per ADR 014"},
        "total_tokens": round(write_tokens + query_tokens + rebuild_tokens + readiness_tokens),
    }


def price_scenario(name: str, spec: dict, measurements: dict | None) -> dict:
    totals, per_plan = account_totals(spec["mix"], spec["usage_fraction"])
    measurements = measurements or {}
    known = {}

    known["ec2_baseline"] = round(rate("ec2_t4g_small_usd_per_hour") * rate("hours_per_month"), 4)
    burst_vcpu_hours = measurements.get("ec2_burst_vcpu_hours")
    burst_kind = "measured" if burst_vcpu_hours is not None else "assumption"
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
    known["secrets_manager_storage"] = round(SECRETS_COUNT * rate("secrets_manager_usd_per_secret_month"), 4)
    secrets_calls = SECRETS_COUNT * ASSUMED_RESTARTS_PER_MONTH
    known["secrets_manager_api_calls"] = {"value": round(secrets_calls / 10000 * rate("secrets_manager_usd_per_10000_calls"), 4), "kind": "assumption", "note": f"assumes {ASSUMED_RESTARTS_PER_MONTH} restarts/month reading all {SECRETS_COUNT} secrets at boot (ADR 014); no other Secrets Manager traffic modeled"}
    known["cloudwatch_alarms"] = round(CLOUDWATCH_ALARM_COUNT * rate("cloudwatch_alarm_usd_per_month"), 4)

    objects_per_backup = OBJECTS_PER_BACKUP_FIXED + max(totals["accounts"], 0)  # manifest.json + control.db + COMPLETE + one bundle per account's primary brain.
    snapshot_size_bytes = measurements.get("snapshot_size_bytes")
    snapshot_kind = "measured" if snapshot_size_bytes is not None else "assumption"
    if snapshot_size_bytes is None:
        snapshot_size_bytes = totals["storage_bytes"] + CONTROL_DB_ASSUMED_GB * 1_000_000_000
    snapshot_size_gb = snapshot_size_bytes / 1_000_000_000
    s3_storage_gb_month = snapshot_size_gb * RETAINED_BACKUP_SETS
    known["s3_storage_backups"] = {
        "value": round(s3_storage_gb_month * rate("s3_standard_usd_per_gb_month"), 4),
        "kind": snapshot_kind,
        "snapshot_size_gb": round(snapshot_size_gb, 4),
        "retained_backup_sets": RETAINED_BACKUP_SETS,
        "objects_per_backup": objects_per_backup,
        "note": f"backup.sh writes a unique timestamped prefix every run (never overwritten); Expiration(30d)+NoncurrentVersionExpiration(30d) in series gives each object a ~{S3_OBJECT_LIFETIME_DAYS}-day bucket lifetime, so ~{RETAINED_BACKUP_SETS} hourly backup-sets are retained at steady state -- see docs/launch/hosted-economics.md.",
    }
    s3_requests = measurements.get("s3_requests_per_month")
    s3_requests_kind = "measured" if s3_requests is not None else "assumption"
    if s3_requests is None:
        s3_requests = BACKUPS_PER_MONTH * objects_per_backup
    known["s3_requests"] = {"value": round(s3_requests / 1000 * rate("s3_put_usd_per_1000_requests"), 4), "kind": s3_requests_kind, "monthly_put_count": s3_requests}
    kms_billable = max(0, s3_requests - rate("kms_free_requests_per_month"))
    known["kms_requests"] = {"value": round(kms_billable / 10000 * rate("kms_usd_per_10000_requests"), 4), "kind": s3_requests_kind, "note": f"SSE-KMS attaches ~1 KMS request per S3 backup PUT; {rate('kms_free_requests_per_month'):.0f}/month free across all regions applied before this line."}

    avg_recall_response_bytes, avg_write_request_bytes = 4096, 1024
    transfer_gb = (totals["recalls"] * avg_recall_response_bytes + totals["writes"] * avg_write_request_bytes) / 1_000_000_000
    billable_transfer_gb = max(0.0, transfer_gb - rate("data_transfer_out_free_gb_per_month"))
    known["data_transfer_out"] = {"value": round(billable_transfer_gb * rate("data_transfer_out_usd_per_gb"), 4), "kind": "assumption", "note": f"{avg_recall_response_bytes}B/recall response, {avg_write_request_bytes}B/write request assumed; first {rate('data_transfer_out_free_gb_per_month'):.0f} GB/month free applied before this line."}

    emails_per_month = totals["accounts"] * ASSUMED_LOGINS_PER_ACCOUNT_PER_MONTH
    emails_per_day = emails_per_month / 30
    within_resend_free = emails_per_month <= rate("resend_free_emails_per_month") and emails_per_day <= rate("resend_free_emails_per_day_cap")
    known["email"] = {
        "value": 0.0 if within_resend_free else None,
        "kind": "assumption",
        "emails_per_month": round(emails_per_month),
        "within_free_tier": within_resend_free,
        "note": f"assumes {ASSUMED_LOGINS_PER_ACCOUNT_PER_MONTH} magic-link emails/account/month; Resend free tier is {rate('resend_free_emails_per_month'):.0f}/month capped {rate('resend_free_emails_per_day_cap'):.0f}/day" + ("" if within_resend_free else f"; exceeds free tier, Pro tier starts at {rate('resend_pro_usd_per_month_low'):.0f} USD/month -- exact overage price not modeled"),
    }

    known_subtotal = sum((v["value"] if isinstance(v, dict) else v) or 0.0 for v in known.values())

    embed = embeddings_tokens(totals)
    unknown = {
        "embeddings": {**embed, "usd_per_1k_tokens": None, "monthly_usd": None, "reason": UNKNOWN_RATES["embeddings_usd_per_1k_tokens"]},
        "cloudwatch_logs": {"priced_usd_per_gb": rate("cloudwatch_logs_ingest_usd_per_gb"), "reason": UNKNOWN_RATES["cloudwatch_logs_volume_gb_per_month"]},
        "cloudwatch_custom_metrics": {"priced_usd_per_metric": rate("cloudwatch_custom_metric_usd_per_month"), "reason": UNKNOWN_RATES["cloudwatch_custom_metrics_count"]},
        "s3_get_list_restore": {"reason": UNKNOWN_RATES["s3_get_list_restore"]},
    }
    if not within_resend_free:
        unknown["email_overage"] = {"reason": "email volume exceeds Resend's free tier; exact Pro-tier overage price not modeled (range 20-35 USD/month base)"}

    variable_categories = {"s3_storage_backups", "s3_requests", "kms_requests", "data_transfer_out", "secrets_manager_api_calls"}
    variable_total = sum((known[c]["value"] if isinstance(known[c], dict) else known[c]) for c in variable_categories if c in known)
    per_plan_contribution = {}
    denom = sum(p["storage_bytes"] for p in per_plan.values())
    for plan_id, plan_totals in per_plan.items():
        share = (plan_totals["storage_bytes"] / denom) if denom else 0.0
        per_plan_contribution[plan_id] = {"accounts": plan_totals["accounts"], "storage_share": round(share, 4), "variable_cost_usd": round(variable_total * share, 4)}

    return {
        "scenario": name,
        "assumption": spec["assumption"],
        "account_totals": {k: round(v) for k, v in totals.items()},
        "storage_risk": {"exceeds_usable_data_volume": storage_risk, "usable_data_volume_bytes": round(usable_data_bytes)},
        "known_categories_usd": known,
        "known_subtotal_usd": round(known_subtotal, 2),
        "per_plan_contribution": {
            "note": "Fixed shared-node infrastructure (EC2, EBS, EIP, KMS key, Secrets Manager storage, CloudWatch alarms) is not allocated per-plan -- one node serves every tenant. Only usage-driven categories are apportioned, by each plan's share of total customer storage bytes.",
            "variable_total_usd": round(variable_total, 4),
            "by_plan": per_plan_contribution,
        },
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
    try:
        manifest = json.loads(args.manifest.read_text())
    except json.JSONDecodeError as e:
        print(json.dumps({"status": "BLOCKED", "reason": f"manifest is not valid JSON: {e}"}, indent=2))
        return 2

    measurements = None
    if args.measurements is not None:
        if not args.measurements.exists():
            print(json.dumps({"status": "BLOCKED", "reason": f"measurements file not found: {args.measurements}"}, indent=2))
            return 2
        try:
            measurements = json.loads(args.measurements.read_text())
        except json.JSONDecodeError as e:
            print(json.dumps({"status": "BLOCKED", "reason": f"measurements file is not valid JSON: {e}"}, indent=2))
            return 2

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
        "citation_note": "Candidate rates retain worker-provided citations and explicit assumptions. Coordinator verified S3 Standard directly against the AWS us-west-2 regional catalog (s3-rate.json). Other region/account-specific rates and free-tier availability require verification before spend approval; the EC2 hourly value remains an unverified regional-parity assumption.",
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
