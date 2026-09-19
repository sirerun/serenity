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
        "seed_receipt_path": "receipts/" + cred_ref + ".json",
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
    m["provider"].update(model="m", version_pin="pin-1", dimensions=1024, serving_provider="sp", privacy_review_ref="pr-1")
    m["environment"] = {
        "kind": "staging", "origin": None,
        "allowed_hosts": ["app.serenity.sire.run", "isolated.app.serenity.sire.run"],
        "production_target_allowed": False,
    }
    m["hosted_mcp"] = _target("https://app.serenity.sire.run/mcp", "T2343_HOSTED_CREDENTIAL", "sha256:" + "a" * 64)
    m["empty_case_hosted_mcp"] = _target(
        "https://isolated.app.serenity.sire.run/mcp", "T2343_EMPTY_CASE_CREDENTIAL", "sha256:" + "b" * 64
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


class TestSeedAndLivePhaseValidation(unittest.TestCase):
    def _seed(self, m):
        return manifest_lib.validate_live_manifest(m, manifest_lib.PHASE_SEED)

    def test_seed_phase_needs_no_receipts_but_needs_explicit_authorization(self):
        m = _fully_authorized_manifest()
        for t in ("hosted_mcp", "empty_case_hosted_mcp"):
            del m[t]["seed_receipt_path"]
            m[t]["corpus_seeded_confirmation"] = None
        self.assertTrue(any("seeding.authorized" in p for p in self._seed(m)))
        m["seeding"] = {"authorized": True, "authorization_ref": "hq-dec-x"}
        self.assertEqual(self._seed(m), [])

    def test_seed_authorization_must_be_the_boolean_true_with_a_reference(self):
        m = _fully_authorized_manifest()
        for value in ("true", 1, "yes"):
            m["seeding"] = {"authorized": value, "authorization_ref": "hq-dec-x"}
            self.assertTrue(any("seeding.authorized" in p for p in self._seed(m)), value)
        m["seeding"] = {"authorized": True}
        self.assertTrue(any("seeding.authorization_ref" in p for p in self._seed(m)))

    def test_live_phase_requires_a_receipt_path_and_a_sha256_confirmation(self):
        m = _fully_authorized_manifest()
        del m["hosted_mcp"]["seed_receipt_path"]
        self.assertTrue(any("hosted_mcp.seed_receipt_path" in p for p in manifest_lib.validate_live_manifest(m)))
        m = _fully_authorized_manifest()
        for hand_typed in ("receipt-2026-09-18", "sha256:short", "SHA256:" + "a" * 64):
            m["hosted_mcp"]["corpus_seeded_confirmation"] = hand_typed
            self.assertTrue(any("sha256:<64 hex>" in p for p in manifest_lib.validate_live_manifest(m)), hand_typed)

    def test_provider_pin_fields_are_required(self):
        for field in ("model", "version_pin", "serving_provider", "privacy_review_ref", "dimensions"):
            with self.subTest(field=field):
                m = _fully_authorized_manifest()
                m["provider"][field] = None
                self.assertTrue(any(f"provider.{field}" in p for p in manifest_lib.validate_live_manifest(m)))

    def test_dimensions_must_be_a_positive_integer(self):
        for bad in (True, 0, -8, 1.5, "1024"):
            m = _fully_authorized_manifest()
            m["provider"]["dimensions"] = bad
            self.assertTrue(any("provider.dimensions" in p for p in manifest_lib.validate_live_manifest(m)), bad)

    def test_endpoint_host_must_be_in_the_environment_allowlist(self):
        m = _fully_authorized_manifest()
        m["environment"]["allowed_hosts"] = ["some-other-host.example"]
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("not in environment.allowed_hosts" in p for p in problems))

    def test_environment_allowlist_and_kind_are_required(self):
        m = _fully_authorized_manifest()
        m["environment"]["allowed_hosts"] = []
        self.assertTrue(any("allowed_hosts" in p for p in manifest_lib.validate_live_manifest(m)))
        m = _fully_authorized_manifest()
        m["environment"]["kind"] = None
        self.assertTrue(any("environment.kind" in p for p in manifest_lib.validate_live_manifest(m)))

    def test_a_production_kind_without_the_explicit_flag_is_refused(self):
        m = _fully_authorized_manifest()
        m["environment"]["kind"] = "production"
        self.assertTrue(any("production targets are refused" in p for p in manifest_lib.validate_live_manifest(m)))

    def test_both_targets_may_not_share_one_credential_reference(self):
        m = _fully_authorized_manifest()
        m["empty_case_hosted_mcp"]["credential_secret_ref"] = m["hosted_mcp"]["credential_secret_ref"]
        self.assertTrue(any("one credential is one account" in p for p in manifest_lib.validate_live_manifest(m)))

    def test_a_credential_reference_must_be_an_env_var_name_and_is_never_echoed(self):
        m = _fully_authorized_manifest()
        secret_looking = "sk-live-abc123-DO-NOT-ECHO"
        m["hosted_mcp"]["credential_secret_ref"] = secret_looking
        problems = manifest_lib.validate_live_manifest(m)
        self.assertTrue(any("environment variable NAME" in p for p in problems))
        self.assertFalse(any(secret_looking in p for p in problems))


if __name__ == "__main__":
    unittest.main()
