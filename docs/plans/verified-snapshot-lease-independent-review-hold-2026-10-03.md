# Independent producer-source review — historical e67 baseline

**Verdict: HOLD (historical baseline; not final candidate)**
**Exact head:** `e67b5fb8f90281928627d98993dee980143681d7`
**Detached clone:** clean at exact head.
**Scope:** backup-owned snapshot lease producer, lifecycle and local absence proof. No source edits, provider/startup factory qualification, or production acceptance.

## Blocking finding

**P1 — Stage retains expanded verification clones outside its byte accounting.** `InspectSnapshot` copies each raw bundle into `scratch/<brain>.bundle`, then `verifyStagedBrain` runs `gitrun.CloneBundle` and `restoreAllBranches` into `scratch/<brain>.repo` (`internal/hosted/backup/snapshot_inspect.go:192-198, 358-381`). That verification directory is never removed. `Stage` later writes a lease record whose `Files` list contains only the control DB, raw bundles, and manifest, then renames the entire inspection scratch tree into the durable lease (`internal/hosted/backup/snapshot_lease.go:891-925`). The stored `ArtifactBytes` and `usageLocked` totals therefore count raw declared artifact bytes but omit the retained expanded repositories (`snapshot_lease.go:2041-2056`). `MaxRestoreScratchBytes` is separately documented to cover lease-backed raw copies, not expanded output (`snapshot_lease.go:56-59`), and is not checked around `CloneBundle`.

Raw bundle length does not bound expanded Git object bytes. A large/incompressible expansion can be written before post-hoc size measurement, retained for the lease lifetime, and bypass both the retained-artifact aggregate and raw scratch limit. Move clone verification into a uniquely owned temporary verification directory with captured identity and confined cleanup before publishing the staged lease, and account any bytes intentionally retained. Keep transient expansion/physical quota as a separate, explicitly open acceptance item unless an enforceable quota is added; do not claim the raw-byte option is a hard expansion bound.

## Reviewed paths that appeared coherent at this head

- The required constructor `SnapshotStoreIdentity` binds root and stable lock device/inode identities; root and lock are rechecked before and after lock acquisition, and the blocking-lock replacement test exists.
- Stage/Pin persists `PIN_PENDING` and syncs before owner commit; recovery handles pending/committed local tuples without deleting committed owner bytes. The test for owner `COMMITTED` with local `STAGED` checks both `Close` and `Reconcile` refuse cleanup.
- `ResumePin` reconciles exact plan/digest/reservation/lease/attempt tuples; permanent exact cancellation tombstones prevent replay, and tests show delayed cancellation N leaves fresh-lease N+1 pending.
- `VerifiedPinAbsence` is private-fielded, shares one mutex state across aliases, checks exact tuple and identity plus current root/store-lock/lease/lease-lock objects, and expires at callback return. Cancellation holds both locks while authority consumes the proof; a retained proof cannot be consumed after callback exit.
- Owner attempt listings are bounded and validated before cleanup; malformed/duplicate/conflicting entries fail closed. Unknown scratch without a valid stage intent is preserved. Borrow/close coordination uses in-process borrower accounting plus a cross-process lease lock, and canceled close preserves borrowed bytes.
- Candidate projection and RestoreVerified scratch track raw copied bytes and identity. They do not bound expanded Git restore output or destination database/WAL growth; this is expressly documented and remains outside this utility component's acceptance.

## Verification

The focused package stage ran `go test ./internal/hosted/backup -run '^TestSnapshotLease' -count=1` under shared lease `e45caeb0db680763417ca10f3bae2fe119dcc826`; exit code 0, lease release exit code 0, source fingerprint unchanged. Evidence is in `stage-e67-snapshot-lease/` within this report directory. Root's full qualification at e67 is historical and likewise cannot close the blocking byte-accounting finding.
