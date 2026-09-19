"""Manifest validation: a live run must refuse missing/null budget or
credential rather than defaulting to unlimited access
(docs/launch/hosted-completion/evidence.md).
"""

from __future__ import annotations

import copy
import json
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from lib import manifest as manifest_lib  # noqa: E402

TEMPLATE_PATH = HERE.parent.parent / "docs" / "launch" / "hosted-completion" / "qualification.example.json"


def _base_manifest() -> dict:
    m = json.loads(TEMPLATE_PATH.read_text())
    m["task_id"] = "T23.43"
    return m


def _target(endpoint: str, cred_ref: str, receipt: str) -> dict:
    from urllib.parse import urlsplit

    parts = urlsplit(endpoint)
    return {
        "endpoint_url": endpoint,
        "credential_secret_ref": cred_ref,
        "corpus_seeded_confirmation": receipt,
        "allowed_origins": [f"{parts.scheme}://{parts.netloc}"],
    }


def _fully_authorized_manifest() -> dict:
    m = _base_manifest()
    m["budget"] = {
        "approved_max_usd": 1.0,
        "max_calls": 100,
        "max_input_tokens": 5000,
        "max_elapsed_seconds": 600,
        "max_cost_per_call_usd": 0.001,
        "authorization_ref": "hq-dec-example",
        "automatic_reset": False,
        "auto_top_up": False,
    }
    m["provider"]["secret_ref"] = "EMBEDDINGS_API_KEY"
    m["provider"]["allow_fallback"] = False
    m["hosted_mcp"] = _target("https://app.serenity.sire.run/mcp", "T2343_HOSTED_CREDENTIAL", "receipt-2026-09-18")
    m["empty_case_hosted_mcp"] = _target(
        "https://isolated.app.serenity.sire.run/mcp", "T2343_EMPTY_CASE_CREDENTIAL", "receipt-2026-09-18-empty"
    )
    m["corpus_sha256"] = "0" * 64
    return m


class TestManifestTemplateIsBlockedByDefault(unittest.TestCase):
    def test_template_example_has_problems(self):
        problems = manifest_lib.validate_live_manifest(_base_manifest())
        self.assertTrue(problems, "the null-budget template must never validate as ready")


class TestManifestValidation(unittest.TestCase):
    def test_fully_authorized_manifest_has_no_problems(self):
        problems = manifest_lib.validate_live_manifest(_fully_authorized_manifest())
        self.assertEqual(problems, [])

    def test_missing_max_calls_blocks(self):
        m = _fully_authorized_manifest()
        m["budget"]["max_calls"] = None
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("max_calls" in p for p in problems))

    def test_zero_max_calls_blocks(self):
        m = _fully_authorized_manifest()
        m["budget"]["max_calls"] = 0
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("max_calls" in p for p in problems))

    def test_missing_authorization_ref_blocks(self):
        m = _fully_authorized_manifest()
        m["budget"]["authorization_ref"] = None
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("authorization_ref" in p for p in problems))

    def test_auto_top_up_blocks(self):
        m = _fully_authorized_manifest()
        m["budget"]["auto_top_up"] = True
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("auto_top_up" in p for p in problems))

    def test_allow_fallback_blocks(self):
        m = _fully_authorized_manifest()
        m["provider"]["allow_fallback"] = True
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("allow_fallback" in p for p in problems))

    def test_missing_hosted_endpoint_blocks(self):
        m = _fully_authorized_manifest()
        m["hosted_mcp"]["endpoint_url"] = None
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("endpoint_url" in p for p in problems))

    def test_missing_seeded_confirmation_blocks(self):
        m = _fully_authorized_manifest()
        del m["hosted_mcp"]["corpus_seeded_confirmation"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("corpus_seeded_confirmation" in p for p in problems))

    def test_missing_corpus_sha_blocks(self):
        m = _fully_authorized_manifest()
        m["corpus_sha256"] = None
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("corpus_sha256" in p for p in problems))

    def test_bool_max_calls_blocks(self):
        # bool is an int subclass in Python -- True must never pass as a
        # valid numeric cap (it would silently behave as max_calls=1).
        m = _fully_authorized_manifest()
        m["budget"]["max_calls"] = True
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("max_calls" in p for p in problems))

    def test_nan_max_calls_blocks(self):
        m = _fully_authorized_manifest()
        m["budget"]["max_calls"] = float("nan")
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("max_calls" in p for p in problems))

    def test_infinite_approved_max_usd_blocks(self):
        m = _fully_authorized_manifest()
        m["budget"]["approved_max_usd"] = float("inf")
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("approved_max_usd" in p for p in problems))

    def test_missing_max_cost_per_call_blocks(self):
        m = _fully_authorized_manifest()
        del m["budget"]["max_cost_per_call_usd"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("max_cost_per_call_usd" in p for p in problems))

    def test_missing_empty_case_target_blocks(self):
        m = _fully_authorized_manifest()
        del m["empty_case_hosted_mcp"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("empty_case_hosted_mcp" in p for p in problems))

    def test_empty_case_target_cannot_equal_positive_target_silently(self):
        # Not itself forbidden by validate_live_manifest (that's a
        # provisioning-time decision, not a shape check) -- but confirms
        # the field is independently validated, not merely defaulted from
        # hosted_mcp.
        m = _fully_authorized_manifest()
        del m["empty_case_hosted_mcp"]["allowed_origins"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("empty_case_hosted_mcp.allowed_origins" in p for p in problems))

    def test_missing_allowed_origins_blocks(self):
        m = _fully_authorized_manifest()
        del m["hosted_mcp"]["allowed_origins"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("hosted_mcp.allowed_origins" in p for p in problems))

    def test_endpoint_url_not_in_allowed_origins_blocks(self):
        m = _fully_authorized_manifest()
        m["hosted_mcp"]["allowed_origins"] = ["https://a-different-host.example"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("hosted_mcp.endpoint_url" in p for p in problems))

    def test_http_scheme_on_non_loopback_host_blocks(self):
        m = _fully_authorized_manifest()
        m["hosted_mcp"]["endpoint_url"] = "http://app.serenity.sire.run/mcp"
        m["hosted_mcp"]["allowed_origins"] = ["http://app.serenity.sire.run"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("hosted_mcp.endpoint_url" in p for p in problems))

    def test_userinfo_in_endpoint_url_blocks(self):
        m = _fully_authorized_manifest()
        m["hosted_mcp"]["endpoint_url"] = "https://user:pass@app.serenity.sire.run/mcp"
        m["hosted_mcp"]["allowed_origins"] = ["https://app.serenity.sire.run"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("hosted_mcp.endpoint_url" in p for p in problems))


if __name__ == "__main__":
    unittest.main()
