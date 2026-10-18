# Episode erasure and production capability extension — contract r1

T5.1 design freeze. Existing synthetic EPISODES_v1, external replay identity,
legacy governed retraction, backup manifests and writer ownership stay compatible.
This contract is not a capability implementation or a production grant.

## Required erasure transaction

The existing maintenance owner serializes an operation against one authenticated
brain, exact active repository HEAD, source inventory and independently admitted
external deletion-history boundary. Inputs identify only canonical opaque brain,
conversation/command hashes, expected generation and immutable source/receipt
pins. Authority is acquired through existing verified startup/recovery history,
not a model-supplied seal, caller boolean, or local canonical copy. Recheck that
boundary and current exclusions before activating or disclosing a prepared copy.
No physical-complete receipt follows from a local tombstone or inactive candidate.

Inventory every retained Git ref/object/history, working tree, index, extracted
fact/claim/checkpoint, queue, owned stage, export and backup version. Each item has
an exact immutable content identity, owning store and classification: excluded,
independently retained, invalidated/rebuildable, or unsupported. Unknown dependency
or custody blocks that scope; no preserve-everything or discard-everything fallback.
Unrelated canonical material stays byte-identical. Shared attachments retain only
independently authorized references; conversation deletion is not global-content
erasure. Derived claims need independently retained provenance or invalidation.

Construct a new candidate in a private bounded stage, recreating only retained
objects and fixed metadata. Never copy old packs/reflogs or assume unreachable
objects were erased. Verify complete closure and absent excluded objects before
publication. Bind candidate/source/history/inventory digests and generation in
content-free receipts. Under current maintenance ownership atomically install a
verified active pointer, with crash-recoverable checkpoints and compare-and-swap.
No reads open against a stale source or candidate merely because it exists.
A stale HEAD, history change, unknown publication or interrupted cleanup retains
exclusion and a parked/reconcilable operation. Never silently retry by minting a
new command, unfreeze an account or advertise physical completion.

Controlled old repositories, stages, backups and exports remain sensitive copies
until their own bounded cleanup completes. Before a terminal purge receipt, verify
actual object absence and disposition of every controlled reference. Expiring
provider backups carry an explicit enforced deadline and pending-copy state;
source deletion alone does not close them. Unreachable devices and user-held
exports are named residual scope, not fabricated complete erasure. Content-free
replay identity is retained under the existing brain authority after bytes go.
No promise of physical disk sanitization or deleting independent host originals.

## Export, restore and rejoin

Portable export binds exact acknowledged event/checkpoint/attachment bytes,
coverage and current fences. Restore remains frozen until current independently
admitted full deletion history reconciles all source and dependent state. Older
snapshots cannot restore deleted evidence or old permission. Queue drain and
returning devices synchronize the current complete fence boundary before upload;
server publication independently rechecks generation, closing sync/upload races.
Missing custody, stale/missing history, expired cursor and unknown provider
outcome stay unavailable. Existing backup cut/persistence and authority owner
contracts are consumed; no second journal or new provider exists.

## Negotiated production capability

Keep synthetic v1 and its explicit qualification field unchanged. Real-history
support requires an additive negotiated schema/capability, authenticated selected
brain, recorded enrollment and standing policy, current producer/workspace mapping,
versioned qualified service/client pair and safe erasure/copy/restore receipts.
A missing or false capability denies admission before local capture/disclosure.
Do not turn a synthetic allowlist entry into a real enrollment. Service defaults
remain off. Advertise separate scopes/limits for text, attachments, exact export,
physical purge, managed restore, extraction and native receiver continuation.
Unsupported fields remain false rather than inheriting another capability.

Enrollment, pause, private mode, remapping, disconnect and deletion are distinct.
Pause prevents new collection; it cannot erase acknowledged evidence. Removing
owned host configuration cannot delete the shared brain. Native host trust is
preserved. Automatic extraction runs only under existing scope and provider policy,
outside canonical writer sections; unavailable extraction cannot lose archived
originals. Ordinary historical content never approves execution or widens access.

## Qualification envelope and source gates

The first source cohort is synthetic, offline, one exact brain per request,
128 linear Git revisions, at most4096 canonical files/revision,256MiB aggregate
Git/read budget,32MiB admitted episode archive and existing episode text/attachment
bounds. Exceeding any bound refuses without truncation or active-state mutation.
Queue bounds remain explicit caller-selected limits within existing package caps.
No model calls, paid services, real history or production writes enter this cohort.
Local activation/restore fixtures operate only on isolated test directories.

Tests must prove mixed retained/excluded sources, stale/foreign/missing authority,
unknown derivatives, shared attachments, old history/object/stage copies, concurrent
exclusion, denied cleanup, quota exhaustion, cancellation and crash at every
publication boundary. Exact HTTP status/body and post-restart invariants are
required when actual routes change. Canonical unrelated bytes, source identity,
unknown outcomes and immutable operation accounting must remain inspectable.
Full T1.3 waits for these real owner integrations and controlled-copy evidence;
a narrower preparer increment retains its unsupported flags and open parent.
