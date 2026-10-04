# PR #358 independent source review

**Verdict: CLEAR for the narrow snapshot pin-owner and backup release-recovery source components at this exact head.** This does not authorize merge by itself, production READY/effects, a startup factory or epoch owner, provider/runtime acceptance, CI acceptance, or physical capacity qualification.

## Exact source and evidence boundary

Reviewed PR #358 at head `8321d6ee229ce01879158bb8ea76b784d0c3f26d`, tree `1e9e2d260f89361dc2569ca3e3d13cf2738189b0`, against base `bee4790cf9e53e858802b059f3e6f0af38153e41`. The isolated review clone was clean after each mutation and at final verification. Candidate Go/module bytes match the qualified `c916bcd7ed30592efb065c876455d1bb51509523`; the current Go-source fingerprint is `5176070520a9496599cbf0aabe44cfcc2daae34fa26681c13b231f6c4d1c0425`.

The coordinator's integrated full qualification receipt `results.json` in external archive `serenity-recovery-pin-owner-final-validation-v2-20261003` records 3,160 passing test/subtest events across 84 packages, nine test/subtest skips, four no-test packages, and passing full-module race, vet, lint and Linux ARM64 compilation. Its SHA-256 is `7a5ab72977d7c2e549925fe0d380c03edec13064ae70cdbf6f850f76f15059c4`. All four full-run leases were released. This is local source evidence; the recorded GitHub account billing lock prevented CI execution.

## Independent controls

I independently mutated only the isolated clone, ran each focused test with the private APFS fixture environment, and restored the exact source bytes after each run. Every RED below exited at its intended behavioral assertion under a WON build lease; each release exited 0 and each stage confirmed source stability during execution.

- The owner absence-proof mutant bypassed `proof.Consume` before cancellation append. `TestPinOwnerCancelRequiresConsumedProducerAbsenceProof` failed with `zero/unconsumed absence proof accepted: <nil>`. Lease `ed34bb2413e921732b7bfb7d470da88fafaa9c99`; stage `absence-proof-red-stage`.
- The producer receipt mutant deleted the exact RELEASED receipt after owner completion. `TestPinOwnerBackupReleaseReceiptSurvivesRepeatedReconcileAndReopen` failed when reopened producer reconciliation could no longer match the committed owner attempt. Lease `d623239c6b7b282a559106a9932e268695db7718`; stage `receipt-retention-red-stage-2`.
- The pre-owner peak mutant removed the capacity reservation before owner release authorization while leaving later capacity checks intact. `TestPinOwnerReleasePeakBudgetRefusalPrecedesOwnerReleaseBegin` failed with `budget gap did not refuse before release: <nil>`. Lease `412554c425089564951db328e0549d1a0ef8b0ba`; stage `pre-owner-peak-red-stage`.
- The actual writer-order mutant moved RELEASED tombstone publication before the live lease removal in `releaseLease`. The partial-prefix subtest `TestSnapshotLeaseCrashBarrierMatrix/snapshot_release_tombstone_temp_created#01` failed at its explicit live-absence witness: `write reachable release-tombstone temp prefix: hosted/backup: verified snapshot lease conflict`. The plain phase case passed. Lease `0a437ccd07e64a74e86a68679759534cd00c1c4c`; stage `tombstone-order-red-stage-2`.

After restoration, the exact-head three-package tagged race run (`go test -race -tags=hostedtest ./internal/hosted/backup ./internal/hosted/recovery ./internal/hosted/testhooks`) passed. It executed the tagged subprocess crash matrix and had no cached test results. Lease `53c62379d67190427dfc43e8df7f76eb91a8ee40`, exit 0, exact release exit 0, source fingerprint unchanged. Stage `restored-tagged-race-stage`.

The first absence-proof attempt is preserved but excluded: it omitted the private fixture environment and therefore skipped the targeted test despite Go exiting 0. A first tombstone-order mutant changed only the later reconciliation path and did not qualify the actual live-removal ordering; it too is excluded. Neither setup outcome is counted as a RED.

## Source review and limits

The owner component keeps direct owner opens inactive and activates only through the verified producer/owner pair gate. Active operations recheck the current superblock and identities under the shared lock; unsupported future formats and mismatched tuples fail closed. Cancellation requires a consumed, exact producer absence proof. The producer requires fresh exact owner authorization, verifies the live/store identities and record bytes before deletion, retains the exact terminal receipt for later owner reconciliation, and reserves the complete bounded release metadata peak before owner release begins. The tagged crash fixtures use captured identities and actual serializer/call sites; the tombstone partial fixture explicitly refuses a live lease. This is reachable-prefix testing, not proof of interruption inside `RemoveAll` or power-loss persistence.

The accepted frozen boundary remains intact: owner `RESERVED`/`PIN_PENDING` records do not mint a `PinID`; unbound tuples are refused and resolved through exact attempt history. The producer's 4,096-entry journal cap, checked accounting and permanent terminal receipt retention fail closed without compaction or orphan repair. The matrix does not grant startup, READY, effect authorization, or provider authority. An 8 GiB local fixture is not physical expanded-destination quota evidence. Those broader runtime, provider, CI, capacity and acceptance gates remain open.

The changed documentation preserves the separate T-PNO.4/T-SPS.4 independent review and subsequent normal-merge/landed-verification dependencies, records current integrated checks rather than treating historical baseline failures as current, and does not include private workspace or fixture paths in the new component receipt. No source or module bytes changed in the review clone.


## Evidence handling

Raw stage JSON, command stdout/stderr, lease claim/release logs, and mutant patches are retained in the external review evidence archive. This report is the sanitized narrative; its SHA-256 identifies this report only, not the raw artifacts. The coordinator's raw full-run `results.json` has the SHA-256 stated above. No private filesystem path or fixture directory is included in this report.
