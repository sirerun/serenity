#!/usr/bin/env python3
"""Hosted Serenity monthly cost model (T23.60).

Prices four fixed scenarios (0 accounts idle, an assumed light-usage 10- and
100-account mix, and the ratified 1 Scale + 3 Builder + 10 Free full-limit
mix) against deploy/hosted/stack.json's actual resources (t4g.small, 8 GB
root + 30 GB data gp3 EBS, one Elastic IP, one KMS key with rotation enabled,
four Secrets Manager secrets, two CloudWatch alarms, hourly S3 backups written
by deploy/hosted/backup.sh to a unique timestamped key prefix per run, with
30-day Expiration + 30-day NoncurrentVersionExpiration).

This is arithmetic over the rates and assumptions below. A successful run
reports CALCULATED, never a launch cost qualification: every result carries
launch_cost_qualification NOT_QUALIFIED with the unknowns that block a full
maximum, and unknown categories are reported, never zeroed.

Rates. Every rate in RATE_TABLE carries a "verification":

- aws_regional_catalog_primary: read from AWS's own US West (Oregon) price
  list. The entry names the receipt file kept beside the evidence
  (docs/launch/evidence/T23.60/aws-rates/, plus s3-rate.json), the rate code,
  the catalog unit and price, and the factor that converts the catalog unit to
  the rate's unit. The receipts carry the source's version, publication date
  and SHA-256.
- secondary_summary_not_regionally_verified: taken from a summary page of a
  worker's search this session and not confirmed against a regional AWS
  catalog; a reviewer must verify it before any spend decision.
- vendor_page_secondary: a non-AWS vendor's published page (Resend).

Measurements. --measurements takes a document in the owned schema
serenity-hosted-cost-measurements v1 (see MEASUREMENT_SCHEMA and
parse_measurements). Each record binds one quantity, in an explicit unit, to
one scenario and to a source and observation time. A record changes only the
named scenario and only the categories that quantity feeds. Any malformed,
unattributed, out-of-range or ambiguous record blocks the run with a
sanitized reason; nothing is coerced. scripts/hosted/load.py does not produce
this document.
"""
from __future__ import annotations

import argparse
import json
import math
import re
from datetime import datetime, timedelta, timezone
from pathlib import Path

EVIDENCE_DIR_NAME = "docs/launch/evidence/T23.60"
_EC2_RECEIPT = "aws-rates/ec2-oregon-rate-receipt.json"
_EC2_EBS_RECEIPT = "aws-rates/ec2-ebs-oregon-rate-extract.json"
_REGIONAL_RECEIPT = "aws-rates/aws-regional-rates-selected.json"
_S3_RECEIPT = "s3-rate.json"

# Receipt files, relative to EVIDENCE_DIR_NAME, with the SHA-256 of each file as committed and the
# upstream sources they extract from. The full AWS EC2 price list (474,950,676 bytes) was streamed and
# hashed, not stored.
RATE_RECEIPTS = {
    _EC2_RECEIPT: {
        "sha256": "d37a8d3dc613187bdbc105ab1cc105abaa86cea3e45a7491ffe4ed043526e370",
        "sources": [{
            "name": "ec2-ondemand-without-sec-sel, US West (Oregon), Linux",
            "url": "https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/US%20West%20(Oregon)/Linux/index.json",
            "sha256": "858d2bdd6067c75ef9ae994b9640ab0153507723a676f161e165f364b603bb6a",
            "publication_date": "2026-09-18T20:33:44Z",
        }],
    },
    _EC2_EBS_RECEIPT: {
        "sha256": "946d3ed7b200b5a9b278602e92f54a00c24ff85b98e519d2d6e941d8f5056eb4",
        "sources": [{
            "name": "AmazonEC2 us-west-2 price list (EBS gp3 and T4g CPU credits)",
            "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/20260918212757/us-west-2/index.json",
            "sha256": "ec1e0d93973aa692ae4c350df1f52688b0c5b76abc2523a2d536e374dca8e904",
            "version": "20260918212757",
            "publication_date": "2026-09-18T21:27:57Z",
        }],
    },
    _REGIONAL_RECEIPT: {
        "sha256": "e823dca1ab9c8367b8bb5bfb473d0ef951b2e99042d95ffc66f199e4ef399c78",
        "sources": [
            {"name": "awskms", "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/awskms/current/us-west-2/index.json",
             "sha256": "1b76589db555229b457c8d8c22549bd4b39ed20ff70b8eacc37fc1871b9f4ed7", "version": "20260911124601", "publication_date": "2026-09-11T12:46:01Z"},
            {"name": "AWSSecretsManager", "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AWSSecretsManager/current/us-west-2/index.json",
             "sha256": "eca00c993943f761212d1df8cc90dd12f85b85e9f865ce2f82379baa99c4ce7b", "version": "20260911124610", "publication_date": "2026-09-11T12:46:10Z"},
            {"name": "AmazonVPC", "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonVPC/current/us-west-2/index.json",
             "sha256": "135af0c0e46094eb449afcd990a970fded51f409a3998c727ac0bad09e09a0e8", "version": "20260917190528", "publication_date": "2026-09-17T19:05:28Z"},
            {"name": "AmazonCloudWatch", "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonCloudWatch/current/us-west-2/index.json",
             "sha256": "cbc665d7f5c6ebf83792a44f342ce10c0e002dc2bf128d5b0b8ea088160c6c21", "version": "20260918142158", "publication_date": "2026-09-18T14:21:58Z"},
        ],
    },
    _S3_RECEIPT: {
        "sha256": "d266bbb0bb6bde3f92833f123531535f783f039df1f8121444592bfd7bb09691",
        "sources": [{
            "name": "AmazonS3 us-west-2 price list (Standard storage tiers)",
            "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonS3/current/us-west-2/index.json",
            "publication_date": "2026-09-18T17:47:47Z",
        }],
    },
}

PRIMARY = "aws_regional_catalog_primary"
SECONDARY = "secondary_summary_not_regionally_verified"
VENDOR = "vendor_page_secondary"
VERIFICATIONS = (PRIMARY, SECONDARY, VENDOR)


def _primary(value, receipt, rate_code, catalog_unit, catalog_usd, *, factor=1.0, source, note=None):
    """A rate read from an AWS regional price list. factor converts the catalog price to value."""
    entry = {
        "value": value, "kind": "published", "verification": PRIMARY,
        "source": source,
        "receipt": {"file": receipt, "rate_code": rate_code, "catalog_unit": catalog_unit, "catalog_usd": catalog_usd, "catalog_to_rate_factor": factor},
    }
    if note:
        entry["note"] = note
    return entry


_EC2_SRC = "AWS EC2 on-demand price list, US West (Oregon), Linux, published 2026-09-18T20:33:44Z"
_EBS_SRC = "AWS AmazonEC2 us-west-2 price list version 20260918212757, published 2026-09-18T21:27:57Z"
_KMS_SRC = "AWS awskms us-west-2 price list version 20260911124601, published 2026-09-11T12:46:01Z"
_SM_SRC = "AWS AWSSecretsManager us-west-2 price list version 20260911124610, published 2026-09-11T12:46:10Z"
_VPC_SRC = "AWS AmazonVPC us-west-2 price list version 20260917190528, published 2026-09-17T19:05:28Z"
_CW_SRC = "AWS AmazonCloudWatch us-west-2 price list version 20260918142158, published 2026-09-18T14:21:58Z"

RATE_TABLE = {
    "ec2_t4g_small_usd_per_hour": _primary(
        0.0168, _EC2_RECEIPT, "NTSJZ6S2KD2YFRVB.JRTCKXETXF.6YS6EN2CT7", "Hrs", "0.0168000000", source=_EC2_SRC,
        note="On-demand Linux t4g.small in US West (Oregon), before credits, storage and network. No reservation, savings plan or account-specific offer is implied.",
    ),
    "ec2_t4g_burst_credit_usd_per_vcpu_hour": _primary(
        0.04, _EC2_EBS_RECEIPT, "U8TTEWHUFZA4FZYG.JRTCKXETXF.6YS6EN2CT7", "vCPU-Hours", "0.0400000000", source=_EBS_SRC,
        note="T4g surplus CPU credit, billed only when a burstable instance runs in unlimited credit mode and exhausts its balance.",
    ),
    "ebs_gp3_usd_per_gb_month": _primary(
        0.08, _EC2_EBS_RECEIPT, "BB8UJWJ4XPFJB95G.JRTCKXETXF.6YS6EN2CT7", "GB-Mo", "0.0800000000", source=_EBS_SRC,
        note="Baseline 3,000 IOPS and 125 MiB/s are included; this deployment's 8 GB root and 30 GB data volumes are not modeled beyond that baseline.",
    ),
    "ebs_gp3_extra_iops_usd_per_iops_month": _primary(
        0.005, _EC2_EBS_RECEIPT, "XBTB827YUJSN6SSV.JRTCKXETXF.6YS6EN2CT7", "IOPS-Mo", "0.0050000000", source=_EBS_SRC,
        note="Beyond the 3,000 free baseline IOPS. Not applied: no evidence of exceeding baseline.",
    ),
    "ebs_gp3_extra_throughput_usd_per_mibps_month": _primary(
        0.04, _EC2_EBS_RECEIPT, "3MQHJKUUZSKTF82F.JRTCKXETXF.6YS6EN2CT7", "GiBps-mo", "40.9600000000", factor=1 / 1024, source=_EBS_SRC,
        note="The catalog prices 40.96 USD per GiBps-month, which is 0.04 USD per MiBps-month (divide by 1,024). Beyond the 125 MiB/s free baseline. Not applied: no evidence of exceeding baseline.",
    ),
    "s3_standard_usd_per_gb_month": _primary(
        0.023, _S3_RECEIPT, "sku:Z3FQZG73HYSPVABR@first-50-TB", "GB-Mo", "0.0230000000", source="AWS AmazonS3 us-west-2 price list, published 2026-09-18T17:47:47Z, first 50 TB tier",
        note="First 50 TB tier. The earlier claim of 0.0265 for Oregon was incorrect.",
    ),
    "s3_put_usd_per_1000_requests": {
        "value": 0.005, "kind": "published", "verification": SECONDARY, "source": "https://aws.amazon.com/s3/pricing/ (worker search summary, fetched 2026-09-19)",
        "note": "Not confirmed against the us-west-2 catalog. Needs a regional primary receipt before any spend decision.",
    },
    "s3_get_usd_per_1000_requests": {
        "value": 0.0004, "kind": "published", "verification": SECONDARY, "source": "https://aws.amazon.com/s3/pricing/ (worker search summary, fetched 2026-09-19)",
        "note": "Not confirmed against the us-west-2 catalog. No routine GET is modeled.",
    },
    "kms_key_usd_per_month": _primary(
        1.00, _REGIONAL_RECEIPT, "S8HBXBVJKWKDP9AS.JRTCKXETXF.6YS6EN2CT7", "Keys", "1.0000000000", source=_KMS_SRC,
        note="One customer managed key version. The template sets EnableKeyRotation, and AWS adds 1 USD per month for each of the first two rotations (prorated hourly), capped after the second (https://aws.amazon.com/kms/pricing/), so storage can rise from 1 to 3 USD per month. 1 USD is the figure before any rotation.",
    ),
    "kms_rotation_billed_versions_max": {
        "value": 2, "kind": "published", "verification": SECONDARY,
        "source": "https://aws.amazon.com/kms/pricing/ ('the first and second rotation of the key adds $1/month (prorated hourly) ... capped at the second rotation'), page saved by the coordinator",
        "note": "Number of rotations that add a key-version charge. A count from AWS's pricing page text, not a catalog rate, so it is not regionally verified.",
    },
    "kms_usd_per_10000_requests": _primary(
        0.03, _REGIONAL_RECEIPT, "SE9KXT6M6JTP7E4W.JRTCKXETXF.6YS6EN2CT7", "Requests", "0.0000030000", factor=10000, source=_KMS_SRC,
        note="The catalog prices one request at 0.000003 USD.",
    ),
    "kms_free_requests_per_month": _primary(
        20000, _REGIONAL_RECEIPT, "VTFSAGP364M6QE4A.A429C66SYZ.7K6Z2V4Y4Q", "Requests", "0.0000000000", source=_KMS_SRC,
        note="A global free tier shared across every AWS KMS use in the account and all regions. The model cannot assume the whole allowance is unused; the peak exposure uses none of it.",
    ),
    "secrets_manager_usd_per_secret_month": _primary(
        0.40, _REGIONAL_RECEIPT, "DWJP9S4V3HP98UNC.JRTCKXETXF.6YS6EN2CT7", "Secrets", "0.4000000000", source=_SM_SRC,
    ),
    "secrets_manager_usd_per_10000_calls": _primary(
        0.05, _REGIONAL_RECEIPT, "AEBQHWFEG8Q4Y7AT.JRTCKXETXF.6YS6EN2CT7", "API Requests", "0.0000050000", factor=10000, source=_SM_SRC,
        note="The catalog prices one API request at 0.000005 USD. No free tier for API calls is listed.",
    ),
    "cloudwatch_alarm_usd_per_month": _primary(
        0.10, _REGIONAL_RECEIPT, "SJTFAZNHSW2WVZB2.JRTCKXETXF.6YS6EN2CT7", "Alarms", "0.1000000000", source=_CW_SRC,
        note="Standard resolution alarm metric.",
    ),
    "cloudwatch_custom_metric_usd_per_month": _primary(
        0.30, _REGIONAL_RECEIPT, "CN6TP6ZEVS58RK7M.JRTCKXETXF.A6VD9GXV7W", "Metrics", "0.3000000000", source=_CW_SRC,
        note="First 10,000 metrics tier.",
    ),
    "cloudwatch_logs_ingest_usd_per_gb": _primary(
        0.50, _REGIONAL_RECEIPT, "CWY7X4MZ4F3MP5SD.JRTCKXETXF.6YS6EN2CT7", "GB", "0.5000000000", source=_CW_SRC,
        note="Custom log data ingested in the Standard log class.",
    ),
    "cloudwatch_logs_storage_usd_per_gb_month": _primary(
        0.03, _REGIONAL_RECEIPT, "MN45SJANDTCPR9QA.JRTCKXETXF.6YS6EN2CT7", "GB-Mo", "0.0300000000", source=_CW_SRC,
    ),
    "eip_usd_per_hour": _primary(
        0.005, _REGIONAL_RECEIPT, "NBHXEKTE88TJDDQF.JRTCKXETXF.6YS6EN2CT7", "Hrs", "0.0050000000", source=_VPC_SRC,
        note="In-use public IPv4 address. An idle address is priced the same (rate code 4KKZ7RH6GMEH6Q4Q.JRTCKXETXF.6YS6EN2CT7).",
    ),
    "data_transfer_out_usd_per_gb": {
        "value": 0.09, "kind": "published", "verification": SECONDARY,
        "source": "AWS data transfer pricing summary via worker search (fetched 2026-09-19), first 10 TB tier, US regions including us-west-2",
        "note": "Not confirmed against the us-west-2 catalog.",
    },
    "data_transfer_out_free_gb_per_month": {
        "value": 100, "kind": "published", "verification": SECONDARY, "source": "same as data_transfer_out_usd_per_gb",
        "note": "An account-wide free allowance across services and regions. The model cannot assume it is unused; the peak exposure uses none of it.",
    },
    "resend_free_emails_per_month": {"value": 3000, "kind": "published", "verification": VENDOR, "source": "https://resend.com/pricing (worker search summary, fetched 2026-09-19)"},
    "resend_free_emails_per_day_cap": {"value": 100, "kind": "published", "verification": VENDOR, "source": "same as resend_free_emails_per_month"},
    "resend_pro_usd_per_month_low": {
        "value": 20, "kind": "published", "verification": VENDOR, "source": "same as resend_free_emails_per_month",
        "note": "Pro tier range (20-35 USD/mo) if free-tier volume or the day cap is exceeded; not applied in these scenarios (see email category).",
    },
    "hours_per_month": {"value": 730, "kind": "constant", "note": "365*24/12."},
}
UNKNOWN_RATES = {
    "embeddings_usd_per_1k_tokens": "pending task42 provider/serving-provider pin (perplexity/pplx-embed-v1-0.6b terms are unqualified per docs/launch/hosted-plan.md)",
    "cloudwatch_logs_volume_gb_per_month": "rate is priced (cloudwatch_logs_ingest_usd_per_gb) but no log shipping is wired yet (ops go through SSM Run Command per ADR 014); volume unknown pending T23.53 telemetry",
    "cloudwatch_custom_metrics_count": "rate is priced (cloudwatch_custom_metric_usd_per_month) but T23.53's metric cardinality is not yet implemented",
    "s3_get_list_restore": "no routine GET/List/restore is modeled (no scheduled integrity check or restore drill in the current design); a restore drill or T23.50 recovery testing adds GET/List cost on top of this baseline, not included here",
    "control_db_lifetime_growth": "control.db is copied whole into every snapshot and no runtime code prunes committed reservations or audit_log rows, so it grows with lifetime writes and recalls. The 20 MB constant is a scenario assumption at snapshot time, not a lifetime bound. Each scenario reports a sensitivity at sampled row sizes, which is not an upper bound",
    "ec2_cpu_credit_mode": "the template sets no CreditSpecification, so a t4g.small uses the account default for T4g, which is unverified (AWS's default is unlimited). Unlimited mode bills surplus credits; standard mode bills none and throttles at baseline. An owner must confirm the mode; the model sets neither",
    "s3_multipart_requests": "aws s3 cp uploads objects larger than the CLI multipart threshold in parts, and each part is a request. The deployed CLI's setting is not in the template; each scenario reports a sensitivity at the CLI default, not in any subtotal",
    "global_free_allowances": "the KMS request allowance and the data-transfer allowance are account-wide, and the shared AWS account's other use is unknown, so neither can be assumed unused; the peak exposure uses none of them",
    "email_peak_volume": "Resend cost is modeled from average logins per month, and an average day is not a maximum: a burst past the 100-per-day free cap needs a paid tier. Peak login volume is unknown, so no email maximum is claimed",
}

# deploy/hosted/stack.json.
ROOT_EBS_GB = 8
DATA_EBS_GB = 30
DATA_VOLUME_OPERATOR_HEADROOM_FRACTION = 0.20  # interfaces.md "Storage admission" seam requires reserved operator headroom.
SECRETS_COUNT = 4
CLOUDWATCH_ALARM_COUNT = 2

# t4g.small: 2 vCPUs, baseline 20% of each vCPU (24 CPU credits earned per hour).
# https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-credits-baseline-concepts.html
T4G_SMALL_VCPUS = 2
T4G_SMALL_BASELINE_PER_VCPU = 0.20
PEAK_MEAN_CPU_UTILIZATION = 0.70  # a sustained-load sensitivity point.
PEAK_SUSTAINED_CPU_UTILIZATION = 1.00  # every vCPU pinned, credit balance exhausted.
# Mean-case assumption used in the known subtotal at full usage: 2% of vCPU-hours billed as surplus. It is NOT a peak.
ASSUMED_MEAN_SURPLUS_FRACTION = 0.02

# deploy/hosted/backup.sh writes to s3://bucket/snapshots/<UTC timestamp>/ -- a
# unique key prefix every run, never overwritten. internal/hosted/backup/backup.go
# Create() writes exactly manifest.json + control.db + one <brain.ID>.bundle per
# brain into that prefix; backup.sh uploads those then COMPLETE separately.
BACKUPS_PER_MONTH = 24 * 30  # hourly, per ADR 014.
OBJECTS_PER_BACKUP_FIXED = 3  # manifest.json, control.db, COMPLETE.
CONTROL_DB_ASSUMED_GB = 0.02  # scenario assumption at snapshot time; NOT a lifetime bound (see CONTROL_DB_*).
MULTIPART_CHUNK_BYTES = 8 * 1024 * 1024  # AWS CLI default multipart threshold and chunk size; the deployed CLI's setting is not in the template.

# Sampled control.db row sizes from the real-writer audit (one account, one brain, 1,500 writes and 11 recalls,
# a 3-dimension stub embedder), launch-audit-real-writer-handoff.md sha256
# cff5bb74012f404b345240e54acd83b62a8fdaeb3a4ae8cf9e0ed8b818a4ef40: about 301 B per reservation row and 229 B
# per audit row including indexes; a write adds 2 of each and a recall 1 of each. The recall figure is derived
# from row sizes, not from measured growth, and a real provider's audit rows may be larger. A sample, not a bound.
CONTROL_DB_ROW_BYTES = {"reservation": 301, "audit": 229}
CONTROL_DB_BYTES_PER_WRITE = 2 * 301 + 2 * 229
CONTROL_DB_BYTES_PER_RECALL = 301 + 229
CONTROL_DB_HORIZONS_MONTHS = (1, 12, 24)

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
# This is the deployed-template retention and stays the baseline; a thinning proposal is not modeled here.
S3_OBJECT_LIFETIME_DAYS = 30 + 30
RETAINED_BACKUP_SETS = (S3_OBJECT_LIFETIME_DAYS * 24 + 1)  # +1 for the in-progress current hour.

# internal/hosted/plans/plans.go V1; kept in sync by hand, no cross-language import (a test parses the Go table).
PLAN_ALLOWANCES = {
    "free": {"brains": 1, "memories": 1000, "recalls": 10000, "writes": 500, "input_tokens": 100000, "storage_bytes": 100_000_000},
    "builder": {"brains": 3, "memories": 10000, "recalls": 100000, "writes": 5000, "input_tokens": 1_000_000, "storage_bytes": 1_000_000_000},
    "scale": {"brains": 10, "memories": 50000, "recalls": 300000, "writes": 20000, "input_tokens": 4_000_000, "storage_bytes": 5_000_000_000},
}

ONE_DEFAULT_BRAIN = "one_default_brain_per_account"
ALL_ALLOWED_BRAINS = "all_allowed_brains"
SCENARIOS = {
    "idle_0_accounts": {"mix": {}, "usage_fraction": 0.0, "brains": ONE_DEFAULT_BRAIN, "assumption": "no signups yet; fixed infrastructure cost floor"},
    "10_accounts_light": {"mix": {"free": 8, "builder": 2, "scale": 0}, "usage_fraction": 0.10, "brains": ONE_DEFAULT_BRAIN, "assumption": "early-adoption mix at 10% of each plan's allowance, each account with its default brain only; product/growth input not yet recorded"},
    "100_accounts_light": {"mix": {"free": 90, "builder": 8, "scale": 2}, "usage_fraction": 0.10, "brains": ONE_DEFAULT_BRAIN, "assumption": "at ADR 014's 100 free-account cap, light usage, each account with its default brain only; product/growth input not yet recorded"},
    "full_limit_mix": {"mix": {"free": 10, "builder": 3, "scale": 1}, "usage_fraction": 1.00, "brains": ALL_ALLOWED_BRAINS, "assumption": "the ratified 1 Scale + 3 Builder + 10 Free mix at full advertised allowance (90,000 total facts) with every allowed brain (29) present, per docs/launch/hosted-completion/evidence.md"},
}

AVG_QUERY_TOKENS = 74  # midpoint of workload.json query_tokens 20-128.
AVG_FACT_TOKENS = 272  # midpoint of workload.json fact_tokens 32-512.
ASSUMED_COLD_REOPENS_PER_ACCOUNT_PER_MONTH = 30  # one per day; documented assumption, not measured.
ASSUMED_REBUILD_REEMBED_FRACTION = 0.01  # fraction of an account's stored facts assumed to lack a vector on a cold reopen.
ASSUMED_READINESS_PROBES_PER_MONTH = 30 * 24 * 12  # every 5 minutes (matches the CloudWatch alarm 300s period), 30-day month.
ASSUMED_READINESS_PROBE_TOKENS = 10  # a fixed small probe string, not a customer query.
ASSUMED_LOGINS_PER_ACCOUNT_PER_MONTH = 10  # magic-link signup/login emails; documented assumption.
ASSUMED_RESTARTS_PER_MONTH = 5  # deploys/rehearsals; drives Secrets Manager API call volume.


# --- Measurements -----------------------------------------------------------------------------------------------------

MEASUREMENT_SCHEMA = "serenity-hosted-cost-measurements"
MEASUREMENT_SCHEMA_VERSION = 1
MAX_MEASUREMENTS_FILE_BYTES = 1 << 20
MAX_FUTURE_SKEW = timedelta(minutes=5)

_SURPLUS_VCPU_HOURS_MAX = round(T4G_SMALL_VCPUS * (1 - T4G_SMALL_BASELINE_PER_VCPU) * RATE_TABLE["hours_per_month"]["value"], 6)

# Each quantity has one unit, one type and a physical range. A snapshot cannot exceed the data volume it is read
# from, a month has at most one billed surplus vCPU-hour per non-baseline vCPU-hour, and the request ceiling sits
# four orders of magnitude above the modeled hourly backup.
MEASURED_QUANTITIES = {
    "snapshot_size_bytes": {"unit": "bytes", "integer": True, "min": 1, "max": DATA_EBS_GB * 1_000_000_000},
    "s3_put_requests_per_month": {"unit": "requests_per_month", "integer": True, "min": 1, "max": 10_000_000},
    "ec2_surplus_credit_vcpu_hours": {"unit": "vcpu_hours_per_month", "integer": False, "min": 0, "max": _SURPLUS_VCPU_HOURS_MAX},
}
_RECORD_KEYS = {"scenario", "quantity", "unit", "value", "observed_at", "source"}
_SOURCE_KEYS = {"kind", "ref", "sha256"}
_SOURCE_KIND = re.compile(r"[a-z0-9_]{1,64}")
_SHA256 = re.compile(r"[0-9a-f]{64}")
_REF = re.compile(r"[\x21-\x7e]{1,256}")  # printable ASCII, no space or control character.
MAX_MEASUREMENT_RECORDS = len(SCENARIOS) * len(MEASURED_QUANTITIES)


class MeasurementError(ValueError):
    """A measurement failed validation. The message names fields and fixed limits, never an input value."""


def _is_number(value: object) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def _parse_time(value: object, label: str, now: datetime) -> datetime:
    if not isinstance(value, str) or not 10 <= len(value) <= 40:
        raise MeasurementError(f"{label}.observed_at must be an RFC 3339 UTC timestamp string")
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00") if value.endswith("Z") else value)
    except ValueError:
        raise MeasurementError(f"{label}.observed_at must be an RFC 3339 UTC timestamp string") from None
    if parsed.tzinfo is None or parsed.utcoffset() != timedelta(0):
        raise MeasurementError(f"{label}.observed_at must be an RFC 3339 UTC timestamp string")
    if parsed > now + MAX_FUTURE_SKEW:
        raise MeasurementError(f"{label}.observed_at is in the future")
    return parsed


def validate_record(record: object, label: str, expected_scenario: str | None = None, now: datetime | None = None) -> dict:
    """Return a normalized measurement record or raise MeasurementError.

    Requires an explicit scenario, quantity, unit, value, observation time and source. Never coerces: a boolean,
    string, NaN, infinity, negative, out-of-range or wrongly typed value is rejected."""
    now = now or datetime.now(timezone.utc)
    if not isinstance(record, dict):
        raise MeasurementError(f"{label} must be a JSON object")
    if set(record) != _RECORD_KEYS:
        raise MeasurementError(f"{label} must have exactly the keys {sorted(_RECORD_KEYS)}")
    scenario = record["scenario"]
    if not isinstance(scenario, str) or scenario not in SCENARIOS:
        raise MeasurementError(f"{label}.scenario must be one of {sorted(SCENARIOS)}")
    if expected_scenario is not None and scenario != expected_scenario:
        raise MeasurementError(f"{label}.scenario does not match the scenario being priced")
    quantity = record["quantity"]
    if not isinstance(quantity, str) or quantity not in MEASURED_QUANTITIES:
        raise MeasurementError(f"{label}.quantity must be one of {sorted(MEASURED_QUANTITIES)}")
    spec = MEASURED_QUANTITIES[quantity]
    if record["unit"] != spec["unit"]:
        raise MeasurementError(f"{label}.unit must be exactly {spec['unit']!r} for {quantity}")
    value = record["value"]
    if not _is_number(value):
        raise MeasurementError(f"{label}.value must be a JSON number")
    if spec["integer"] and not isinstance(value, int):
        raise MeasurementError(f"{label}.value must be a JSON integer for {quantity}")
    try:
        finite = math.isfinite(value)
    except OverflowError:  # an int too large to convert to a float
        finite = False
    if not finite or value < spec["min"] or value > spec["max"]:
        raise MeasurementError(f"{label}.value must be a finite number from {spec['min']} to {spec['max']} {spec['unit']}")
    source = record["source"]
    if not isinstance(source, dict) or set(source) != _SOURCE_KEYS:
        raise MeasurementError(f"{label}.source must be an object with exactly the keys {sorted(_SOURCE_KEYS)}")
    if not isinstance(source["kind"], str) or not _SOURCE_KIND.fullmatch(source["kind"]):
        raise MeasurementError(f"{label}.source.kind must match [a-z0-9_]{{1,64}}")
    if not isinstance(source["ref"], str) or not _REF.fullmatch(source["ref"]):
        raise MeasurementError(f"{label}.source.ref must be 1 to 256 printable ASCII characters with no space")
    if not isinstance(source["sha256"], str) or not _SHA256.fullmatch(source["sha256"]):
        raise MeasurementError(f"{label}.source.sha256 must be 64 lowercase hex characters")
    observed = _parse_time(record["observed_at"], label, now)
    return {
        "scenario": scenario, "quantity": quantity, "unit": spec["unit"], "value": value,
        "observed_at": observed.astimezone(timezone.utc).isoformat().replace("+00:00", "Z"),
        "source": {"kind": source["kind"], "ref": source["ref"], "sha256": source["sha256"]},
    }


def parse_measurements(document: object, now: datetime | None = None) -> dict[str, dict[str, dict]]:
    """Validate a serenity-hosted-cost-measurements v1 document and return {scenario: {quantity: record}}.

    A document that is not this schema (a load result, an empty file, a list) is rejected, so no receipt is
    ever labeled measured by accident."""
    if not isinstance(document, dict) or set(document) != {"schema", "schema_version", "records"}:
        raise MeasurementError(f"measurements must be an object with exactly the keys schema, schema_version and records (schema {MEASUREMENT_SCHEMA!r})")
    if document["schema"] != MEASUREMENT_SCHEMA or document["schema_version"] != MEASUREMENT_SCHEMA_VERSION or isinstance(document["schema_version"], bool):
        raise MeasurementError(f"measurements schema must be {MEASUREMENT_SCHEMA!r} version {MEASUREMENT_SCHEMA_VERSION}")
    records = document["records"]
    if not isinstance(records, list):
        raise MeasurementError("measurements.records must be a JSON array")
    if not records:
        raise MeasurementError("measurements.records is empty: an empty file measures nothing and is not applied")
    if len(records) > MAX_MEASUREMENT_RECORDS:
        raise MeasurementError(f"measurements.records has more than {MAX_MEASUREMENT_RECORDS} entries, one per scenario and quantity")
    now = now or datetime.now(timezone.utc)
    out: dict[str, dict[str, dict]] = {}
    for index, raw in enumerate(records):
        record = validate_record(raw, f"records[{index}]", None, now)
        by_quantity = out.setdefault(record["scenario"], {})
        if record["quantity"] in by_quantity:
            raise MeasurementError(f"records[{index}] repeats a scenario and quantity already given")
        by_quantity[record["quantity"]] = record
    return out


def _scenario_measurements(name: str, measurements: object) -> dict[str, dict]:
    """Re-validate the per-scenario mapping price_scenario receives, so a direct caller cannot bypass parse_measurements."""
    if measurements is None:
        return {}
    if not isinstance(measurements, dict):
        raise MeasurementError("measurements for a scenario must be a JSON object keyed by quantity")
    out = {}
    for quantity, raw in measurements.items():
        record = validate_record(raw, f"measurements[{quantity!r}]" if isinstance(quantity, str) and quantity in MEASURED_QUANTITIES else "measurements[<unknown quantity>]", name)
        if record["quantity"] != quantity:
            raise MeasurementError("a measurement is filed under a different quantity than it names")
        out[quantity] = record
    return out


# --- Arithmetic -------------------------------------------------------------------------------------------------------

def rate(name: str) -> float:
    return RATE_TABLE[name]["value"]


def _amount(entry) -> float:
    value = entry["value"] if isinstance(entry, dict) else entry
    return value or 0.0


def account_totals(mix: dict, usage_fraction: float) -> tuple[dict, dict]:
    totals = {"accounts": 0, "brains_allowed": 0, "recalls": 0.0, "writes": 0.0, "input_tokens": 0.0, "storage_bytes": 0.0}
    per_plan = {}
    for plan_id, count in mix.items():
        allowance = PLAN_ALLOWANCES[plan_id]
        plan_totals = {
            "accounts": count,
            "brains_allowed": count * allowance["brains"],
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


def control_db_lifetime_growth(totals: dict) -> dict:
    """Sensitivity of the snapshot's control.db to lifetime growth. Not an upper bound: no runtime code prunes it."""
    monthly_bytes = totals["writes"] * CONTROL_DB_BYTES_PER_WRITE + totals["recalls"] * CONTROL_DB_BYTES_PER_RECALL
    horizons = []
    for months in CONTROL_DB_HORIZONS_MONTHS:
        grown_gb = months * monthly_bytes / 1_000_000_000
        horizons.append({
            "months": months,
            "control_db_gb": round(CONTROL_DB_ASSUMED_GB + grown_gb, 4),
            "added_s3_backup_storage_usd_per_month": round(grown_gb * RETAINED_BACKUP_SETS * rate("s3_standard_usd_per_gb_month"), 2),
        })
    return {
        "status": "unbounded_in_current_runtime",
        "monthly_usd": None,
        "assumed_control_db_gb_in_snapshot": CONTROL_DB_ASSUMED_GB,
        "assumed_control_db_kind": "scenario assumption at snapshot time, never a lifetime bound",
        "sampled_row_bytes": {**CONTROL_DB_ROW_BYTES, "per_write": CONTROL_DB_BYTES_PER_WRITE, "per_recall": CONTROL_DB_BYTES_PER_RECALL},
        "sample": "launch-audit-real-writer-handoff.md sha256 cff5bb74012f404b345240e54acd83b62a8fdaeb3a4ae8cf9e0ed8b818a4ef40: one account, one brain, 1,500 writes, 11 recalls, 3-dimension stub embedder; the recall size is derived from row sizes",
        "growth_bytes_per_month_at_scenario_usage": round(monthly_bytes),
        "sensitivity_by_horizon": horizons,
        "sensitivity_basis": "every month at this scenario's usage, no pruning, and every one of the retained backup sets carrying the full grown size (an upper approximation of a database that grows over the 60-day retention). A sensitivity at sampled row sizes, not a forecast and not a bound.",
        "reason": UNKNOWN_RATES["control_db_lifetime_growth"],
    }


def multipart_sensitivity(snapshot_bytes: float, objects_per_backup: int) -> dict:
    parts = math.ceil(snapshot_bytes / MULTIPART_CHUNK_BYTES)
    extra_per_backup = max(0, parts - objects_per_backup)
    extra_per_month = extra_per_backup * BACKUPS_PER_MONTH
    return {
        "monthly_usd": None,
        "chunk_bytes_assumed": MULTIPART_CHUNK_BYTES,
        "approx_parts_per_backup": parts,
        "extra_put_requests_per_month_beyond_one_per_object": extra_per_month,
        "sensitivity_usd_per_month": round(extra_per_month / 1000 * rate("s3_put_usd_per_1000_requests"), 4),
        "basis": "ceil(snapshot bytes / chunk) parts per backup, less one request per object already counted; KMS requests per part are unverified and not counted; not in any subtotal",
        "reason": UNKNOWN_RATES["s3_multipart_requests"],
    }


def cpu_credit_exposure() -> dict:
    """Peak T4g surplus-credit exposure. A sensitivity, not measured usage, and scenario-independent because one node serves every tenant."""
    hours = rate("hours_per_month")
    price = rate("ec2_t4g_burst_credit_usd_per_vcpu_hour")

    def at(utilization: float) -> dict:
        surplus = T4G_SMALL_VCPUS * max(0.0, utilization - T4G_SMALL_BASELINE_PER_VCPU) * hours
        return {"mean_cpu_utilization": utilization, "surplus_vcpu_hours": round(surplus, 4), "monthly_usd": round(surplus * price, 4)}

    return {
        "kind": "sensitivity_not_measured_usage",
        "instance": {"type": "t4g.small", "vcpus": T4G_SMALL_VCPUS, "baseline_per_vcpu": T4G_SMALL_BASELINE_PER_VCPU, "credits_earned_per_hour": round(T4G_SMALL_VCPUS * T4G_SMALL_BASELINE_PER_VCPU * 60)},
        "hours_per_month": hours,
        "usd_per_surplus_vcpu_hour": price,
        "sustained_100_percent_credits_exhausted": at(PEAK_SUSTAINED_CPU_UTILIZATION),
        "mean_70_percent": at(PEAK_MEAN_CPU_UTILIZATION),
        "mean_case_assumption_in_known_subtotal": {"fraction_of_vcpu_hours_billed_as_surplus": ASSUMED_MEAN_SURPLUS_FRACTION, "applies_when": "usage_fraction >= 1.0 only", "note": "not a peak"},
        "credit_mode": {
            "status": "unverified",
            "template": "deploy/hosted/stack.json sets no CreditSpecification, so the instance takes the account default for T4g",
            "requirement": "An owner confirms unlimited or standard mode. Unlimited bills the surplus above. Standard bills none and throttles the node at baseline, a capacity limit instead of a cost. The model sets neither and changes no infrastructure, threshold or retention.",
        },
    }


def price_scenario(name: str, spec: dict, measurements: dict | None) -> dict:
    """Price one scenario. `measurements` is this scenario's {quantity: record} (see validate_record), or None."""
    measured = _scenario_measurements(name, measurements)
    totals, per_plan = account_totals(spec["mix"], spec["usage_fraction"])
    known = {}

    known["ec2_baseline"] = round(rate("ec2_t4g_small_usd_per_hour") * rate("hours_per_month"), 4)
    surplus_record = measured.get("ec2_surplus_credit_vcpu_hours")
    if surplus_record is not None:
        burst_vcpu_hours = surplus_record["value"]
    else:
        # Mean-case assumption: the full-limit mix's steady 4 req/s workload bills a small share of vCPU-hours as
        # surplus. Light scenarios bill none. This is not a peak; see peak_exposure.
        burst_vcpu_hours = T4G_SMALL_VCPUS * rate("hours_per_month") * ASSUMED_MEAN_SURPLUS_FRACTION if spec["usage_fraction"] >= 1.0 else 0.0
    known["ec2_burst_credits"] = {"value": round(burst_vcpu_hours * rate("ec2_t4g_burst_credit_usd_per_vcpu_hour"), 4), "kind": "measured" if surplus_record else "assumption", **({"measurement_source": surplus_record["source"]} if surplus_record else {"note": "mean-case assumption, not a peak, and only if the account default credit mode is unlimited; see peak_exposure"})}

    known["ebs_fixed"] = round((ROOT_EBS_GB + DATA_EBS_GB) * rate("ebs_gp3_usd_per_gb_month"), 4)
    usable_data_bytes = DATA_EBS_GB * 1_000_000_000 * (1 - DATA_VOLUME_OPERATOR_HEADROOM_FRACTION)
    storage_risk = totals["storage_bytes"] > usable_data_bytes

    known["eip"] = round(rate("eip_usd_per_hour") * rate("hours_per_month"), 4)
    known["kms_key"] = {"value": round(rate("kms_key_usd_per_month"), 4), "kind": "published", "note": "key storage before any rotation; the template enables rotation, so it can reach 3 USD/month; see peak_exposure"}
    known["secrets_manager_storage"] = round(SECRETS_COUNT * rate("secrets_manager_usd_per_secret_month"), 4)
    secrets_calls = SECRETS_COUNT * ASSUMED_RESTARTS_PER_MONTH
    known["secrets_manager_api_calls"] = {"value": round(secrets_calls / 10000 * rate("secrets_manager_usd_per_10000_calls"), 4), "kind": "assumption", "note": f"assumes {ASSUMED_RESTARTS_PER_MONTH} restarts/month reading all {SECRETS_COUNT} secrets at boot (ADR 014); no other Secrets Manager traffic modeled"}
    known["cloudwatch_alarms"] = round(CLOUDWATCH_ALARM_COUNT * rate("cloudwatch_alarm_usd_per_month"), 4)

    # manifest.json + control.db + COMPLETE, plus one bundle per brain. A brain never opened has no .git and no bundle, so the
    # all-allowed count is the conservative ceiling. Light scenarios model each account's default brain only.
    brains_modeled = totals["brains_allowed"] if spec.get("brains") == ALL_ALLOWED_BRAINS else totals["accounts"]
    objects_per_backup = OBJECTS_PER_BACKUP_FIXED + max(brains_modeled, 0)
    snapshot_record = measured.get("snapshot_size_bytes")
    if snapshot_record is not None:
        snapshot_size_bytes = snapshot_record["value"]
    else:
        snapshot_size_bytes = totals["storage_bytes"] + CONTROL_DB_ASSUMED_GB * 1_000_000_000
    snapshot_size_gb = snapshot_size_bytes / 1_000_000_000
    s3_storage_gb_month = snapshot_size_gb * RETAINED_BACKUP_SETS
    known["s3_storage_backups"] = {
        "value": round(s3_storage_gb_month * rate("s3_standard_usd_per_gb_month"), 4),
        "kind": "partly_measured" if snapshot_record else "assumption",
        "measured_inputs": ["snapshot_size_bytes"] if snapshot_record else [],
        "modeled_inputs": ["retained_backup_sets", "objects_per_backup"] + ([] if snapshot_record else ["snapshot_size_bytes"]),
        **({"measurement_source": snapshot_record["source"]} if snapshot_record else {}),
        "snapshot_size_gb": round(snapshot_size_gb, 4),
        "retained_backup_sets": RETAINED_BACKUP_SETS,
        "objects_per_backup": objects_per_backup,
        "brains_in_backup": brains_modeled,
        "allowed_brains_ceiling": totals["brains_allowed"],
        "note": f"backup.sh writes a unique timestamped prefix every run (never overwritten); Expiration(30d)+NoncurrentVersionExpiration(30d) in series gives each object a ~{S3_OBJECT_LIFETIME_DAYS}-day bucket lifetime, so ~{RETAINED_BACKUP_SETS} hourly backup-sets are retained at steady state, the deployed-template baseline -- see docs/launch/hosted-economics.md. The control database is a scenario assumption at snapshot time, not a lifetime bound; see unknown_categories.control_db_lifetime_growth.",
    }
    puts_record = measured.get("s3_put_requests_per_month")
    s3_requests = puts_record["value"] if puts_record else BACKUPS_PER_MONTH * objects_per_backup
    known["s3_requests"] = {"value": round(s3_requests / 1000 * rate("s3_put_usd_per_1000_requests"), 4), "kind": "measured" if puts_record else "assumption", "monthly_put_count": s3_requests, **({"measurement_source": puts_record["source"]} if puts_record else {})}
    kms_billable = max(0, s3_requests - rate("kms_free_requests_per_month"))
    known["kms_requests"] = {"value": round(kms_billable / 10000 * rate("kms_usd_per_10000_requests"), 4), "kind": "assumption", "derived_from": "measured s3_put_requests_per_month" if puts_record else "modeled PUT count", "note": f"SSE-KMS attaches ~1 KMS request per S3 backup PUT (assumed; no bucket key is set in the template); {rate('kms_free_requests_per_month'):.0f}/month global free tier applied before this line, and an account-wide allowance cannot be assumed unused, so peak_exposure uses none of it."}

    avg_recall_response_bytes, avg_write_request_bytes = 4096, 1024
    transfer_gb = (totals["recalls"] * avg_recall_response_bytes + totals["writes"] * avg_write_request_bytes) / 1_000_000_000
    billable_transfer_gb = max(0.0, transfer_gb - rate("data_transfer_out_free_gb_per_month"))
    known["data_transfer_out"] = {"value": round(billable_transfer_gb * rate("data_transfer_out_usd_per_gb"), 4), "kind": "assumption", "note": f"{avg_recall_response_bytes}B/recall response, {avg_write_request_bytes}B/write request assumed; first {rate('data_transfer_out_free_gb_per_month'):.0f} GB/month free applied before this line, and an account-wide allowance cannot be assumed unused, so peak_exposure uses none of it."}

    emails_per_month = totals["accounts"] * ASSUMED_LOGINS_PER_ACCOUNT_PER_MONTH
    emails_per_day = emails_per_month / 30
    within_resend_free = emails_per_month <= rate("resend_free_emails_per_month") and emails_per_day <= rate("resend_free_emails_per_day_cap")
    known["email"] = {
        "value": 0.0 if within_resend_free else None,
        "kind": "assumption",
        "emails_per_month": round(emails_per_month),
        "within_free_tier": within_resend_free,
        "note": f"assumes {ASSUMED_LOGINS_PER_ACCOUNT_PER_MONTH} magic-link emails/account/month, an average day and not a maximum; Resend free tier is {rate('resend_free_emails_per_month'):.0f}/month capped {rate('resend_free_emails_per_day_cap'):.0f}/day" + ("" if within_resend_free else f"; exceeds free tier, Pro tier starts at {rate('resend_pro_usd_per_month_low'):.0f} USD/month -- exact overage price not modeled"),
    }

    known_subtotal = sum(_amount(v) for v in known.values())

    embed = embeddings_tokens(totals)
    unknown = {
        "embeddings": {**embed, "usd_per_1k_tokens": None, "monthly_usd": None, "reason": UNKNOWN_RATES["embeddings_usd_per_1k_tokens"]},
        "cloudwatch_logs": {"priced_usd_per_gb": rate("cloudwatch_logs_ingest_usd_per_gb"), "reason": UNKNOWN_RATES["cloudwatch_logs_volume_gb_per_month"]},
        "cloudwatch_custom_metrics": {"priced_usd_per_metric": rate("cloudwatch_custom_metric_usd_per_month"), "reason": UNKNOWN_RATES["cloudwatch_custom_metrics_count"]},
        "s3_get_list_restore": {"reason": UNKNOWN_RATES["s3_get_list_restore"]},
        "control_db_lifetime_growth": control_db_lifetime_growth(totals),
        "s3_multipart_requests": multipart_sensitivity(snapshot_size_bytes, objects_per_backup),
        "ec2_cpu_credit_mode": {"monthly_usd": None, "reason": UNKNOWN_RATES["ec2_cpu_credit_mode"]},
        "global_free_allowances": {"reason": UNKNOWN_RATES["global_free_allowances"]},
        "email_peak_volume": {"reason": UNKNOWN_RATES["email_peak_volume"]},
    }
    if not within_resend_free:
        unknown["email_overage"] = {"reason": "email volume exceeds Resend's free tier; exact Pro-tier overage price not modeled (range 20-35 USD/month base)"}

    peak = peak_exposure(known, known_subtotal, totals, s3_requests, transfer_gb)

    variable_categories = {"s3_storage_backups", "s3_requests", "kms_requests", "data_transfer_out", "secrets_manager_api_calls"}
    variable_total = sum(_amount(known[c]) for c in variable_categories if c in known)
    per_plan_contribution = {}
    denom = sum(p["storage_bytes"] for p in per_plan.values())
    for plan_id, plan_totals in per_plan.items():
        share = (plan_totals["storage_bytes"] / denom) if denom else 0.0
        per_plan_contribution[plan_id] = {"accounts": plan_totals["accounts"], "storage_share": round(share, 4), "variable_cost_usd": round(variable_total * share, 4)}

    return {
        "scenario": name,
        "assumption": spec["assumption"],
        "measurements_applied": [{"quantity": q, "unit": r["unit"], "value": r["value"], "observed_at": r["observed_at"], "source": r["source"]} for q, r in sorted(measured.items())],
        "account_totals": {k: round(v) for k, v in totals.items()},
        "storage_risk": {"exceeds_usable_data_volume": storage_risk, "usable_data_volume_bytes": round(usable_data_bytes)},
        "known_categories_usd": known,
        "known_subtotal_usd": round(known_subtotal, 2),
        "peak_exposure": peak,
        "per_plan_contribution": {
            "note": "Fixed shared-node infrastructure (EC2, EBS, EIP, KMS key, Secrets Manager storage, CloudWatch alarms) is not allocated per-plan -- one node serves every tenant. Only usage-driven categories are apportioned, by each plan's share of total customer storage bytes.",
            "variable_total_usd": round(variable_total, 4),
            "by_plan": per_plan_contribution,
        },
        "unknown_categories": unknown,
        "within_60usd_ceiling_known_subtotal": known_subtotal <= 60.0,
    }


def peak_exposure(known: dict, known_subtotal: float, totals: dict, s3_requests: float, transfer_gb: float) -> dict:
    """A conservative peak over the known subtotal. It replaces four lines with their worst case and adds nothing else:

    - T4g surplus credits at sustained 100% CPU with the credit balance exhausted (and, separately, 70% mean CPU).
    - KMS key storage after the two billed rotations.
    - KMS requests and data transfer with none of the account-wide free allowance.

    Control-database lifetime growth, embeddings, logs, metrics, restore activity, multipart requests and peak email
    volume are unknown, so this is a peak of the priced categories and not a full maximum."""
    cpu = cpu_credit_exposure()
    kms_key_peak = rate("kms_key_usd_per_month") * (1 + rate("kms_rotation_billed_versions_max"))
    kms_requests_peak = s3_requests / 10000 * rate("kms_usd_per_10000_requests")
    transfer_peak = transfer_gb * rate("data_transfer_out_usd_per_gb")
    replaced = _amount(known["ec2_burst_credits"]) + _amount(known["kms_key"]) + _amount(known["kms_requests"]) + _amount(known["data_transfer_out"])
    base = known_subtotal - replaced + kms_key_peak + kms_requests_peak + transfer_peak
    at_100 = base + cpu["sustained_100_percent_credits_exhausted"]["monthly_usd"]
    at_70 = base + cpu["mean_70_percent"]["monthly_usd"]
    return {
        "kind": "sensitivity_not_forecast",
        "note": "Replaces the mean-case CPU surplus, the pre-rotation KMS key, and the free-allowance offsets on KMS requests and transfer with their worst case. Every unknown category stays out, so this is not a full maximum. It is not a spending recommendation.",
        "peak_known_subtotal_usd_sustained_100_percent_cpu": round(at_100, 2),
        "peak_known_subtotal_usd_mean_70_percent_cpu": round(at_70, 2),
        "components_usd": {
            "ec2_surplus_credits_sustained_100_percent_cpu": cpu["sustained_100_percent_credits_exhausted"]["monthly_usd"],
            "ec2_surplus_credits_mean_70_percent_cpu": cpu["mean_70_percent"]["monthly_usd"],
            "kms_key_storage_after_two_rotations": round(kms_key_peak, 4),
            "kms_requests_no_free_allowance": round(kms_requests_peak, 4),
            "data_transfer_out_no_free_allowance": round(transfer_peak, 4),
        },
        "replaced_known_lines_usd": round(replaced, 4),
        "not_included": ["embeddings", "cloudwatch_logs", "cloudwatch_custom_metrics", "s3_get_list_restore", "control_db_lifetime_growth", "s3_multipart_requests", "email_peak_volume"],
        "full_maximum_computable": False,
        "within_60usd_ceiling_peak_known_subtotal": at_100 <= 60.0,
    }


# --- CLI --------------------------------------------------------------------------------------------------------------

class _LoadError(Exception):
    """A file could not be read as a JSON document. The message is fixed text and never quotes the file."""


def _reject_constant(_: str):
    raise ValueError("non-finite JSON constant")


def _reject_duplicate_keys(pairs: list) -> dict:
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError("duplicate object key")
        out[key] = value
    return out


def _load_json(path: Path, label: str):
    try:
        with open(path, "rb") as f:
            data = f.read(MAX_MEASUREMENTS_FILE_BYTES + 1)
    except FileNotFoundError:
        raise _LoadError(f"{label} not found") from None
    except OSError:
        raise _LoadError(f"{label} could not be read") from None
    if len(data) > MAX_MEASUREMENTS_FILE_BYTES:
        raise _LoadError(f"{label} is larger than {MAX_MEASUREMENTS_FILE_BYTES} bytes")
    try:
        return json.loads(data.decode("utf-8"), parse_constant=_reject_constant, object_pairs_hook=_reject_duplicate_keys)
    except RecursionError:
        raise _LoadError(f"{label} is nested too deeply") from None
    except (UnicodeDecodeError, ValueError):  # JSONDecodeError is a ValueError
        raise _LoadError(f"{label} is not valid JSON (no NaN or Infinity, no repeated key)") from None


def _blocked(reason: str) -> int:
    print(json.dumps({"status": "BLOCKED", "launch_cost_qualification": "NOT_QUALIFIED", "reason": reason}, indent=2))
    return 2


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--measurements", type=Path, default=None, help=f"optional {MEASUREMENT_SCHEMA} v{MEASUREMENT_SCHEMA_VERSION} document; each record binds one quantity to one scenario, unit and source. load.py output is not this document")
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    try:
        manifest = _load_json(args.manifest, "manifest")
    except _LoadError as e:
        return _blocked(str(e))
    if not isinstance(manifest, dict):
        return _blocked("manifest must be a JSON object")
    profile = manifest.get("profile", "paid")
    if not isinstance(profile, str) or not re.fullmatch(r"[a-z]{1,16}", profile):
        return _blocked("manifest.profile must be a short lowercase string")

    measurements: dict[str, dict[str, dict]] = {}
    if args.measurements is not None:
        try:
            measurements = parse_measurements(_load_json(args.measurements, "measurements file"))
        except _LoadError as e:
            return _blocked(str(e))
        except MeasurementError as e:
            return _blocked(f"measurements rejected: {e}")

    try:
        scenarios = {name: price_scenario(name, spec, measurements.get(name)) for name, spec in SCENARIOS.items()}
    except MeasurementError as e:
        return _blocked(f"measurements rejected: {e}")
    applied = [rec for s in scenarios.values() for rec in s["measurements_applied"]]
    blocking = sorted({key for s in scenarios.values() for key in s["peak_exposure"]["not_included"]})
    result = {
        "task_id": "T23.60",
        "profile": profile,
        "calculation": {"status": "CALCULATED", "meaning": "arithmetic over the listed rates, assumptions and validated measurements; it is not a launch cost qualification"},
        "launch_cost_qualification": {
            "status": "NOT_QUALIFIED",
            "full_maximum_computable": False,
            "blocking_unknowns": blocking + ["ec2_cpu_credit_mode", "global_free_allowances"],
            "note": "A calculated subtotal or peak is not an approved spend, a measured bill or a capacity acceptance.",
        },
        "rate_table": RATE_TABLE,
        "rate_receipts": RATE_RECEIPTS,
        "cpu_credit_exposure": cpu_credit_exposure(),
        "unknown_rates": UNKNOWN_RATES,
        "measurements": {
            "supplied": args.measurements is not None,
            "schema": MEASUREMENT_SCHEMA if args.measurements is not None else None,
            "applied_count": len(applied),
            "note": "A record changes only the scenario and quantity it names. A scenario with no record is priced on assumptions, and a measurement is never evidence of capacity acceptance.",
        },
        "scenarios": scenarios,
        "historical_ceiling_usd": 60.0,
        "ceiling_note": "The historical USD 60/month figure is a ceiling, not a budget target or new spend authorization (docs/launch/hosted-plan.md).",
        "citation_note": "Rates marked aws_regional_catalog_primary are read from AWS's own US West (Oregon) price lists, with a receipt file, rate code, source hash and publication date each (docs/launch/evidence/T23.60/aws-rates/, s3-rate.json). Rates marked secondary_summary_not_regionally_verified (S3 request prices, data transfer) and vendor_page_secondary (Resend) still need a regional primary or vendor receipt. Receipts imply no account, credit, free-tier or spend authority. The retention baseline is the deployed template's; any thinning proposal is unapproved and not modeled.",
    }
    try:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(result, indent=2) + "\n")
    except OSError:
        return _blocked("output could not be written")

    worst = max(scenarios.values(), key=lambda s: s["known_subtotal_usd"])
    print(json.dumps({
        "status": "CALCULATED",
        "launch_cost_qualification": "NOT_QUALIFIED",
        "output": str(args.output),
        "measurements_applied": len(applied),
        "idle_usd": scenarios["idle_0_accounts"]["known_subtotal_usd"],
        "full_limit_mix_known_subtotal_usd": scenarios["full_limit_mix"]["known_subtotal_usd"],
        "full_limit_mix_peak_known_subtotal_usd": scenarios["full_limit_mix"]["peak_exposure"]["peak_known_subtotal_usd_sustained_100_percent_cpu"],
        "worst_known_scenario": worst["scenario"],
        "worst_known_subtotal_usd": worst["known_subtotal_usd"],
        "any_scenario_exceeds_60usd_ceiling_on_known_costs_alone": any(not s["within_60usd_ceiling_known_subtotal"] for s in scenarios.values()),
        "full_maximum_computable": False,
    }, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
