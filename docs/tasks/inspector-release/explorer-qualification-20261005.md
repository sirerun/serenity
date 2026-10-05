# Explorer source qualification

Target base: `aebf7f5a5680b8ef704cefc20338a29707012326`. Source includes the owner-gated inspector API, same-origin private explorer, separate public synthetic demo, bounded paging, dataset-wide time/type facets, graph/list/detail, cancellation and session clearing. No customer records or model calls were used. Source delivery is separate from production release.

The original full `go test -race -short -p 1 ./...` passed in 85 packages (three without tests); vet and lint passed. Subsequent dashboard/site race checks, fixture compilation and embedded binary build passed. Frontend nine helper tests, pinned build and static site check passed. Final browser receipts and exact-head review are recorded in the delivery PR after completion, never inferred from this document.

The final synthetic 10K measurement exercised the actual dashboard handler with one CPU. A single graph read took 3.434308s with sampled additional heap 29,193,128 bytes; a subsequent graph/facets pair took 2.194025333s with 33,154,816 bytes; two concurrent pairs took 2.570808208s with 39,186,960 bytes. The test passed response/core/paging/heap ceilings. Ordering warmed the store; these values do not prove target-service first usable view, filter latency, frame rate, browser heap or host headroom. The earlier colder component pair around 6.7s remains a miss. INS-F9 remains a hard release gate.

Native browser acceptance uses four reduced-motion public viewport profiles (390, 1024, 1440 and 2880 pixels) and three signed-in HTTP fixture profiles (400 and 1280 pixels, including dark). Tests exercise real graph rendering and gestures, list paging/core counts, time/Unknown facets, inert adversarial text, source detail, foreign denial, empty other-account data and expired-session clearing. Fixtures are test-only, stop normally, and never create production authentication routes. The existing one-brain-per-account invariant prevents a second owned-brain fixture; account invalidation and abort epochs cover the supported identity boundary.

Accepted findings have explicit fix/verification/re-review dependencies in the plan: detail reset now asserts the actual empty note card; initial phone camera fits label extents and separates instructions; graph cleanup releases WebGL contexts and preserves camera state across detail changes. A preliminary full browser run was interrupted after existing landing navigations timed out waiting for remote background media. The test now waits for DOM readiness and retains all UI assertions; the fresh full run must pass without retries or skips.

Both baseline and candidate strict documentation builds report seven identical existing link warnings. This is a failing strict build, not a pass. They concern older recovery/startup evidence links and repository-source links outside the documentation tree, with identical target files at the base. The bounded inspector source change does not alter those contracts or targets. Proposed source-merge disposition is unchanged-baseline non-regression, subject to independent evidence review; residual B6/B7 documentation and hosted surface qualification remain open.

Legacy hosted signup browser startup fails because the unchanged CLI intentionally requires an admitted journal dependency factory. Three startup cases fail and three dependent cases do not run. The new signed-in explorer checks use the actual local dashboard handler with a synthetic account; they do not qualify that CLI, production startup, login/dashboard release, journal factory or recovery authority. B6/B7 and R0 remain blocked. No factory guard, branch protection, billing setting or deployment policy was weakened.

Reproducible generated asset bytes, final test counts, independent reviewers, exact head/base, trusted hold check and landed tree identity must be attached to the ordinary guarded rebase-merge receipt. GitHub billing-only failure is documented separately from passing local checks under the existing ADR024 policy. No production deployment is claimed.

## Final local receipts

Source and generated assets: `82520ceb4f00f4d5c909518178bcce765a17296f`; base remains `aebf7f5a5680b8ef704cefc20338a29707012326`. Private SSD artifact names below are receipts, not repository links.

| Check | Actual result | Receipt |
| --- | --- | --- |
| `npm test --prefix web/inspector` | 9 passed, zero skipped | helper-final.log |
| `npm run build --prefix web/inspector`, repeated | passed; tracked generated assets unchanged after rebuild | reproducible-build.log |
| `python3 scripts/site/check.py` | 25 pages, 410 links passed | command output |
| `npm run test:site -- --workers=1` plus `node tests/site/check-results.js` | 60 passed, 15 distinct flows across four profiles, zero skipped | site-list-final.log; site-list-final-results |
| `npm run test:hosted -- explorer.spec.js --workers=1` with explicit compiled fixture binary | 6 passed, zero skipped across three profiles | hosted-list-final.log; hosted-list-final-results |
| `go test -race -p 1 -count=1 ./internal/hosted/dashboard ./site` | both packages passed against rebuilt embedded assets | race-list-final.log |
| final strict docs build | failed with identical seven baseline warnings; no new warning | docs-final.log compared with docs-baseline.log |

Final browser fixtures were recompiled from the source candidate with one-core Go settings. Signed-in screenshots include bounded paged list and populated detail, plus separate detail captures. Tests enforce the480px list ceiling, count all223 eligible core facts separately from223 context sources, load all pages explicitly, and verify public/private brand destinations. Public map screenshots cover all four viewport profiles and no-WebGL fallback, late detail invalidation and20 reconstructions; pinch/wheel, orbit and Home are actual rendered-image checks.

The full 85-package race-short, vet/lint and10K receipts originate from earlier integrated revisions (full race before responsive CSS/JS changes, final10K at421a1c7). API/budget-test production Go inputs are unchanged through82520ceb; changed embedded assets and browser acceptance were checked again at82520ceb. This is scoped evidence reuse, not a claim that historical checks were rerun at the final docs-only head. The exact final reviewed source/doc SHA and independent decisions will be in the PR receipt.

Strict documentation warning disposition is limited to source merge: seven unchanged warnings belong to pre-existing recovery evidence/source links outside this inspector change. No failing strict build is marked passed; independent review must explicitly accept non-regression for this bounded source gate. B6/B7 remain open for broader hosted/docs acceptance and production release.
