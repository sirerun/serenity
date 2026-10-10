import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

RENDER = Path(__file__).resolve().parents[1] / "render_invoke_policy.py"
spec = importlib.util.spec_from_file_location("invoke_policy", RENDER)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
ARN = "arn:aws:lambda:us-west-2:123456789012:function:gateway"

class InvokePolicyTests(unittest.TestCase):
    def test_separate_per_resource_policies(self):
        base, version = module.policies(ARN, "7", "abc123", "123456789012", "us-west-2")
        self.assertEqual(len(base["Statement"]), 2)
        self.assertTrue(all(s["Effect"] == "Deny" for s in base["Statement"]))
        self.assertEqual(len(version["Statement"]), 3)
        for s in base["Statement"]:
            self.assertEqual(s["Resource"], [ARN, ARN + ":*"])
        for s in version["Statement"]:
            self.assertEqual(s["Resource"], ARN + ":7")
        for policy in (base, version):
            denies = policy["Statement"][:2]
            self.assertTrue(all(s["Principal"] == "*" for s in denies))
            self.assertEqual(denies[0]["Condition"], {"ArnNotEquals": {"aws:SourceArn": [
                "arn:aws:execute-api:us-west-2:123456789012:abc123/prod/POST/runtime",
                "arn:aws:execute-api:us-west-2:123456789012:abc123/prod/POST/owner"]}})
            self.assertEqual(denies[1]["Condition"], {"StringNotEquals": {"aws:SourceAccount": "123456789012"}})
        allow = version["Statement"][2]
        self.assertEqual(allow["Principal"], {"Service": "apigateway.amazonaws.com"})
        self.assertEqual(allow["Condition"], {
            "ArnEquals": base["Statement"][0]["Condition"]["ArnNotEquals"],
            "StringEquals": base["Statement"][1]["Condition"]["StringNotEquals"]})

    def test_cli_requires_both_outputs_before_writing(self):
        with tempfile.TemporaryDirectory() as d:
            base, version = Path(d)/"base.json", Path(d)/"version.json"
            args = [sys.executable, str(RENDER), "--function-arn", ARN,
                "--version", "7", "--api-id", "abc123", "--account", "123456789012",
                "--region", "us-west-2", "--output", str(base)]
            result = subprocess.run(args, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(base.exists())
            result = subprocess.run(args + ["--version-output", str(version)], capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((json.loads(base.read_text()), json.loads(version.read_text())),
                module.policies(ARN, "7", "abc123", "123456789012", "us-west-2"))
