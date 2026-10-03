# Independent review: journal read observation source integration

**Verdict: CLEAR for the bounded journal observation component at exact head `f3b9c26713dd790e4ed762b25de680975da7a0b3`.** The diff is based on landed main `a52554cf6199e95fa96d492469e4322881806538`. I found no critical source seam defect. This verdict covers the contracts/deletion observer and reserved journal append/seal behavior in the assigned scope; it does not qualify a production startup factory, provider/authority adapter, service, CLI, schema, module, or deployment. Root's full-module qualification remains separate and had not started at the time of these checks.

## Review evidence

I reviewed a fresh detached clone at `[external evidence store]`. It is clean at the exact SHA above. `git diff --check a52554cf6199e95fa96d492469e4322881806538..HEAD` passed. The change contains reader contract/tests, observer and reserved append/seal source/tests, and documented evidence/plan changes.

The observer validates the selected generation/head, scans the namespace for malformed and future-generation keys, traverses history in sequence, validates seals, chain hashes and writer-use provenance, checks the computed head against the selected position, and refuses objects after that position. It includes context checks and bounds generation, object, page, per-object-byte, and aggregate-byte processing. The reserved writer validates the full observed position and historical writer uses before its first PUT. Ambiguous writes retain exact pending bytes; seal retries verify the exact own seal and reject foreign seals or later tail objects. `NewJournal`/legacy `Seal` continue through the old path when no reserved observer is configured. Adapter-side transport allocation remains a separate bound as documented.

The existing focused tests cover genesis and empty successor positions, seal-only writer use, cross-generation writer reuse, malformed keys, oversized objects/pages, cancellation, generation limits, stale heads, exact pending append/seal retries, foreign seals, and read-only capability shape. I did not run a full-module build or race test; that is root's separate lane.

## Independent behavioral controls

Both mutations were applied only in the detached clone, compiled, and failed at the intended runtime assertions. I restored the exact original source after each run and verified the original SHA-256s, clean status, and exact HEAD afterward.

- Cross-generation writer guard disabled: targeted `TestNewJournalAtRejectsCrossGenerationWriterBeforePut` failed with `append err=<nil>, conditional puts=1`. Run record: `control-crossgen-red-20261003T0500Z/stage.json`; test output: `control-crossgen-red-20261003T0500Z/stdout.log`. The shared lease was won and released (`96e400b046b640c11a6866b5932f4622717f30a7`, release exit 0); load stayed below 10.
- Stale-head equality guard disabled: targeted `TestObserveRejectsStaleHeadAndPositionShape` failed with `stale head error = <nil>`. Run record: `control-stalehead-red-20261003T0501Z/stage.json`; test output: `control-stalehead-red-20261003T0501Z/stdout.log`. The shared lease was won and released (`74f9b3d64e5697bb6359d6d7dae7736240264b46`, release exit 0); load stayed below 10.

Restored file SHA-256s:

- `internal/hosted/deletion/journal_observation.go`: `42abd0a58022d494dc59a77d27594fb8fd9a85c3ad787e4f39d5afb965d86e8f`
- `internal/hosted/deletion/journal_observation_reserved.go`: `abd6b3395a659eec183107aa92eab19328cfae060f0f93652d96eac09a0778bc`

No source change, push, PR edit, merge, or claim release was made by this review.
