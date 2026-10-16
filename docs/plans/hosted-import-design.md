# Hosted brain import design

Status: proposal for `HM.3` architecture review. No Hosted import endpoint or
portable local transfer bundle is implemented by this document.

## Existing capability and missing piece

`serenity -C <brain> import --from-gbrain <checkout>` imports gbrain Markdown
pages into a local Serenity brain. It preserves page metadata and provenance,
maps facts and takes to review-qualified claims, checks field-level round trips,
and resumes an interrupted local import. It does not read database-only
gbrain rows, and it cannot write to Serenity Hosted.

Serenity Hosted currently has an account-owned brain export, but no matching
import operation. Calling MCP `remember` repeatedly is not a substitute: it
turns each input into an individual memory fact and cannot preserve canonical
pages, graph links, original file structure, or a whole-corpus transaction.

## Proposed customer flow

1. A local command reads a user-reviewed, immutable source manifest and creates
   a deterministic Serenity transfer bundle. The manifest names each included
   file, original source and revision, visibility, SHA-256, and the explicit
   transformation used for gbrain pages. It rejects paths outside the approved
   roots, symlinks, non-regular files, duplicate destinations, and undeclared
   files. The existing gbrain importer remains the translator for gbrain page
   content; ordinary Markdown documents remain source documents with their
   original paths and attribution.
2. `serenity hosted import --bundle ... --brain ...` authenticates with the
   selected Hosted brain's existing scoped bearer credential (`memory:write`).
   The service derives account and brain identity only from that credential;
   the upload cannot select another account or override the credential's brain.
3. The service begins a durable, brain-scoped import operation with an
   idempotency key and content digest, accepts bounded chunks into private
   staging, and reports a resumable operation ID. A retry with the same key
   and digest resumes; a different digest conflicts.
4. Commit validates the complete archive and manifest, reconstructs the
   canonical content in a private destination, runs the same source-field and
   visibility checks as local import, checks actual storage and record
   allowances before publication, then publishes the complete change under the
   brain writer fence. No staged content is available to recall or embedding.
5. The client verifies the import by exporting the brain and comparing source
   hashes, canonical page hashes, claim fields, source references, visibility,
   lifecycle, and graph links against the local manifest. A second unchanged
   import is a no-op. Any conflict with existing human-authored content stops
   with a field-level report and never overwrites it.

## Required server invariants

- Every begin, chunk, status, and commit action is authenticated and authorized
  against the same account and brain. Account identity is never accepted from
  request JSON, path metadata, or archive contents.
- Only one active import mutates a brain. The existing writer ownership and
  account/brain lock ordering are reused; retries cannot race with writes,
  deletion, or another import.
- Compressed size, expanded size, entry count, per-entry size, and total staged
  allocation are bounded before publication. Paths must be relative, canonical,
  unique, and confined. Archives reject symlinks, hard links, duplicate names,
  traversal, special files, excessive compression ratios, and unrecognized
  entries. Limits derive from plan allowances and the qualified staging bound,
  not a new undocumented allowance.
- Storage admission reserves staged and published bytes transactionally and
  includes Git objects, indexes, and unpublished growth. Until T23.44's
  allocation accounting and OS-enforced staging bound exist, production import
  remains disabled.
- Imported private content is never sent to embedding or composition
  providers. Imported gbrain predicate/confidence interpretations retain
  `review: true`; transfer never promotes them to accepted decisions.
- Quota exhaustion, malformed input, timeout, disconnect, or restart cannot
  publish a readable partial corpus or lose agreement between the operation
  ledger, canonical Git state, and usage counters. Rollback preserves prior
  human changes and is rehearsed through the qualified backup/restore path.
- Import data, archive names, and page contents are excluded from logs,
  telemetry, and public error responses. Diagnostics stay with the
  authenticated owner and redact source text by default.

## Interfaces to freeze before implementation

Architecture review must approve:

- The versioned bundle schema and whether canonical raw Markdown uses a
  separate source-document namespace alongside gbrain entity pages.
- The begin/chunk/status/commit protocol, chunk size, maximum active uploads,
  operation-key scope, durable checkpoint format, and cancellation behavior.
- Whether the target is an existing brain (with atomic additive merge) or a
  newly allocated brain (with publication and rollback semantics). The default
  Hosted brain already contains three owner-approved Claude preferences, so an
  implementation must preserve them or prove they are represented in the
  bundle; replacement is not safe by assumption.
- Storage and cardinality accounting for imported claims and ordinary source
  documents, including Free/Builder/Scale limits and the old and new bytes
  simultaneously required during commit.
- Writer/maintenance lock order, DB migration ownership, backup journal
  interaction, index rebuild behavior, and service recovery after every
  operation phase.

## Verification before any live import

Use a synthetic account and an empty, disposable brain first. Cover exact
export/import round trip, retry before/during/after commit, service restart,
concurrent writers, cancellation, corruption, decompression bomb, unsafe path,
symlink, duplicate file, forged brain ID, cross-account access, quota failure,
provider egress, existing-page conflict, and rollback. Verify records from a
second account remain unchanged. Then import only the reviewed personal
manifest to the intended account after its actual plan headroom and all Hosted
recovery gates are qualified. Historical email remains excluded.

## Release boundary

This proposal does not authorize a live upload, plan change, public activation,
merge, or deployment. The Hosted import path changes the authenticated data
path and requires chief-architect review, the integration owner's interface
approval, passing CI, and deployment through the existing release pipeline.
The current E23 qualification/recovery tasks remain prerequisites for a live
personal-corpus test.
