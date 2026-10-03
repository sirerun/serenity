# Independent contract review: verified snapshot lease proposal

**Verdict: CLEAR for source freeze on the exact proposed producer contract**, commit `6a013b49b94e7f9286b782ad2827441d2e5bd737`.

This is a contract/API review, not implementation or acceptance evidence. The reviewer checkout is detached at the exact commit and clean. The author checkout is also at that exact commit and clean. The prior reports for `31209b4` and `95d65e1` remain preserved.

## Exact change from the prior HOLD

The prior HOLD identified that `CancelPin` accepted only plan ref, reservation version and digest, allowing a delayed retry to resolve to a later attempt. This revision changes it to `CancelPin(ctx, attempt PinAttemptRef)`. The contract requires the exact token returned by `BeginPinAttempt`/`FindPinAttempt`, binds all tuple fields including `attemptVersion`, compares them under the per-ref lock, and CASes only that exact pending attempt. A repeated request after a lost cancellation response resolves through that attempt's durable tombstone and cannot inspect or mutate a later attempt. Stale/mismatched tokens conflict without mutation. The contract also adds the explicit N-cancel response-loss -> N+1 pending -> retry N test, requiring N+1 remain pending. (Contract lines 34, 126-128, 193.) This closes the sole finding in the `95d65e1` HOLD.

## Combined producer-contract assessment

The reviewed contract now has a bounded, implementable producer seam for the concerns covered in the previous reviews:

- Staging and reopening use verifier-owned references and retained verified bytes; callers cannot choose arbitrary artifact paths or forge persisted handles. Inspection can be supplied per stage. (Lines 16-53, 108-120.)
- Limits separate artifact and metadata budgets, bound candidate projection and checkout rows, and count active leases and pins across owner records, including pending attempts protecting a staged lease. The contract explicitly distinguishes accounting caps from filesystem physical quota. (Lines 156-177.)
- The caller reserves a unique plan ref, stages, durably records `PIN_PENDING` with reservation version, pins the exact staged lease/digest, commits that exact attempt, then persists immutable READY plan state. Restart lookup/resume covers pin-file and owner-record crash windows without claiming a recovery backend implementation. (Lines 72-102, 124-134.)
- Borrow/Close behavior protects bytes while readers and restore borrowers are active; cancellation leaves owned bytes retryable. Lifecycle authority supplies KEEP, RELEASE, or error, with release requiring an authoritative terminal disposition and a guarded owner-record version. `ListPinAttempts` protects pending leases from reconciliation cleanup. (Lines 37-53, 72-102, 130-134.)
- The plan hash is independent of generated pin ID; full-plan approval is bound after pinning. Candidate projection preserves nullable checkout state and minimizes untrusted billing hints. (Lines 136-154.)

These clauses are concrete enough to freeze the producer contract and begin implementation. CLEAR is limited to this proposed contract's API/ordering/lifecycle/bounds design; it does not validate implementation, production authority wiring, durable-volume behavior, recovery availability, provider state, or acceptance. The document itself states these limits at lines 199-200. The separate recovery planner/admission and zero-eligible activation gates remain outside this review.

## Scope and evidence

Reviewed exact diff `95d65e1a99008a113f19ab4e2322c9e29e855417..6a013b49b94e7f9286b782ad2827441d2e5bd737` and the full proposed source-contract document. No source files were edited; no build, tests, or provider actions were run. Ajent tooling was unavailable; local project feed/coordination records were checked in the prior review lane.

## Evidence preservation

Machine-local paths are replaced with labels in this copy. Original report SHA-256: `040502e7df2344b8db91b24569fa6032dc9a234377d480b7a485e01e9ac23373`. CLEAR applies to the proposed producer contract only; source implementation and qualification remain pending.
