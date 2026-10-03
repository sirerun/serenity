# Startup journal observation implementation receipt

Status: source implementation complete at the worker worktree. This receipt records local source evidence only; independent exact-head review and coordinator integration remain open.

## Scope and behavior

Implemented the frozen `startup-journal-observation-v1` component from base `243d563cb9c381ac7d65f2f8c735be49661cfd99` on `implementation/journal-read-observation-20261003`.

- Added the read-only `JournalObjectReader`, `DeletionJournalReader`, `JournalPosition`, `JournalObservation`, and `JournalWriterUse` contracts. The existing `JournalObjectStore` satisfies the read interface structurally.
- Added `NewJournalReader` and `ReadOnlyJournal.Observe`, which verifies canonical ordered history through the positive authority-provided active generation, including entry and seal bytes, provenance, the exact active head or empty-generation predecessor seal, and SHA-256 over big-endian length-prefixed canonical bytes.
- Bounded traversal to 10,000 generations, 100,000 objects, 64 KiB per object, and 64 MiB cumulative canonical bytes. Namespace keys, pagination progress, context cancellation, writer identity reuse, and keys beyond the selected generation fail closed.
- Added `NewJournalAt` with exact cursor initialization, full-history preflight before first write, requested-writer cross-generation refusal before conditional PUT, immutable pending bytes across ambiguous append/seal outcomes, and same-instance seal replay that checks exact seal bytes and absence of a tail.
- Kept legacy `NewJournal`, `ProductionJournal`, `ReadThrough`, `Seal`, and contract conformance behavior on their existing path. Test doubles are in `_test.go` files.

No provider authentication, namespace reservation, startup factory, service admission, CLI, credential, deployment, purge, or spend authority is implemented or claimed. The caller must establish a stable exclusive namespace reservation. The read interface documents that adapters must bound transport allocation before returning object bytes; this source bounds processing after `Get` returns.

## Verification

Focused race, vet, and lint all passed with the same Go-source fingerprint `bad88c03bfcd701fcb406394c56a41edd46c60bec442f488a458df5ddcde3d96`. Each command used the durable worker-stage runner, a fresh load below 10, a verified `R-build-lease` claim, and an exact same-stage release:

- Race: `focused-race-20261003T0436Z`, exit 0, load 6.94, lease `55d59241505c1f0113917ebde731553ad93c0e13`.
- Vet: `focused-vet-20261003T0437Z`, exit 0, load 4.33, lease `f34d9f29291fb6f20c5a528b112fa3051617c267`.
- Lint: `focused-lint-20261003T0437Z`, exit 0, load 4.06, lease `4b6f6a88b4c2eb7b57f1ce05b473c45349d6a260`.

Behavioral mutation controls compiled and failed at the intended assertions:

- Removing the historical writer-generation check made `TestNewJournalAtRejectsCrossGenerationWriterBeforePut` fail after it observed an unexpected write. Stage `mutant-cross-generation-writer-20261003T0438Z`, mutated Go-source fingerprint `015fd80895d8cce1c0ac9289032d0b3090c8857e0eaed4a17954bcc5ba623454`.
- Disabling the active-head equality made `TestObserveRejectsStaleHeadAndPositionShape` fail because stale history was accepted. Stage `mutant-stale-head-behavioral-20261003T0440Z`, mutated Go-source fingerprint `7d4d3c7fd955c3c60d29c1b364b3927c2322c1fa7869a6793caa74a5adeea30b`.

The first stale-head mutation attempt removed the guard and did not compile because its local became unused; it is excluded as RED evidence. Both valid mutants were restored, and the final source fingerprint matches the passing stages. The task-owned APFS fixture and its mode/owner were recorded in the external evidence directory. The fixture's size is not physical-capacity evidence.

Raw focused command records, source fingerprints, lease logs, mutant stdout, and fixture verification are under `/Volumes/BuildOffload/serenity-journal-read-observation-implementation-evidence-20261003/`. The session had no Ajent MCP tools; the actual project `ajent.social` feed and coordination board were read.

## Handoff

Worker ownership covers only the new contracts reader and tests, `internal/hosted/deletion/journal.go`, new `journal_observation*.go` files and tests, and this receipt. No source claim was created or released, and no source was pushed, opened as a PR, or merged. Proposed roadmap line: `T-JRO.1–6 — complete bounded journal observation and reserved-writer source; focused race/vet/lint and behavioral mutants pass; pending independent exact-head review and coordinator full-module qualification.`
