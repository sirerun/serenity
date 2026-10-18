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

## Frozen v2 negotiation and admission shapes

The additive route is `POST /episodes/v2`, schema `serenity.episodes/v2`, with
existing bearer authentication, exact authenticated `project_id` brain binding,
JSON duplicate-key/unknown-field rejection and existing response byte limits.
It neither changes nor silently upgrades `/episodes/v1`. No redirect or URL
fallback is permitted. An old server's404/405 is unsupported, not enrollment.

The capability request has required `schema`, `project_id`, `operation` equal to
`capabilities`, and `producer_id` (existing opaque EpisodeID grammar), plus only
optional `workspace_id` as defined below. It requires
memory:read. The response contains exactly `schema`, `project_id`,
`service_revision` (immutable source/release identifier), `supported_schemas`
(array of exact schema strings), `mode` (`disabled`, `synthetic`, `enrolled`),
`admission` (null or the reference below), `operations` (explicit operation names),
`limits` (the existing v1 numerical capability keys), and `capture_ready` boolean.
A successful negotiation is not authority to capture. A client must recognize
v2, compare brain and producer, require enrolled mode, current non-null admission,
append in operations, capture_ready true and adequate finite limits. Any absent,
unknown, inconsistent or unsupported field refuses collection/upload; never infer
support from a healthy service or another client’s negotiation.

Admission reference has exactly `enrollment_id`, `enrollment_version`, `policy_id`,
`policy_version`, `producer_id`, `workspace_id`, `release_id`, `valid_from` and
`valid_until`. IDs use the existing opaque EpisodeID grammar, versions are
positive integers, and times are UTC RFC3339 timestamps. The selected reference
is the exact one server-authorized mapping for that producer/workspace; ambiguous
or absent mappings return mode disabled/admission null. A producer with several
mappings negotiates separately with an additional optional `workspace_id` field;
when omitted it is accepted only if there is exactly one authorized mapping.

The server-owned record `serenity.episode-enrollment/v1` contains that exact
reference plus `project_id`, `enabled`, `policy_digest` (canonical lowercase
SHA256), `allowed_operations` (subset of v1 operation names), `limits` (only
finite values at or below qualified service bounds), and `qualification_digest`
(exact verified safety/release/copy evidence). It is written only by the existing
trusted configuration/maintenance owner through its reviewed change path and
never created by an agent request, repository config, capability negotiation or
model field. Policy bytes resolving that digest are an existing trusted client
policy/mapping version; records do not embed credentials. New administrative
transport is not introduced by this protocol. No record is effective without
current verified release/safety evidence and existing brain/grant authority.

Every v2 operation uses the exact existing v1 body for that operation, replacing
schema with v2, removing qualification and adding required `admission` with the
reference above. There is no qualification override in v2. Append/checkpoint
mutation producer/workspace and brain must match admission, current policy and
current bearer grant. The server re-resolves the record and validates the exact
reference, release, current scope, enabled flag, policy digest, validity and
safety state before every request and inside serialized publication. A successful
capability response is not cached write authority. Existing generation, command
identity, quota, read eligibility, unknown outcome and exclusion replay semantics
remain; retries retain their exact old identity and cannot bypass revoked policy.

Only existing v1 operations may be listed at initial v2 negotiation. Production
export and physical purge routes are not invented by listing feature names:
they require their own implemented qualified contract and are absent until then.
Automatic extraction uses the exact policy outside the append transaction; no
model provider is implied by a capability response. Qualification must still
prove the export/purge/restore safety prerequisites before capture_ready is true.

Unsupported schema/operation is400 `unsupported_version`/`unsupported_operation`;
malformed/duplicate/unknown fields are400 `invalid_request`; missing grant or
wrong authenticated brain is403 `forbidden`; absent/disabled/expired/revoked
admission is403 `enrollment_unavailable`; a changed reference, policy or release
is409 `admission_changed`; missing current safety/replay evidence is503
`capability_unavailable`. All deny without collecting/publishing new content or
revealing another brain. Malformed and incomplete negotiation fails closed.
No original payload or credentials appear in failure bodies. Existing ambiguous
post-send outcomes require status/reconciliation and never mean nonexecution.

Disconnect/pause/revoke changes the server record version or enabled state
through its existing owner; there is no model-callable enablement endpoint.
Clients stop capture on denied renewal and retain only policy-permitted queued
bytes until their governed disposition. Re-enrollment is a new owner-approved
reference, not silent reassignment of old queued evidence. Tests must cover old
clients, unknown future schema, foreign/mismatched mapping, version drift,
expiry, disablement between negotiate/publish and changed release safety state.
