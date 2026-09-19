import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "hosted" / "load.py"
spec = importlib.util.spec_from_file_location("load_cli", SCRIPT)
load_cli = importlib.util.module_from_spec(spec)
spec.loader.exec_module(load_cli)

MANIFEST_PATH = Path(__file__).resolve().parents[2] / "docs" / "launch" / "evidence" / "T23.60" / "manifest.json"


def base_manifest(**overrides):
    m = json.loads(MANIFEST_PATH.read_text())
    m.update(overrides)
    return m


class EnvironmentGuardTests(unittest.TestCase):
    def test_missing_origin_blocked(self):
        self.assertIsNotNone(load_cli.check_environment_guard(base_manifest()))

    def test_host_not_allowlisted_blocked(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "https://evil.example.com", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False})
        self.assertIsNotNone(load_cli.check_environment_guard(m))

    def test_production_target_allowed_flag_always_refused(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "https://127.0.0.1:9443", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": True})
        self.assertIsNotNone(load_cli.check_environment_guard(m))

    def test_production_hostname_refused_even_if_allowlisted(self):
        m = base_manifest(environment={"kind": "disposable", "origin": "https://app.serenity.sire.run", "allowed_hosts": ["app.serenity.sire.run"], "production_target_allowed": False})
        self.assertIsNotNone(load_cli.check_environment_guard(m))

    def test_allowlisted_non_production_host_passes_real_sensitivity(self):
        """Proves the guard actually discriminates rather than always refusing."""
        m = base_manifest(environment={"kind": "disposable", "origin": "http://127.0.0.1:9443", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False})
        self.assertIsNone(load_cli.check_environment_guard(m))


class BudgetGuardTests(unittest.TestCase):
    def test_missing_budget_field_blocked(self):
        m = base_manifest(budget={"approved_max_usd": None, "max_calls": 10, "max_input_tokens": 10, "max_elapsed_seconds": 10, "authorization_ref": "x"})
        self.assertIsNotNone(load_cli.check_budget(m))

    def test_complete_budget_passes(self):
        m = base_manifest(budget={"approved_max_usd": 0, "max_calls": 10, "max_input_tokens": 10, "max_elapsed_seconds": 10, "authorization_ref": "x"})
        self.assertIsNone(load_cli.check_budget(m))


class RunFixturesTests(unittest.TestCase):
    def test_produces_valid_structure_with_replay_verified(self):
        manifest = base_manifest()
        workload = load_cli.harness.load_workload(Path(__file__).with_name("workload.json"))
        result = load_cli.run_fixtures(manifest, workload)
        self.assertTrue(result["replay_determinism_verified"])
        self.assertEqual(len(result["repetitions"]), workload["repetitions"])
        self.assertFalse(result["resource_usage"]["available"])


class RunLiveBudgetCapTests(unittest.TestCase):
    def test_stops_at_max_calls_and_reports_blocked(self):
        """Real cap-stopping behavior: a fake transport that always succeeds
        instantly must still halt once calls_used reaches the small cap,
        proving the loop enforces the budget rather than ignoring it."""
        manifest = base_manifest(
            environment={"kind": "disposable", "origin": "http://127.0.0.1:9443", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False},
            budget={"approved_max_usd": 0, "max_calls": 5, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 600, "authorization_ref": "test"},
        )
        workload = load_cli.harness.load_workload(Path(__file__).with_name("workload.json"))

        def fake_transport(origin, credential, verb, tokens, timeout_s):
            return True, 0.01, 200

        result = load_cli.run_live(manifest, workload, transport=fake_transport)
        self.assertEqual(result["status"], "BLOCKED")
        self.assertEqual(result["calls_used"], 5)
        self.assertEqual(len(result["results"]), 5)

    def test_never_sends_real_credential_value(self):
        manifest = base_manifest(
            environment={"kind": "disposable", "origin": "http://127.0.0.1:9443", "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False},
            budget={"approved_max_usd": 0, "max_calls": 1, "max_input_tokens": 10_000_000, "max_elapsed_seconds": 600, "authorization_ref": "test"},
        )
        workload = load_cli.harness.load_workload(Path(__file__).with_name("workload.json"))
        seen = {}

        def fake_transport(origin, credential, verb, tokens, timeout_s):
            seen["credential"] = credential
            return True, 0.01, 200

        load_cli.run_live(manifest, workload, transport=fake_transport)
        self.assertTrue(seen["credential"].startswith("REDACTED:"))


class CLIEndToEndTests(unittest.TestCase):
    def test_fixtures_mode_exits_zero_and_writes_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "load-fixture.json"
            argv = sys.argv
            sys.argv = ["load.py", "--fixtures", "--manifest", str(MANIFEST_PATH), "--output", str(out)]
            try:
                code = load_cli.main()
            finally:
                sys.argv = argv
            self.assertEqual(code, 0)
            data = json.loads(out.read_text())
            self.assertEqual(data["mode"], "fixtures")
            self.assertTrue(data["replay_determinism_verified"])

    def test_live_mode_blocked_without_environment_origin(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest_path = Path(tmp) / "manifest.json"
            manifest_path.write_text(json.dumps(base_manifest()))
            out = Path(tmp) / "load.json"
            argv = sys.argv
            sys.argv = ["load.py", "--live", "--manifest", str(manifest_path), "--output", str(out)]
            try:
                with self.assertRaises(SystemExit) as ctx:
                    load_cli.main()
            finally:
                sys.argv = argv
            self.assertEqual(ctx.exception.code, 2)
            self.assertFalse(out.exists())


if __name__ == "__main__":
    unittest.main()
