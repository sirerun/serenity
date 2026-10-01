# T23.50 next-slice design: truthful recovery inspection

Reviewed merged main `6d62fe8e1a1766dbb85c05f861284cdc90a51280` after PR340. This is a read-only design receipt. No source or frozen contract was changed; no Go tests, provider calls, cloud actions, or restore activation were performed.

## Recommendation

The smallest useful local slice is a read-only, non-authoritative snapshot inspection result. Do not yet implement `contracts.RecoveryPlanner.Plan` as an eligible recovery plan, and do not implement `RecoveryApplier.Apply` or any account activation path. A valid frozen `RecoveryPlan` needs a trusted journal generation, complete deletion history, current billing truth, and a nonempty eligible account set. Current code cannot produce those facts safely from the `SnapshotPath` request alone, and its billing contract deliberately withholds eligibility while `restore_pending`.

The inspector can validate the exact snapshot bytes and report its opaque account/brain inventory and journal watermark, while identifying all external decisions as unresolved. It must not label any account eligible, synthesize provider truth or generation, or write a valid `RecoveryPlan` artifact. This is useful preparation for a later planner and preserves the task's read-only/fail-closed requirement.

## Current callable surfaces and gaps

`contracts.RecoveryPlanner.Plan(ctx, RecoveryPlanRequest{SnapshotPath})` returns a `RecoveryPlan` containing `SourceSnapshot`, `JournalWatermark`, `Generation`, `ProviderTruthAt`, and eligible account IDs. `RecoveryPlan.Validate` requires a nonempty account set. `RecoveryApplyRequest` is single-account and `RecoveryApplyResult.Consistent` binds the result to that plan; an unfreeze requires `FenceReceipt.SufficientFor`. `WriterFencer.Fence(generation)` is frozen, while its receipt has booleans and a timestamp rather than persisted provider evidence references. These types define shape and consistency; they do not obtain or authenticate the evidence.

`internal/hosted/recovery` currently contains only `CreatePlan`/`LoadPlan` for an immutable local artifact. Its `PlanInput` requires callers to supply `SnapshotSHA256`, `JournalWatermark`, `FenceGeneration`, `ProviderObserved`, and `Accounts`; hashing and validation cannot make those caller-supplied values true. Its local `Plan` is not the frozen `contracts.RecoveryPlan`: it stores the exact raw manifest SHA-256 and a string timestamp, while the contract stores `SourceSnapshot` and `ProviderTruthAt`. `ManifestV2.Source.BuildSHA` is a build identity, not the digest of exact `manifest.json` bytes. Before conversion, the integrator must pin the mapping for `RecoveryPlan.SourceSnapshot`; do not silently substitute build SHA or raw manifest SHA for the other.

`backup.Restore` (`internal/hosted/backup/backup.go:631`) already uses the hardened `os.Root` manifest reader and parser (`:752,792`), checks artifact lengths/digests (`:1028`), checks schema and the manifest-vs-database brain inventory (`:1081`), verifies bundle heads, freezes the restored database, and atomically publishes a fully staged restore. It returns only an error. It does not expose the exact manifest bytes hash or a verified inventory to a planner. Calling it for `Plan` would publish a full scratch restore and mutate the staged control DB, so it is not equivalent to a read-only inspection.

Request a narrow reusable `backup.InspectSnapshot(ctx, snapshotPath, opts)` over one opened `os.Root` and one verification pass, returning `ManifestSHA256` (the exact bytes read), manifest source/build SHA kept separately, schema version, journal watermark, sorted opaque account IDs/statuses from a read-only view of the verified control DB, sorted brain IDs/empty flags/heads, and verified artifact count/total declared bytes. It must verify all control-DB and brain bytes against length/SHA, actual DB schema and inventory against the manifest, and each bundle's recorded heads. A manifest-only parser is insufficient. Copy/hash each source artifact once and use only verified private copies afterward; no re-open of the untrusted source after validation. Scratch must be an explicit, owner-only path on the configured SSD, bounded by a configured maximum and refused before copying when declared size exceeds it; do not silently fall back to system temp or truncate validation. The inspector does not mutate the snapshot or publish a destination. Do not claim an OS hard quota unless the scratch volume enforces one. This requires extracting a shared validation/staging helper from Restore without sharing its migration, freeze, or destination-publication steps; tests must prove Restore still uses the same verifier. Request this T23.49-owned extension through the documented integrator handshake; T23.50 should not duplicate manifest parsing or edit backup ownership.

The frozen `RecoveryPlanRequest` contains only `SnapshotPath`; it has no caller-supplied approved source digest. Preserve the boundary explicitly: the inspector returns the raw manifest digest, and the immutable plan must bind it; `Source.BuildSHA` is separate metadata. If policy requires a source hash approved before planning, request an additive `ExpectedSnapshotSHA256` field through the integration handshake and compare it to the exact inspector result before writing the plan. Do not reinterpret the build SHA as that approval. The reviewed plan hash remains the later Apply approval key.

The journal's `ReadThrough(from)` returns only entries strictly after `from` and reports whether the terminal object read is a seal. It verifies a contiguous chain relative to visible storage, but a valid generation-1 seal can be returned while a configured generation 2 has no entries; `Sealed` is not proof that generation 1 is the adopted writer generation. Snapshot-cut recovery must also inspect complete history from zero: a pending intent at or before the snapshot watermark can still be absent from a restored database. A real fence must stop the old instance, revoke every credential/session, then seal and read through the barrier. Neither `ReadThrough` nor a static generation setting proves stop, revocation, or successor adoption. No `CurrentGeneration`/adoption capability is exposed today.

The dependency registry still marks T23.47, T23.48, T23.49, T23.50, and T23.54 `planned`. `Service.RecoveryAdmission` (`internal/hosted/service/service.go:67-84`) is a startup admission capability for the shared journal; it is not a recovery planner, provider observer, or old-writer fencer. There is no `internal/cli/hosted_recovery.go` or registered plan/apply command.

## Billing-contract conflict and proposed bounded extension

The frozen `BillingReconciler` contract requires `ErrBillingAccountFrozen` instead of eligibility for frozen accounts (`internal/hosted/contracts/billing.go`). The implementation (`internal/hosted/billing/reconcile.go:194-210,321-338`) reads provider state for `restore_pending`, updates subscription/checkout rows and plan projection, sets the account projection to Free because it is not active, then returns `ErrBillingAccountFrozen` with an empty `ReconcileResult`. This is consistent with the frozen contract but cannot tell recovery whether a restored paid account is currently eligible. T23.50's demand to use current provider truth before reactivation therefore cannot be met by calling `ReconcileCustomer` and reading its result. Do not temporarily change status to `active` to bypass the contract.

Use the interfaces.md file-change handshake for a separately reviewed, additive read-only capability; keep `ReconcileCustomer` unchanged:

```go
type RecoveryBillingObservation struct {
    AccountID string
    Eligible bool
    PlanID string
    CurrentWindowStart, CurrentWindowEnd, GraceUntil time.Time
    ObservedAt time.Time
    Source string
}

type RecoveryBillingObserver interface {
    ObserveRestorePending(ctx context.Context, accountID string) (RecoveryBillingObservation, error)
}
```

The concrete billing service must acquire the existing per-account lock, freshly load the account and its server-owned customer binding, and accept only `restore_pending`; deleting/deleted, missing, changed, or otherwise frozen rows fail closed. It may never accept a customer ID from the request or returned payload. After provider I/O it must reread status and customer binding under the same lock and refuse if either changed. It performs no SQL entitlement/subscription/checkout/audit writes and never unfreezes. It reuses the existing price ownership, duplicate/unknown subscription, invoice/grace, and provider-error validation paths; ambiguity or unavailable provider state is an error, never Free eligibility. Any pending checkout whose state cannot be safely established remains a blocker; this observer must not expire sessions or delete checkout rows. A separate explicitly authorized reconcile phase is required to clean stale checkout state. The explicit billing-disabled Free pilot rule must be independently approved and cannot be inferred from a missing customer ID.

Tests for the extension should prove restore_pending observation returns the provider-derived result without modifying account/subscription/checkout rows; a deleting or deleted status returns no provider eligibility; a customer-binding/status change during the provider call is caught by the final reread; wrong-price, multiple nonterminal subscriptions, pending/ambiguous checkout, provider timeout and malformed grace all block without SQL mutation; and canceled historical billing returns Free only when the current provider result proves that outcome. Use fake provider responses, with no live Stripe calls.

## Safe sequence and open contract decisions

1. Request the backup inspection API and billing observer through the documented integrator handshake. Preserve the existing frozen `ReconcileCustomer` semantics.
2. Implement/test the snapshot inspector and a sanitized preview. It should fail on malformed/legacy manifests, mismatched artifact hash/length, schema or inventory mismatch, symlink/special-file inputs, and cancellation; it must not create a published restore destination. Report the raw manifest digest separately from build SHA.
3. Do not persist an eligible `RecoveryPlan` until an authority can provide the current old-writer generation, a verified journal cut/full-history boundary, current per-account provider observation, and exact account scope. `Plan` is read-only, so it must refuse when the journal is unsealed or completeness cannot be established; it must not call `Seal` to manufacture its own evidence.
4. Before the first real plan/apply implementation, resolve three frozen-shape questions with the integrator: whether `SourceSnapshot` means the exact raw manifest digest or a distinct snapshot/object identity; how a zero-eligible-account restore is represented even though `RecoveryPlan.Validate` rejects an empty `Accounts`; and where durable, independently verifiable stop/revocation/adoption evidence references are stored beyond `FenceReceipt` booleans.

Until these seams are approved, the only safe existing work is local snapshot inspection/preview and continued storage hardening. That slice is reversible and useful but does not close T23.50 acceptance, produce a trusted recovery plan, authorize provider changes, or qualify restore activation.
