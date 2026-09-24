"""Tests for local_service_fixture.py (T23.43): the opt-in fixture that runs the
ACTUAL hosted service binary on loopback behind a synthetic embedding provider.

Two groups:

- Safety and mechanics tests that need no binary. They pin the rules the fixture
  enforces before it starts anything: it refuses existing paths, non-loopback
  addresses and any provider but its own; it hands the child a minimal
  environment; it redacts every synthetic credential; it signals only the exact
  PID it spawned; and it leaves no process or directory behind when a start fails.
- One opt-in end-to-end test, skipped unless T2343_LOCAL_SERVICE=1 and
  SERENITY_BIN name a pre-built binary. It starts the real service, so it is
  never part of a default run. T2343_LOCAL_SERVICE_SMOKE=1 shortens the TTL for a
  quick look; a smoke run is not the frozen protocol and the report says so.

Nothing here spends money, calls a paid provider, sends mail, or leaves
127.0.0.1.

Run via: python3 -m unittest discover -s evals/hosted -p "test_*.py"
"""

from __future__ import annotations

import contextlib
import io
import json
import math
import os
import stat
import subprocess
import sys
import tempfile
import time
import types
import unittest
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import local_service_fixture as lsf  # noqa: E402
from lib import forgotten_targets as ft, manifest as manifest_lib  # noqa: E402

CORPUS = json.loads((HERE / "corpus.json").read_text())
FACTS = {f["id"]: f for f in json.loads((HERE / "facts.json").read_text())["facts"]}


def reap(proc: subprocess.Popen) -> None:
    """Kills and waits for a child the test itself started (never anything else)."""
    if proc.poll() is None:
        proc.kill()
    proc.wait(timeout=20)


class TmpCase(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = Path(tempfile.mkdtemp(prefix="t2343-lsf-test-", dir="/tmp"))
        self.addCleanup(lambda: __import__("shutil").rmtree(self.tmp, ignore_errors=True))

    def owned_dirs(self, parent: Path) -> list[str]:
        return sorted(p.name for p in parent.iterdir() if p.name.startswith(lsf.OWNED_PREFIX))

    def python_stub(self, name: str, body: str) -> Path:
        path = self.tmp / name
        path.write_text(f"#!{sys.executable}\n{body}\n")
        path.chmod(0o755)
        return path


class TestOwnedTree(TmpCase):
    def test_refuses_a_path_that_already_exists(self):
        existing = self.tmp / f"{lsf.OWNED_PREFIX}existing"
        existing.mkdir()
        (existing / "keep.txt").write_text("precious")
        with self.assertRaises(lsf.FixtureRefused):
            lsf.OwnedTree.create_at(existing)
        self.assertEqual((existing / "keep.txt").read_text(), "precious")  # untouched

    def test_refuses_a_dangling_symlink_and_a_name_without_the_prefix(self):
        link = self.tmp / f"{lsf.OWNED_PREFIX}link"
        os.symlink(self.tmp / "nowhere", link)
        with self.assertRaises(lsf.FixtureRefused):
            lsf.OwnedTree.create_at(link)
        with self.assertRaises(lsf.FixtureRefused):
            lsf.OwnedTree.create_at(self.tmp / "not-owned-name")
        with self.assertRaises(lsf.FixtureRefused):
            lsf.OwnedTree.create(self.tmp / "missing-parent")

    def test_create_makes_one_private_marked_directory_and_cleanup_removes_only_it(self):
        neighbour = self.tmp / "neighbour"
        neighbour.mkdir()
        tree = lsf.OwnedTree.create(self.tmp)
        self.assertEqual(stat.S_IMODE(tree.root.stat().st_mode), 0o700)  # private to its owner
        self.assertTrue(tree.owns())
        sub = tree.subdir("data")
        self.assertEqual(stat.S_IMODE(sub.stat().st_mode), 0o700)
        with self.assertRaises(lsf.FixtureRefused):
            tree.subdir("data")  # a second use of a name is refused, not merged
        with self.assertRaises(lsf.FixtureRefused):
            tree.subdir("../escape")
        tree.cleanup()
        self.assertFalse(os.path.lexists(tree.root))
        self.assertTrue(neighbour.is_dir())
        tree.cleanup()  # safe twice

    def test_cleanup_refuses_when_the_marker_no_longer_matches(self):
        tree = lsf.OwnedTree.create(self.tmp)
        (tree.root / lsf.OWNED_MARKER).write_text("someone else's marker")
        with self.assertRaises(lsf.FixtureRefused):
            tree.cleanup()
        self.assertTrue(tree.root.is_dir())  # left alone

    def test_cleanup_retries_a_transient_failure_and_finishes(self):
        tree = lsf.OwnedTree.create(self.tmp)
        tree.subdir("data")
        real = lsf.shutil.rmtree
        calls = {"n": 0}

        def flaky(path, *a, **k):
            calls["n"] += 1
            if calls["n"] == 1:
                real(Path(path) / "data")  # a partial pass, then the failure a late writer causes
            if calls["n"] < 3:
                raise OSError(66, "Directory not empty")
            return real(path, *a, **k)

        with mock.patch.object(lsf.shutil, "rmtree", side_effect=flaky), mock.patch.object(lsf.time, "sleep"):
            tree.cleanup()
        self.assertFalse(os.path.lexists(tree.root))
        self.assertEqual(calls["n"], 3)

    def test_cleanup_that_keeps_failing_says_so_and_names_the_leftover(self):
        tree = lsf.OwnedTree.create(self.tmp)
        with mock.patch.object(lsf.shutil, "rmtree", side_effect=OSError(66, "Directory not empty")), mock.patch.object(lsf.time, "sleep"):
            with self.assertRaises(lsf.FixtureRefused) as cm:
                tree.cleanup()
        self.assertIn(tree.root.name, str(cm.exception))
        self.assertIn("OSError", str(cm.exception))
        self.assertIn("left for the operator", str(cm.exception))
        self.assertTrue(tree.root.is_dir())
        tree.cleanup()  # and once the cause is gone a later attempt succeeds
        self.assertFalse(os.path.lexists(tree.root))

    def test_a_retry_refuses_when_the_directory_is_swapped_for_a_symlink_between_attempts(self):
        victim = self.tmp / "victim"
        victim.mkdir()
        (victim / "file").write_text("must survive")
        tree = lsf.OwnedTree.create(self.tmp)

        def swap_then_fail(path, *a, **k):
            Path(path).rename(self.tmp / "moved-aside")
            os.symlink(victim, path)
            raise OSError(66, "Directory not empty")

        with mock.patch.object(lsf.shutil, "rmtree", side_effect=swap_then_fail), mock.patch.object(lsf.time, "sleep"):
            with self.assertRaises(lsf.FixtureRefused) as cm:
                tree.cleanup()
        self.assertIn("changed identity", str(cm.exception))
        self.assertEqual((victim / "file").read_text(), "must survive")
        os.unlink(tree.root)

    def test_cleanup_refuses_a_directory_swapped_for_a_symlink(self):
        victim = self.tmp / "victim"
        victim.mkdir()
        (victim / "file").write_text("must survive")
        tree = lsf.OwnedTree.create(self.tmp)
        moved = self.tmp / "moved-aside"
        tree.root.rename(moved)
        os.symlink(victim, tree.root)
        with self.assertRaises(lsf.FixtureRefused):
            tree.cleanup()
        self.assertEqual((victim / "file").read_text(), "must survive")
        os.unlink(tree.root)


class TestLoopbackRules(TmpCase):
    def test_only_the_literal_loopback_address_is_accepted(self):
        lsf.require_loopback_url("http://127.0.0.1:8080/v1", "u")
        for bad in (
            "http://localhost:8080", "http://0.0.0.0:8080", "http://192.168.1.5:8080", "http://[::1]:8080",
            "http://127.0.0.1.example.com:8080", "https://127.0.0.1:8080", "http://127.0.0.1", "http://user:pw@127.0.0.1:8080",
            "http://127.0.0.1:8080/v1?x=1", "http://127.0.0.1:8080/v1#f", "ftp://127.0.0.1:8080", "not a url", "",
        ):
            with self.subTest(bad):
                with self.assertRaises(lsf.FixtureRefused):
                    lsf.require_loopback_url(bad, "u")

    def config(self, **over) -> dict:
        root = self.tmp / "root"
        cfg = {
            "bind": "127.0.0.1:5000", "data_dir": str(root / "data"), "secrets_dir": str(root / "secrets"),
            "public_origin": "http://127.0.0.1:5000", "embedding_model": lsf.MODEL, "embedding_version": lsf.MODEL_VERSION,
            "embedding_base_url": "http://127.0.0.1:6000/v1",
        }
        cfg.update(over)
        return cfg

    def validate(self, **over) -> None:
        lsf.validate_service_config(self.config(**over), root=self.tmp / "root", provider_base_url="http://127.0.0.1:6000/v1")

    def test_a_config_that_stays_on_loopback_and_under_the_root_is_accepted(self):
        self.validate()

    def test_a_non_loopback_bind_origin_or_provider_is_refused(self):
        for label, over in {
            "bind on every interface": {"bind": "0.0.0.0:5000"},
            "bind on a name": {"bind": "localhost:5000"},
            "bind without a port": {"bind": "127.0.0.1"},
            "public origin elsewhere": {"public_origin": "https://serenity.example.com"},
            "public origin with a path": {"public_origin": "http://127.0.0.1:5000/app"},
            "a real provider": {"embedding_base_url": "https://openrouter.ai/api/v1"},
            "a loopback provider that is not this fixture's": {"embedding_base_url": "http://127.0.0.1:6001/v1"},
            "billing on": {"billing_enabled": True},
            "data dir outside the owned root": {"data_dir": "/tmp/elsewhere"},
            "relative secrets dir": {"secrets_dir": "secrets"},
        }.items():
            with self.subTest(label):
                with self.assertRaises(lsf.FixtureRefused):
                    self.validate(**over)

    def test_an_admin_socket_path_the_platform_cannot_bind_is_refused_before_starting(self):
        deep = self.tmp / "root" / ("x" * 90) / "data"
        with self.assertRaises(lsf.FixtureRefused) as cm:
            self.validate(data_dir=str(deep))
        self.assertIn("socket", str(cm.exception))


class TestRedaction(unittest.TestCase):
    def test_registered_secrets_and_token_shaped_values_are_replaced(self):
        r = lsf.Redactor()
        token = "sk_live_a1b2c3d4_" + "A" * 43
        r.add("bearer", token)
        r.add("cookie", "session-cookie-value-123456")
        r.add("short", "abc")  # too short to register: it would match everywhere
        text = f"auth {token} cookie session-cookie-value-123456 url token=" + "Z" * 30 + " keep abc"
        scrubbed = r.scrub(text)
        self.assertNotIn(token, scrubbed)
        self.assertNotIn("session-cookie-value-123456", scrubbed)
        self.assertNotIn("Z" * 30, scrubbed)
        self.assertIn("keep abc", scrubbed)
        self.assertIn("[redacted:bearer]", scrubbed)
        self.assertEqual(r.count(), 2)

    def test_url_quoted_secrets_are_replaced_too(self):
        r = lsf.Redactor()
        r.add("link", "http://127.0.0.1:1/login?token=abc def&x=1")
        self.assertNotIn("abc%20def", r.scrub("see http%3A%2F%2F127.0.0.1%3A1%2Flogin%3Ftoken%3Dabc%20def%26x%3D1"))

    def test_assert_clean_names_the_label_and_never_the_value(self):
        r = lsf.Redactor()
        r.add("embedding-key", "sk-synthetic-VERYSECRETVALUE")
        r.assert_clean("nothing here")
        with self.assertRaises(lsf.FixtureRefused) as cm:
            r.assert_clean("leaked sk-synthetic-VERYSECRETVALUE")
        self.assertIn("embedding-key", str(cm.exception))
        self.assertNotIn("VERYSECRET", str(cm.exception))
        with self.assertRaises(lsf.FixtureRefused):
            r.assert_clean("bearer sk_live_a1b2c3d4_" + "B" * 43)

    def test_the_account_repr_shows_no_credential(self):
        account = lsf.Account(label="x", email="x@example.invalid", cookie="COOKIE-SECRET-VALUE", csrf="CSRF-SECRET-VALUE", token="TOKEN-SECRET-VALUE")
        for secret in ("COOKIE-SECRET", "CSRF-SECRET", "TOKEN-SECRET"):
            self.assertNotIn(secret, repr(account))


class TestLoopbackProvider(unittest.TestCase):
    def setUp(self) -> None:
        self.provider = lsf.LoopbackEmbeddingProvider("sk-synthetic-unit-key", oracle={"the question": "the answer text"})
        self.base = self.provider.start()
        self.addCleanup(self.provider.stop)
        self.port = int(self.base.rsplit(":", 1)[1].split("/")[0])

    def post(self, body: dict, *, key: str = "sk-synthetic-unit-key", path: str = "/v1/embeddings"):
        return lsf.http_exchange(
            self.port, "POST", path, headers={"Authorization": f"Bearer {key}", "Content-Type": "application/json"},
            body=json.dumps(body).encode(),
        )

    def vector(self, text: str) -> list[float]:
        status, _h, raw = self.post({"model": "m", "input": text})
        self.assertEqual(status, 200)
        return json.loads(raw)["data"][0]["embedding"]

    def test_it_listens_on_the_loopback_address_only(self):
        self.assertTrue(self.base.startswith("http://127.0.0.1:"))
        self.assertEqual(self.provider._server.server_address[0], "127.0.0.1")

    def test_vectors_are_deterministic_unit_length_and_the_documented_width(self):
        a, b = self.vector("a synthetic fact"), self.vector("a synthetic fact")
        self.assertEqual(a, b)
        self.assertEqual(len(a), lsf.DIMENSIONS)
        self.assertAlmostEqual(math.sqrt(sum(x * x for x in a)), 1.0, places=4)
        self.assertNotEqual(a, self.vector("an entirely different sentence"))

    def test_the_oracle_makes_a_query_share_its_answers_vector(self):
        self.assertEqual(self.vector("the question"), self.vector("the answer text"))  # a cached vector, nothing semantic

    def test_a_wrong_key_a_wrong_path_and_a_bad_body_are_refused_and_counted(self):
        self.assertEqual(self.post({"model": "m", "input": "x"}, key="sk-wrong")[0], 401)
        self.assertEqual(self.post({"model": "m", "input": "x"}, path="/v1/other")[0], 404)
        self.assertEqual(self.post({"model": "m"})[0], 400)
        self.assertEqual((self.provider.auth_failures, self.provider.bad_requests, self.provider.requests), (1, 2, 0))

    def test_it_records_inputs_by_hash_only(self):
        self.vector("private-looking input text")
        self.assertEqual(self.provider.saw("private-looking input text"), 1)
        self.assertNotIn("private-looking", json.dumps(self.provider.input_sha256))


class TestExactPidOwnership(TmpCase):
    def spawn(self, marker: str) -> subprocess.Popen:
        return subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)", marker], stdin=subprocess.DEVNULL)

    def test_it_stops_exactly_the_process_it_spawned_and_confirms_it_is_gone(self):
        marker = f"t2343-marker-{os.getpid()}-{time.time_ns()}"
        proc = self.spawn(marker)
        self.addCleanup(reap, proc)
        owned = lsf.OwnedProcess(proc, marker)
        record = owned.stop(term_timeout=10)
        self.assertEqual(record["signals"], ["SIGTERM"])
        self.assertTrue(record["confirmed_gone"])
        self.assertIsNone(lsf.command_line_of(owned.pid))
        self.assertEqual(owned.stop()["signals"], [])  # a second stop signals nothing

    def test_it_refuses_to_signal_a_pid_whose_command_line_lost_the_marker(self):
        proc = self.spawn("some-other-marker")
        self.addCleanup(reap, proc)
        owned = lsf.OwnedProcess(proc, "a-marker-this-process-does-not-carry")
        with self.assertRaises(lsf.FixtureRefused):
            owned.stop()
        self.assertIsNone(proc.poll())  # still running: it was not signalled

    def test_a_process_that_already_exited_is_recorded_without_any_signal(self):
        proc = subprocess.Popen([sys.executable, "-c", "pass", "quick-marker"])
        proc.wait(timeout=20)
        record = lsf.OwnedProcess(proc, "quick-marker").stop()
        self.assertEqual((record["signals"], record["confirmed_gone"]), ([], True))

    def test_command_line_of_a_missing_pid_is_none(self):
        proc = subprocess.Popen([sys.executable, "-c", "pass"])
        proc.wait(timeout=20)
        self.assertIsNone(lsf.command_line_of(proc.pid))


class TestStartupFailuresLeaveNothingBehind(TmpCase):
    def test_a_binary_that_exits_at_once_leaves_no_directory_process_or_thread(self):
        stub = self.python_stub("dies", "import sys\nsys.exit(3)")
        parent = self.tmp / "parent"
        parent.mkdir()
        threads_before = sum(1 for t in __import__("threading").enumerate() if t.name.startswith("Thread"))
        svc = lsf.LocalHostedService(stub, parent=parent)
        with self.assertRaises(lsf.FixtureRefused):
            with svc:
                self.fail("the context body must not run")
        self.assertEqual(self.owned_dirs(parent), [])
        self.assertIsNone(svc.process)
        self.assertLessEqual(sum(1 for t in __import__("threading").enumerate() if t.name.startswith("Thread")), threads_before + 1)

    def test_a_binary_that_never_becomes_ready_is_stopped_by_its_exact_pid_and_cleaned_up(self):
        stub = self.python_stub("hangs", "import time\ntime.sleep(300)")
        parent = self.tmp / "parent"
        parent.mkdir()
        svc = lsf.LocalHostedService(stub, parent=parent)
        with self.assertRaises(lsf.FixtureRefused):
            svc.start(ready_timeout=2)
        pid = svc.process.pid if svc.process else None
        svc.close()
        if pid is not None:
            self.assertIsNone(lsf.command_line_of(pid))
        self.assertEqual(self.owned_dirs(parent), [])
        svc.close()  # safe twice

    def test_the_child_gets_a_minimal_environment_with_no_proxy_or_credentials(self):
        stub = self.python_stub("dumps-env", "import json, os, time\nopen(os.path.join(os.environ['TMPDIR'], 'env.json'), 'w').write(json.dumps(dict(os.environ)))\ntime.sleep(300)")
        parent = self.tmp / "parent"
        parent.mkdir()
        canaries = {"HTTPS_PROXY": "http://proxy.invalid:1", "OPENAI_API_KEY": "canary-key", "AWS_SECRET_ACCESS_KEY": "canary-aws",
                    "EMBEDDINGS_API_KEY": "canary-emb", "STRIPE_SECRET_KEY": "canary-stripe"}
        svc = lsf.LocalHostedService(stub, parent=parent)
        with mock.patch.dict(os.environ, canaries):
            with self.assertRaises(lsf.FixtureRefused):
                svc.start(ready_timeout=3)
        env = json.loads((svc.tree.root / "tmp" / "env.json").read_text())
        svc.close()
        self.assertLessEqual(set(env), {"PATH", "HOME", "TMPDIR", "LANG", "SERENITY_HOSTED_DEV", "__CF_USER_TEXT_ENCODING", "LC_CTYPE"})
        for name in canaries:
            self.assertNotIn(name, env)
        self.assertTrue(env["HOME"].startswith(str(svc.tree.root)))
        self.assertTrue(env["TMPDIR"].startswith(str(svc.tree.root)))

    def test_a_missing_or_non_executable_binary_is_refused_before_anything_is_created(self):
        parent = self.tmp / "parent"
        parent.mkdir()
        for path in (self.tmp / "absent", self.tmp):
            with self.assertRaises(lsf.FixtureRefused):
                lsf.LocalHostedService(path, parent=parent)
        self.assertEqual(self.owned_dirs(parent), [])


class TestScenarioBuildingBlocks(TmpCase):
    def test_the_oracle_covers_every_query_and_the_sentinel(self):
        oracle = lsf.build_oracle(CORPUS, FACTS)
        self.assertEqual(len(oracle), len(CORPUS["cases"]) + 1)
        empty = [c for c in CORPUS["cases"] if c["category"] == "empty"]
        self.assertEqual(len(empty), 5)
        for case in empty:
            self.assertEqual(oracle[case["query"]], ft.FORGOTTEN_TARGET_TEXT[case["id"]])
        for case in CORPUS["cases"]:
            if case["category"] != "empty":
                self.assertEqual(oracle[case["query"]], FACTS[case["expected_fact_id"]]["text"])

    def test_the_manifest_the_fixture_builds_is_a_valid_seed_phase_manifest_for_a_local_fixture(self):
        svc = types.SimpleNamespace(origin="http://127.0.0.1:5000", provider=types.SimpleNamespace(base_url="http://127.0.0.1:6000/v1"))
        template = json.loads((HERE.parent.parent / "docs/launch/hosted-completion/qualification.example.json").read_text())
        m = lsf.build_manifest(svc, template, CORPUS["meta"]["corpus_hash_sha256"], self.tmp / "ledger.jsonl")
        with mock.patch.dict(os.environ, {lsf.POS_ENV: "a" * 20, lsf.EMPTY_ENV: "b" * 20}):
            problems = manifest_lib.validate_live_manifest(m, manifest_lib.PHASE_SEED)
        self.assertEqual(problems, [])
        self.assertEqual(m["environment"]["kind"], "local-fixture")
        self.assertEqual(m["seeding"]["authorization_ref"], lsf.AUTHORIZATION_REF)
        self.assertFalse(m["budget"]["automatic_reset"] or m["budget"]["auto_top_up"])

    def test_the_canonical_reader_only_reads(self):
        good = self.tmp / "brain" / "sources" / "ab" / ("ab" * 32)
        good.mkdir(parents=True)
        (good / "bytes").write_text(json.dumps({"record_type": "memory_fact", "operation_key": "k", "fact": "f"}))
        skipped = self.tmp / "brain" / "sources" / "cd" / ("cd" * 32)
        skipped.mkdir(parents=True)
        (skipped / "bytes").write_text("not json")
        other = self.tmp / "brain" / "sources" / "ef" / ("ef" * 32)
        other.mkdir(parents=True)
        (other / "bytes").write_text(json.dumps({"record_type": "something_else"}))
        before = {p: (p.read_bytes(), p.stat().st_mtime_ns) for p in self.tmp.rglob("bytes")}
        records = lsf.read_canonical_records(self.tmp)
        self.assertEqual([(r["record_type"], r["operation_key"]) for r in records], [("memory_fact", "k")])
        self.assertEqual(before, {p: (p.read_bytes(), p.stat().st_mtime_ns) for p in self.tmp.rglob("bytes")})

    def test_the_rate_window_waits_only_for_what_is_left_and_records_it(self):
        window = lsf.RateWindow()
        window.last = time.monotonic() - (lsf.RateWindow.WINDOW_SECONDS - 0.2)
        started = time.monotonic()
        window.wait("next phase")
        self.assertGreaterEqual(time.monotonic() - started, 0.15)
        self.assertEqual([w["before"] for w in window.waits], ["next phase"])
        window.last = time.monotonic() - 1000
        window.wait("later")
        self.assertEqual(len(window.waits), 1)  # the window had already cleared: no wait, no record

    def test_only_a_429_is_retried(self):
        waits: list[float] = []
        calls = {"n": 0}

        def flaky():
            calls["n"] += 1
            if calls["n"] < 3:
                raise RuntimeError("HTTP 429 from the hosted endpoint (body not logged)")
            return "ok"

        self.assertEqual(lsf.with_rate_limit_retry(flaky, waits, pause=0.01), "ok")
        self.assertEqual(len(waits), 2)
        with self.assertRaises(RuntimeError):
            lsf.with_rate_limit_retry(mock.Mock(side_effect=RuntimeError("HTTP 500")), waits, pause=0.01)


class TestCommandLine(TmpCase):
    def test_it_refuses_to_run_without_the_opt_in(self):
        with mock.patch.dict(os.environ, {}, clear=False), contextlib.redirect_stderr(io.StringIO()):
            os.environ.pop(lsf.OPT_IN_ENV, None)
            self.assertEqual(lsf.main(["--output", str(self.tmp / "report.json")]), 2)
        self.assertFalse((self.tmp / "report.json").exists())

    def test_it_refuses_an_existing_output_and_a_missing_binary(self):
        out = self.tmp / "report.json"
        out.write_text("existing")
        with mock.patch.dict(os.environ, {lsf.OPT_IN_ENV: "1"}), contextlib.redirect_stderr(io.StringIO()) as err:
            self.assertEqual(lsf.main(["--output", str(out), "--serenity-bin", "/nonexistent"]), 2)
            self.assertEqual(out.read_text(), "existing")  # never overwritten
            self.assertEqual(lsf.main(["--output", str(self.tmp / "new.json"), "--serenity-bin", "/nonexistent"]), 2)
        self.assertFalse((self.tmp / "new.json").exists())
        self.assertIn("executable file", err.getvalue())  # a clean refusal, not a crash


BINARY = os.environ.get(lsf.BIN_ENV, "")


@unittest.skipUnless(
    os.environ.get(lsf.OPT_IN_ENV) == "1" and BINARY,
    f"opt-in: set {lsf.OPT_IN_ENV}=1 and {lsf.BIN_ENV} to a pre-built serenity binary to run the actual service on loopback",
)
class TestActualService(unittest.TestCase):
    """Starts the real `serenity hosted serve`. With the production constants it
    waits out a 60 s TTL plus the 5 s margin and the service's per-minute limit,
    so it takes several minutes; T2343_LOCAL_SERVICE_SMOKE=1 shortens the TTL and
    the report then says it is not the frozen protocol."""

    def test_the_seed_protocol_holds_against_the_actual_service(self):
        smoke = os.environ.get("T2343_LOCAL_SERVICE_SMOKE") == "1"
        report = lsf.run_verification(
            BINARY, allow_dirty_tree=True, ttl_seconds=12 if smoke else None, margin_seconds=3 if smoke else None,
        )
        failed = [c["name"] + ": " + c["observed"] for c in report["checks"] if not c["passed"]]
        self.assertEqual(failed, [])
        self.assertIn(report["status"], ("PARTIAL", "SMOKE-NOT-QUALIFYING"))
        self.assertEqual(report["provider_and_semantic_qualification"], "BLOCKED")
        self.assertEqual(report["evidence_level"], "local-process")
        self.assertTrue(report["redaction"]["verified_clean"])
        self.assertEqual(report["live"]["evidence_level"], "fixture")  # 95/95 correct, and still not quality evidence
        self.assertEqual(report["live"]["positive_hits"], "95/95")
        self.assertNotIn("real server", json.dumps(report).lower().replace("the real service", ""))
        # Nothing was left behind.
        self.assertEqual([p for p in Path("/tmp").glob(lsf.OWNED_PREFIX + "*")], [])


if __name__ == "__main__":
    unittest.main()
