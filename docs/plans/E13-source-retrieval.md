# E13 — Raw source retrieval consistency

On 2026-09-08, a real CLI search returned an ordinary raw source. After its
canonical source directory was removed from the synthetic brain, the same search
still returned the old indexed source text. Generic source eligibility treats an
unknown source digest as eligible. Native canonical claim projection is separately
protected by E12; raw-source chunks need their own current authority check.

- [ ] T13.1 Revalidate raw-source chunks before retrieval and disclosure  Owner: pool  Est: 60m  verifies: [UC-010]  deps: [T12.1]  acc: [real CLI and MCP omit deleted or changed raw-source evidence without rebuild; newly index-only raw sources remain available to local owner search but do not reach remote recall or providers; legitimate source chunks retain their attribution and query behavior; forged stale index fields cannot confer source authority; meaningful negative controls fail and named suites disclose execution counts/skips]

Raw source history and current claim truth remain distinct. This task does not
perform source deletion or claim retraction on behalf of the operator; it checks
current canonical authority before using an existing derived chunk.
