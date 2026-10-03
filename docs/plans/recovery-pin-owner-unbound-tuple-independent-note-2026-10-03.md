# Read-only disposition review: ReconcilePin before pin commitment

**Disposition: agree with fail-closed `Conflict` for an unbound pin tuple.** This is a semantic clarification supporting the exact-tuple rule; it does not clear prior source findings or qualify implementation.

The frozen `6588d950610a9fca704d9afcd342f4079f64144e` contract requires `ReconcilePin` to validate the exact pin tuple before deciding (contract lines 190, 202). A `PIN_BEGIN`/`PIN_PENDING` owner row intentionally leaves `PinID` empty (lines 171, 185 and transition matrix lines 244-245). Consequently, when the owner is `RESERVED` or `PIN_PENDING`, it has no pin ID against which to authenticate a nonempty `SnapshotPinRef.ID()`. Returning `PinKeep` based only on plan and digest would accept an arbitrary, unbound pin ID and would not satisfy the exact-tuple rule.

This does not disrupt the real pending transaction. The producer allocates a random pin ID only in `finishPinLocked` after `BeginPinAttempt` returns, writes the local `PIN_PENDING` record, then calls `CommitPinAttempt` (`internal/hosted/backup/snapshot_lease.go:1419-1459`). Pending crash recovery uses `ResumePin`, which calls `FindPinAttempt`, verifies the exact local record and either commits that pin or continues the pending attempt (`snapshot_lease.go:981-1037`). The producer's `ReconcilePin` call sites operate on already-`PINNED` local records (`snapshot_lease.go:1055`, `1084`, `1388`, `1704`). No legitimate pending-recovery flow needs `ReconcilePin` to accept an unbound ID.

Therefore `ReconcilePin` should return `ErrPinOwnerConflict` for `RESERVED`/`PIN_PENDING` whenever the supplied pin tuple cannot be fully matched to an owner-recorded pin. For a committed `PINNED` or exact abandoned/releasing pin, it should continue requiring every exposed tuple field. Unknown/mismatched tuple errors must not authorize cleanup.

The frozen contract's broad wording that an active `RESERVED`/`PIN_PENDING`/`PINNED` reservation returns `PinKeep` should be narrowed in a clarifying amendment: `PINNED` may return `PinKeep` only after full tuple match; `RESERVED` and `PIN_PENDING` have no committed pin identity and must conflict for `ReconcilePin`; recover those states via `FindPinAttempt`/`ResumePin`. This keeps the safe disposition consistent with the contract's controlling exact-tuple requirement and source call graph.

No source was edited and no build was run. Prior independent source HOLD findings remain open.
