"""Durable, cumulative accounting for one qualification (T23.43).

The budget caps in a manifest bind the WHOLE qualification: the seed run, every
retry of it and every live run, in any process, in any order. A per-invocation
counter cannot do that (three `--live` runs under `max_calls=237` sent 447
requests, and two seed retries under `max_calls=50` sent 100), so every request
is reserved here first, in an append-only ledger, before any socket opens.

What the ledger is

- An append-only JSON-lines file. Each record carries the SHA-256 of the record
  before it, so a middle edit, a dropped record or a reordering is detected on
  the next read. The first record binds the ledger to ONE authorization: the
  manifest's `budget.authorization_ref`, its four spend caps, and an identity
  hash over the corpus, the supplemental plan, the declared provider pin, the
  environment and the two endpoints.
- Written under an inter-process lock (`flock` on a sibling lock file). A
  reservation reads the whole ledger, checks every cap against the CUMULATIVE
  totals, appends its record and fsyncs, and only then returns. Concurrent
  processes serialize on the lock, so they cannot each spend the remaining cap.
- Reserved before the request, never refunded. A process that dies after
  reserving leaves an uncertain call; it stays counted, because the request may
  have left. Nothing here refunds automatically.
- Fail closed. A corrupt, truncated, reordered or unreadable ledger, an EMPTY
  file, a lock that cannot be taken, and a ledger opened by a manifest with a
  different authorization, caps or identity all raise, and the caller blocks
  before any request. Changing a cap or an authorization reference never resets
  a ledger. Ledger and manifest must agree.
- Created only on purpose. A ledger begins in one place, `Ledger.initialize`
  (the CLI's `--init-ledger`), which writes a complete opening record to a temp
  file and links it into place, so a ledger is either absent or whole. It
  refuses a path where a ledger, an empty file or a lock file already exists.
  A missing ledger is never re-created by a seed or a live run, and a lock file
  that outlives its ledger blocks a new one: a new authorization gets a new
  path, chosen by whoever holds that authority.
- Bounded by the run's own deadline. The absolute `max_elapsed_seconds`
  deadline is passed into the lock wait and checked again before the record is
  written, so a long lock timeout cannot outlast a shorter run cap.

Units are the conservative ones the budget guard has always used: one call per
HTTP request (the handshake notification included), one token per serialized
request byte, and a per-call USD ceiling. `cost.actual_usd` stays null.

What the ledger is NOT: it is an operator-side accident and crash guard. It is
not tamper-proof against its owner, who can delete every file and start over,
rewrite it with fresh hashes, or restore an older copy, and it cannot see spend
that did not go through this harness. The hash chain detects an edited, dropped
or reordered record and a torn final line; it cannot detect the removal of a
whole suffix that leaves a previously valid prefix (a rollback), and nothing
here claims every possible truncation is caught. Owner tampering and rollback
are outside this guard. Receipts never establish a remaining budget: a
receipt's usage counters are only cross-checked against what this ledger
recorded for the same invocation.
"""

from __future__ import annotations

import errno
import fcntl
import hashlib
import json
import os
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path

from .mcp_client import BudgetExceeded

LEDGER_VERSION = 1
GENESIS = "0" * 64
LOCK_TIMEOUT_SECONDS = 30.0
CAP_FIELDS = ("max_calls", "max_input_tokens", "approved_max_usd", "max_cost_per_call_usd")
MODES = ("seed", "live")
_SHA256_HEX_LEN = 64

OPERATOR_GUARD_STATEMENT = (
    "The ledger is an operator-side accident and crash guard. It is not tamper-proof against its owner, who can "
    "delete or rewrite it or restore an older copy; its hash chain cannot detect the removal of a whole suffix that "
    "leaves a valid prefix; and it cannot see spend that did not go through this harness."
)


class LedgerError(BudgetExceeded):
    """The ledger cannot be used. Fixed text only; never a path, a record or
    anything an upstream sent. Subclasses BudgetExceeded so every caller that
    already treats an exhausted budget as BLOCKED treats this the same way."""


class LedgerCorrupt(LedgerError):
    pass


class LedgerConflict(LedgerError):
    pass


class LedgerDeadline(LedgerError):
    """The run's own max_elapsed_seconds deadline passed while it waited for the
    ledger. Nothing was reserved."""


def canonical(obj: object) -> str:
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def _is_int(v: object) -> bool:
    return isinstance(v, int) and not isinstance(v, bool)


def now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def caps_of(budget: dict) -> dict:
    """The four spend caps, as numbers. `max_elapsed_seconds` is deliberately
    absent: it is per invocation, a wall clock cannot be carried across
    processes honestly."""
    return {name: float(budget[name]) if name != "max_calls" else int(budget[name]) for name in CAP_FIELDS}


def identity_of(m: dict, *, corpus_sha256: str, facts_sha256: str, supplemental_sha256: str) -> dict:
    """What an authorization is FOR. Two manifests with the same authorization
    reference but a different corpus, plan, declared provider, environment or
    endpoint do not share a ledger. Credentials are not part of it: a seed
    retry uses fresh accounts and must still draw on the same budget."""
    provider = m.get("provider") or {}
    environment = m.get("environment") or {}
    endpoints = {}
    for key in ("hosted_mcp", "empty_case_hosted_mcp"):
        target = m.get(key) or {}
        endpoints[key] = {
            "endpoint_url": target.get("endpoint_url"),
            "allowed_origins": sorted(target.get("allowed_origins") or []),
        }
    return {
        "corpus_sha256": corpus_sha256,
        "facts_sha256": facts_sha256,
        "supplemental_sha256": supplemental_sha256,
        "provider": {
            k: provider.get(k)
            for k in ("base_url", "model", "version_pin", "dimensions", "serving_provider", "allow_fallback")
        },
        "environment": {
            "kind": environment.get("kind"),
            "origin": environment.get("origin"),
            "allowed_hosts": sorted(environment.get("allowed_hosts") or []),
            "production_target_allowed": bool(environment.get("production_target_allowed")),
        },
        "endpoints": endpoints,
    }


def cap_reason(calls: int, request_bytes: int, next_request_bytes: int, caps: dict) -> str | None:
    """Why one more request of the given exact size may not be reserved, or
    None. The same three cumulative tests the guard has always applied."""
    if calls >= caps["max_calls"]:
        return f"max_calls={caps['max_calls']} reached"
    if request_bytes + next_request_bytes > caps["max_input_tokens"]:
        return (
            f"max_input_tokens={caps['max_input_tokens']} would be exceeded: {request_bytes} charged, "
            f"next request needs {next_request_bytes}"
        )
    if (calls + 1) * caps["max_cost_per_call_usd"] > caps["approved_max_usd"]:
        return (
            f"approved_max_usd={caps['approved_max_usd']} would be exceeded by the next call "
            f"(max_cost_per_call_usd={caps['max_cost_per_call_usd']})"
        )
    return None


@dataclass
class LedgerState:
    ledger_id: str
    authorization_ref: str
    identity_sha256: str
    caps: dict
    calls: int = 0
    request_bytes: int = 0
    reservations: int = 0
    last_hash: str = GENESIS
    by_invocation: dict = field(default_factory=dict)  # (invocation, role) -> [calls, request_bytes]

    def attributed(self, invocation: str, role: str) -> tuple[int, int]:
        c, b = self.by_invocation.get((invocation, role), (0, 0))
        return c, b


def parse_ledger(text: str) -> LedgerState:
    """Validates every record and returns the cumulative state. Raises
    LedgerCorrupt on anything but a clean, chained, canonical ledger."""
    if not text:
        raise LedgerCorrupt("the ledger file is empty")
    if not text.endswith("\n"):
        raise LedgerCorrupt("the ledger's final record is incomplete (a write was interrupted); an operator must inspect it")
    prev, seq = GENESIS, 0
    state: LedgerState | None = None
    for raw in text.split("\n")[:-1]:
        try:
            rec = json.loads(raw)
        except ValueError as e:
            raise LedgerCorrupt("a ledger record is not valid JSON") from e
        if not isinstance(rec, dict) or canonical(rec) != raw:
            raise LedgerCorrupt("a ledger record is not in canonical form")
        seq += 1
        if rec.get("v") != LEDGER_VERSION or rec.get("seq") != seq or rec.get("prev") != prev:
            raise LedgerCorrupt("the ledger chain is broken (a record was edited, removed or reordered)")
        kind = rec.get("type")
        if seq == 1:
            if kind != "open":
                raise LedgerCorrupt("the ledger does not begin with its opening record")
            caps = rec.get("caps")
            if not (
                isinstance(rec.get("ledger_id"), str) and isinstance(rec.get("authorization_ref"), str)
                and isinstance(rec.get("identity_sha256"), str) and len(rec["identity_sha256"]) == _SHA256_HEX_LEN
                and isinstance(caps, dict) and set(caps) == set(CAP_FIELDS)
                and all(isinstance(v, (int, float)) and not isinstance(v, bool) and v > 0 for v in caps.values())
            ):
                raise LedgerCorrupt("the ledger's opening record is malformed")
            state = LedgerState(rec["ledger_id"], rec["authorization_ref"], rec["identity_sha256"], caps)
        else:
            if kind != "reserve" or state is None:
                raise LedgerCorrupt("an unknown ledger record type")
            calls, size = rec.get("calls"), rec.get("request_bytes")
            if not (
                calls == 1 and _is_int(size) and size >= 0 and rec.get("mode") in MODES
                and isinstance(rec.get("invocation"), str) and isinstance(rec.get("role"), str)
            ):
                raise LedgerCorrupt("a reservation record is malformed")
            state.calls += calls
            state.request_bytes += size
            state.reservations += 1
            slot = state.by_invocation.setdefault((rec["invocation"], rec["role"]), [0, 0])
            slot[0] += calls
            slot[1] += size
        prev = sha256_text(raw)
        if state is not None:
            state.last_hash = prev
    if state is None:
        raise LedgerCorrupt("the ledger has no opening record")
    return state


def _full_fsync(fd: int) -> None:
    """Flushes to the device where the platform can (macOS needs F_FULLFSYNC;
    plain fsync there only reaches the drive's cache)."""
    full = getattr(fcntl, "F_FULLFSYNC", None)
    if full is not None:
        try:
            fcntl.fcntl(fd, full)
            return
        except OSError:
            pass
    os.fsync(fd)


class Ledger:
    """One authorization's cumulative spend. Build with `Ledger.open`."""

    def __init__(self, path: Path, lock_path: Path, state: LedgerState) -> None:
        self.path = path
        self._lock_path = lock_path
        self.state = state
        self.reserved_here = 0

    # -- opening -----------------------------------------------------------
    @staticmethod
    def _check_path(path: Path) -> Path:
        if not path.name or path.is_dir():
            raise LedgerError("the ledger path must name a file")
        if not path.parent.is_dir():
            raise LedgerError("the ledger's directory does not exist")
        return path.with_name(path.name + ".lock")

    @classmethod
    def initialize(cls, path: str | os.PathLike, *, authorization_ref: str, caps: dict, identity: dict) -> "Ledger":
        """Begins a ledger. The ONE place one is created, and always an
        intentional act by whoever holds the authorization (the CLI's
        `--init-ledger`); a seed or a live run never comes through here.

        Refuses a path where anything already exists: a ledger, an empty or
        damaged file, a symlink, or a lock file left by a ledger that has since
        gone missing. It never overwrites, resets or renews a budget. The opening
        record is written complete to a temp file, made durable, and linked into
        place, so the path is either absent or a whole ledger, never a
        half-written one (a crash before the link leaves nothing to reset)."""
        path = Path(path)
        lock_path = cls._check_path(path)
        if os.path.lexists(path) or os.path.lexists(lock_path):
            raise LedgerConflict(
                "a ledger, a stale file or a lock file already exists at budget.ledger_path; initialization never "
                "overwrites or resets one, and a new authorization needs a new ledger path chosen on purpose"
            )
        first = {
            "v": LEDGER_VERSION, "seq": 1, "prev": GENESIS, "type": "open", "at": now_iso(),
            "ledger_id": uuid.uuid4().hex, "authorization_ref": authorization_ref,
            "identity_sha256": sha256_text(canonical(identity)), "caps": caps,
            "units": {"calls": "one per HTTP request", "tokens": "one per serialized request byte",
                      "usd": "calls x max_cost_per_call_usd (a ceiling, never actual spend)"},
        }
        data = (canonical(first) + "\n").encode("utf-8")
        temp = path.with_name(f"{path.name}.init-{uuid.uuid4().hex}")
        lock_fd = None
        try:
            # O_EXCL on the lock file is the mutual exclusion between two initializers.
            lock_fd = os.open(lock_path, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            try:
                view = memoryview(data)
                while view:
                    view = view[os.write(fd, view):]
                _full_fsync(fd)
            finally:
                os.close(fd)
            os.link(temp, path)  # atomic; fails if the path appeared meanwhile
            _fsync_dir(path.parent)
        except FileExistsError as e:
            if lock_fd is not None:
                _remove_quietly(lock_path)
            raise LedgerConflict("a ledger or lock file appeared at budget.ledger_path during initialization") from e
        except OSError as e:
            if lock_fd is not None and not os.path.lexists(path):
                _remove_quietly(lock_path)  # this call created it and no ledger stands behind it
            raise LedgerError("the ledger could not be initialized") from e
        finally:
            if lock_fd is not None:
                os.close(lock_fd)
            _remove_quietly(temp)
        return cls.open(path, authorization_ref=authorization_ref, caps=caps, identity=identity)

    @classmethod
    def open(cls, path: str | os.PathLike, *, authorization_ref: str, caps: dict, identity: dict) -> "Ledger":
        """Opens the EXISTING ledger at `path`; it never creates one. Raises
        LedgerConflict when the ledger belongs to another authorization, caps or
        identity, and LedgerCorrupt when it is damaged, empty, or gone while its
        lock file remains. A missing ledger is an error, not a fresh start: a seed
        or a live run continues an authorization, and receipts alone cannot
        establish the remaining budget."""
        path = Path(path)
        lock_path = cls._check_path(path)
        if os.path.islink(path):
            raise LedgerError("the ledger path is a symlink")
        if not os.path.lexists(path):
            # Checked before the lock file is touched, so a failed open leaves no trace.
            if os.path.lexists(lock_path):
                raise LedgerCorrupt(
                    "the ledger is missing but its lock file remains (it was deleted or moved); it is not re-created, "
                    "and a new authorization needs a new ledger path chosen on purpose"
                )
            raise LedgerError(
                "no ledger exists at budget.ledger_path; run --init-ledger first (a seed or live run continues a "
                "ledger and never starts one, and receipts alone cannot establish the remaining budget)"
            )
        identity_sha = sha256_text(canonical(identity))
        ledger = cls(path, lock_path, LedgerState("", "", "", {}))
        with ledger._locked():
            if os.path.islink(path) or not path.is_file():
                raise LedgerError("the ledger path is not a regular file")
            state = ledger._read()  # an empty or damaged file raises LedgerCorrupt here, never re-initializes
            if (
                state.authorization_ref != authorization_ref
                or state.identity_sha256 != identity_sha
                or state.caps != caps
            ):
                raise LedgerConflict(
                    "the ledger belongs to a different authorization, caps or identity than this manifest; a changed "
                    "cap or reference does not reset it, and a new authorization needs a new ledger file"
                )
            ledger.state = state
        return ledger

    # -- locking and IO ----------------------------------------------------
    def _locked(self, deadline: float | None = None):
        return _FileLock(self._lock_path, deadline)

    def _read(self) -> LedgerState:
        try:
            text = self.path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError) as e:
            raise LedgerError("the ledger cannot be read") from e
        return parse_ledger(text)

    def _append(self, line: str) -> None:
        data = (line + "\n").encode("utf-8")
        try:
            fd = os.open(self.path, os.O_WRONLY | os.O_APPEND | os.O_NOFOLLOW)  # never O_CREAT: append cannot start a ledger
        except OSError as e:
            raise LedgerError("the ledger cannot be opened for writing") from e
        try:
            view = memoryview(data)
            while view:
                written = os.write(fd, view)
                view = view[written:]
            _full_fsync(fd)
        except OSError as e:
            raise LedgerError("the ledger write failed") from e
        finally:
            os.close(fd)

    # -- reservations ------------------------------------------------------
    def snapshot(self, deadline: float | None = None) -> LedgerState:
        """The cumulative state, read under the lock. Used for reporting and
        the cheap between-steps check; `reserve` is the authoritative one.
        `deadline` is an absolute `time.monotonic()` instant that bounds the
        lock wait."""
        with self._locked(deadline):
            self.state = self._read()
            return self.state

    def reserve(
        self, request_bytes: int, *, invocation: str, mode: str, role: str, deadline: float | None = None
    ) -> LedgerState:
        """Reserves one call of `request_bytes` against the cumulative caps and
        makes the record durable BEFORE returning. Raises BudgetExceeded (a
        cap) or LedgerError (an unusable ledger); the caller sends nothing.

        `deadline` is the run's absolute `time.monotonic()` cap. It bounds the
        lock wait, and is checked once more after the lock is held and the
        ledger read, before anything is written: a run whose cap has passed
        reserves nothing. A record written just before the cap stays counted
        (there is no refund); the caller rechecks the clock before it sends."""
        if mode not in MODES or not _is_int(request_bytes) or request_bytes < 0:
            raise LedgerError("an invalid reservation was requested")
        with self._locked(deadline):
            state = self._read()
            self._check_bound(state)
            reason = cap_reason(state.calls, state.request_bytes, request_bytes, state.caps)
            if reason:
                self.state = state
                raise BudgetExceeded(reason)
            _check_deadline(deadline)
            record = {
                "v": LEDGER_VERSION, "seq": state.reservations + 2, "prev": state.last_hash, "type": "reserve",
                "at": now_iso(), "invocation": invocation, "mode": mode, "role": role, "calls": 1,
                "request_bytes": request_bytes, "pid": os.getpid(),
            }
            self._append(canonical(record))
            self.reserved_here += 1
            self.state = self._read()
            return self.state

    def _check_bound(self, state: LedgerState) -> None:
        if self.state.ledger_id and state.ledger_id != self.state.ledger_id:
            raise LedgerConflict("the ledger was replaced while the run was in progress")

    # -- provenance --------------------------------------------------------
    def provenance(self, invocation: str, role: str) -> dict:
        """What a seed receipt records about its own spend. The values come from
        the ledger, not from the client's counters."""
        state = self.snapshot()
        calls, size = state.attributed(invocation, role)
        return {
            "ledger_id": state.ledger_id, "identity_sha256": state.identity_sha256, "invocation": invocation,
            "role": role, "calls": calls, "request_bytes": size,
        }

    def receipt_problems(self, receipt: dict, role: str) -> list[str]:
        """Cross-checks a seed receipt against this ledger. A receipt with no
        ledger provenance, from another ledger, or claiming usage the ledger
        never recorded cannot stand in for spend."""
        prov = receipt.get("ledger")
        if not isinstance(prov, dict):
            return ["the receipt has no ledger-backed provenance, so it cannot establish what was spent (re-seed under a ledger)"]
        state = self.snapshot()
        problems = []
        if prov.get("ledger_id") != state.ledger_id or prov.get("identity_sha256") != state.identity_sha256:
            problems.append("the receipt was produced under a different ledger than this manifest's")
            return problems
        if prov.get("role") != role or not isinstance(prov.get("invocation"), str):
            problems.append("the receipt's ledger provenance names a different role or no invocation")
            return problems
        calls, size = state.attributed(prov["invocation"], role)
        usage = receipt.get("usage") or {}
        if (prov.get("calls"), prov.get("request_bytes")) != (calls, size):
            problems.append("the receipt's recorded spend does not match what the ledger holds for that invocation")
        elif (usage.get("calls_made"), usage.get("request_bytes")) != (calls, size):
            problems.append("the receipt's usage counters do not match the ledger (edited counters cannot earn budget)")
        return problems

    def report(self, invocation: str | None = None) -> dict:
        """A path-free summary for a result file."""
        state = self.snapshot()
        out = {
            "file": self.path.name,
            "ledger_id": state.ledger_id,
            "authorization_ref": state.authorization_ref,
            "identity_sha256": state.identity_sha256,
            "cumulative_calls": state.calls,
            "cumulative_request_bytes": state.request_bytes,
            "caps": state.caps,
            "remaining_calls": max(state.caps["max_calls"] - state.calls, 0),
            "reservations_this_process": self.reserved_here,
            "refunds": "none: a reserved call whose outcome is unknown stays counted",
            "guard": OPERATOR_GUARD_STATEMENT,
        }
        if invocation is not None:
            out["invocation"] = invocation
            out["invocation_calls"] = sum(v[0] for (inv, _r), v in state.by_invocation.items() if inv == invocation)
        return out


def _check_deadline(deadline: float | None) -> None:
    if deadline is not None and time.monotonic() >= deadline:
        raise LedgerDeadline("the run's max_elapsed_seconds deadline passed before the ledger could be used; nothing was reserved")


def _remove_quietly(path: Path) -> None:
    try:
        os.unlink(path)
    except OSError:
        pass


def _fsync_dir(directory: Path) -> None:
    try:
        fd = os.open(directory, os.O_RDONLY)
    except OSError:
        return
    try:
        os.fsync(fd)
    except OSError:
        pass
    finally:
        os.close(fd)


class _FileLock:
    """`flock` on a sibling lock file, with a deadline. Blocks other processes
    for the few milliseconds one reservation takes; never held across a
    network call."""

    def __init__(self, path: Path, deadline: float | None = None) -> None:
        self.path = path
        self.deadline = deadline
        self.fd: int | None = None

    def __enter__(self):
        _check_deadline(self.deadline)
        try:
            self.fd = os.open(self.path, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
        except OSError as e:
            raise LedgerError("the ledger lock file cannot be opened") from e
        # The wait ends at the lock timeout or the run's own deadline, whichever
        # is sooner: a long lock timeout must not outlast a shorter run cap.
        give_up = time.monotonic() + LOCK_TIMEOUT_SECONDS
        if self.deadline is not None:
            give_up = min(give_up, self.deadline)
        while True:
            try:
                fcntl.flock(self.fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                return self.fd
            except OSError as e:
                if e.errno not in (errno.EWOULDBLOCK, errno.EAGAIN):
                    self._release()
                    raise LedgerError("the ledger lock could not be taken") from e
                now = time.monotonic()
                if now >= give_up:
                    self._release()
                    if self.deadline is not None and now >= self.deadline:
                        raise LedgerDeadline(
                            "the run's max_elapsed_seconds deadline passed while it waited for the ledger lock; "
                            "nothing was reserved"
                        ) from e
                    raise LedgerError("the ledger lock could not be taken within the timeout (another run holds it)") from e
                time.sleep(min(0.005, max(give_up - now, 0.0005)))

    def _release(self) -> None:
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None

    def __exit__(self, *_exc) -> None:
        if self.fd is not None:
            try:
                fcntl.flock(self.fd, fcntl.LOCK_UN)
            finally:
                self._release()
