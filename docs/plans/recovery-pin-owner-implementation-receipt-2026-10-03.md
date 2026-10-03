# Recovery snapshot pin owner implementation receipt

Status: implementation WIP, independently reviewable, not source acceptance. The source remains subject to root integration, review, and full-module qualification. This component does not authorize production READY, effects, or a general recovery release.

## Source identity

Base: `1ab1ce39877d62cc048dab3f3a825885c63cf314`. WIP implementation commit: `41a9cf4e8477a41e23b6ecd9aaa1217bbafdeb04` on `implementation/recovery-pin-owner-20261003`, tree `d2cbfec951d60f3f3cd9032540820ff8020bb6bd`. The committed change contains six owned `internal/hosted/recovery/pin_owner*.go` files only. The focused stages below ran immediately before the commit against identical source bytes; the restored source fingerprint is `158a74d15c682ad77739f5347f296e8e67702df68b249cf602cb49f7b7994080`, matching the pre-mutation package/race/vet/lint stages. The worktree is clean; the external receipt and stage logs are not repository files.

## Implementation and focused evidence

The owner persists bounded, canonical, checksummed versioned records with exact transition and tuple validation, no-replace publication, fsync/readback, bounded fresh directory scans, root/lock identity checks, lifecycle replay, cancellation-proof consumption, pin reservation/commit, abandonment, release reconciliation, and a private actual backup+owner pair gate. Tests use the actual backup store and verify reopen/replay and child-process crash recovery for owner-owned CAS/write boundaries, proof consumption, and pin handoff.

On the restored fingerprint above, the following focused stages passed with exact build-lease acquisition and release (`release_exit=0`):

- `package-tests-final2` (`go test ./internal/hosted/recovery`): lease `ef8f685e42110fae1bf158c142a55a947a09da70`.
- `race-final-current` (`go test -race ./internal/hosted/recovery`): lease `4aec5553fec7b9882ca9b46c2cb3c3a1174349d6`.
- `vet-final-current` (`go vet ./internal/hosted/recovery`): lease `2ecdbfd68846bd0d187f4b0283a8c5aa470503dd`.
- `lint-final-current` (focused `golangci-lint`): lease `e740e6197f05b9e0dd079bf97d56f43f04c5dd58`.

Two intentional compiled behavioral mutants produced targeted REDs and were restored; neither mutant fingerprint is presented as qualified source:

- `mutant-release-history-red`, lease `1cf28b6b136d326082e1c663ea28b2e9c7380c9e`: malformed RELEASE_BEGIN replay caused the real abandon/reconcile lifecycle test to fail.
- `mutant-bounds-red`, lease `0254456c314981f950276ac476190f97ecde7444`: removing the max+1 overflow rejection made `TestPinOwnerBoundedDirectoryScannerUsesFreshSortedDescriptions` fail with `over-limit directory scan = <nil>`.

Both RED stages exited nonzero as intended and each exact lease release returned 0. Initial compile failures, a fixture golden-domain mismatch, and runner admission holds are retained as diagnostics, not counted as behavioral RED evidence. Detailed stdout, stderr, claim, release, and stage JSON are stored in the corresponding relative stage directories beside this receipt.

## Limits and open gates

- Producer-internal crash boundaries remain untested and held: backup pin-file temp/partial-write/fsync/publish/root-sync, exact lease-byte deletion, and release-tombstone crash prefixes need producer-owned hooks and tests. This owner-only crash coverage does not satisfy those producer requirements.
- The actual production factory/wiring, an effects/epoch authority, and authorization to publish READY or complete a full recovery release are outside this component and remain unimplemented. The private gate only assembles and checks the actual backup store and owner identity pair.
- Root integration, independent source review, and full-module qualification remain outstanding. This receipt does not claim full contract acceptance or production readiness.

Root integrated WIP source `3dce044f2f69d9b9ebfb6924e59fcf670c44c362` onto landed envelope main. Exact six owned Go file bytes match the author commit. All parent frozen producer crash requirements remain held; this is not full source/contract acceptance.
