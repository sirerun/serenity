# Fresh memory semantic indexing review

Local qualification, 2026-09-10, on the CLI ownership/keyed recovery stack.

- Full race suite: 1,811 test/subtest passes, zero failures, six skips. Skips
  remain the opt-in 10K import budget, deliberately broken conformance build,
  crash helper, external gbrain conformance and two unstructured drift cases.
- Lint reports zero issues; vet and builds pass. Shared build lease released.
- Two independently initialized HTTP MCP clients used a real loopback embedding
  provider (all-minilm:22m). Fresh remember returned semantic readiness; a lexical
  control missed the target; semantic recall ranked it first without stopping or
  reindexing the server. Withdrawal hid it from the second client. Seven proof
  assertions passed on a disposable four-record brain.
- Focused tests cover exact-key vector reuse, durable success despite provider
  failure followed by retry recovery, private/index-only exclusion, eligible
  positive control, withdrawal ordering, cancellation and TTL crossing before
  egress or during embedding.
- Read-only peer review caught required v1 additions and TTL races. Both response
  additions are now optional, and expiry is checked around provider work with
  remaining lifetime bounding its context. Final source review found no concrete
  blocker. It did not execute tests. The TTL test advances a clock; the waiting
  provider test proves caller-deadline cancellation rather than independently
  timing the TTL-derived deadline.
- One existing acceptance test initially failed because the degraded indexing
  message lost its sync recovery hint. Restored the hint before qualification.

This is retrieval mechanics evidence, not an effectiveness benchmark, hosted
Ajent gateway, team ACL proof or release. Only an explicitly configured provider
is used. Synchronous indexing can occupy the writer queue up to the ten-second
context deadline (or remaining TTL); providers must honor cancellation. Canonical
success and search readiness remain separate. Optional readiness absence from
older v1 servers means unknown, never confirmed semantic availability.
