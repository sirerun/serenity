"""One budget guard shared by every phase that talks to a hosted endpoint
(--seed and --live), so the manifest's caps bind the whole qualification
and not each process separately.

Enforcement happens inside MCPClient._post, on the exact serialized request,
before any counter moves and before any socket opens (precharge). A caller
cannot forget the check, and a request that would cross a cap is rejected
whole rather than sent and counted afterwards.

Units, stated once:

- max_calls counts every HTTP request, including each initialize().
- max_input_tokens is charged at ONE TOKEN PER SERIALIZED REQUEST BYTE. No
  tokenizer emits more tokens than bytes, so this is a true upper bound on
  what the request can cost, unlike the chars/4 estimate it replaces (which
  under-counts non-English text and JSON punctuation, and was only checked
  against bytes already spent). It runs roughly four times stricter than
  chars/4; size the cap from the --preflight plan's exact byte total.
- max_cost_per_call_usd is an operator ceiling per request, not an invoice.
- max_elapsed_seconds is a monotonic deadline for this invocation; every
  request's socket timeout is bounded by the time remaining.

`prior_calls` and `prior_request_bytes` carry spend recorded in the seed
receipts, so a seed run followed by a live run cannot together exceed a cap
each would satisfy alone. Elapsed time is per invocation (a wall clock
cannot be carried across processes honestly).
"""

from __future__ import annotations

import time

from .mcp_client import BudgetExceeded, MCPClient


class BudgetGuard:
    def __init__(self, budget: dict, prior_calls: int = 0, prior_request_bytes: int = 0):
        self.max_calls = int(budget["max_calls"])
        self.max_elapsed = float(budget["max_elapsed_seconds"])
        self.max_input_tokens = float(budget["max_input_tokens"])
        self.max_cost_per_call_usd = float(budget["max_cost_per_call_usd"])
        self.approved_max_usd = float(budget["approved_max_usd"])
        self.prior_calls = prior_calls
        self.prior_request_bytes = prior_request_bytes
        self.started = time.monotonic()
        self.clients: list[MCPClient] = []

    def client(self, endpoint_url: str, token: str, allowed_origins: list[str]) -> MCPClient:
        """The only way this module's callers obtain a client, so every
        client is both tracked and precharged."""
        c = MCPClient(endpoint_url, token, allowed_origins=allowed_origins, budget=self)
        self.clients.append(c)
        return c

    def calls_made(self) -> int:
        """Calls made by this invocation only."""
        return sum(c.calls_made for c in self.clients)

    def total_calls(self) -> int:
        return self.prior_calls + self.calls_made()

    def request_bytes(self) -> int:
        return sum(c.usage().request_bytes for c in self.clients)

    def response_bytes(self) -> int:
        return sum(c.usage().response_bytes for c in self.clients)

    def tokens_used(self) -> int:
        """Upper-bound input tokens charged so far, including seed receipts."""
        return self.prior_request_bytes + self.request_bytes()

    def elapsed(self) -> float:
        return time.monotonic() - self.started

    def remaining_seconds(self) -> float:
        return max(self.max_elapsed - self.elapsed(), 0.001)

    def exhausted_reason(self, next_request_bytes: int = 0) -> str | None:
        """Why the next request (of the given exact size) may not be sent,
        or None. With next_request_bytes=0 this is the cheap between-steps
        check; precharge() is the authoritative one."""
        calls = self.total_calls()
        if calls >= self.max_calls:
            return f"max_calls={self.max_calls} reached"
        if self.elapsed() >= self.max_elapsed:
            return f"max_elapsed_seconds={self.max_elapsed} reached"
        if self.tokens_used() + next_request_bytes > self.max_input_tokens:
            return (
                f"max_input_tokens={self.max_input_tokens} would be exceeded: {self.tokens_used()} charged, "
                f"next request needs {next_request_bytes}"
            )
        if (calls + 1) * self.max_cost_per_call_usd > self.approved_max_usd:
            return (
                f"approved_max_usd={self.approved_max_usd} would be exceeded by the next call "
                f"(max_cost_per_call_usd={self.max_cost_per_call_usd})"
            )
        return None

    def precharge(self, request_bytes: int) -> None:
        reason = self.exhausted_reason(request_bytes)
        if reason:
            raise BudgetExceeded(reason)

    def projected_cost_usd(self) -> float:
        """Conservative: total calls so far times the operator's per-call ceiling."""
        return round(self.total_calls() * self.max_cost_per_call_usd, 6)
