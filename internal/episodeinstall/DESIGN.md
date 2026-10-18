# Prepared episode installation and forward recovery

This internal library builds on episodeerase.BuildHosted, writer ownership and
index maintenance qualification. It is not an HTTP/model capability, a second
deletion journal or an authenticated production activator. Capture stays off.
A trusted offline owner must qualify cold namespace/service custody and fresh
complete external exclusions before using installation. Live preparation cuts
alone do not establish that cold boundary. Production wiring remains a separate
qualified owner task (T5.34 and successors).

## API and authority boundary

- Prepare(ctx, Request) returns PreparedRef plus error. It acquires ordinary
  writer ownership itself; a live pool owner refuses preparation. Request names
  exact source/operation roots, account/brain/operation IDs, expected source HEAD,
  complete current native exclusion records and independent raw/path pins.
- PreparedRef carries private Source and Directory routing (omitted from JSON),
  exact HeaderSHA256 and writer.MaintenanceIdentity. On post-directory failure
  its ResidualCopies flag is true and HeaderSHA256 is empty: it is not prepared.
  Successful preparation also has residual copies; success never means purge.
- Install(ctx, ref, *writer.BrainMaintenance, ColdAdmission) and Resume with the
  same signature return Result plus error. ColdAdmission.Check(ctx, AdmissionPins)
  is a trusted internal capability, never a request boolean or local manifest.
  Pins include account/brain/operation, source/candidate HEAD, fence/header digest
  and private source routing. The actual owner must verify current cold custody,
  actual account-brain-root authority and complete current relevant exclusions.
  Relevant fence changes refuse; an expired grant requires fresh admission.
- Ref/header/marker match both directions. ValidateFor binds the actual original
  root and stable lock/marker inodes. Local metadata proves neither account
  authority nor journal completeness. Nil/unknown admission always refuses.

The library leaves a valid marker and locks held even on successful installation.
Result states installed-marker-held separately from incomplete/retained copies.
Only the actual cold owner may independently close out and Finish/Close. All
errors are static/content-free; private routing never enters exported receipts.

## Inactive preparation before the persistent gate

OperationDirectory is an absent canonical directory named by a 64-hex operation
ID under an existing private owner-controlled parent, outside the source. Create
it exclusively, sync ownership, and attribute all stages/copies to that root.
Never adopt an old attempt or delete another operation. Failure returns explicit
residual state; normal writer release does not claim cleanup.

Under ordinary writer ownership:

1. Validate exact source HEAD, source paths/types and clean canonical Git state.
   Freeze the complete old component inventory and supported runtime inventory.
   Source runtime allows only stable writer/view locks and index.db/WAL/SHM;
   any existing maintenance marker or unknown artifact/directory refuses.
2. Copy the five canonical components into original/. No links, alternates,
   borrowed objects or provider access. Preserve exact bytes/modes with bounded
   file checks; sync the copy. Keep index copies separately inventoried.
3. Qualify a dedicated closed old-index family copy with index.QualifyMaintenanceCopy.
   Nonempty jobs, spend, caches, events or other controls refuse this cohort.
   Owned inspection sidecars are retained and described; originals are unchanged.
4. BuildHosted from the immutable original proof with the complete supplied
   fences and independent pins. Its deterministic new history supplies candidate/.
   Materialize a private nonbare replacement/ using hardened local Git plumbing,
   no hardlinks/alternates/hooks/network. Verify exact HEAD/history/object set,
   clean worktree, eligible configuration and canonical exclusion projection.
5. Build new-index/ from retained canonical replacement data with index.Rebuild
   and actual configuration; no embedding/provider call. Preserve lexical entity,
   shard and supported source projections. Vectors remain unavailable until a
   separately admitted rebuild; existing lexical fallback is required.
6. Close/checkpoint all new SQLite handles. Freeze new family and canonical
   component inventories. Sync every file/directory and publish a canonical,
   bounded prepared.json with exact header digest only after preparation finishes.
   Hash private source/operation routing into the header, rather than storing
   absolute host paths. Immutable header includes future marker identity and
   complete native fence/preservation inventory. Its digest is an expected pin,
   not self-issued authority.

Suggested bounds reuse the source qualification envelope: 256MiB source,
65536 source entries, 512-byte relative paths; 2GiB/300000 entries per operation,
64MiB canonical header and 10-minute contextual operation deadline. Enforce
bounded copies/walks/Git stdout and source-schema limits; verify sufficient local
capacity before copying. Exceeding a bound refuses without truncation or a
physical receipt. No global umask, credentials or runtime configuration changes.

## Fixed forward transaction

After Prepare returns durable success, its ordinary writer closes. The trusted
owner acquires BeginBrainMaintenance with the exact prepared identity. A handoff
race is possible: Install must recheck the ENTIRE old inventory and runtime
eligibility under the new owner before its first rename. Any change invalidates
preparation. Never edit the old header to bless a changed source.

Fixed component order: .git, brain, .dira, serenity.yml, .gitignore, then the
three .serenity/index.db family names. Never move the brain root or .serenity,
replace lock inodes, follow links, or run a whole-root directory swap. Missing
old/new optional components are represented explicitly. New index files live
outside replacement/ so its canonical proof remains independent of runtime.

Before mutation, verify header/copies/ref/owner and fresh cold admission, then
persist/sync install intent. For each component:

- Persist intent before rename; move old canonical component to its absent
  fixed retained/ slot, sync both affected parents.
- Publish its prepared new component, sync both parents and record completion.
- Recheck the actual component inventories, owner and admission at boundaries.
  Cancellation, denied rename/fsync or expiry leaves marker/operation held and
  returns incomplete/retained state. Do not run an old-state rollback.

Resume uses fresh admission and the same header plus actual inventories. The
three normal locations describe states: old at canonical/new staged/no retained;
old retained/canonical absent/new staged; old retained/new canonical/no stage.
Optional absences are handled explicitly. Matching old/new bytes do not confuse
slot presence. Completed-but-unrecorded renames may advance after directory
resynchronization. Unknown identities, impossible duplicate slots or extra files
refuse; a phase flag never authenticates storage. Original proof is reconstruction
evidence, never an automatic source for resurrecting excluded content.

Before reporting installed-marker-held, verify all new component inventories,
Git HEAD/clean tree/object set, canonical fences, rebuilt index identity, stable
ownership and fresh admission, then persist/sync completion. Retained old .git,
working components, indexes, original proof, bare candidate, failed preparation
stages and other copies remain sensitive controlled copies. Installation is not
physical completion. A separate bounded owner-qualified disposition is required.

## Qualification and explicit remaining gates

Use actual provision/pool/Git/episode fixtures, preserve an independent source
and prove removed events/checkpoints cannot be read from the installed tree or
any new Git object. Preserve unrelated canonical bytes and lexical retrieval.
Exercise real process death before/after every rename and critical fsync, replay
with stale phase records, wrong roots/IDs/header/marker, changed source during
handoff, unsafe paths, relevant fence drift, expired/replaced authority, control
rows, unknown runtime artifacts, cancellation and denied cleanup. Reopening must
use fresh admission and preserve stable lock inodes. No provider-backed tests.

This increment qualifies forward recovery from a valid durable marker plus a
durable prepared header. An unknown/partial marker created by interrupted Begin
remains a distinct fail-closed reconciliation case. It does not claim universal
automatic recovery or complete T5.2/T5.3/T1.3. Cold owner integration, runtime/control
preservation beyond this cohort, ambiguous legacy derivative closure, all managed
copy disposal, actual host/power-loss evidence and real capture remain open.

## Frozen shared seam

`types.go` defines exact request/ref/admission/result/header/inventory fields,
schema and component order. All inventories have a mandatory `.` entry; explicit
absence is a single absent entry, empty directories are preserved, and modes,
slash-relative paths and byte hashes are bound by canonical JSON. Fixed layout
is documented beside the header. Root owns shared types, canonical header and
inventory helpers; preparation and transaction files have separate authors.

Original proof includes exactly the five canonical components, including the
unchanged `.gitignore` runtime exclusion. Old-index and mutable SQLite inspection
copies live outside it. Reject quarantined entity pages using non-logging
`index.AuditPages` BEFORE Rebuild; do not change global slog configuration.
ColdAdmission must work against partially installed canonical roots and establish
cold custody/current exclusions independently of the original worktree HEAD.
All new-index handles close/checkpoint before pinning its exact family. Actual
post-install lexical readers must verify retrieval, not just file hashes.

Shared root-owned helper signatures (errors always static):

```go
validateRoutes(source, operation, operationID string) error
inventoryPath(ctx context.Context, path string) (inventory, error)
verifyInventory(ctx context.Context, path string, want inventory) error
copyInventory(ctx context.Context, source, destination string, want inventory) error
syncTree(ctx context.Context, path string) error
syncDirectory(path string) error
writeHeader(ctx context.Context, directory string, h preparedHeader) (string, error)
loadHeader(ctx context.Context, ref PreparedRef) (preparedHeader, error)
validateRuntime(ctx context.Context, source string, maintenance bool) error
validateCanonicalLayout(ctx context.Context, source string) error
```

Header loading validates strict canonical bytes/digest/schema, exact ref identity,
route hashes, fixed component count/order and inventories (including no invalid
relative paths). It is not authority. Layout validation permits exactly the five
canonical names plus `.serenity`; runtime validation permits only stable locks,
index family and (maintenance=true only) the held marker. Both reject unknown
files or directories. inventoryPath uses the source bounds; preparation also
enforces aggregate operation bounds. Helpers never remove anything.

Transaction durable record refinement: `install.json` is an immutable complete
intent for ALL fixed component moves, synced before the first rename. Actual
slot inventories determine progress, avoiding mutable phase counters. A separate
fixed `installed.json` is exclusive-created and synced after final verification;
existing exact canonical completion bytes are idempotent. Fixed content-free
`install.json.pending` and `installed.json.pending` stages are written and synced
before exclusive final publication; these two stages are the only extra names.
A private owned regular single-link pending file must match a byte prefix of the
exact deterministic record; Resume may append the missing expected bytes after
actual-slot and fresh-authority validation, never truncate or remove unknown
bytes. A final record plus pending stage is ambiguous and refuses. Pending
completion requires all actual components final. Resume syncs the operation
directory before another canonical mutation. Unknown or malformed final records
refuse, but process interruption at creation/midwrite of the admitted pending
records recovers forward. Tests cover both record create/write/sync/publish
boundaries as well as component renames.

Header freeze correction during integration: `Inspection inventory` pins the
complete closed post-qualification inspection directory (including SQLite-owned
sidecar effects). Install/Resume verify it just like original/old-index/candidate;
unknown or changed inspection copies refuse. No sensitive copy is left unpinned.
