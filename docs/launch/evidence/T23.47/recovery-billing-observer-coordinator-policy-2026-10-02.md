# Recovery billing observer: same-second cross-kind policy

Coordinator decision, 2026-10-02. Preserve the frozen contract's conservative treatment of differing selected-subscription current status projections at the same second, including created and updated event types. Do not use inferred provider causality to grant eligibility when those timestamped projections disagree. Matching created/updated active or trialing reset outcomes remain acceptable because they establish the same reset anchor; identical duplicate updates remain acceptable.

This is an explicit conservative observation policy, not a claim that subscription creation and later updates lack causal order. The independent f29 rereview initially raised the created-active/updated-past_due example as a further blocker, then recognized that creation causally precedes updates of that subscription ID. Both findings are retained in its external report. This coordinator decision resolves the contract interpretation before source acceptance; it does not alter provider truth or infer an alternative timeline.

The added current-status group rejects different validated selected outcomes at one second before choosing failure/reset anchors. The public HTTP control copied from the independent reviewer failed against unchanged f29 source with a populated eligible observation and passes with the group added. Separate active/active and trialing/trialing controls succeed and retain the first post-reset failure anchor.

Focused verification after the added guard and tests: all past-due boundary tests pass; billing package race passes in 10.205s; vet passes; lint reports 0 issues. The pre-fix Go test exit was 1 at its intended nonzero-observation assertion. The surrounding initial transcript-validation script also exited 1 because it expected different assertion wording; that script error is not the regression evidence. The captured actual Go failure is preserved.

Evidence SHA-256:
- root-created-reset-update-red-20261002.log: 3bc82bb2dba6b7d67414257090e03041c8d4a8a26d2ffcf039cc7bd6726ba9b1
- root-created-update-boundary-green-20261002.log: 64abd1984a1f9bd67dc327988cfd78def831ef1db1179c57a15ad11367d648b8
- root-created-update-race-20261002.log: 7a72e21c500d7fdcc99dd81018f015460f74b18a3d1980d12d57a29526121f54
- root-created-update-vet-20261002.log: 1d4bb8a0125d0a1055200abd3f44850c4e7f5e1c0deaf9dd738af177822384a7
- root-created-update-lint-20261002.log: b0b24d4d9bc6a341f0d8b3a14d3b37b8346abb3be29549f0559ef849b629241b

The earlier full Go ladder passed assembly 9d63fd9557188b63be4c7dfab898174d41a96a80 (f29 source) under exact build lease 628d38e32d3c013796df88bd9e7903816ebdaed2, which was released. Those historical passing checks do not qualify this additional source change or lift a review hold. Fresh full checks and independent rereview remain required for the assembled final candidate. Original held 1cb source must not merge. No live provider, credential, activation, deployment or spend action occurred.
