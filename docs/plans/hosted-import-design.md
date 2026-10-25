# Hosted brain import design

Status: HM.2 proposal for chief-architect re-review; **not implemented or
approved**. This document does not create tasks, accept a protocol, authorize an
upload, or qualify a deployment. Chief-architect review remains required.

## Authority and scope

The [PR277 review](https://github.com/sirerun/serenity/pull/277#issuecomment-5853879867)
and [scope addendum](https://github.com/sirerun/serenity/pull/277#issuecomment-5853885821)
are incorporated subject to the later founder rulings:

- [dec-1178](https://github.com/sirerun/serenity/pull/277#issuecomment-5853948257):
  use David's existing account and its live server allowances; import into its
  **existing default brain**, preserving remembered claims and preferences;
  provenance-keyed deduplication; operator-only measured v1 exception to T23.44.
- [dec-1179](https://github.com/sirerun/serenity/pull/277#issuecomment-5854016681):
  intended v1 founder input is **native dira plus review-qualified distilled
  claims from HQ-DISTILL-01**, not a mirror of retired gbrain or raw HQ documents.
  The earlier new-empty-brain, Builder/comped-plan, and raw-document namespace
  recommendations are superseded. HQ prose is eligible for distillation, not
  categorically excluded because it lacks frontmatter.

HQ-DISTILL-01 owns classification of HQ brain, plan, design, docs, receipts,
sitreps and memory sources as current / superseded / duplicate / scratch. Only
current knowledge becomes claims, precepts or entity summaries, with canonical
source references and `review: true` pending founder disposition. Superseded and
scratch material is dropped; duplicate sources do not produce duplicate claims.
Source documents remain in their owning Git repositories; their bodies are not
mirrored. A distilled current claim may cite a receipt or plan. This replaces the
old HM.5 source-document placeholder; it is not a new accepted ingest contract.

Native dira input uses the module boundary in ADR008 and the **dira-module**
ADR014, not a reimplementation of dira's parser. Proposed mapping preserves the
decision text and why, rejected alternatives and `why_not` as provenance, and
`confirmed_by` / tier as attribution. A rejected alternative is not automatically
an accepted fact or a Serenity supersession edge. Actual supersession requires
the explicit relationship and review semantics required by ADR009. Dira
precepts remain unmodified entries with `applies_when` in the body (ADR008), not
invented frontmatter or silently accepted preferences. The exact native-entry
mapping and review disposition need schema-qualified design review against the
pinned dira `entry.schema.json`; importing native entries into a hosted claim
projection is not claimed to exist.

## Existing behavior and canonical references

Source baseline inspected: `29936ab2bc0c0c659683750cd1e935ad2ce226e6`; this
worktree's starting head is `2f681b6bc6a80b513adab45d7ddd74592a801a93`.
The relevant source at those revisions agrees outside this design document.
These are source observations, not live account or production verification.

- `serenity import --from-gbrain <path>` is supported **local legacy import**,
  implemented in `internal/cli/import.go` and `internal/import/gbrain/` under
  ADR009. It parses eligible frontmatter/entity pages, validates mappings and
  destinations, and has a checkpoint and advisory `gbrain.lock` in
  `.serenity/import`. Changed existing pages require explicit migration
  resolution. Its `Result` and `FieldReport` are local evidence, not hosted
  counters or a founder-corpus specification. It does not parse arbitrary HQ
  prose or supply the proposed native dira/distillation transfer path.
- `Gateway.Export` in `internal/hosted/gateway/lifecycle.go` emits a ZIP with
  `brain.bundle` (Git bundle) and `facts.json` (nonexpired memory projection).
  It takes maintenance/account locks, obtains `Pool.Acquire`, locks
  `BrainRuntime.Mutations`, flushes and exports. There is no transfer manifest
  in that output and no import inverse implied by the export format.
- [ADR008](../adr/008-precepts-on-dira-applies-when-in-body.md),
  [ADR009](../adr/009-gbrain-import-field-mapping.md),
  [dira-module ADR014](../adr/014-dira-as-a-module-not-vendored.md),
  [hosted-topology ADR014](../adr/014-hosted-tenancy-topology.md),
  [ADR016](../adr/016-hosted-plans-metering-stripe-billing.md) and
  [ADR017](../adr/017-hosted-operation-and-recovery-contracts.md) govern the
  mapping, ownership, billing and recovery boundaries. The two ADR014 files
  concern different boundaries; neither is an import implementation.
- Current [hosted interfaces](../launch/hosted-completion/interfaces.md),
  `internal/hosted/contracts/{operations,fence,commitfence}.go`,
  `internal/hosted/operation/` and the gateway/service reconciliation paths
  contain operation identity, accounting and fencing contracts. Where older
  interface prose disagrees (for example, whether Export takes maintenance
  exclusion), the current source owns the observation. They do not
  provide a qualified whole-brain atomic import publisher. ADR017 explicitly
  notes that the live Git working tree and `writer.Flush` do not give a
  temp-and-rename publication step.
- Later [ADR025](../adr/025-postcanonical-search-indexing-for-hosted-remember.md)
  is accepted for the locally reviewed recovery implementation and makes
  search indexing postcanonical for hosted remember; import must not
  require a successful index write to decide whether canonical data committed.
  [ADR029](../adr/029-episodic-erasure-and-replay-fences.md) is a proposed
  episodic erasure extension, not an accepted or implemented replay guarantee;
  this import proposal requires non-resurrection and leaves its fence adapter
  unresolved. [ADR030](../adr/030-zati-context-and-gist-boundaries.md) is accepted
  direction for planning, not runtime qualification; it does not grant another
  product authority over Serenity records or turn a gist into memory.

## Transfer v1 boundary and proposed schema

V1 is an **operator-run, local-on-VM transfer**, not an authenticated customer
upload route. A proposed offline builder takes revision-pinned native dira and
the review-qualified HQ-DISTILL-01 output, validates them in a scratch Serenity
brain, and emits the following transfer. Builder and operator command names,
CLI flags, schema files and mapper implementation remain unresolved; none is
introduced by this document. The hosted service must never invoke
`gbrain.Import` directly against a live brain. A legacy gbrain conversion, if separately requested, remains an offline
local scratch conversion and does not redefine the founder v1 input.
V2 may later add authenticated customer upload,
status and commit routes, gated on T23.44 and separate review.

Proposed ZIP v1 has exactly three root regular-file members: `brain.bundle`,
`facts.json`, `manifest.json`. It extends the existing export container with a
manifest; plain export is not automatically a valid import. No raw checkout,
mail archive, runtime DB, credentials, Git hooks, index database, symlinks,
submodules, executable payload or additional archive members are accepted.
The bundle contains only an allowlisted canonical scratch tree, not source
repository history or a replacement target repository. A restricted bundle
loader must reject extra refs/history/paths and never run hooks or import Git
configuration. The exact canonical path allowlist and bundle ref name require
review before any transfer is accepted.

Proposed manifest object (field names here are a schema proposal, not a shipped
JSON Schema or accepted interface):

| Field | Required meaning / validation |
| --- | --- |
| `version` | Integer `1`; unknown versions rejected before target writes. |
| `inputs` | Sorted list of canonical repo identity, full commit OID, allowed source paths, input kind (`dira` or `hq-distill`), and mapper name/version. Distillation also identifies the HQ-DISTILL-01 output revision and review receipt; no floating branch references. |
| `payloads` | Exactly `brain.bundle` and `facts.json`, each with SHA256 and uncompressed byte length; no manifest self-hash. |
| `canonical` | Bundle ref, source commit OID, canonical tree OID and Git object format; independently verified from bundle contents. |
| `files` | Sorted canonical-tree relative paths with SHA256 and byte length for every regular file; validate against the bundle tree and the reviewed allowlist. No undeclared paths or filesystem-supplied permissions. |
| `counts` | Input entries, candidate claims, reviewed/pending claims and canonical payload bytes. These are recomputed, never trusted for admission. |
| `claims` | Sorted records described below, covering every transported claim exactly once. No payload claim without provenance and no orphan manifest claim. |
| `audit` | Mapper version and structured field-loss report; every input classified, excluded inputs yield zero claims. Unmapped fields or lossy transformations require explicit reviewed resolution, never silent truncation. |

Each `claims` record requires `claim_id` (scratch-local only), `kind`,
`provenance_key`, `content_sha256`, `source_refs`, `transform`, `visibility`,
`review` and `payload_locator`. Locator identifies the canonical record in the
scratch tree, not an arbitrary destination path. `kind` distinguishes claim,
precept and entity summary. V1 visibility is private to the target account;
public/team widening is rejected. `review` is a boolean: distilled records and
founder preferences are `true`; changing that requires explicit founder
acceptance evidence, never inference from import success. Source truth remains
canonical records; `facts.json` must match their nonexpired projection rather
than overwrite them as a separate authority.

Each source reference contains canonical repo identity, repo-relative path,
full source commit, stable entry/row/section identifier and attribution tier;
multiple refs are sorted. For dira it includes the `dec-NNNN` entry identity;
for distillation it retains the **original** HQ source references as well as
output revision/transform provenance, not a retired gbrain-derived citation.
Repo identities must be allowlisted and credential-free; absolute paths,
traversal and credential-bearing URLs are rejected. Row identity stability for
prose distillation is an unresolved producer contract; fail closed if absent.

Proposed dedupe key is a versioned SHA256 over a length-delimited canonical
encoding of `(canonical repo, stable source path, entry/row id, output role)`.
Source commit, transform version and content digest are recorded separately so
a revision does not accidentally mint a duplicate of the same logical claim.
Relocation/alias handling needs explicit producer review; no heuristic matching
by similar text. Exact JSON canonicalization, digest encoding and role vocabulary
must be frozen with the eventual schema. Array ordering, timestamps and input
pinning must make repeated builds produce byte-identical `manifest.json`;
ZIP/Git byte identity is not required when tree and payload digests verify.

## Additive publication into the default brain

Resolve the account's default brain from authoritative server ownership state,
never a caller-supplied filesystem path. Recheck account binding, plan, deletion
state and default-brain identity at commit. Existing claims and preferences are
the base; no empty-brain precondition, whole-tree replacement or resetting HEAD
to the transfer's commit is permitted.

- Absent provenance key: add a reviewed candidate with fresh target identity.
- Same key and same canonical content digest: skip without canonical mutation
  or repeat charge; retain existing disposition and preferences.
- Same key with changed content, incompatible attribution, target ID collision
  or explicit erasure fence: report conflict and refuse publication until
  reviewed resolution. Do not overwrite, silently supersede or resurrect.
- Existing records lacking a provenance key remain untouched. Similar text does
  not authorize merging them or changing an existing preference. Explicit
  cross-source equivalence remains a separate unresolved review interface.

Dry-run resolves all keys and references against a pinned target base and
produces additions/skips/conflicts plus the expected **merged tree OID**. An
unresolved conflict blocks the whole batch. Importing identical input twice
succeeds as an unchanged no-op on the second run, including after a lost reply.
Only new records count toward live-memory growth. Pending precepts remain
pending and do not change active standing preferences.

Verification uses canonical Git tree hashes first: validate the scratch tree
against the manifest, then require export's `brain.bundle` tree to equal the
expected merged tree after publication. For a nonempty default brain that is
**not** equality with the input scratch tree. Show preservation of the base
claims/preferences and equality of the expected additions. The field-level
`FieldReport` and projection/citation checks are secondary, not substitutes for
tree equality. Future live recall checks must return a known decision with its
canonical repo/path/commit and `dec-NNNN`; a raw document body must not appear
merely because it was an input to distillation.

## Admission, exclusion, crash and counter invariants

The following are required proposed behavior, not evidence that the importer or
its publisher exists:

1. **Live allowances and measured v1 exception.** Read the existing account's
   current plan/allowance from the server at preflight and recheck under commit
   exclusion. Measure candidate/new/skipped/conflicting/live claim counts;
   entry or page counts are not claim counts. For legacy local conversion,
   `Import.Result.Claims` is an input to measurement, never a substitute for
   final deduped live cardinality. Return counted headroom/shortfall and required
   versus available bytes; no assumed Free/Builder plan, comp, upgrade or silent
   truncation. The review's historical corpus estimates are not measurements
   of this transfer or the account.
2. **Operator-only physical preconditions.** Before touching the default brain,
   record measured allocation for archive expansion, scratch tree, Git objects,
   temporary merge/copy, indexes/WAL and expected final growth, plus free device
   space and a reviewed safety margin. A configurable proposed `MaxImportBytes`
   and measured disk-headroom threshold refuse oversize imports. Exact thresholds
   and measurement receipt require operator review. Recheck space before commit;
   do not proceed on unknown allocation. These are cooperative measured
   safeguards, **not an OS-enforced hard bound**. The dec-1178 exception is for
   one operator-run v1 path and does not waive T23.44 for customer uploads.
3. **Private validation.** Stage outside the live brain on private storage;
   reject traversal, links, duplicate paths, malformed records, expanded-size
   violations, undeclared members and digest/projection mismatches. Define
   compressed-size, expansion-ratio, per-entry-size and entry-count refusal
   thresholds before accepting an archive; these are parser safeguards, not
   hard physical storage enforcement. Validate
   schema, source allowlists, review flags, vocabulary, explicit supersession
   edges, expiry and erasure/replay fences before canonical mutation. Preserve
   source timestamps and TTL; do not refresh expired content merely by import.
   Parsing and index building must not execute input programs. Raw text never reaches operational
   logs, telemetry or public errors; only restricted IDs, counts, sizes, hashes
   and outcomes do. Owner-only diagnostics redact source text by default.
4. **One writer, shared fencing.** Follow the existing lock order: maintenance
   guard, account exclusion, `Pool.Acquire`, `BrainRuntime.Mutations`, then
   ADR017 `EnterCommit` / `EnterCanonical` narrow section around canonical
   publication. Release in reverse; no SQL transaction spans provider/disk I/O.
   Reconciliation uses the shared `BrainFence.Fence` exclusive side under
   maintenance read exclusion, never acquiring account or mutation locks while
   fenced; this preserves ADR017's separate reconciler lock order. The local
   gbrain advisory lock does not exclude hosted remember or housekeeping; index workers and flush paths must also
   respect exclusion. Until that coverage is qualified, drain/fence the target
   runtime (including reads for multi-file publication), close cached handles
   and refuse to publish if another writer can operate. An in-process mutex
   alone does not fence another process or host. Same-host ownership uses
   `writer.AcquireBrain`'s lifetime flock; cross-host exclusion requires the
   restore fence, not that flock. Lease/ownership loss stops
   publication. Revalidate the target base while fenced; a changed base forces
   a new merge/measurement rather than using a stale plan.
5. **Durable identity and receipt.** Before canonical entry, persist an import
   operation bound to account+brain, request retry identity, bundle fingerprint,
   source manifest digest, base/expected merged tree OIDs, original quota period
   and reservations. Same retry identity with another fingerprint is refused.
   Existing operation-ledger/fence contracts are the integration boundary;
   import-specific batch evidence and persistence fields are unresolved.
   An operator restart must recover this same operation, not create another
   charge or guess from an absent response.
6. **Canonical publication boundary.** Private fetch/validation never advances
   live refs or edits the live working tree. Publication needs a reviewed
   whole-batch mechanism with durable recovery evidence for the expected merged
   tree. Git fetch, reset, sequential file copies, or index rebuild alone do not
   provide atomic visible publication. The current writer has no qualified
   import swap primitive. If unavailable, refuse import; do not describe an
   operator command as safe merely because staging finished. Reads stay fenced
   during any incomplete multi-file publication/recovery. Once canonical data
   commits, a search-index failure is repairable postcanonical work under
   ADR025, not evidence for rolling back data or reapplying claims.
7. **Crash/retry matrix.** Before canonical entry: discard private stage and
   release holds only with durable no-write proof. After entry but before a
   proven commit: exclusively fence, inspect recorded base/expected trees and
   canonical operation evidence, restore/finish only by a reviewed publisher
   recovery protocol; unknown/partial state stays blocked for operator review.
   After canonical commit but before ledger finalization: prove the recorded
   resulting tree/additions, finalize that same operation once and rebuild
   derived state. After finalization but before response: return the recorded
   receipt. No uncertain case retries canonical writes, refunds on timeout,
   or declares success from index state. A kill during private bundle fetch
   leaves the live tree equal to the base; kills during publication must not
   expose partial claims and require qualified recovery before reopening.
8. **Counters and postcommit reconciliation.** Reserve only measured new live
   cardinality and defined write/input units against live allowances before
   publication; skip-only retries consume no duplicate units. Exact unit
   mapping for this batch is unresolved and must be reviewed under ADR016/017,
   not improvised as one page equals one memory. Finalize write/input counters
   together using the existing ledger transaction and original quota period.
   Reconcile stored-memory count and measured final storage after commit;
   reconcile staged-byte release after cleanup. Committed-but-unfinalized work
   cannot lose its hold or be treated as absent; unknown evidence blocks
   release/admission. Report input, additions, skips, pending review, conflicts,
   resulting live count and measured bytes separately. No counters inferred
   solely from manifest declarations, archive size or a successful HTTP reply.
9. **Privacy and lifecycle.** Authenticate/authorize the operator's account
   scope and use existing private per-brain access controls. Never transfer
   account/API credentials, runtime DBs, foreign history or erased records.
   Imported private text is not sent to embedding or composition providers by
   this transfer; any future provider indexing needs separate policy review.
   Source paths/digests can reveal private activity: manifests, staging,
   receipts and backups share the brain's private custody. Terminal failure
   and success remove staged payloads under a reviewed bounded retention and
   cleanup policy; restart cleanup must preserve unresolved recovery evidence.
   Delete/account erasure must invalidate staged or pending imports and block
   replay. An import never grants publish, cross-account or cross-product
   authority. V2 must separately qualify authenticated ownership-bound upload,
   limits, expiry, cancellation and task44 accounting before exposure.

## Remaining design and acceptance work

This table records dependencies for review, not new roadmap tasks or accepted
contracts. Proposed command names, routes and schema locations from the review
remain proposals. No implementation, build, deployment or personal upload is
part of HM.2.

| Design boundary | Required future evidence / unresolved interface |
| --- | --- |
| Native dira and HQ-DISTILL-01 producers | Version-pinned `entry.schema.json` mapping, decision/why/alternative attribution, stable provenance identities, current-only distillation, founder review dispositions and canonical original-source citations; no retired gbrain/raw-doc mirroring. |
| Transfer format | Reviewed JSON schema/canonicalization/path allowlist; existing export payloads plus generated manifest validate; repeated builds have identical manifest; malformed/private/foreign content refuses without target writes. |
| Default-brain additive publisher | Shared writer/process exclusion and whole-batch recovery mechanism; existing claims/preferences preserved, same-key no-op, conflicting revisions refuse; expected merged tree verified through export with secondary field audit. |
| Operation and allowance adapter | Live account allowance read, counted shortfall, measured VM admission receipt, agreed batch units, ledger evidence and counter reconciliation; no hard-bound claim. |
| Crash and restore qualification | Kill points around fetch, canonical entry/publication, finalization and lost reply; no partial visible batch or duplicate charge. Later E23 off-host restore rehearsal verifies artifact consistency and accounting separately from ADR017's three-fact restore fence: sealed journal generation, provider-verified old-instance stop, and revocation of all old credentials/sessions. None is supplied here. |
| Customer upload v2 | Authenticated account/brain-scoped begin/upload/status/commit design only after T23.44 storage admission qualification and separate review. The operator exception cannot be reused as customer-route qualification. |

Chief-architect re-review must resolve the schema, producer identities, whole-batch
publisher/recovery, lock coverage, batch accounting and retention interfaces
before implementation authority is inferred. This document claims no third-party
approval, live import, measured corpus/account allowance, round-trip result,
recovery rehearsal or production acceptance.
