import json
import os
import tempfile
import unittest
from pathlib import Path
import importlib.util

ROOT = Path(__file__).parents[2]
SPEC = importlib.util.spec_from_file_location("provider_preflight", ROOT / "scripts/hosted/provider_preflight.py")
MOD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MOD)


class ProviderPreflightTests(unittest.TestCase):
    def manifest(self):
        return {
            "provider": {"base_url": MOD.BASE_URL, "model": MOD.MODEL, "allow_fallback": False, "secret_ref": "EMBEDDINGS_API_KEY"},
            "mail": {"secret_ref": "RESEND_API_KEY"},
            "stripe": {"secret_ref": "STRIPE_SECRET_KEY", "webhook_secret_ref": "STRIPE_WEBHOOK_SECRET"},
            "budget": {"approved_max_usd": 1, "max_calls": 2, "max_input_tokens": 1000, "max_elapsed_seconds": 60, "automatic_reset": False, "auto_top_up": False},
        }

    def test_missing_secrets_are_named_without_values(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "manifest.json"
            path.write_text(json.dumps(self.manifest()))
            old = {k: os.environ.pop(k, None) for k in ("EMBEDDINGS_API_KEY", "RESEND_API_KEY", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET")}
            try:
                result = MOD.preflight(path)
            finally:
                for k, v in old.items():
                    if v is not None:
                        os.environ[k] = v
            self.assertEqual(result["status"], "BLOCKED")
            self.assertEqual(result["network_calls"], 0)
            self.assertNotIn("synthetic-secret-value", json.dumps(result))
            self.assertEqual(set(result["secrets"]), {"EMBEDDINGS_API_KEY", "RESEND_API_KEY", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET"})

    def test_valid_metadata_is_ready_for_review_but_not_pass(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "manifest.json"
            path.write_text(json.dumps(self.manifest()))
            names = ("EMBEDDINGS_API_KEY", "RESEND_API_KEY", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET")
            old = {k: os.environ.get(k) for k in names}
            try:
                for k in names:
                    os.environ[k] = "synthetic-placeholder-for-test"
                result = MOD.preflight(path)
            finally:
                for k, v in old.items():
                    if v is None:
                        os.environ.pop(k, None)
                    else:
                        os.environ[k] = v
            self.assertEqual(result["status"], "READY_FOR_REVIEW")
            self.assertEqual(result["network_calls"], 0)
            self.assertNotEqual(result["status"], "PASS")

    def test_provider_mismatch_is_blocked(self):
        with tempfile.TemporaryDirectory() as td:
            manifest = self.manifest()
            manifest["provider"]["allow_fallback"] = True
            path = Path(td) / "manifest.json"
            path.write_text(json.dumps(manifest))
            result = MOD.preflight(path)
            self.assertEqual(result["status"], "BLOCKED")
            self.assertIn("provider_fallback_must_be_disabled", result["errors"])


if __name__ == "__main__":
    unittest.main()
