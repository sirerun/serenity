# Preliminary static preflight — pin-owner corrections

**Status: PRELIMINARY HOLD; not a final review or source acceptance.**  
**Exact integrated head:** `9d9b2a4979d9dc7a4f8ca48112bd1a1870b97f1a` (tree `cf77a5b4a30705d60b250a6105363e1df237fd22`).  
**Clone:** `[external evidence locator] detached and clean.  
**Scope:** static only. No tests/builds/mutations were run because the shared lease is held for the foreign SereneDB workload.

## Earlier findings 2 and 3: static corrections present

- `ReconcilePin` now conflicts for `RESERVED` and `PIN_PENDING`, where no owner-authenticated PinID exists; it continues exact tuple comparison for `PINNED`, abandoned, releasing, and released states (`internal/hosted/recovery/pin_owner.go:570-613`). This matches the source call graph: real pending recovery uses `ResumePin`/`FindPinAttempt`; producer `ReconcilePin` call sites are supplied already-pinned local records. It does not weaken cleanup authority.
- The child-process matrix now includes `after-event:PIN_CANCEL`; the child performs the cancel and recovery verifies the durable tombstone, fresh N+1 attempt, and delayed Cancel(N) isolation (`pin_owner_test.go:818-940`). This closes the prior missing named event boundary at source level; execution is still required.

## Earlier finding 1: decoder approach is bounded; evidence/test path needs correction

`pinOwnerRecordVersionHint` rejects empty/oversized input before constructing a JSON decoder, rejects duplicate top-level keys and malformed/non-positive/non-integer versions, and returns `ErrPinOwnerUnavailable` for a valid positive version other than v1 before v1 strict decoding (`pin_owner.go:1201-1278`). The record reader checks file size against `pinOwnerMaxRecordBytes` before `io.ReadAll(io.LimitReader(..., limit+1))` (`1318-1352`), so the implementation has a fixed 4 KiB per-record allocation ceiling rather than allocation proportional to an oversized file. `OpenSnapshotPinOwner` and `ListPinAttempts` both invoke full history load/validation before returning results (`Open` at 251-254; list at 523-551), so an existing-superblock owner containing a future record should fail closed without publishing a partial list or rewriting history.

At this exact head, however, `TestPinOwnerFutureRecordVersionUnavailablePreservesBytes` mutates an existing history record, then expects `OpenSnapshotPinOwner` to succeed (`pin_owner_test.go:494-508`). The constructor is expected to reject during `pinOwnerLoadAll`; this assertion is inconsistent with the implementation and likely fails. The coordinator reports a later correction at `d28b31b40dfaad9cc86553f7b98391fba30976cf`; that object was not present in this review clone, so the correction is not qualified here. The corrected exact integrated head should assert the intended constructor error (`errors.Is(..., ErrPinOwnerUnavailable)` for a valid future version), test an already-open owner's `ListPinAttempts` error/no partial output, and compare bytes before/after. Add an oversized-record case and a positive future integer edge (including uint64 overflow classification) to the persisted controls. Static code bounds the oversized read, but this review did not execute or independently measure allocations.

## Additional constructor no-mutation edge to resolve

`OpenSnapshotPinOwner` writes a fresh superblock if the superblock is absent (`pin_owner.go:226-241`) and only afterward runs `pinOwnerLoadAll` (`251-252`). If a reservations directory already contains a record/unknown child while the superblock is missing, opening can publish a new identity-bearing superblock before discovering and refusing the pre-existing history. That is not the ordinary valid future-version case, where a superblock exists, but it is an owner-initialization ambiguity. Before source acceptance, consider failing closed without initialization whenever the superblock is absent and the owner root/reservations directory is not otherwise empty. Test this as a no-mutation constructor refusal so initialization cannot bless or alter an incomplete/unrecognized history.

## Remaining gates

This is not final CLEAR. The corrected test commit must be reviewed at its exact integrated head; source must remain unchanged during test qualification; the independent behavioral mutants/restored passes and full qualification are still pending. Producer terminal-receipt and primitive crash seams remain separate requirements. No startup factory, READY, provider, or full recovery acceptance is inferred.

Coordinator note: constructor-test expectation corrected by author d28b31b and integrated3f9c8b79 after this exact-head review. Earlier decoder/tuple/cancel source fixes passed focused package/race/vet on d28; lint and behavioral controls are pending. Missing-superblock/history and existing-superblock/missing-lock initialization mutation guard is a new required source fix; historical checks cannot qualify its future changed bytes.
