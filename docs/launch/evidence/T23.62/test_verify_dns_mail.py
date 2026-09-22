import importlib.util
import json
import unittest
from pathlib import Path

HERE = Path(__file__).parent
spec = importlib.util.spec_from_file_location("verify_dns_mail", HERE / "verify_dns_mail.py")
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


class VerifyDnsMailTests(unittest.TestCase):
    def test_missing_origin_is_blocked_without_mutation(self):
        expected = {"app_origin": "https://app.example", "expected_a": "203.0.113.5", "expected_cname": None, "sender_domain": "example", "controlled_recipient_ref": "test@example"}
        result = mod.verify(expected, resolver=lambda *_: [], https=lambda *_: {"ok": False, "status": None})
        self.assertEqual(result["status"], "BLOCKED")
        self.assertEqual(result["network_mutations"], 0)
        self.assertEqual(result["mail"]["delivery_status"], "NOT_RUN")

    def test_matching_dns_and_https_are_passable_but_mail_stays_unrun(self):
        expected = {"app_origin": "https://app.example", "expected_a": "203.0.113.5", "expected_cname": None, "sender_domain": "example", "controlled_recipient_ref": "test@example"}
        result = mod.verify(expected, resolver=lambda _name, kind: ["203.0.113.5"] if kind == "A" else [], https=lambda *_: {"ok": True, "status": 200, "url": "https://app.example/healthz"})
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["mail"]["delivery_status"], "NOT_RUN")


if __name__ == "__main__":
    unittest.main()
