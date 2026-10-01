# Independent review: deletion-journal adapter candidate

**Reviewed revisions:** initial candidate `82763466b4b33679508350803041b327c458e8f7`; correction `cd6d44489b68e9c05ef9bd26f5109736d50d5f27`, matching the integrated tree's correction `89f07e5583f04299cd3ddf274c3d88440c925735`. This is local code review only, not T23.48 acceptance, production assembly, or AWS qualification.

## Verdict

The correction closes both local fail-closed defects from my first review. Resuming at a seal now rejects any same-generation tail; a root-prefix scan catches visible later-generation keys behind a missing generation while retaining valid successor traversal above the configured generation. Focused race tests pass. I found no remaining adapter correctness blocker in the reviewed correction.

Two boundaries remain explicit. First, visible-key scanning cannot prove that empty or wholly invisible later generations do not exist. T50 must bind the returned watermark and seal to its authoritative adopted generation and verify every predecessor before activation. A `Sealed` result at a lower generation is only a seal for that generation, not proof that it is the adopted generation's complete tail. Second, this package is safe to merge only as an unused library with its pinned AWS dependencies; it is not production wiring or AWS qualification. `NewAWSJournal` uses the default AWS credential chain, which can use IMDS, while the hosted app unit denies IMDS. Do not wire this constructor into the app until a separately reviewed credential source is scoped to work under that deny.

## Shape validation and test evidence

The strict object decoder rejects unknown fields and trailing/noncanonical data; `validateJournalObject` checks known kind, matching positive coordinates, writer identity, UTC timestamp, predecessor hash shape, and kind-specific payload. Resume loading verifies that the object coordinates match the requested watermark and its stored-byte hash matches `EntryHash` (`internal/hosted/deletion/journal.go:302-330`). The shape cases cover unknown kind, malformed entry, a seal carrying entry fields, and malformed resume objects (`internal/hosted/deletion/journal_shape_test.go:34-137`). The shared conformance suite covers missing middle/tail, replaced objects, wrong watermark hash, an object beyond a seal, and valid generation rollover (`internal/hosted/contracts/contractstest/journal_suite.go:102-151,174-269`).

The initial strict-shape mutation was reproduced in the isolated tree: bypassing the validator caused seven malformed-record/resume subcases to fail at runtime. It was restored. The correction's new tests exercise a valid same-generation entry after a seal, a malformed tail, a delete-marker key with no current body, a visible generation after a missing intermediate generation, and a valid generation successor above the configured generation (`internal/hosted/deletion/journal_seal_resume_test.go:54-160`). They also preserve the case where a generation-2 reader sees a valid generation-1 seal with no generation-2 writes.

The focused package race check passed on the corrected tree:

`go test -race -mod=mod -modfile=/Volumes/BuildOffload/tmp/t23-48-journal-port-20261001/followup.mod -count=1 ./internal/hosted/deletion`

The external modfile was copied from the integration worktree's pinned `go.mod`; its SHA-256 was `70d1496574bdbc68f9ac7f61b271c44da33f19c2cec8072b1fa949610f9337e0`. Go module/cache files remained on the external SSD. No cloud test was enabled, and no AWS API call was made.

## Correction details and residual limits

`ReadThrough` validates the resume object, then explicitly checks for any key after a seal in that generation before continuing (`internal/hosted/deletion/journal.go:302-330`). This catches valid entries, malformed bytes, and current delete markers. The regression tests reproduce each form. A valid post-seal same-generation entry now returns `ErrDeletionJournalFenceViolated` even when the read starts at the seal watermark.

`firstVisibleGenerationAfter` uses root-prefix `ListAfter` and parses the earliest visible generation; `verifyNoTailAfter` rejects visible keys after a missing generation or after a terminal unsealed/sealed read (`journal.go:265-300,353-388`). The S3 implementation uses `ListObjectVersions`, carries both key and version markers across truncated pages, deduplicates repeated versions, and includes delete markers (`internal/hosted/deletion/store.go:121-178`). Limit one is sufficient for this probe because it only needs the first visible key after the current key. No live S3 pagination or delete-marker qualification was performed; the local marker-overlay test is not evidence about bucket configuration.

The reader can still return an older seal when later configured generations contain no visible objects. The adapter has no authoritative expected-generation argument, and absence alone cannot prove which generation T50 adopted. T50 must compare the receipt with the approved plan/current generation and validate the predecessor chain before using it as an activation barrier. The candidate evidence and this review do not claim that the adapter alone provides this authority.

`NewAWSJournal` uses `config.LoadDefaultConfig` (`internal/hosted/deletion/config.go:26-51`). Its comment correctly says that production needs an independently scoped credential source because the hosted app unit denies instance metadata. No service, backup, or partner assembly was changed; no S3 qualification, cloud call, or deployment was performed. The package may land as a library with pinned SDK modules while remaining unreferenced by production assembly, but neither T23.48 nor T50 is thereby accepted.

The review worktree contains only this receipt correction. Temporary tests and the earlier validator mutation were removed/restored; no source or module edits are part of this review.
