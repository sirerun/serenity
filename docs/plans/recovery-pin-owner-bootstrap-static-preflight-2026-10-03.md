# Preliminary static preflight — pin-owner corrections and bootstrap guard

**Status: PRELIMINARY HOLD, not a final exact-head review or source acceptance.**  
**Reviewed exact head:** `03401d923e387d78fa614e242a340511fb35da38`.  
**Tree:** `cf77a5b4a30705d60b250a6105363e1df237fd22`.  
**Scope:** static review only; no tests, builds, or source mutation. The shared build lease was held, so no runtime evidence is claimed.

## Findings 1–4: static source disposition

1. **Future record version:** The decoder now reads a bounded top-level version hint before the v1 strict decode. The file reader checks `stat.Size <= pinOwnerMaxRecordBytes` before allocating/read-all and applies a `limit+1` reader bound if the file grows (`internal/hosted/recovery/pin_owner.go:1201-1285, 1381-1410`). A positive representable non-v1 integer returns `ErrPinOwnerUnavailable`; zero, null, signed/fractional/string, duplicate top-level keys, trailing garbage, missing version, and non-object records fail closed. All allocations during hint scanning remain bounded by the existing 4096-byte record cap; this is bounded allocation, not zero allocation.

   Tests now call Reserve, List, and Open against a malformed/future row and compare original row bytes (`pin_owner_test.go:394-510`). The earlier constructor assertion error is corrected. However, the table has no oversized-record case, no upper-bound positive future version / uint64 overflow case, and discards the returned List slice instead of asserting it is empty. Static implementation clears any partial result in `pinOwnerWithLock`, but these remain useful controls before source acceptance. No allocation measurement or runtime proof was performed.

2. **Unbound active tuple:** `ReconcilePin` now returns conflict for `RESERVED` and `PIN_PENDING`, where the owner has no committed pin ID to match, and keeps full digest/PinID/state checks for committed and terminal states (`pin_owner.go:555-617`). This is consistent with the contract clarification and producer call graph: pending recovery is through `FindPinAttempt`/`ResumePin`, while Reconcile call sites supply local `PINNED` records. The new cases check no decision, unchanged owner head, and unchanged state/version (`pin_owner_test.go:550-606`). They are static evidence only; not executed here.

3. **Durable cancellation boundary:** The child-process lifecycle matrix includes `after-event:PIN_CANCEL`, retries the exact cancel after reopen, verifies the tombstone, starts N+1, and checks delayed Cancel(N) cannot modify N+1 (`pin_owner_test.go:818-940`). This closes the missing source-level test case; no crash process was run in this review.

4. **Bootstrap rebinding:** `pinOwnerValidateBootstrapPrefix` bounds root enumeration to three entries, allows only lock/superblock/reservations, opens the existing lock with no-follow validation, and permits a missing-superblock prefix only when its reservations directory is empty. Open runs the check before lock creation and again under the flock before creating reservations/superblock (`pin_owner.go:152-170, 224-247, 1329-1379`). Lock creation is exclusive with an EEXIST open-existing fallback; flock serializes concurrent initializers. The new tests cover retained future history with missing superblock, an existing superblock with its lock removed, and an unknown owner-root entry, verifying the root/history/lock are not rebound (`pin_owner_test.go:526-647`). Static ordering appears fail-closed, and legitimate empty/lock-only/empty-reservations crash prefixes remain recoverable.

   I found no obvious path-following or unbounded bootstrap scan: enumeration uses the pinned directory descriptor and bounded `ReadDir(max+1)`, while lock opens use `O_NOFOLLOW|O_EXCL` for creation and later checks bind names to captured identities. A concurrent first-open regression test is not present; code-level O_EXCL plus flock ordering appears to serialize it, but this remains for the exact source qualification run. The tests also do not cover every root-identity failure interleaving.

## Remaining gates

This is a preliminary review only. The version classifier, unbound tuple rejection, cancellation crash case, and bootstrap guard are directionally consistent with the frozen contract. Before source acceptance, run the exact-head package/focused controls including oversized record/no-allocation-bound and boundary integer cases, assert zero/empty List output on error, exercise concurrent first-open, then complete independent behavioral mutants/restored passes and full-module qualification at the final integrated bytes. The terminal-receipt amendment and producer primitive crash tests are separate gates. No READY, production factory, provider, or full recovery acceptance is inferred.

Coordinator note: later author390 adds integer/size/list-result/concurrent-open controls and e880 narrows the direct oversize decoder case; all new source and test bytes remain unqualified pending genuine shared build admission. Final source review must cover the final integrated revision.
