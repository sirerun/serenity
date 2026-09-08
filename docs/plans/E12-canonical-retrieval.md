# E12 — Native canonical claim retrieval

On 2026-09-08, a real CLI confirmed and committed a human assertion through the
extraction inbox. `serenity search "Human Company"` then returned no results.
Rebuilding indexes canonical claim rows but only indexes title/summary text for
native entity pages; the edited assertion has no matching raw source text. Imported
claims already have derived per-claim chunks with canonical eligibility checks.

- [ ] T12.1 Make native canonical facts retrievable with current eligibility  Owner: pool  Est: 90m  verifies: [UC-005, UC-010]  deps: [T11.3]  acc: [a confirmed human assertion and active fence/shard facts are found by real local CLI search and eligible MCP recall; remote recall and provider embedding exclude private, index-only, retracted, superseded, expired, changed and deleted evidence even with a stale index; derived chunks rebuild deterministically and preserve attribution; meaningful negative controls fail and named suites disclose executed counts/skips]

Scope is derived retrieval wiring for canonical facts, not new canonical storage,
live model-quality claims or automatic resolution of existing contradictions.
