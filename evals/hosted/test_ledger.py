"""Unit tests for lib/ledger.py and its use by lib/budget.py (T23.43).

The ledger is the qualification's cumulative budget: every request is reserved
in it, durably, before a socket opens. These tests drive it directly, with real
files, real second processes and a loopback server whose requests are counted
independently of the ledger. Nothing here touches a network beyond 127.0.0.1,
a credential or a paid provider.

The ledger is an operator-side accident and crash guard. `TestStatedLimits`
pins what it does NOT detect, so the docs cannot claim more than the code does.

Run via: python3 -m unittest discover -s evals/hosted -p "test_*.py"
"""

from __future__ import annotations

import contextlib
import json
import os
import signal
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from fake_hosted_mcp import FakeHostedMCP, RealClock  # noqa: E402
from lib import ledger as ledger_lib  # noqa: E402
from lib.budget import BudgetGuard  # noqa: E402
from lib.mcp_client import BudgetExceeded  # noqa: E402

TOKEN = "unit-token-SENTINEL"
CAPS = {"max_calls": 25, "max_input_tokens": 10**9, "approved_max_usd": 1000.0, "max_cost_per_call_usd": 0.001}
REF = "unit-test-authorization"

HOLDER = r"""
import fcntl, os, sys, time
fd = os.open(sys.argv[1], os.O_RDWR | os.O_CREAT, 0o600)
fcntl.flock(fd, fcntl.LOCK_EX)
print("locked", flush=True)
time.sleep(float(sys.argv[2]))
"""

WORKER = r"""
import json, sys, time
sys.path.insert(0, sys.argv[1])
from lib import ledger as L
from lib.budget import BudgetGuard
from lib.mcp_client import BudgetExceeded
path, endpoint, origin, token, start_at, caps_json, ref = sys.argv[2:9]
caps = json.loads(caps_json)
ledger = L.Ledger.open(path, authorization_ref=ref, caps=L.caps_of(caps), identity={})
guard = BudgetGuard(dict(caps, max_elapsed_seconds=120), ledger, mode="live")
while time.time() < float(start_at):
    time.sleep(0.001)
handshakes = 0
while True:
    client = guard.client(endpoint, token, [origin], role="positive")
    try:
        client.initialize()
        handshakes += 1
    except BudgetExceeded:
        break
print(json.dumps({"handshakes": handshakes, "reserved": ledger.reserved_here}))
"""

RESERVE_THEN_DIE = r"""
import json, os, signal, sys
sys.path.insert(0, sys.argv[1])
from lib import ledger as L
ledger = L.Ledger.open(sys.argv[2], authorization_ref=sys.argv[3], caps=L.caps_of(json.loads(sys.argv[4])), identity={})
for _ in range(int(sys.argv[5])):
    ledger.reserve(100, invocation="child", mode="live", role="positive")
os.kill(os.getpid(), signal.SIGKILL)  # after reserving, before any request could be sent or a receipt written
"""


class CountingFake:
    """A FakeHostedMCP plus a request counter that does not depend on the fake's
    own log: every POST that reaches the socket is counted at the handler."""

    def __init__(self) -> None:
        self.fake = FakeHostedMCP({TOKEN: "acct"}, clock=RealClock())
        self.wire = 0
        self._lock = threading.Lock()
        original = self.fake._handle

        def counting(handler):
            with self._lock:
                self.wire += 1
            return original(handler)

        self.fake._handle = counting
        self.origin = self.fake.start()
        self.endpoint = f"{self.origin}/mcp"

    def stop(self) -> None:
        self.fake.stop()


class LedgerCase(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = Path(tempfile.mkdtemp(prefix="t2343-ledger-"))
        self.addCleanup(lambda: __import__("shutil").rmtree(self.tmp, ignore_errors=True))
        self.path = self.tmp / "ledger.jsonl"
        self.lock = self.tmp / "ledger.jsonl.lock"

    def new_ledger(self, path: Path | None = None, *, caps: dict | None = None, ref: str = REF, identity: dict | None = None):
        return ledger_lib.Ledger.initialize(
            path or self.path, authorization_ref=ref, caps=ledger_lib.caps_of(caps or CAPS), identity=identity or {}
        )

    def reopen(self, *, caps: dict | None = None, ref: str = REF, identity: dict | None = None):
        return ledger_lib.Ledger.open(
            self.path, authorization_ref=ref, caps=ledger_lib.caps_of(caps or CAPS), identity=identity or {}
        )

    def counting_fake(self) -> CountingFake:
        fake = CountingFake()
        self.addCleanup(fake.stop)
        return fake

    def reserve_n(self, ledger, n: int, size: int = 100) -> None:
        for _ in range(n):
            ledger.reserve(size, invocation="inv", mode="seed", role="positive")

    @contextlib.contextmanager
    def competing_lock(self, ledger, seconds: float):
        """A REAL second process holds the ledger's flock."""
        proc = subprocess.Popen(
            [sys.executable, "-c", HOLDER, str(ledger._lock_path), str(seconds)], stdout=subprocess.PIPE, text=True
        )
        try:
            self.assertEqual(proc.stdout.readline().strip(), "locked")
            yield proc
        finally:
            proc.kill()
            proc.wait(timeout=10)
            proc.stdout.close()
            self.assertIsNotNone(proc.poll())  # the exact child we started is gone

    def guard(self, ledger, *, elapsed: float = 60, mode: str = "live") -> BudgetGuard:
        return BudgetGuard(dict(CAPS, max_elapsed_seconds=elapsed), ledger, mode=mode)


class TestInitialization(LedgerCase):
    def test_open_never_creates_and_leaves_no_trace(self):
        with self.assertRaises(ledger_lib.LedgerError) as cm:
            self.reopen()
        self.assertIn("no ledger exists", str(cm.exception))
        self.assertIn("--init-ledger", str(cm.exception))
        self.assertEqual(list(self.tmp.iterdir()), [])  # not even a lock file

    def test_initialize_writes_one_whole_record_and_open_continues_it(self):
        first = self.new_ledger()
        self.assertEqual(first.snapshot().calls, 0)
        lines = self.path.read_text().splitlines()
        self.assertEqual(len(lines), 1)
        self.assertEqual(json.loads(lines[0])["type"], "open")
        self.assertEqual(sorted(p.name for p in self.tmp.iterdir()), ["ledger.jsonl", "ledger.jsonl.lock"])  # no temp file left
        self.reserve_n(first, 2)
        again = self.reopen()
        self.assertEqual(again.snapshot().calls, 2)

    def test_initialize_refuses_every_pre_existing_thing_and_changes_nothing(self):
        cases = {
            "a live ledger": lambda: (self.new_ledger(), self.reserve_n(self.reopen(), 1)),
            "an empty file": lambda: self.path.write_text(""),
            "a file with garbage": lambda: self.path.write_text("not a ledger\n"),
            "a lock file alone": lambda: self.lock.write_text(""),
            "a symlink": lambda: os.symlink(self.tmp / "elsewhere", self.path),
        }
        for label, prepare in cases.items():
            with self.subTest(label):
                for f in self.tmp.iterdir():
                    f.unlink()
                prepare()
                before = {f.name: f.read_bytes() for f in self.tmp.iterdir() if not f.is_symlink()}
                with self.assertRaises(ledger_lib.LedgerConflict):
                    self.new_ledger()
                after = {f.name: f.read_bytes() for f in self.tmp.iterdir() if not f.is_symlink()}
                self.assertEqual(before, after)

    def test_initialize_refuses_a_directory_and_a_missing_parent(self):
        (self.tmp / "adir").mkdir()
        with self.assertRaises(ledger_lib.LedgerError):
            ledger_lib.Ledger.initialize(self.tmp / "adir", authorization_ref=REF, caps=ledger_lib.caps_of(CAPS), identity={})
        with self.assertRaises(ledger_lib.LedgerError):
            ledger_lib.Ledger.initialize(self.tmp / "nope" / "l.jsonl", authorization_ref=REF, caps=ledger_lib.caps_of(CAPS), identity={})

    def test_a_ledger_truncated_to_an_empty_file_blocks_and_is_never_reinitialized(self):
        """The coordinator's first WIP defect: a one-call ledger at cap, truncated
        to an existing empty file, reopened with calls=0 and a fresh budget."""
        caps = dict(CAPS, max_calls=1)
        ledger = self.new_ledger(caps=caps)
        ledger.reserve(1, invocation="one", mode="seed", role="positive")
        self.assertEqual(ledger.snapshot().calls, 1)
        self.path.write_text("")
        with self.assertRaises(ledger_lib.LedgerCorrupt):
            self.reopen(caps=caps)
        with self.assertRaises(ledger_lib.LedgerConflict):
            self.new_ledger(caps=caps)
        with self.assertRaises(ledger_lib.LedgerCorrupt):
            ledger.reserve(1, invocation="two", mode="seed", role="positive")  # the open handle blocks too
        self.assertEqual(self.path.read_bytes(), b"")  # nothing was written into it

    def test_a_deleted_ledger_with_its_lock_left_is_not_recreated(self):
        ledger = self.new_ledger()
        self.reserve_n(ledger, 2)
        self.path.unlink()
        with self.assertRaises(ledger_lib.LedgerCorrupt) as cm:
            self.reopen()
        self.assertIn("lock file remains", str(cm.exception))
        with self.assertRaises(ledger_lib.LedgerConflict):
            self.new_ledger()
        with self.assertRaises(ledger_lib.LedgerError):
            ledger.reserve(1, invocation="x", mode="seed", role="positive")
        self.assertFalse(self.path.exists())  # neither open, initialize nor reserve brought it back

    def test_a_reserve_can_never_create_the_ledger_it_appends_to(self):
        ledger = self.new_ledger()
        self.path.unlink()
        self.lock.unlink()
        with self.assertRaises(ledger_lib.LedgerError):
            ledger.reserve(1, invocation="x", mode="seed", role="positive")
        self.assertFalse(self.path.exists())

    def test_initialize_leaves_nothing_behind_when_it_cannot_finish(self):
        with mock.patch.object(ledger_lib.os, "link", side_effect=OSError("no hard links here")):
            with self.assertRaises(ledger_lib.LedgerError):
                self.new_ledger()
        self.assertEqual(list(self.tmp.iterdir()), [])  # no ledger, no lock, no temp file: a retry starts clean
        self.new_ledger()  # and a retry works

    def test_two_initializers_racing_yield_exactly_one_ledger(self):
        barrier = threading.Barrier(8)
        outcomes: list[str] = []

        def attempt(i: int) -> None:
            barrier.wait()
            try:
                ledger_lib.Ledger.initialize(
                    self.path, authorization_ref=f"racer-{i}", caps=ledger_lib.caps_of(CAPS), identity={}
                )
                outcomes.append("won")
            except ledger_lib.LedgerConflict:
                outcomes.append("lost")

        threads = [threading.Thread(target=attempt, args=(i,)) for i in range(8)]
        for t in threads:
            t.start()
        for t in threads:
            t.join(timeout=20)
        self.assertEqual(sorted(outcomes), ["lost"] * 7 + ["won"])
        state = ledger_lib.parse_ledger(self.path.read_text())
        self.assertTrue(state.authorization_ref.startswith("racer-"))
        self.assertEqual(sorted(p.name for p in self.tmp.iterdir()), ["ledger.jsonl", "ledger.jsonl.lock"])

    def test_a_symlinked_ledger_and_a_non_file_are_refused(self):
        target = self.tmp / "real.jsonl"
        self.new_ledger(target)
        os.symlink(target, self.path)
        with self.assertRaises(ledger_lib.LedgerError):
            self.reopen()


class TestBinding(LedgerCase):
    """A ledger belongs to ONE authorization, its four caps and one identity."""

    def test_a_changed_reference_cap_or_identity_never_resets_the_ledger(self):
        ledger = self.new_ledger()
        self.reserve_n(ledger, 3)
        before = self.path.read_bytes()
        variants = {
            "authorization_ref": dict(ref="a-different-authorization"),
            "max_calls": dict(caps=dict(CAPS, max_calls=999)),
            "max_input_tokens": dict(caps=dict(CAPS, max_input_tokens=10**12)),
            "approved_max_usd": dict(caps=dict(CAPS, approved_max_usd=10**6)),
            "max_cost_per_call_usd": dict(caps=dict(CAPS, max_cost_per_call_usd=0.00001)),
            "identity": dict(identity={"environment": {"kind": "disposable"}}),
        }
        for label, kwargs in variants.items():
            with self.subTest(label):
                with self.assertRaises(ledger_lib.LedgerConflict):
                    self.reopen(**kwargs)
                self.assertEqual(self.path.read_bytes(), before)
        self.assertEqual(self.reopen().snapshot().calls, 3)  # the original binding still works, spend intact

    def test_identity_covers_corpus_plan_provider_environment_and_endpoints_but_not_credentials(self):
        base = {
            "provider": {"base_url": "https://p.example/v1", "model": "m", "version_pin": "v1", "dimensions": 8,
                         "serving_provider": "s", "allow_fallback": False},
            "environment": {"kind": "disposable", "origin": None, "allowed_hosts": ["127.0.0.1"], "production_target_allowed": False},
            "hosted_mcp": {"endpoint_url": "http://127.0.0.1:1/mcp", "allowed_origins": ["http://127.0.0.1:1"], "credential_secret_ref": "A"},
            "empty_case_hosted_mcp": {"endpoint_url": "http://127.0.0.1:1/mcp", "allowed_origins": ["http://127.0.0.1:1"], "credential_secret_ref": "B"},
        }
        pins = dict(corpus_sha256="c" * 64, facts_sha256="f" * 64, supplemental_sha256="s" * 64)
        ident = lambda m, **over: ledger_lib.sha256_text(ledger_lib.canonical(ledger_lib.identity_of(m, **dict(pins, **over))))  # noqa: E731
        same = ident(base)

        def changed(mutate=None, **over):
            m = json.loads(json.dumps(base))
            if mutate:
                mutate(m)
            return ident(m, **over)

        for label, value in {
            "corpus": changed(corpus_sha256="d" * 64),
            "facts": changed(facts_sha256="e" * 64),
            "supplemental plan": changed(supplemental_sha256="t" * 64),
            "provider model": changed(lambda m: m["provider"].update(model="other")),
            "provider pin": changed(lambda m: m["provider"].update(version_pin="v2")),
            "environment kind": changed(lambda m: m["environment"].update(kind="local-fixture")),
            "positive endpoint": changed(lambda m: m["hosted_mcp"].update(endpoint_url="http://127.0.0.1:2/mcp")),
            "empty-case endpoint": changed(lambda m: m["empty_case_hosted_mcp"].update(endpoint_url="http://127.0.0.1:3/mcp")),
        }.items():
            self.assertNotEqual(value, same, label)
        # A seed retry uses fresh accounts, so credentials must not change the identity.
        self.assertEqual(changed(lambda m: m["hosted_mcp"].update(credential_secret_ref="C")), same)

    def test_max_elapsed_is_not_part_of_the_binding(self):
        self.new_ledger()
        ledger = self.reopen(caps=dict(CAPS, max_elapsed_seconds=1))  # per invocation, so free to differ
        self.assertEqual(ledger.snapshot().calls, 0)

    def test_a_ledger_replaced_while_a_run_is_in_progress_is_detected(self):
        ledger = self.new_ledger()
        self.reserve_n(ledger, 1)
        replacement = self.tmp / "other.jsonl"
        self.new_ledger(replacement)
        os.replace(replacement, self.path)
        with self.assertRaises(ledger_lib.LedgerConflict):
            ledger.reserve(100, invocation="x", mode="seed", role="positive")


class TestCapArithmetic(unittest.TestCase):
    caps = ledger_lib.caps_of(CAPS)

    def test_the_three_cumulative_tests(self):
        self.assertIsNone(ledger_lib.cap_reason(24, 0, 10, self.caps))
        self.assertIn("max_calls=25", ledger_lib.cap_reason(25, 0, 10, self.caps))
        tight = dict(self.caps, max_input_tokens=100)
        self.assertIsNone(ledger_lib.cap_reason(0, 90, 10, tight))
        self.assertIn("max_input_tokens=100", ledger_lib.cap_reason(0, 91, 10, tight))
        usd = dict(self.caps, approved_max_usd=0.003, max_cost_per_call_usd=0.001)
        self.assertIsNone(ledger_lib.cap_reason(2, 0, 1, usd))
        self.assertIn("approved_max_usd=0.003", ledger_lib.cap_reason(3, 0, 1, usd))


class TestCorruptionFailsClosed(LedgerCase):
    def setUp(self) -> None:
        super().setUp()
        self.ledger = self.new_ledger()
        self.reserve_n(self.ledger, 3)
        self.lines = self.path.read_text().split("\n")[:-1]  # open + 3 reservations
        self.assertEqual(len(self.lines), 4)

    def chained(self, body: dict, prev_line: str) -> str:
        record = dict(body, v=ledger_lib.LEDGER_VERSION, prev=ledger_lib.sha256_text(prev_line))
        return ledger_lib.canonical(record)

    def variants(self) -> dict[str, str]:
        a, b, c, d = self.lines
        return {
            "truncated final record": "\n".join(self.lines)[:-15],
            "final record cut and newline-terminated": "\n".join(self.lines[:3]) + "\n" + d[:20] + "\n",
            "empty file": "",
            "only a newline": "\n",
            "garbage": "not json at all\n",
            "middle record edited": "\n".join([a, b.replace('"request_bytes":100', '"request_bytes":1'), c, d]) + "\n",
            "middle record removed": "\n".join([a, b, d]) + "\n",
            "records reordered": "\n".join([a, c, b, d]) + "\n",
            "opening record removed": "\n".join([b, c, d]) + "\n",
            "duplicate record": "\n".join([a, b, b, c, d]) + "\n",
            "non-canonical whitespace": "\n".join([a, b.replace(",", ", ", 1), c, d]) + "\n",
            "unknown record type": "\n".join([*self.lines, self.chained(
                {"seq": 5, "type": "refund", "calls": 1, "request_bytes": 1, "mode": "seed", "invocation": "i", "role": "r"}, d)]) + "\n",
            "a refund forged as negative calls": "\n".join([*self.lines, self.chained(
                {"seq": 5, "type": "reserve", "calls": -1, "request_bytes": 100, "mode": "seed", "invocation": "i", "role": "r"}, d)]) + "\n",
            "an unknown mode": "\n".join([*self.lines, self.chained(
                {"seq": 5, "type": "reserve", "calls": 1, "request_bytes": 100, "mode": "refund", "invocation": "i", "role": "r"}, d)]) + "\n",
        }

    def test_parse_rejects_every_variant(self):
        for label, text in self.variants().items():
            with self.subTest(label):
                with self.assertRaises(ledger_lib.LedgerCorrupt):
                    ledger_lib.parse_ledger(text)

    def test_open_snapshot_and_reserve_block_and_write_nothing(self):
        for label, text in self.variants().items():
            with self.subTest(label):
                self.path.write_text(text)
                before = self.path.read_bytes()
                for action in (
                    self.reopen,
                    self.ledger.snapshot,
                    lambda: self.ledger.reserve(100, invocation="x", mode="live", role="positive"),
                ):
                    with self.assertRaises(ledger_lib.LedgerCorrupt):
                        action()
                self.assertEqual(self.path.read_bytes(), before)

    def test_a_corrupt_ledger_stops_the_guard_before_any_request(self):
        fake = self.counting_fake()
        for label, text in self.variants().items():
            with self.subTest(label):
                self.path.write_text(text)
                client = self.guard(self.ledger).client(fake.endpoint, TOKEN, [fake.origin], role="positive")
                with self.assertRaises(BudgetExceeded):
                    client.initialize()
                self.assertEqual(fake.wire, 0)

    def test_a_torn_write_from_a_killed_process_blocks_until_an_operator_looks(self):
        """A process killed mid-write leaves a final line with no newline."""
        with open(self.path, "ab") as f:
            f.write(b'{"v":1,"seq":5,"prev":"')
        with self.assertRaises(ledger_lib.LedgerCorrupt) as cm:
            self.ledger.reserve(100, invocation="x", mode="live", role="positive")
        self.assertIn("operator must inspect", str(cm.exception))


class TestStatedLimits(LedgerCase):
    """What the hash chain does NOT catch. The docs say so; these tests keep the
    claim no stronger than the code."""

    def test_removing_a_whole_suffix_back_to_a_valid_prefix_is_not_detected(self):
        ledger = self.new_ledger()
        self.reserve_n(ledger, 3)
        lines = self.path.read_text().split("\n")[:-1]
        self.path.write_text("\n".join(lines[:2]) + "\n")  # a rollback to a previously valid prefix
        self.assertEqual(self.reopen().snapshot().calls, 1)  # accepted: spend was rolled back, undetected

    def test_editing_only_the_final_record_is_not_detected(self):
        ledger = self.new_ledger()
        self.reserve_n(ledger, 2, size=500)
        lines = self.path.read_text().split("\n")[:-1]
        lines[-1] = lines[-1].replace('"request_bytes":500', '"request_bytes":1')  # no successor holds its hash
        self.path.write_text("\n".join(lines) + "\n")
        self.assertEqual(self.reopen().snapshot().request_bytes, 501)

    def test_the_operator_statement_names_these_limits(self):
        text = ledger_lib.OPERATOR_GUARD_STATEMENT
        for phrase in ("not tamper-proof", "older copy", "whole suffix", "did not go through this harness"):
            self.assertIn(phrase, text)
        doc = (ledger_lib.__doc__ or "")
        self.assertIn("cannot detect the removal of a\nwhole suffix", doc)
        self.assertIn("nothing\nhere claims every possible truncation is caught", doc)


class TestReservations(LedgerCase):
    def test_a_reservation_is_on_disk_before_the_request_arrives(self):
        ledger = self.new_ledger()
        fake = self.counting_fake()
        seen: list[int] = []
        original = fake.fake._handle

        def spy(handler):
            seen.append(sum(1 for line in self.path.read_text().splitlines() if '"type":"reserve"' in line))
            return original(handler)

        fake.fake._handle = spy
        client = self.guard(ledger).client(fake.endpoint, TOKEN, [fake.origin], role="positive")
        client.initialize()
        self.assertEqual(seen, [1, 2])  # by the time request N arrives, N reservations are already durable
        self.assertEqual(ledger.snapshot().calls, 2)

    def test_a_failed_durable_write_sends_nothing(self):
        ledger = self.new_ledger()
        fake = self.counting_fake()
        client = self.guard(ledger).client(fake.endpoint, TOKEN, [fake.origin], role="positive")
        with mock.patch.object(ledger_lib.Ledger, "_append", side_effect=ledger_lib.LedgerError("the ledger write failed")):
            with self.assertRaises(BudgetExceeded):
                client.initialize()
        self.assertEqual(fake.wire, 0)
        self.assertEqual(ledger.snapshot().calls, 0)

    def test_a_reservation_counts_the_exact_request_bytes_and_the_role(self):
        ledger = self.new_ledger()
        fake = self.counting_fake()
        guard = self.guard(ledger)
        guard.client(fake.endpoint, TOKEN, [fake.origin], role="positive").initialize()
        state = ledger.snapshot()
        self.assertEqual(state.calls, 2)
        self.assertEqual(state.request_bytes, guard.request_bytes())  # ledger bytes == what the client serialized
        self.assertEqual(state.attributed(guard.invocation, "positive")[0], 2)

    def test_the_cap_holds_across_separate_guards_and_reopened_handles(self):
        """The reviewer's reproduction: a per-invocation counter let three runs
        each spend the whole cap. One ledger lets them spend it once."""
        fake = self.counting_fake()
        self.new_ledger(caps=dict(CAPS, max_calls=7))
        for _ in range(3):
            ledger = self.reopen(caps=dict(CAPS, max_calls=7))
            guard = BudgetGuard(dict(CAPS, max_calls=7, max_elapsed_seconds=60), ledger, mode="live")
            client = guard.client(fake.endpoint, TOKEN, [fake.origin], role="positive")
            with contextlib.suppress(BudgetExceeded):
                for _ in range(10):
                    client.initialize()
        self.assertEqual(fake.wire, 7)
        self.assertEqual(self.reopen(caps=dict(CAPS, max_calls=7)).snapshot().calls, 7)


class TestInterruptedAndConcurrentProcesses(LedgerCase):
    def test_a_process_killed_after_reserving_leaves_its_calls_counted(self):
        caps = dict(CAPS, max_calls=5)
        ledger = self.new_ledger(caps=caps)
        child = subprocess.run(
            [sys.executable, "-c", RESERVE_THEN_DIE, str(HERE), str(self.path), REF, json.dumps(caps), "3"],
            capture_output=True, timeout=60,
        )
        self.assertEqual(child.returncode, -signal.SIGKILL)
        state = ledger.snapshot()
        self.assertEqual(state.calls, 3)  # never refunded: those requests may have left
        self.assertEqual(state.request_bytes, 300)
        # Only 2 calls of headroom remain, so a new run gets exactly 2 requests: the handshake.
        fake = self.counting_fake()
        guard = BudgetGuard(dict(caps, max_elapsed_seconds=60), self.reopen(caps=caps), mode="live")
        client = guard.client(fake.endpoint, TOKEN, [fake.origin], role="positive")
        client.initialize()
        with self.assertRaises(BudgetExceeded):
            client.initialize()
        self.assertEqual(fake.wire, 2)
        self.assertEqual(self.reopen(caps=caps).snapshot().calls, 5)

    def test_concurrent_processes_cannot_each_spend_the_remaining_cap(self):
        caps = dict(CAPS, max_calls=25)
        self.new_ledger(caps=caps)
        fake = self.counting_fake()
        start_at = time.time() + 1.5
        workers = [
            subprocess.Popen(
                [sys.executable, "-c", WORKER, str(HERE), str(self.path), fake.endpoint, fake.origin, TOKEN,
                 str(start_at), json.dumps(caps), REF],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
            )
            for _ in range(6)
        ]
        reports = []
        try:
            for w in workers:
                out, err = w.communicate(timeout=120)
                self.assertEqual(w.returncode, 0, err)
                reports.append(json.loads(out))
        finally:
            for w in workers:
                if w.poll() is None:
                    w.kill()
                    w.wait(timeout=10)
        self.assertEqual(sum(r["reserved"] for r in reports), 25)  # every process reserved; the total is the cap
        self.assertTrue(sum(1 for r in reports if r["reserved"] > 0) > 1, reports)  # the work really was shared
        self.assertEqual(fake.wire, 25)  # counted at the socket, independently of the ledger
        state = self.reopen(caps=caps).snapshot()  # and the chain still parses end to end
        self.assertEqual(state.calls, 25)
        self.assertEqual(state.reservations, 25)


class TestLockAndDeadline(LedgerCase):
    """The run's absolute deadline bounds the lock wait, and is checked again
    after the durable write. Only this process's monotonic clock and the socket
    deadline are controlled; nothing here promises a wall-clock or DNS bound."""

    def test_waiting_on_a_real_competing_process_ends_at_the_run_cap_and_sends_nothing(self):
        ledger = self.new_ledger()
        fake = self.counting_fake()
        guard = self.guard(ledger, elapsed=0.4)
        client = guard.client(fake.endpoint, TOKEN, [fake.origin], role="positive")
        with self.competing_lock(ledger, seconds=120):  # far longer than the 30 s lock timeout
            started = time.monotonic()
            with self.assertRaises(ledger_lib.LedgerDeadline):
                client.initialize()
            waited = time.monotonic() - started
        self.assertGreaterEqual(waited, 0.35)  # it really waited for the lock through the cap...
        self.assertLess(waited, 5.0)  # ...and not for the 30 s lock timeout
        self.assertEqual(fake.wire, 0)  # nothing reached the socket
        self.assertEqual(guard.calls_made(), 0)
        self.assertEqual(guard.reserved_not_sent, 0)  # nothing was reserved either
        self.assertEqual(ledger.snapshot().calls, 0)

    def test_a_shorter_wait_than_the_cap_succeeds_and_sends(self):
        ledger = self.new_ledger()
        fake = self.counting_fake()
        guard = self.guard(ledger, elapsed=30)
        client = guard.client(fake.endpoint, TOKEN, [fake.origin], role="positive")
        with self.competing_lock(ledger, seconds=0.5):
            client.initialize()  # waits for the lock, then reserves and sends
        self.assertEqual(fake.wire, 2)
        self.assertEqual(ledger.snapshot().calls, 2)

    def test_without_a_run_deadline_the_lock_timeout_applies_and_is_not_a_deadline_error(self):
        ledger = self.new_ledger()
        with mock.patch.object(ledger_lib, "LOCK_TIMEOUT_SECONDS", 0.3):
            with self.competing_lock(ledger, seconds=60):
                started = time.monotonic()
                with self.assertRaises(ledger_lib.LedgerError) as cm:
                    ledger.reserve(1, invocation="x", mode="seed", role="positive")
                self.assertNotIsInstance(cm.exception, ledger_lib.LedgerDeadline)
                self.assertLess(time.monotonic() - started, 5.0)
        self.assertEqual(ledger.snapshot().calls, 0)

    def test_a_deadline_that_has_already_passed_reserves_nothing(self):
        ledger = self.new_ledger()
        before = self.path.read_bytes()
        with self.assertRaises(ledger_lib.LedgerDeadline):
            ledger.reserve(1, invocation="x", mode="seed", role="positive", deadline=time.monotonic() - 1)
        with self.assertRaises(ledger_lib.LedgerDeadline):
            ledger.snapshot(deadline=time.monotonic() - 1)
        self.assertEqual(self.path.read_bytes(), before)

    def test_the_deadline_is_checked_again_after_the_lock_is_held_and_before_the_write(self):
        ledger = self.new_ledger()
        before = self.path.read_bytes()
        original_read = ledger_lib.Ledger._read

        def slow_read(self_):
            state = original_read(self_)
            time.sleep(0.2)  # the cap passes while the ledger is being read under the lock
            return state

        with mock.patch.object(ledger_lib.Ledger, "_read", slow_read):
            with self.assertRaises(ledger_lib.LedgerDeadline):
                ledger.reserve(1, invocation="x", mode="seed", role="positive", deadline=time.monotonic() + 0.1)
        self.assertEqual(self.path.read_bytes(), before)  # nothing was written: no reservation to retain

    def test_the_between_steps_check_reports_a_reason_instead_of_waiting_on_the_lock(self):
        ledger = self.new_ledger()
        guard = self.guard(ledger, elapsed=0.3)
        with self.competing_lock(ledger, seconds=60):
            started = time.monotonic()
            reason = guard.exhausted_reason()
            self.assertLess(time.monotonic() - started, 5.0)
        self.assertIn("deadline", reason)

    def test_a_reserve_delayed_past_the_cap_is_refused_before_it_reserves(self):
        """The coordinator's second WIP defect, as reproduced: reserve delayed
        50 ms under a 10 ms cap returned success at 63 ms."""
        ledger = self.new_ledger()
        fake = self.counting_fake()
        guard = self.guard(ledger, elapsed=0.01)
        original = ledger.reserve

        def slow(*args, **kwargs):
            time.sleep(0.05)
            return original(*args, **kwargs)

        client = guard.client(fake.endpoint, TOKEN, [fake.origin], role="positive")
        with mock.patch.object(ledger, "reserve", side_effect=slow):
            with self.assertRaises(BudgetExceeded):
                client.initialize()
        self.assertEqual(fake.wire, 0)
        self.assertEqual(ledger.snapshot().calls, 0)  # the deadline check inside reserve saw the cap had passed

    def test_a_reservation_that_lands_after_the_cap_stays_counted_and_nothing_is_sent(self):
        """The cap passes DURING the durable write: the record is on disk, it is
        not refunded, and the request never leaves."""
        ledger = self.new_ledger()
        fake = self.counting_fake()
        guard = self.guard(ledger, elapsed=0.1)
        original = ledger._append

        def slow_append(line):
            time.sleep(0.25)
            return original(line)

        client = guard.client(fake.endpoint, TOKEN, [fake.origin], role="positive")
        with mock.patch.object(ledger, "_append", side_effect=slow_append):
            with self.assertRaises(BudgetExceeded) as cm:
                client.initialize()
        self.assertIn("after the reservation was recorded", str(cm.exception))
        self.assertEqual(fake.wire, 0)
        self.assertEqual(guard.calls_made(), 0)  # the client never sent
        self.assertEqual(guard.reserved_not_sent, 1)
        self.assertEqual(ledger.snapshot().calls, 1)  # yet the reservation stays: no refund
        # A later run sees that call as spent.
        again = BudgetGuard(dict(CAPS, max_elapsed_seconds=60), ledger, mode="live")
        self.assertEqual(again.total_calls(), 1)


if __name__ == "__main__":
    unittest.main()
