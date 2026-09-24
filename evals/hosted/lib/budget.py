"""One budget guard shared by every phase that talks to a hosted endpoint
(--seed and --live). The manifest's caps bind the whole qualification, so the
cumulative accounting lives in a durable ledger (lib/ledger.py) shared by every
invocation and every process, and this guard reserves against it.

Enforcement happens inside MCPClient._send, on the exact serialized request,
before any counter moves and before any socket opens (precharge). A caller
cannot forget the check, and a request that would cross a cap is rejected
whole rather than sent and counted afterwards. With a ledger, precharge is a
durable reservation taken under an inter-process lock: the record is on disk
before the request leaves, and it is never refunded, so a crash after it leaves
an uncertain call that stays counted.

Units, stated once:

- max_calls counts every HTTP request, including each initialize() and its
  notifications/initialized follow-up.
- max_input_tokens is charged at ONE TOKEN PER SERIALIZED REQUEST BYTE. No
  tokenizer emits more tokens than bytes, so this is a true upper bound on
  what the request can cost, unlike the chars/4 estimate it replaces (which
  under-counts non-English text and JSON punctuation, and was only checked
  against bytes already spent). It runs roughly four times stricter than
  chars/4; size the cap from the --preflight plan's exact byte total.
- max_cost_per_call_usd is an operator ceiling per request, not an invoice.
- max_elapsed_seconds is a monotonic deadline for this invocation; every
  request's socket timeout is bounded by the time remaining. Elapsed time is
  per invocation (a wall clock cannot be carried across processes honestly).
  The absolute deadline is handed to the ledger, so a wait for its lock ends at
  the run's cap and not at the lock timeout, and the clock is read again AFTER
  the reservation is durable and before the request goes out. A reservation
  that lands past the cap stays counted (no refund) and nothing is sent. What
  is controlled is this process's monotonic clock around the reservation and
  the socket deadline; nothing here promises a wall-clock or DNS bound beyond
  that.

Without a ledger the guard counts only this invocation. That mode exists for
unit tests of the guard itself; `--seed` and `--live` always pass a ledger.
"""

from __future__ import annotations

import time
import uuid

from .ledger import Ledger, cap_reason
from .mcp_client import BudgetExceeded, MCPClient


class _RoleBudget:
    """What one client sees: precharge attributed to a role, and the time left."""

    def __init__(self, guard: "BudgetGuard", role: str) -> None:
        self._guard = guard
        self._role = role

    def precharge(self, request_bytes: int) -> None:
        self._guard.precharge(request_bytes, role=self._role)

    def remaining_seconds(self) -> float:
        return self._guard.remaining_seconds()


class BudgetGuard:
    def __init__(self, budget: dict, ledger: Ledger | None = None, *, mode: str = "live", invocation: str | None = None):
        self.max_calls = int(budget["max_calls"])
        self.max_elapsed = float(budget["max_elapsed_seconds"])
        self.max_input_tokens = float(budget["max_input_tokens"])
        self.max_cost_per_call_usd = float(budget["max_cost_per_call_usd"])
        self.approved_max_usd = float(budget["approved_max_usd"])
        self.ledger = ledger
        self.mode = mode
        self.invocation = invocation or uuid.uuid4().hex
        self.started = time.monotonic()
        self.deadline = self.started + self.max_elapsed  # absolute, on this process's monotonic clock
        self.reserved_not_sent = 0  # reservations that landed past the cap: counted, never sent
        self.clients: list[MCPClient] = []
        self._local_calls = 0
        self._local_bytes = 0

    def client(self, endpoint_url: str, token: str, allowed_origins: list[str], role: str = "") -> MCPClient:
        """The only way this module's callers obtain a client, so every
        client is both tracked and precharged."""
        c = MCPClient(endpoint_url, token, allowed_origins=allowed_origins, budget=_RoleBudget(self, role))
        self.clients.append(c)
        return c

    # -- this invocation ---------------------------------------------------
    def calls_made(self) -> int:
        """Calls made by this invocation only."""
        return sum(c.calls_made for c in self.clients)

    def request_bytes(self) -> int:
        return sum(c.usage().request_bytes for c in self.clients)

    def response_bytes(self) -> int:
        return sum(c.usage().response_bytes for c in self.clients)

    # -- cumulative --------------------------------------------------------
    def total_calls(self) -> int:
        """Calls charged to the whole qualification: every invocation in the
        ledger, or just this one when there is no ledger."""
        return self.ledger.state.calls if self.ledger is not None else self._local_calls

    def tokens_used(self) -> int:
        """Upper-bound input tokens charged so far, across the qualification."""
        return self.ledger.state.request_bytes if self.ledger is not None else self._local_bytes

    def elapsed(self) -> float:
        return time.monotonic() - self.started

    def remaining_seconds(self) -> float:
        return max(self.max_elapsed - self.elapsed(), 0.001)

    def _caps(self) -> dict:
        if self.ledger is not None:
            return self.ledger.state.caps
        return {
            "max_calls": self.max_calls, "max_input_tokens": self.max_input_tokens,
            "approved_max_usd": self.approved_max_usd, "max_cost_per_call_usd": self.max_cost_per_call_usd,
        }

    def exhausted_reason(self, next_request_bytes: int = 0) -> str | None:
        """Why the next request (of the given exact size) may not be sent,
        or None. This is the cheap between-steps check; precharge() is the
        authoritative one. With a ledger it reads the cumulative state fresh, so
        another process's spend is seen."""
        if self.elapsed() >= self.max_elapsed:
            return f"max_elapsed_seconds={self.max_elapsed} reached"
        if self.ledger is not None:
            try:
                self.ledger.snapshot(deadline=self.deadline)
            except BudgetExceeded as e:
                return str(e)
        return cap_reason(self.total_calls(), self.tokens_used(), next_request_bytes, self._caps())

    def precharge(self, request_bytes: int, role: str = "") -> None:
        if self.elapsed() >= self.max_elapsed:
            raise BudgetExceeded(f"max_elapsed_seconds={self.max_elapsed} reached")
        if self.ledger is not None:
            self.ledger.reserve(request_bytes, invocation=self.invocation, mode=self.mode, role=role, deadline=self.deadline)
            if self.elapsed() >= self.max_elapsed:
                # The record is durable and stays counted: the request never left, but nothing refunds it.
                self.reserved_not_sent += 1
                raise BudgetExceeded(
                    f"max_elapsed_seconds={self.max_elapsed} reached after the reservation was recorded; "
                    "the reserved call stays counted and was not sent"
                )
            return
        reason = cap_reason(self._local_calls, self._local_bytes, request_bytes, self._caps())
        if reason:
            raise BudgetExceeded(reason)
        self._local_calls += 1
        self._local_bytes += request_bytes

    def projected_cost_usd(self) -> float:
        """Conservative: total calls so far times the operator's per-call ceiling."""
        return round(self.total_calls() * self.max_cost_per_call_usd, 6)
