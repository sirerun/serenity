# MEMORY_VERBS v1 pinned fixtures (T4.20)

Vendored, unmodified, from `dndungu/gbrain@d35c9c9e441e`
(`docs/protocol/MEMORY_VERBS_v1.md`, the frozen memory-verbs contract):

- `memory-cases-upstream.json` — the 17 pinned conformance cases
  (`test/fixtures/memory-verbs/cases.json` upstream). Drives
  `TestMemoryV1AllPinnedCases`.
- `pinned-response-schemas.json` — the response-shape registry extracted
  from the pinned TypeScript operation catalog (`upstream-verbs.ts`'s
  `RESPONSE_SCHEMAS`/`ERROR_SCHEMA`). Drives `TestMemoryV1PinnedWireContracts`'s
  offline shape validation.
- `gbrain-LICENSE` — gbrain's own MIT license, carried alongside its
  fixtures per the license's own attribution requirement.

This is a narrow copy for this package's own tests, not T4.13's broader
"vendored gbrain memory-verbs cases.json" task (`docs/plans/E4-m4-serve-protocols.md`),
which still owns the full conformance-harness job.
