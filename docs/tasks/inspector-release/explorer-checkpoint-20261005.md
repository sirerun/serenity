# Explorer integration checkpoint

Source delivery remains unmerged. Integration branch: `ship/explorer-20261005`;
base: `aebf7f5a5680b8ef704cefc20338a29707012326`. Planning PR362 and brand/API
PR361 are merged. Source review is provisional, not final approval.

Verification completed: 85 Go packages passed `go test -race -short -p 1 ./...`,
three packages had no tests; vet and lint passed. Subsequent dashboard/site
race checks and hosted binary compilation passed. Frontend build and nine
helper tests passed. Documentation strict builds both fail with seven identical
baseline warnings; this is non-regression evidence, not a strict pass.

Initial headless website run: 47 passed, five failed. Four failures expose
WebGL contexts retained after repeated map/list switches; cleanup now calls
`forceContextLoss`, pending rebuilt-asset verification. One mobile assertion
looked for a desktop-only demo badge; it now checks the visible synthetic
collection label. No-WebGL fallback and pending-detail cancellation passed.
The local acceptance helper's attempted second owned brain violated the
existing one-brain-per-account constraint; that invalid setup was removed.

Component-only synthetic10K measurements: one graph6.5s, one graph/facets
pair6.7s, two pairs8.1s; sampled heap deltas29.4/38.6/44.4MB. Bounds passed,
latency target missed. Final integrated10K measurement remains required.
INS-F9 preserves target-profile latency/frame-time/resource-growth as a hard
release gate. Source merge does not authorize a performance pass or deployment.

Next: rebuild/commit assets; rerun affected browser/Go checks; run real local
synthetic-account browser acceptance and final10K qualification; record all
findings and independent exact-head code/visual review; refresh trusted holds,
perform guarded ordinary rebase merge, then verify landed tree identity.
Do not merge this checkpoint as acceptance or bypass unfinished INS-F5/F6.
Production deployment remains gated separately; no production change occurred.
