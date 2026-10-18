# Initial maintenance gate reconciliation — v1

Status: source design/API frozen after independent review on 2026-10-18;
implementation and fault qualification remain required. This does not authenticate production coldness,
activate canonical data, finish maintenance or dispose of retained copies.

## Outcome and authority

Cover every legitimate initial BeginBrainMaintenance failure: absent marker,
zero bytes, any exact short prefix, complete bytes before failed file/directory
sync, and process death at those boundaries. Keep the existing final-marker
presence fence and stable EX canonical-view -> EX writer lock order. Never
remove, truncate or replace the marker to make recovery pass. Non-prefix or
identifiably foreign bytes are not the expected initial-write failure class.

A durable PreparedRef binds preparation, not historical marker ownership.
Operations A and B can be prepared against identical source/fences. A zero-byte
marker from A therefore cannot be called recovery of A merely because B matches.
The new path reconciles an initial gate NOW under fresh explicitly scoped owner
admission. It never asserts the prior actor/operation, even when bytes match.

An InitialRecoveryAdmission must embed existing ColdAdmission and additionally
implement CheckInitialRecovery(context.Context, InitialRecoveryPins) error.
Ordinary ColdAdmission alone is insufficient. The extra method must explicitly
authorize initial reconciliation, including unattributed prior intent, fence
prior workers through actual current cold custody, bind the independently pinned
prepared header and observed marker/root/lock identity, and revalidate complete
CURRENT relevant exclusions. Local JSON, a callback result saved in a file,
matching pins or an approval boolean cannot authenticate this authority.
Production implementation of this trusted owner boundary remains a separate gate.

InitialRecoveryPins contains existing AdmissionPins, fixed purpose
serenity.initial-maintenance-reconciliation/v1, immutable recovery-record digest,
phase, original marker observation and CURRENT marker observation. Observations include
absence versus regular-file state, device/inode, owner/mode/link count, byte
length and SHA256. The wrapper passes actual pinned observations; callers cannot
supply them as assertions. No payload, key or routing is exported in receipts.

## Writer seam

Add a restricted PendingInitialMaintenance owner. Open acquires existing EX view
then EX writer, pins the canonical root/runtime and stable lock inodes, and
inspects an absent or owned0600 single-link bounded marker. Existing marker bytes
must be an exact prefix of maintenanceBytes(expected), including zero/full. It
retains ownership but exposes no Finish, activation or normal BrainMaintenance.
Snapshots are detached; validation rechecks root/runtime/locks/marker inode,
metadata and bytes. Nil/context-cancelled admission refuses without marker edits.

Completion uses a trusted per-call verifier receiving the actual current
observation. The integration wrapper owns canonical/prepared qualification and
fresh authority. For an existing marker append only missing bytes to SAME inode;
for absence exclusively create the existing final marker while locks are held.
Recheck authority and namespace before append/publication and after durability.
Sync marker, runtime directory and root, then return the ordinary validated
BrainMaintenance with the same locks. Failure closes handles but retains the
persistent marker and evidence. Partial writes never return a usable owner.

Existing Begin and exact-full Resume semantics remain; both full and restricted
owners enforce private owned single-link marker metadata. The final marker is
never absent during existing-marker reconciliation, so existing cooperating
writers/readers remain fenced without introducing a new staging-name protocol.
Future atomic Begin publication is not required to recover this full current
failure class and is not silently introduced with incompatible older readers.

## Episode installation wrapper

RecoverInitialMaintenance(ctx, PreparedRef, InitialRecoveryAdmission) returns a
normal maintenance owner only after the following checks, under retained locks:

1. Validate the pre-existing immutable header/routes/identity and all original,
   inspection, old-index, candidate, replacement and new-index inventories.
2. Validate exact supported canonical layout, full canonical Old inventories,
   actual Git HEAD/worktree and runtime/index cohort (including WAL/SHM). Root
   and stable locks must remain pinned. Do not open a root-owned SQLite handle;
   the existing caller/service UID owns this library.
3. Refuse install.json, either progress .pending, installed completion and ALL
   retained-copy progress. Use a pure checker: transactionLayout creates paths
   and cannot be used to establish absence. Refuse unknown operation/runtime
   artifacts and competing recovery identities.
4. Under fresh ColdAdmission plus explicit CheckInitialRecovery, publish an
   immutable deterministic initial-recovery.json BEFORE any marker append. It
   binds header/expected identity, canonical marker digest and the original
   observed marker metadata/bytes (private, bounded content-free identity only).
   This is recovery evidence, never an authority grant or historical intent.
5. Fixed initial-recovery.json.pending is prefix-recoverable from the same exact
   expected record before marker edits. Once complete, recover only its exact
   original marker inode (or a recorded exclusive creation from absence), current
   expected prefix with length >= original, unchanged source and no install
   progress. Fresh authority binds original AND current observations each time.
6. Append missing bytes and synchronize through the writer seam; revalidate
   custody, current authority and unchanged qualified source before owner return.

EnsureGate first obtains fresh explicit initial-reconciliation admission for
actual absence, exclusively creates the final zero-byte marker, pins its actual
inode, and synchronizes file -> runtime directory -> brain root. It remains
restricted. Failure returns no usable owner and publishes no recovery record.
Only then may the wrapper publish recovery evidence binding the ACTUAL inode.
A crash before complete evidence leaves an unattributed initial gate requiring
fresh admission; no surviving creation authority is inferred. Complete evidence
binds the original inode and observations for every continuation. Existing full
marker plus installation progress uses ordinary Resume/episodeinstall.Resume.

## Frozen API and record contract

The writer API is:

```go
type InitialMaintenanceObservation struct {
    Present bool
    Device, Inode uint64
    UID, GID uint32
    Mode uint32 // permission bits
    Links uint64
    Bytes int64
    SHA256 string
    Prefix []byte // detached, excluded from public JSON; bounded marker bytes
}
type InitialMaintenanceVerifier func(context.Context, InitialMaintenanceObservation) error
func OpenInitialBrainMaintenance(string, MaintenanceIdentity) (*PendingInitialMaintenance, error)
func (*PendingInitialMaintenance) Observation() (InitialMaintenanceObservation, error)
func (*PendingInitialMaintenance) ValidateFor(string, MaintenanceIdentity) error
func (*PendingInitialMaintenance) EnsureGate(context.Context, InitialMaintenanceVerifier) error
func (*PendingInitialMaintenance) SyncGate(context.Context, InitialMaintenanceVerifier) error
func (*PendingInitialMaintenance) Complete(context.Context, InitialMaintenanceVerifier) (*BrainMaintenance, error)
func (*PendingInitialMaintenance) Close() error
```

Open and Observation do not mutate the marker. Absent observations have all
other fields zero/empty. EnsureGate requires absence and calls the verifier
before exclusive creation; existing markers are not silently accepted by this
method. SyncGate requires presence and verifies fresh admission before synchronizing
that SAME marker (no append) -> runtime -> root, then validates unchanged custody
and context. Failure closes the restricted handle and retains the gate. This is
required for existing gates too: failed Begin may leave their inode/directory entry
unsynced. The wrapper invokes SyncGate under record-phase admission before any
new or pending evidence publication, then obtains an unchanged actual observation
and fresh record-phase admission immediately before publication. Published-record
continuations also resynchronize without changing their immutable Original.
Repeated record-phase admission calls are legitimate. Complete requires presence, checks the verifier before append and after
file/runtime/root durability, and transfers existing locks exactly once only on
success. Verifiers must not reenter the handle or mutate pinned paths. Nil
verifiers or cancelled contexts refuse. Any mutating-method failure closes the
restricted handle, retaining the gate/evidence. Close is idempotent; after transfer
it never closes the returned ordinary owner. Read-only validation errors do not
transfer ownership. Non-Unix builds fail closed with the same API.

The wrapper API is:

```go
type InitialRecoveryAdmission interface {
    ColdAdmission
    CheckInitialRecovery(context.Context, InitialRecoveryPins) error
}
func RecoverInitialMaintenance(context.Context, PreparedRef, InitialRecoveryAdmission) (*writer.BrainMaintenance, Result, error)
```

InitialRecoveryPins contains AdmissionPins, Purpose, Phase, RecordSHA256,
Original and Current observations. Phase is `precreate`, `record`, `append` or
`return`. RecordSHA256 is empty ONLY for precreate; Original and Current are
both actually absent there. Before record publication, the digest is computed
from actual existing marker observations, never a predicted inode. Every phase
checks ordinary ColdAdmission and the distinct initial-recovery admission.
Complete invokes its verifier exactly twice on success: append before mutation
and return after sync, in that order. The wrapper tracks these two phases; each
callback requalifies complete source without reentering the restricted handle.

Immutable canonical JSON plus newline, at most 4096 bytes, records Schema
`serenity.initial-maintenance-reconciliation/v1`, Purpose (same), HeaderSHA256,
Identity, ExpectedMarkerSHA256, Original (all observation metadata and bounded
original Prefix encoded as base64), and PriorIntent `unattributed`. No timestamp,
nonce, actor or saved approval is recorded. Its SHA256 is the admission pin.
Original is always present: an absent gate is made durable before this record.
The fixed names are initial-recovery.json and initial-recovery.json.pending.
Both must be owned0600 single-link regular files. Unknown schemas, fields,
noncanonical encodings, conflicting published/pending pairs or unexpected
artifacts refuse. A pending exact prefix may be completed only from a fresh
qualification of the SAME unchanged original marker; no marker append precedes
record file sync, publication and operation-directory sync. After publication,
the record's original metadata/inode/prefix remain immutable; current bytes may
only extend that prefix toward the exact expected marker. A later longer prefix
must never be reinterpreted as a new original observation. Missing/replaced
recorded markers refuse, including absence after a recorded exclusive creation.
Strict operation-layout acceptance validates this record against the header and
ordinary full maintenance marker before Install/Resume admits its fixed name.
It does not grant authority. Pending evidence is accepted only by initial recovery,
never by ordinary installation.

The wrapper records state initial-gate-reconciled, marker-held=true and
residual-copies=true. It never removes the marker or emits physical-complete.
Subsequent Install still performs all existing fresh admission, component and
intent checks. The new recovery record is a known fixed operation artifact;
transaction layout can accept only a fully validated matching record, not arbitrary
extra files. Its presence must not become authority for Install or Finish.

## Acceptance

Real process-death/fault cases cover absent/zero/every representative prefix,
short writes, create/file-sync/runtime/root-sync, immutable-record zero/mid/full
stages, authority expiry/cancellation/rejection, retries and source drift. Tests
must prove reader/writer refusal throughout, stable lock/marker inode retention,
no canonical mutation or new payload copies, no Finish before validated completion,
and actual post-reconciliation Install/Resume behavior through the existing pool.

Exercise A/B identical preparations: ordinary cold admission must fail; explicit
initial admission binds the actual predecessor observation and records unattributed
intent without claiming A completed. Wrong prefix/op/head/fence/header/account,
linked/symlink/foreign-owned marker, moved runtime/root/locks, changed WAL/index,
progress/retained files and unknown recovery records stay refused and retained.
Actual cold issuer/provider/installed-host qualification remains separate.
