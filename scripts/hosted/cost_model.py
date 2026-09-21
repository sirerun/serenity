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
- aws_published_pricing_primary: sourced from an AWS-published pricing rule
  or page, but not a regional catalog unit price. The target account's actual
  eligibility or use may still be unknown.
- secondary_summary_not_regionally_verified: taken from a summary page of a
  worker's search this session and not confirmed against a regional AWS
  catalog; a reviewer must verify it before any spend decision.
- vendor_page_secondary: a non-AWS vendor's published page (Resend).

Units. S3 storage is billed in GiB-months (1 GB = 2^30 bytes, per aws-rates/s3-storage-unit-receipt.json), so snapshot bytes convert
by 2^30. Product quotas stay in decimal bytes, and every other AWS unit keeps the unit its own source gives (UNIT_DEFINITIONS).

Unknowns stay unknown. Provider tokens, restore and reopen frequency, the recall response size, outbound bytes beyond recall
responses, the retry and abort counts, the KMS request ratio per multipart part and control database growth have no measured count and
no proven bound here, and no bound is invented for them. Load-scenario averages are shown as averages, never as caps.

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
_S3_UNIT_RECEIPT = "aws-rates/s3-storage-unit-receipt.json"
_S3_REQUEST_TRANSFER_RECEIPT = "aws-rates/s3-request-transfer-price-receipt.json"
_CLI_RECEIPT = "aws-rates/aws-cli-s3-config-receipt.json"

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
    _S3_UNIT_RECEIPT: {
        "sha256": "7a76ac51729ef1a2173006eb5279ec9a867e7a43813ab5700f6390a957111344",
        "sources": [{
            "name": "S3 pricing page: storage usage is calculated in binary gigabytes, 1 GB = 2^30 bytes",
            "url": "https://aws.amazon.com/s3/pricing/",
            "sha256": "c103717eedfa124da9200d5d0c90700006de7eb35a2137a5b2d900ce8d174b76",
            "retrieved_at": "2026-09-19T04:58:40Z",
        }],
    },
    _S3_REQUEST_TRANSFER_RECEIPT: {
        "sha256": "4136f8badd16f401d967ad917057b19c6b85883c274e0dd99984352e3734f3c9",
        "sources": [
            {"name": "AmazonS3 us-west-2 request prices", "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonS3/20260918174747/us-west-2/index.json", "version": "20260918174747", "publication_date": "2026-09-18T17:47:47Z", "retrieved_at": "2026-09-21T15:48:09Z"},
            {"name": "AWSDataTransfer us-west-2 internet egress prices", "url": "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AWSDataTransfer/20260916132208/us-west-2/index.json", "version": "20260916132208", "publication_date": "2026-09-16T13:22:08Z", "retrieved_at": "2026-09-21T15:48:09Z"},
            {"name": "AWS S3 pricing page: global internet data-transfer allowance", "url": "https://aws.amazon.com/s3/pricing/", "sha256": "745b9af7954e9d9af2346b29c17b67af8bfcb01f29699fa8ff0fcd2508888d7c", "retrieved_at": "2026-09-21T15:48:09Z"},
        ],
    },
    _CLI_RECEIPT: {
        "sha256": "e6329d8de6c62954d54b68630904203ec5aa3d89988492ac27f7a28ca9ff8be6",
        "sources": [{
            "name": "AWS CLI S3 configuration: multipart_threshold and multipart_chunksize default 8MB",
            "url": "https://docs.aws.amazon.com/cli/latest/topic/s3-config.html",
            "sha256": "f5512bc7b9611f8568cfd852e4e8eaca8635f2fe59bc5829d1ddba4bf8ea6529",
            "retrieved_at": "2026-09-19T04:58:40Z",
        }],
    },
}

PRIMARY = "aws_regional_catalog_primary"
PUBLISHED_PRIMARY = "aws_published_pricing_primary"
SECONDARY = "secondary_summary_not_regionally_verified"
VENDOR = "vendor_page_secondary"
VERIFICATIONS = (PRIMARY, PUBLISHED_PRIMARY, SECONDARY, VENDOR)


def _primary(value, receipt, rate_code, catalog_unit, catalog_usd, *, factor=1.0, source, note=None, unit_receipt=None):
    """A rate read from an AWS regional price list. factor converts the catalog price to value.

    unit_receipt names the receipt that defines the catalog unit when the unit is not the plain reading of its label."""
    entry = {
        "value": value, "kind": "published", "verification": PRIMARY,
        "source": source,
        "receipt": {"file": receipt, "rate_code": rate_code, "catalog_unit": catalog_unit, "catalog_usd": catalog_usd, "catalog_to_rate_factor": factor},
    }
    if unit_receipt:
        entry["unit_receipt"] = unit_receipt
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
    "s3_standard_usd_per_gib_month": _primary(
        0.023, _S3_RECEIPT, "sku:Z3FQZG73HYSPVABR@first-50-TB", "GB-Mo", "0.0230000000", source="AWS AmazonS3 us-west-2 price list, published 2026-09-18T17:47:47Z, first 50 TB tier",
        note="First 50 TB tier. The earlier claim of 0.0265 for Oregon was incorrect. The catalog unit GB-Mo is a binary gigabyte: S3 bills storage in units of 2^30 bytes (aws-rates/s3-storage-unit-receipt.json), so this rate applies to GiB-months and the billed quantity is bytes / 2^30, never bytes / 10^9.",
        unit_receipt=_S3_UNIT_RECEIPT,
    ),
    "s3_put_usd_per_1000_requests": _primary(
        0.005, _S3_REQUEST_TRANSFER_RECEIPT, "D4PMUVH6F64HK2D6.JRTCKXETXF.6YS6EN2CT7", "Requests", "0.0000050000", factor=1000,
        source="AmazonS3 us-west-2 Price List version 20260918174747, USW2-Requests-Tier1",
        note="PUT/COPY/POST/LIST request rate; the catalog charges per request and the model shows per 1,000. No request count is proven by this rate receipt.",
    ),
    "s3_get_usd_per_1000_requests": _primary(
        0.0004, _S3_REQUEST_TRANSFER_RECEIPT, "E77AQEM2DC4VV3FC.JRTCKXETXF.6YS6EN2CT7", "Requests", "0.0000004000", factor=1000,
        source="AmazonS3 us-west-2 Price List version 20260918174747, USW2-Requests-Tier2",
        note="GET/other request rate; no routine GET count is modeled.",
    ),
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
    "data_transfer_out_usd_per_gb": _primary(
        0.09, _S3_REQUEST_TRANSFER_RECEIPT, "5M4327XEUKBBTWAT.JRTCKXETXF.Q3Z75P77EN", "GB", "0.0900000000",
        source="AWSDataTransfer us-west-2 Price List version 20260916132208, first 10 TB egress tier beyond global free tier",
        note="Price List unit is GB. The model's bytes-to-GB conversion remains an explicit decimal-GB assumption and is not a measured egress quantity.",
    ),
    "data_transfer_out_free_gb_per_month": {
        "value": 100, "kind": "published", "verification": PUBLISHED_PRIMARY,
        "source": "AWS S3 pricing page (hashed in aws-rates/s3-request-transfer-price-receipt.json)",
        "note": "The published first-100-GB monthly internet-egress allowance is aggregated across AWS services and Regions except China and GovCloud. Actual target-account availability/consumption is unverified; the known subtotal applies it and the peak exposure uses none.",
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
    "embeddings_usd_per_1k_tokens": "pending task42 provider/serving-provider pin (perplexity/pplx-embed-v1-0.6b terms are unqualified per docs/launch/hosted-plan.md); the provider's tokenizer is unqualified too, so provider-billable tokens have no measured count and no proven bound",
    "embedding_provider_tokens": "the gateway meters cl100k_base tokens of each remembered fact against Plan.InputTokens. The candidate provider's own tokenizer is unqualified, so the service counter is not a bound on provider-billable tokens, an average token proxy is not a bound either, and no provider-token count is claimed. Recalls, readiness probes and restore re-embeds are not metered by the gateway",
    "embedding_restore_and_reopen": "a restored brain has no vectors (the derived index is not in the Git bundle), and every cold open runs RecoverMemorySearch, which embeds each eligible unexpired fact lacking a vector, one provider call per fact. The number of restore events, cold reopens (max_open is 8 against up to 29 brains) and failed write-time embeds is request-driven and unmeasured, so neither a steady-state missing-vector rate nor a monthly restore count exists. Per event, the call count is at most the brain's stored eligible facts; the tokens and the embedded text bytes are unproven",
    "readiness_probe_actual_calls": "the readiness embed cadence is derived from Caddy health_interval 30s and the service's one-minute cache (60 to 90 second spacing), not measured. Failed probes, the router's retries (up to 3 provider attempts) and probes from other callers are unmeasured, and only successful embeds are ledgered",
    "cloudwatch_logs_volume_gb_per_month": "rate is priced (cloudwatch_logs_ingest_usd_per_gb) but no log shipping is wired yet (ops go through SSM Run Command per ADR 014); volume unknown pending T23.53 telemetry",
    "cloudwatch_custom_metrics_count": "rate is priced (cloudwatch_custom_metric_usd_per_month) but T23.53's metric cardinality is not yet implemented",
    "s3_get_list_restore": "no routine GET/List/restore is modeled (no scheduled integrity check or restore drill in the current design); a restore drill or T23.50 recovery testing adds GET/List cost on top of this baseline, not included here",
    "control_db_lifetime_growth": "control.db is copied whole into every snapshot and no runtime code prunes committed reservations or audit_log rows, so it grows with lifetime writes, recalls, readiness embeds and restore re-embeds. The 20 MB constant is a scenario assumption at snapshot time, not a lifetime bound. Each scenario reports named, non-overlapping components at sampled row sizes over stated horizons, which is a sensitivity and not an upper bound. Actual growth needs a database allocated-size (dbstat) measurement over a bounded window, and the provider ledger cannot attribute rows to write, query, readiness or rebuild without instrumentation",
    "ec2_cpu_credit_mode": "the template sets no CreditSpecification, so a t4g.small uses the account default for T4g, which is unverified (AWS's default is unlimited). Unlimited mode bills surplus credits; standard mode bills none and throttles at baseline. An owner must confirm the mode; the model sets neither",
    "s3_multipart_requests": "the request count is modeled from a per-object-size assumption at the AWS CLI defaults (8 MiB threshold and chunk, classic transfer client): one request for an object below the threshold, otherwise a create, one request per part and a complete. The deployed CLI version, its configuration and transfer client, and the real object-size inventory are not recorded; retries, aborted uploads, list or head checks are not counted; and the number of KMS requests a multipart object makes is not established, so the KMS line keeps an explicit one-per-object assumption",
    "egress_response_size": "the recall response size is unbounded by the current plan: limit accepts any nonnegative integer, the facts arm can return every visible fact, and response bytes are not metered or capped. The 4,096 bytes per recall is an assumed average, not measured and not an enforced size, and a limit times 4,096 product is not a hard bound either (JSON escaping, metadata, result-arm duplication and HTTP compression change the bytes). No exposure is claimed beyond the average model. The lane implements no runtime cap",
    "outbound_bytes_unmodeled": "the egress line counts recall response bytes only. Outbound bytes from the server that are not modeled: write and other tool responses, embedding request text sent to the provider, email API calls, dashboard responses and AWS API traffic; no measurement exists. Inbound write request bytes are not outbound egress and are not counted",
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
CONTROL_DB_ASSUMED_BYTES = 20_000_000  # scenario assumption at snapshot time (20 MB, decimal); NOT a lifetime bound (see CONTROL_DB_*).

# Units. Each quantity in this file has one unit, named where it is used:
# - Product quotas (internal/hosted/plans/plans.go) and every *_bytes value are decimal bytes.
# - The S3 storage bill is in GiB-months. AWS states that S3 storage usage is calculated in binary gigabytes, where 1 GB is
#   2^30 bytes (aws-rates/s3-storage-unit-receipt.json), so bytes convert to the billed quantity by 2^30, never by 10^9.
# - The unit statement above is for S3 storage only. EBS volume size, gp3 throughput, request counts and data transfer each
#   keep the unit their own source gives (see UNIT_DEFINITIONS); none is reinterpreted from it.
BYTES_PER_GIB = 2**30
BYTES_PER_DECIMAL_GB = 10**9
UNIT_DEFINITIONS = {
    "plan_quotas": {"unit": "counts, cl100k_base tokens and decimal bytes", "source": "internal/hosted/plans/plans.go", "note": "kept in bytes; never converted to GiB for a quota comparison"},
    "s3_storage_billed": {"unit": "GiB-month", "definition": "1 GB = 2^30 bytes", "source": _S3_UNIT_RECEIPT, "note": "the only unit this pass re-based"},
    "s3_requests": {"unit": "requests; the catalog price is per request and converts to per 1,000", "source": _S3_REQUEST_TRANSFER_RECEIPT},
    "kms_and_secrets_requests": {"unit": "requests; the catalog price is per request and converts to per 10,000", "source": _REGIONAL_RECEIPT},
    "ebs_volume_size": {"unit": "configured volume size as written in deploy/hosted/stack.json", "note": "priced as written; not re-based from the S3 statement. The 30 read as decimal GB in the storage-risk check is conservative for a volume configured in GiB"},
    "ebs_gp3_throughput": {"unit": "catalog GiBps-month converted to MiBps-month by 1/1024", "source": _EC2_EBS_RECEIPT},
    "data_transfer": {"unit": "GB per month, decimal assumed", "note": "AWS's data-transfer GB is not sourced in this pass, so the decimal reading is an assumption; a binary reading would change the billed GB by at most 7.4%"},
    "readiness_embeds": {"unit": "successful embeds per month", "note": "counted on the 30-day month, with the 730-hour month beside it"},
}

# AWS CLI defaults for `aws s3 cp` (aws-rates/aws-cli-s3-config-receipt.json): an object of at least the threshold uploads in parts.
# The deployed host's CLI version, configuration and transfer client are not recorded, so these are modeled defaults. The page does
# not say whether the MB suffix is 10^6 or 2^20 bytes; 8 MiB is used and 8,000,000 bytes changes a part count by at most 4.9%.
MULTIPART_THRESHOLD_BYTES = 8 * 1024 * 1024
MULTIPART_CHUNK_BYTES = 8 * 1024 * 1024
MANIFEST_ASSUMED_BYTES = 4096  # backup manifest.json and COMPLETE marker: far below the multipart threshold; exact size unrecorded.
# KMS requests per object written to an SSE-KMS bucket with no bucket key (the template sets none). A stated assumption: one data-key
# request per object. AWS documents the KMS operations but not a one-per-part count for multipart uploads, so the per-part ratio stays unknown.
ASSUMED_KMS_REQUESTS_PER_OBJECT = 1

# Sampled control.db row sizes from the real-writer audit (one account, one brain, 1,500 writes and 11 recalls,
# a 3-dimension stub embedder), launch-audit-real-writer-handoff.md sha256
# cff5bb74012f404b345240e54acd83b62a8fdaeb3a4ae8cf9e0ed8b818a4ef40: about 301 B per reservation row and 229 B
# per audit row including indexes; a write adds 2 of each and a recall 1 of each. That sample already contains every audit row its
# write and recall paths produced, provider-ledger rows included, so its per-write and per-recall slopes are never added to a second
# provider-row slope for the same calls. The sample reports no /readyz call and no restore, so those rows are named components of their
# own. The recall figure is derived from row sizes, not measured, and a real provider's audit rows may be larger. A sample, not a bound.
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

# Load-scenario averages from workload.json. They are proxies in tokens of no specific tokenizer, never a cap, never a bound and never
# provider-billable tokens: the gateway meters cl100k_base tokens and the candidate provider's tokenizer is unqualified.
AVG_QUERY_TOKENS = 74  # midpoint of workload.json query_tokens 20-128; a load-scenario average, not a service cap.
AVG_FACT_TOKENS = 272  # midpoint of workload.json fact_tokens 32-512.
AVG_READINESS_PROBE_TOKENS = 10  # proxy for the fixed probe string below; not a tokenizer count.
AVG_RECALL_RESPONSE_BYTES = 4096  # assumed average, not measured and not enforced (recall limit is uncapped). Recall responses only.

# Readiness embed cadence, from source and not measured. deploy/hosted/Caddyfile probes /readyz every health_interval, and
# internal/hosted/service/service.go recomputes (one paid provider embed of READINESS_PROBE_TEXT) only when the cached answer is older
# than one minute. A recompute therefore needs an age above 60 s, so recomputes are at least 60 s apart, and Caddy's 30 s tick makes the
# usual spacing 90 s. No EC2 CloudWatch alarm calls /readyz (stack.json's two alarms read CPUUtilization and StatusCheckFailed).
CADDY_HEALTH_INTERVAL_SECONDS = 30
READINESS_CACHE_SECONDS = 60
READINESS_PROBE_TEXT = "Serenity readiness probe"
SECONDS_PER_MONTH = {"30_day_month": 30 * 24 * 3600, "730_hour_month": 730 * 3600}
EVENT_MONTH = "30_day_month"  # backups (720 a month) and every event count in this file use the 30-day month; hourly-priced lines use 730 h.
ROUTER_RETRY_ATTEMPTS = 3  # internal/router/retry.go defaultRetryAttempts: provider attempts per Complete, only a success is ledgered.
MAX_FRAME_BYTES = 1 << 20  # internal/server/mcp MaxFrameBytes: the request-frame cap that bounds a query's text.
ASSUMED_RESTARTS_PER_MONTH = 5  # deploys/rehearsals; drives Secrets Manager API call volume.
ASSUMED_LOGINS_PER_ACCOUNT_PER_MONTH = 10  # magic-link signup/login emails; documented assumption.


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
    """Plan entitlement totals for a mix at a usage fraction.

    Units: recalls and writes are counts, input_tokens are cl100k_base tokens (the gateway's meter), storage_bytes are decimal bytes,
    memories and brains_allowed are counts. memories is entitlement times the usage fraction, an assumption below usage 1.0."""
    totals = {"accounts": 0, "brains_allowed": 0, "memories": 0.0, "recalls": 0.0, "writes": 0.0, "input_tokens": 0.0, "storage_bytes": 0.0}
    per_plan = {}
    for plan_id, count in mix.items():
        allowance = PLAN_ALLOWANCES[plan_id]
        plan_totals = {
            "accounts": count,
            "brains_allowed": count * allowance["brains"],
            "memories": count * allowance["memories"] * usage_fraction,
            "recalls": count * allowance["recalls"] * usage_fraction,
            "writes": count * allowance["writes"] * usage_fraction,
            "input_tokens": count * allowance["input_tokens"] * usage_fraction,
            "storage_bytes": count * allowance["storage_bytes"] * usage_fraction,
        }
        per_plan[plan_id] = plan_totals
        for k in totals:
            totals[k] += plan_totals[k]
    return totals, per_plan


def readiness_cadence() -> dict:
    """Readiness embeds a month, derived from the Caddy interval and the service cache. Never measured.

    The handler recomputes only when the cached answer is older than READINESS_CACHE_SECONDS, so recomputes are at least that far apart
    (the ceiling for one service instance, whoever calls). With a Caddy tick every CADDY_HEALTH_INTERVAL_SECONDS the usual spacing is the
    cache age plus one tick. Both counts are given on the 30-day and the 730-hour month; the model's event month is EVENT_MONTH."""
    fastest = READINESS_CACHE_SECONDS
    typical = READINESS_CACHE_SECONDS + CADDY_HEALTH_INTERVAL_SECONDS
    per_month = {basis: {"low": seconds // typical, "high": seconds // fastest} for basis, seconds in SECONDS_PER_MONTH.items()}
    return {
        "kind": "cadence_range_derived_from_source_not_measured",
        "spacing_seconds": {"fastest_handler_floor": fastest, "typical_with_caddy_tick": typical},
        "sources": {
            "caddy": "deploy/hosted/Caddyfile health_uri /readyz, health_interval 30s",
            "handler": "internal/hosted/service/service.go readiness(): recompute, with one paid embed of the probe text, only when the cache is older than one minute",
            "not_a_source": "the two EC2 CloudWatch alarms in deploy/hosted/stack.json read CPUUtilization and StatusCheckFailed and never call /readyz",
        },
        "successful_embeds_per_month": per_month,
        "event_month_basis": EVENT_MONTH,
        "low_is": "Caddy-driven typical spacing; the high is the handler's floor and applies to a single running service instance",
        "not_counted": f"failed probes (not ledgered) and the router's retries (up to {ROUTER_RETRY_ATTEMPTS} provider attempts per embed); actual call counts are unmeasured",
        "probe_text_bytes": len(READINESS_PROBE_TEXT.encode("utf-8")),
    }


def embedding_workload(totals: dict) -> dict:
    """Embedding work by unit. No provider token count and no dollar figure exists, and none is invented.

    Three units are kept apart: the gateway's service counter (cl100k_base tokens), provider calls (counts), and provider tokens
    (unknown: the provider tokenizer is unqualified). Load-scenario averages are shown only as proxies, never as a bound."""
    cadence = readiness_cadence()
    readiness = cadence["successful_embeds_per_month"][EVENT_MONTH]
    return {
        "monthly_usd": None,
        "usd_per_1k_tokens": None,
        "reason": UNKNOWN_RATES["embeddings_usd_per_1k_tokens"],
        "units": {
            "service_counter": "cl100k_base tokens the gateway counts on each remembered fact against Plan.InputTokens",
            "provider_calls": "successful embed calls a month; a count, not tokens",
            "provider_tokens": "tokens under the provider's own tokenizer, which is unqualified: unknown",
            "average_proxy_tokens": "load-scenario averages times counts, in tokens of no specific tokenizer",
        },
        "service_counter": {
            "write_input_tokens_at_entitlement_x_usage": {
                "value": round(totals["input_tokens"]), "unit": "cl100k_base_tokens_per_month",
                "kind": "plan entitlement times the scenario's usage fraction; a gateway cap on remembers only at usage 1.0",
                "not_metered": "recalls, readiness probes and restore or reopen re-embeds",
            },
        },
        "provider_calls": {
            "write_embeds": {"value": round(totals["writes"]), "unit": "calls_per_month", "kind": "plan write entitlement times usage fraction; one embed per remember when the index is present; retried and failed attempts are not counted"},
            "query_embeds": {"value": round(totals["recalls"]), "unit": "calls_per_month", "kind": "plan recall entitlement times usage fraction; assumes one query embedding per recall (recall runs search.Search once when an index is present), not measured"},
            "readiness_embeds": {**cadence, "unit": "calls_per_month", "event_month_range": readiness},
        },
        "provider_tokens": {"value": None, "unit": "provider_tokens_per_month", "reason": UNKNOWN_RATES["embedding_provider_tokens"]},
        "average_proxy_tokens": {
            "kind": "average proxy, not provider-billable tokens, not a cap and not an upper bound",
            "avg_fact_tokens": AVG_FACT_TOKENS,
            "avg_query_tokens": AVG_QUERY_TOKENS,
            "avg_readiness_probe_tokens": AVG_READINESS_PROBE_TOKENS,
            "write_proxy_tokens": round(totals["writes"] * AVG_FACT_TOKENS),
            "query_proxy_tokens": round(totals["recalls"] * AVG_QUERY_TOKENS),
            "readiness_proxy_tokens": {"low": readiness["low"] * AVG_READINESS_PROBE_TOKENS, "high": readiness["high"] * AVG_READINESS_PROBE_TOKENS},
            "query_mean_note": "74 is the load scenario's query-token average, not a service cap",
        },
        "text_bytes_handed_to_the_embedder": {
            "kind": "proven only where stated; the provider request's serialization and tokenizer output are not bounded by these",
            "readiness_probe_bytes_per_call": cadence["probe_text_bytes"],
            "query_bytes_per_call_max": MAX_FRAME_BYTES,
            "query_bytes_note": "the request frame caps the whole request at 1 MiB, so the query text cannot exceed it",
            "write_and_reembed_bytes": "not proven: the embedder receives the indexed chunk text, which is not shown to equal the fact's bytes",
        },
    }


def embedding_restore_and_reopen(totals: dict) -> dict:
    """Missing-vector work, in two separate kinds, at counts and never as tokens."""
    return {
        "monthly_usd": None,
        "unit": "calls",
        "restore_full_reembed": {
            "kind": "per_event_count_bound_only",
            "reason": "the Git bundle excludes the derived index, so a restored brain has no vectors and RecoverMemorySearch may call the provider once for every eligible unexpired fact",
            "stored_memories_at_scenario_usage": round(totals["memories"]),
            "provider_calls_per_event_max": round(totals["memories"]),
            "provider_calls_per_event_max_basis": "plan memory entitlement times usage fraction, summed over accounts; memories are account-wide, and an enforced write cap only at usage 1.0",
            "provider_tokens_per_event": None,
            "events_per_month": None,
            "monthly_provider_calls": None,
        },
        "steady_state_missing_vector_retry": {
            "kind": "unknown_rate",
            "reason": "outside a restore a vector is missing only after a failed write-time embed, and every cold reopen retries it. The failure rate and the reopen frequency (max_open 8 against up to 29 brains) are unmeasured",
            "missing_vector_rate": None,
            "reopens_per_month": None,
            "monthly_provider_calls": None,
        },
        "not_governed_by": "Plan.InputTokens: the account input cap does not meter restore, reopen, readiness or query embeds",
        "operational_note": "a restore's embeds run one at a time while the brain pool lock is held, so time to ready scales with facts times provider latency (unmeasured)",
        "reason": UNKNOWN_RATES["embedding_restore_and_reopen"],
    }


def control_db_lifetime_growth(totals: dict) -> dict:
    """Sensitivity of the snapshot's control.db to lifetime growth, in named components that do not overlap. Not an upper bound.

    - write_and_recall_paths: the real-writer sample's slopes, which already include the provider-ledger rows those calls wrote.
    - readiness_probe_rows: one provider-ledger audit row per successful readiness embed; the sample reports no /readyz call.
    - restore_reembed_rows: one provider-ledger row per re-embedded fact; events per month are unknown, so it is a per-event figure.
    Growth is billed in retained backup sets at GiB (2^30 bytes) and every retained set carrying the full grown size is an upper
    approximation of a database that grows over the retention."""
    cadence = readiness_cadence()["successful_embeds_per_month"][EVENT_MONTH]
    audit_bytes = CONTROL_DB_ROW_BYTES["audit"]
    write_recall_bytes = totals["writes"] * CONTROL_DB_BYTES_PER_WRITE + totals["recalls"] * CONTROL_DB_BYTES_PER_RECALL
    readiness_bytes = {"low": cadence["low"] * audit_bytes, "high": cadence["high"] * audit_bytes}
    monthly_bytes = {k: write_recall_bytes + readiness_bytes[k] for k in ("low", "high")}
    price = rate("s3_standard_usd_per_gib_month")
    horizons = []
    for months in CONTROL_DB_HORIZONS_MONTHS:
        entry = {"months": months, "control_db_bytes": {}, "control_db_gib": {}, "added_s3_backup_storage_usd_per_month": {}}
        for k in ("low", "high"):
            grown = months * monthly_bytes[k]
            entry["control_db_bytes"][k] = round(CONTROL_DB_ASSUMED_BYTES + grown)
            entry["control_db_gib"][k] = round((CONTROL_DB_ASSUMED_BYTES + grown) / BYTES_PER_GIB, 6)
            entry["added_s3_backup_storage_usd_per_month"][k] = round(grown / BYTES_PER_GIB * RETAINED_BACKUP_SETS * price, 2)
        horizons.append(entry)
    return {
        "status": "unbounded_in_current_runtime",
        "monthly_usd": None,
        "assumed_control_db_bytes_in_snapshot": CONTROL_DB_ASSUMED_BYTES,
        "assumed_control_db_gib_in_snapshot": round(CONTROL_DB_ASSUMED_BYTES / BYTES_PER_GIB, 6),
        "assumed_control_db_kind": "scenario assumption at snapshot time, never a lifetime bound",
        "components": {
            "write_and_recall_paths": {
                "bytes_per_write": CONTROL_DB_BYTES_PER_WRITE, "bytes_per_recall": CONTROL_DB_BYTES_PER_RECALL,
                "monthly_bytes": round(write_recall_bytes),
                "kind": "sampled slope; includes the provider-ledger rows the sample's own write and recall paths wrote, so no second provider slope is added for them",
            },
            "readiness_probe_rows": {
                "rows_per_month": dict(cadence), "bytes_per_row": audit_bytes,
                "monthly_bytes": {k: round(v) for k, v in readiness_bytes.items()},
                "kind": "one provider-ledger audit row per successful readiness embed, at the sampled audit-row size; the sample is 1,500 writes and 11 recalls and reports no readiness call, so these rows are not in the write and recall slope. A real provider's row may be larger",
            },
            "restore_reembed_rows": {
                "rows_per_event_max": round(totals["memories"]), "bytes_per_row": audit_bytes,
                "bytes_per_event_max": round(totals["memories"] * audit_bytes),
                "events_per_month": None, "monthly_bytes": None,
                "kind": "one provider-ledger row per re-embedded fact; unknown events, so not in any horizon below",
            },
        },
        "sampled_row_bytes": {**CONTROL_DB_ROW_BYTES, "per_write": CONTROL_DB_BYTES_PER_WRITE, "per_recall": CONTROL_DB_BYTES_PER_RECALL},
        "sample": "launch-audit-real-writer-handoff.md sha256 cff5bb74012f404b345240e54acd83b62a8fdaeb3a4ae8cf9e0ed8b818a4ef40: one account, one brain, 1,500 writes, 11 recalls, 3-dimension stub embedder; the recall size is derived from row sizes",
        "growth_bytes_per_month_at_scenario_usage": {k: round(v) for k, v in monthly_bytes.items()},
        "sensitivity_by_horizon": horizons,
        "sensitivity_basis": "write and recall slope plus readiness rows, every month at this scenario's usage, no pruning, and every one of the retained backup sets carrying the full grown size, billed at GiB. A sensitivity at sampled row sizes, not a forecast and not a bound. Restore re-embed rows are excluded because their frequency is unknown.",
        "measurement_required": {
            "what": "control.db allocated size (dbstat page bytes and the file size) at the start and end of a bounded window, bound to the scenario, the window and the source",
            "attribution": "the provider ledger rows carry only task class embedding, so write, query, readiness and rebuild cannot be told apart without instrumentation; failed or retried billable attempts may not be ledgered at all",
            "accepted_by_this_model_today": False,
            "why_not_accepted": "no producer can attest the database's provenance, so a field for it is not added to the measurement schema",
        },
        "reason": UNKNOWN_RATES["control_db_lifetime_growth"],
    }


def _objects_requests(size_bytes: float, count: int) -> dict:
    """S3 requests to upload `count` objects of one size with the AWS CLI's classic transfer client at its defaults."""
    if size_bytes < MULTIPART_THRESHOLD_BYTES:
        return {"objects": count, "multipart_objects": 0, "parts": 0, "requests": count}
    parts = math.ceil(size_bytes / MULTIPART_CHUNK_BYTES)
    return {"objects": count, "multipart_objects": count, "parts": count * parts, "requests": count * (2 + parts)}


def modeled_backup_objects(spec: dict, snapshot_bytes: float | None, brains_modeled: int) -> list[tuple[str, float, int]]:
    """(name, bytes per object, object count) for one modeled backup. Sizes are an assumption, not an inventory.

    manifest.json and COMPLETE are tiny, control.db is the scenario assumption, and every brain bundle is an even share of the data
    bytes (the account's storage quota bytes stand in for bundle bytes, as they do for the snapshot size). A measured snapshot size
    replaces the modeled data bytes and is split evenly over the modeled bundles."""
    objects = [("manifest.json", MANIFEST_ASSUMED_BYTES, 1), ("COMPLETE", MANIFEST_ASSUMED_BYTES, 1), ("control.db", CONTROL_DB_ASSUMED_BYTES, 1)]
    if snapshot_bytes is not None:
        if brains_modeled > 0:
            objects.append(("brain bundle (even share of the measured snapshot)", max(snapshot_bytes - CONTROL_DB_ASSUMED_BYTES, 0) / brains_modeled, brains_modeled))
        return objects
    for plan_id, count in spec["mix"].items():
        allowance = PLAN_ALLOWANCES[plan_id]
        per_account = allowance["brains"] if spec.get("brains") == ALL_ALLOWED_BRAINS else 1
        if count and per_account:
            objects.append((f"{plan_id} brain bundle", allowance["storage_bytes"] * spec["usage_fraction"] / per_account, count * per_account))
    return objects


def s3_backup_requests(objects: list[tuple[str, float, int]]) -> dict:
    total = {"objects": 0, "multipart_objects": 0, "parts": 0, "requests": 0}
    detail = []
    for name, size, count in objects:
        r = _objects_requests(size, count)
        for k in total:
            total[k] += r[k]
        detail.append({"object": name, "bytes_each": round(size), "count": count, **r})
    return {"per_backup": total, "detail": detail}


def s3_request_unknowns(s3_requests: float, objects_per_month: float) -> dict:
    """The inputs of the S3 and KMS request lines that stay unmeasured. Nothing here is in a subtotal or a peak."""
    price = rate("kms_usd_per_10000_requests")
    return {
        "monthly_usd": None,
        "modeled_in_known_line": "s3_requests, from a per-object-size assumption at the AWS CLI defaults",
        "defaults_source": _CLI_RECEIPT,
        "unmeasured_inputs": [
            "deployed AWS CLI version, configuration values and transfer client (preferred_transfer_client defaults to auto, which resolves to classic on this instance type per the same page)",
            "real per-object size inventory of a backup",
            "retries",
            "aborted multipart uploads (the template's lifecycle aborts incomplete uploads after 1 day; the count is unknown)",
            "list or head checks",
            "KMS requests per multipart part",
        ],
        "kms_ratio": {
            "assumed_per_object": ASSUMED_KMS_REQUESTS_PER_OBJECT,
            "status": "explicit assumption; AWS documents the KMS operations but not a one-per-part count, so it is not derived from the S3 request count",
            "sensitivity_if_one_kms_request_per_s3_request": {
                "kms_requests_per_month": round(s3_requests),
                "usd_per_month_no_free_allowance": round(s3_requests / 10000 * price, 4),
                "usd_per_month_with_global_free_tier": round(max(0.0, s3_requests - rate("kms_free_requests_per_month")) / 10000 * price, 4),
                "kind": "unverified sensitivity, not in any subtotal or peak",
            },
            "modeled_kms_requests_per_month": round(objects_per_month * ASSUMED_KMS_REQUESTS_PER_OBJECT),
        },
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
    usable_data_bytes = DATA_EBS_GB * BYTES_PER_DECIMAL_GB * (1 - DATA_VOLUME_OPERATOR_HEADROOM_FRACTION)  # the configured 30 read as decimal GB: conservative, see UNIT_DEFINITIONS.
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
        snapshot_size_bytes = totals["storage_bytes"] + CONTROL_DB_ASSUMED_BYTES
    snapshot_size_gib = snapshot_size_bytes / BYTES_PER_GIB
    s3_storage_gib_months = snapshot_size_gib * RETAINED_BACKUP_SETS
    known["s3_storage_backups"] = {
        "value": round(s3_storage_gib_months * rate("s3_standard_usd_per_gib_month"), 4),
        "kind": "partly_measured" if snapshot_record else "assumption",
        "measured_inputs": ["snapshot_size_bytes"] if snapshot_record else [],
        "modeled_inputs": ["retained_backup_sets", "objects_per_backup"] + ([] if snapshot_record else ["snapshot_size_bytes"]),
        **({"measurement_source": snapshot_record["source"]} if snapshot_record else {}),
        "snapshot_size_bytes": round(snapshot_size_bytes),
        "snapshot_size_gib": round(snapshot_size_gib, 6),
        "billed_quantity": {"value": round(s3_storage_gib_months, 4), "unit": "GiB-month", "conversion": "snapshot bytes / 2^30 times retained backup sets; S3 GB = 2^30 bytes", "unit_receipt": _S3_UNIT_RECEIPT},
        "retained_backup_sets": RETAINED_BACKUP_SETS,
        "objects_per_backup": objects_per_backup,
        "brains_in_backup": brains_modeled,
        "allowed_brains_ceiling": totals["brains_allowed"],
        "note": f"backup.sh writes a unique timestamped prefix every run (never overwritten); Expiration(30d)+NoncurrentVersionExpiration(30d) in series gives each object a ~{S3_OBJECT_LIFETIME_DAYS}-day bucket lifetime, so ~{RETAINED_BACKUP_SETS} hourly backup-sets are retained at steady state, the deployed-template baseline -- see docs/launch/hosted-economics.md. The control database is a scenario assumption at snapshot time, not a lifetime bound; see unknown_categories.control_db_lifetime_growth.",
    }
    puts_record = measured.get("s3_put_requests_per_month")
    request_model = s3_backup_requests(modeled_backup_objects(spec, snapshot_record["value"] if snapshot_record else None, brains_modeled))
    modeled_requests = BACKUPS_PER_MONTH * request_model["per_backup"]["requests"]
    s3_requests = puts_record["value"] if puts_record else modeled_requests
    objects_per_month = BACKUPS_PER_MONTH * request_model["per_backup"]["objects"]
    known["s3_requests"] = {
        "value": round(s3_requests / 1000 * rate("s3_put_usd_per_1000_requests"), 4),
        "kind": "measured" if puts_record else "assumption",
        "monthly_put_count": s3_requests,
        "unit": "requests_per_month",
        **({"measurement_source": puts_record["source"]} if puts_record else {}),
        "modeled_requests_per_month": modeled_requests,
        "per_backup": request_model["per_backup"],
        "per_object_size_assumption": request_model["detail"],
        "basis": f"one request per object below {MULTIPART_THRESHOLD_BYTES} bytes, otherwise a create, one request per {MULTIPART_CHUNK_BYTES}-byte part and a complete (AWS CLI classic client defaults); sizes are an assumption, not an inventory; retries, aborts and list checks are unknown; see unknown_categories.s3_multipart_requests",
    }
    kms_requests = objects_per_month * ASSUMED_KMS_REQUESTS_PER_OBJECT
    kms_billable = max(0, kms_requests - rate("kms_free_requests_per_month"))
    known["kms_requests"] = {
        "value": round(kms_billable / 10000 * rate("kms_usd_per_10000_requests"), 4),
        "kind": "assumption",
        "unit": "requests_per_month",
        "monthly_kms_requests": kms_requests,
        "derived_from": f"modeled objects a month times {ASSUMED_KMS_REQUESTS_PER_OBJECT} KMS request per object; a measured S3 request count does not measure KMS calls",
        "note": f"SSE-KMS attaches an assumed {ASSUMED_KMS_REQUESTS_PER_OBJECT} KMS request per object written (no bucket key is set in the template); the per-part count for multipart objects is unknown and is not set equal to the S3 request count; {rate('kms_free_requests_per_month'):.0f}/month global free tier applied before this line, and an account-wide allowance cannot be assumed unused, so peak_exposure uses none of it.",
    }

    # Egress. Recall responses only, at an assumed average that nothing enforces: recall's limit is uncapped and responses are not metered.
    # Inbound write request bytes are not egress. Decimal GB is assumed for the transfer unit; AWS's data-transfer GB is not sourced here.
    transfer_gb = totals["recalls"] * AVG_RECALL_RESPONSE_BYTES / BYTES_PER_DECIMAL_GB
    billable_transfer_gb = max(0.0, transfer_gb - rate("data_transfer_out_free_gb_per_month"))
    known["data_transfer_out"] = {
        "value": round(billable_transfer_gb * rate("data_transfer_out_usd_per_gb"), 4),
        "kind": "assumption",
        "model": "average_recall_response_unbounded_by_current_plan",
        "unit": "GB_per_month (decimal assumed)",
        "modeled_transfer_gb": round(transfer_gb, 4),
        "note": f"an assumed {AVG_RECALL_RESPONSE_BYTES} B average per recall response, recall responses only; nothing enforces it (limit is uncapped), so this is an average and not an exposure bound. Inbound write bytes are not egress and are not counted. Server and provider outbound bytes are unmodeled. Any measured bytes must name their scope (request mix and limit values) and horizon (window) and are not a bound. First {rate('data_transfer_out_free_gb_per_month'):.0f} GB/month free applied before this line, and an account-wide allowance cannot be assumed unused, so peak_exposure uses none of it. See unknown_categories.egress_response_size.",
    }

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

    unknown = {
        "embeddings": embedding_workload(totals),
        "embedding_restore_and_reopen": embedding_restore_and_reopen(totals),
        "cloudwatch_logs": {"priced_usd_per_gb": rate("cloudwatch_logs_ingest_usd_per_gb"), "reason": UNKNOWN_RATES["cloudwatch_logs_volume_gb_per_month"]},
        "cloudwatch_custom_metrics": {"priced_usd_per_metric": rate("cloudwatch_custom_metric_usd_per_month"), "reason": UNKNOWN_RATES["cloudwatch_custom_metrics_count"]},
        "s3_get_list_restore": {"reason": UNKNOWN_RATES["s3_get_list_restore"]},
        "control_db_lifetime_growth": control_db_lifetime_growth(totals),
        "s3_multipart_requests": s3_request_unknowns(s3_requests, objects_per_month),
        "egress_response_size": {"monthly_usd": None, "per_response_bound": None, "modeled_average_bytes_per_recall": AVG_RECALL_RESPONSE_BYTES, "reason": UNKNOWN_RATES["egress_response_size"]},
        "outbound_bytes_unmodeled": {"monthly_usd": None, "reason": UNKNOWN_RATES["outbound_bytes_unmodeled"]},
        "ec2_cpu_credit_mode": {"monthly_usd": None, "reason": UNKNOWN_RATES["ec2_cpu_credit_mode"]},
        "global_free_allowances": {"reason": UNKNOWN_RATES["global_free_allowances"]},
        "email_peak_volume": {"reason": UNKNOWN_RATES["email_peak_volume"]},
    }
    if not within_resend_free:
        unknown["email_overage"] = {"reason": "email volume exceeds Resend's free tier; exact Pro-tier overage price not modeled (range 20-35 USD/month base)"}

    peak = peak_exposure(known, known_subtotal, kms_requests, transfer_gb)

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


def peak_exposure(known: dict, known_subtotal: float, kms_requests: float, transfer_gb: float) -> dict:
    """A conservative peak over the known subtotal. It replaces four lines with their worst case and adds nothing else:

    - T4g surplus credits at sustained 100% CPU with the credit balance exhausted (and, separately, 70% mean CPU).
    - KMS key storage after the two billed rotations.
    - KMS requests and data transfer with none of the account-wide free allowance.

    The transfer line is the average recall-response model, which nothing enforces, so it is not an egress bound. Control-database
    lifetime growth, embeddings, restore and reopen re-embeds, logs, metrics, S3 request inputs, unbounded response size, unmodeled
    outbound bytes and peak email volume are unknown, so this is a peak of the priced categories and not a full maximum."""
    cpu = cpu_credit_exposure()
    kms_key_peak = rate("kms_key_usd_per_month") * (1 + rate("kms_rotation_billed_versions_max"))
    kms_requests_peak = kms_requests / 10000 * rate("kms_usd_per_10000_requests")
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
        "data_transfer_model": "average_recall_response_unbounded_by_current_plan: an average, not an egress bound",
        "replaced_known_lines_usd": round(replaced, 4),
        "not_included": ["embeddings", "embedding_restore_and_reopen", "cloudwatch_logs", "cloudwatch_custom_metrics", "s3_get_list_restore", "control_db_lifetime_growth", "s3_multipart_requests", "egress_response_size", "outbound_bytes_unmodeled", "email_peak_volume"],
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
        "unit_definitions": UNIT_DEFINITIONS,
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
        "citation_note": "Rates marked aws_regional_catalog_primary are read from AWS's own US West (Oregon) price lists, with a receipt file, rate code, price-list version, source hash and publication date each (docs/launch/evidence/T23.60/aws-rates/, s3-rate.json). S3 request and egress prices now use the regional Price List; the 100-GB shared internet-egress allowance is sourced from AWS's published pricing page, but target-account consumption is unknown. Resend prices remain vendor_page_secondary. Receipts imply no account, credit, free-tier availability or spend authority. The retention baseline is the deployed template's; any thinning proposal is unapproved and not modeled.",
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
