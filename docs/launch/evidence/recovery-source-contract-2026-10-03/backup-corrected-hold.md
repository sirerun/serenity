# Supplemental independent review: verified snapshot lease contract

**Verdict: HOLD** for exact revision `31209b4c3e1cc1171afca71de204990a81a4e70d`. This supplements and does not replace the earlier HOLD for `ac2367f7a033428217f68a0f1793fe40887716e2`; that earlier verdict remains historical evidence for its exact head.

The revision resolves the prior pin-result, per-stage-digest, nullable-checkout, and separate artifact/metadata budget findings. Its API now matches the recovery consumer's principal candidate and reserved-`planRef` methods and ordering (`recovery-planner-and-admission-proposed-contract-2026-10-03.md:31-39, 294`; backup proposal `:28-32, 41-63, 101-115`). `SnapshotCheckoutAttempt` distinguishes a missing row from NULL and empty session refs, matching the nullable source schema (`internal/hosted/store/migrations.go:18`). The new release states, identity checks, and crash-repair sequence are directionally adequate (`:105-109, 168`).

## Remaining blockers

1. **A crash after durable `Pin` but before READY persistence is not recoverable through the exported API.** The contract promises that a retry resolves the unique pin by `(planRef, digest)` (`docs/plans/verified-snapshot-lease-proposed-source-contract-2026-10-03.md:103`), but the only resolver requires `pinID` as well (`:30, :99`). In the specified order, the pin ID exists only in the return from `Pin`; if the process dies before the recovery store persists READY with that ID, the recovery owner has only its reserved `planRef` and digest. `Reconcile` returns only `error` and exposes no recovered reference (`:32, :107`). Add an unambiguous restart lookup such as `FindPinned(ctx, planRef, expectedDigest) (SnapshotPinRef, error)` that requires exactly one matching pin and returns its persistable reference, or persist a durable pre-pin intent containing the pin ID before the crash window. The former aligns with the selected planRef-before-pin ordering. Cover lost response and duplicate/mismatched record cases.

2. **The lifecycle interface does not define how normal live pins are retained during `Reconcile`.** The interface offers `AuthorizePinRelease(...)(PinReleaseAuthorization, error)` with only release dispositions (`:68-92`). `Reconcile` calls it for every pinned record and says authority errors are returned (`:107`), yet it must successfully retain pins for `RESERVED`, `PLANNING`, `READY`, `APPLYING`, and forward-resumable plans (`:105, :107`; the recovery owner enumerates these states in its contract `:280`). There is no explicit non-error KEEP/NOT-RELEASABLE result or classification method, so an implementation must guess whether an empty authorization and nil error means “keep” or whether a live pin turns normal startup reconciliation into an error. Define a typed tri-state decision (`KEEP`, `RELEASE`, `ERROR`) or equivalent and require `KEEP` to preserve the pin while reconciliation succeeds.

3. **Pin authorization can race a reservation transition unless the owner contract supplies a durable guard/CAS boundary.** `ValidateReservedPlanRef` returns only a public `{PlanRef, State, RecordVersion}` value, with no reservation token or completion operation (`:68-80`). `Pin` then writes its durable pin (`:101`). The contract does not say the owner keeps the exact reservation protected from transition to abandoned between that validation and pin durability. READY publication can fail closed later, but the promised `Pin` behavior (“still-RESERVED”) and the crash case could leave a durable pin attached to an already-abandoned reservation. Specify an atomic owner-side pinning claim/version transition that remains recoverable across process death, or an equivalent serialized owner invariant, plus the retry transition after durable pin creation. A versioned read by itself does not reserve the row.

## Small contract correction

The proposed API comment says `PinnedSnapshotRef` is “produced by Pin” (`:39`), but `Pin` returns `SnapshotPinRef` (`:45`) and the prose correctly says `PinnedSnapshotRef` is produced by `ResolvePinned` (`:99`). Correct the type comment so implementers and callers do not treat the durable record as the process-local reopen capability.

The backup proposal's handoff direction now matches the coordinator's selected sequence: unique reservation, `Stage`, `Pin(planRef,digest)`, then immutable READY plan containing pin ID; approvals bind the completed full plan hash (`:101`; recovery consumer `:294`). Zero-eligible planning and the recovery activation guard remain separate recovery-owner gates; no recovery, source, or production acceptance is inferred here.

## Evidence and limits

- Reviewer clone: `[isolated external evidence]`, fetched from the author clone and detached at exact `31209b4c3e1cc1171afca71de204990a81a4e70d`; clean before review.
- Prior report remains unchanged at `review.md` in this evidence directory.
- Current root feed and coordination board were checked at the lane boundary. They record the planRef ordering and require independent CLEAR before source implementation. Ajent tooling remains unavailable. No PR/issue comment was sent by this review lane.
- No source edits, builds/tests, provider operations, or acceptance actions were performed.

## Evidence preservation

This repository copy replaces machine-local paths with labels. The exact external original is retained; its SHA-256 is `0f957462d30593188acf83048edc13403b738549f1a356280c37caa2ae627d21`. This historical HOLD is not current source clearance.
