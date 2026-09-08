# E13 — Raw source retrieval consistency

On 2026-09-08, a real CLI search returned an ordinary raw source. After its
canonical source directory was removed from the synthetic brain, the same search
still returned the old indexed source text. Generic source eligibility treats an
unknown source digest as eligible. Native canonical claim projection is separately
protected by E12; raw-source chunks need their own current authority check.

- [x] T13.1 Revalidate raw-source chunks before retrieval and disclosure  Owner: pool  Est: 60m  verifies: [UC-010]  deps: [T12.1]  acc: [real CLI and MCP omit deleted or changed raw-source evidence without rebuild; newly index-only raw sources remain available to local owner search but do not reach remote recall or providers; legitimate source chunks retain their attribution and query behavior; forged stale index fields cannot confer source authority; meaningful negative controls fail and named suites disclose execution counts/skips]

Raw source history and current claim truth remain distinct. This task does not
perform source deletion or claim retraction on behalf of the operator; it checks
current canonical authority before using an existing derived chunk.

Verified 2026-09-08: CLI and real MCP stdio omit deleted source rows without
rebuild, retain index-only bytes locally, and fail explicitly on corrupt source
bytes. Every raw, memory-fact and page projection field is checked against
canonical content. The embedded Go facade uses the same local-owner eligibility
as CLI search; its provider composition keeps the separate egress policy.

Final race suite: 1,642 passing cases, 58 packages, six explicit skips. Focused
source/page authority suite: 25 passing cases, no skips. Embedded facade regression:
three passing cases, no skips; both source and claim deletion failed before the
wiring repair. Three deliberate policy faults were detected and restored.
Matching cached 10K run: 194.774 seconds, +10.09% against 176.927 seconds, below
the unchanged 20% limit; 10,000 claims, 20,050 vectors, zero live model calls.
Evidence: `docs/evals/source-retrieval.json` and `source-retrieval-budget.json`.
