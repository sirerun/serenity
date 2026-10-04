# Recovery approval authority decision packet — v2

**Status: Proposed for founder and owner review; not approved, not source-frozen.** This revision corrects the binding matrix and records the required contract pins. It does not select an issuer, provider, key, evidence backend, or production authority. It grants no recovery effects, startup, deployment, restore, or readiness authority. The v1 packet remains preserved as historical; use this v2 for review.

## Decision requested

Assign accountable owners for the four decision areas below. Each owner must accept the candidate semantics or return an exact correction, followed by independent review. Until those owners' contracts are reviewed and accepted by recovery, approval verification remains unavailable and recovery remains fail-closed.

This asks for organizational authority and durable evidence decisions. Permission to run local commands, edit source, or use a repository cannot create a signer, trust root, immutable evidence record, provider identity, or recovery approval.

## Authorization scopes and exact bindings

There are three primary approvals plus one separate continuation authorization. They are distinct signed bindings and replay domains; one cannot stand in for another.

| Kind | Exact binding | Authority it may grant | Boundary |
| --- | --- | --- | --- |
| **Plan creation** | Raw source-manifest SHA-256; sorted, unique account allowlist (empty allowed); exact `operation_id`; nonce; expiry. | Building one plan for those exact source bytes and allowed scope, after verifying before provider reads. | Does not bind a future envelope hash, authorize effects, fence a writer, or activate accounts. |
| **Global epoch approval** | Final `RecoveryEnvelopeHash`; direct `SnapshotSHA256`; complete sorted snapshot account inventory and its digest; exact eligible/activation scope; old-writer digest, generation, and journal-store identity; `operation_id`; fixed action set; nonce; expiry. | The plan-wide recovery transaction before its first effect. The exact action set is `quiesce`, `stop`, `revoke`, `seal`, `restore-frozen`, `publish-frozen`, `adopt`. | Cannot unfreeze any account, change plan or inventory, or authorize rollback after effects may have started. |
| **One-account activation** | Exact `RecoveryEnvelopeHash`; already committed `EpochRecordDigest`; direct `SnapshotSHA256`; exactly one `AccountID`; `operation_id`; nonce; expiry. | One current-state-guarded activation for the named account. | Cannot activate another account, repeat restore/seal/adoption, or change the committed epoch. |
| **Continuation authorization** | Same `RecoveryEnvelopeHash`; exact `CurrentEpochRecordDigest`; exact one `ExactNextPhase`; same `operation_id`; nonce; expiry. | The next named forward-only phase of the same recovery epoch after effects may have started. | Cannot alter the epoch's source/scope, skip phases, authorize another effect, or roll back. It is separate from the three primary approval scopes and is never implied by a global approval retry. |

The account binding includes `SnapshotSHA256` directly as well as the envelope and committed epoch digests. The epoch approval also includes `SnapshotSHA256` directly, so the verifier compares the raw source identity without relying only on transitive containment in the outer hash.

The global action set above is fixed exactly as written: quiesce, stop, revoke, seal, restore-frozen, publish-frozen, adopt. These labels denote the bounded plan-wide transaction in the recovery contract. Any change to that set requires a contract change and a newly bound approval; no implementation may infer extra actions.

## Candidate wire, time, nonce, and verification rules

The held approval-verifier proposal's plan-case v1 wire is a candidate, not frozen. It uses canonical UTF-8 JSON with fields `version`, `authority`, `key_version`, `issued_at`, `expires_at`, `snapshot_sha256`, `account_ids`, `operation_id`, `approval_nonce`, and `signature`; the candidate signature algorithm is Ed25519 and the candidate domain separator is `serenity.recovery.plan-approval.v1\x00`. Maximum evidence size is 16 KiB. Exact field order, unique/sorted account IDs, no duplicate/unknown/trailing values, and canonical bytes are required if owners accept this proposal.

Each signed evidence wire carries `issued_at` and `expires_at`; all such time text uses canonical UTC RFC3339Nano and parse/format round-trips byte-identically. Verification requires `issued_at <= now < expires_at`, `expires_at > issued_at`, and configured maximum approval age and maximum lifetime. The binding types below carry their specified expiry field; `issued_at` is signed evidence metadata and is checked by the verifier, not an extra caller-selected binding input. For the candidate plan wire, `approval_nonce` is exactly 64 lowercase hexadecimal characters representing 32 bytes from a cryptographic random source; the issuer owns uniqueness and retention. Proposed common wire rule for the global epoch, one-account, and continuation bindings is the same 64-lowercase-hex encoding. The approval-policy owner must explicitly accept that common encoding or specify an exact alternative for each binding before source freeze; the encoding and replay domain may not be inferred from an untyped string.

The verifier obtains the exact evidence version, resolves the current `(authority, key_version)` trust record, checks signature and time policy, and compares every expected binding field before any provider query or effect. It returns a private-field opaque verified value. A locator, digest, decoded struct, boolean, database row, local test key, or signature-shaped document is not approval authority.

The continuation path has an additional owner requirement: the recovery epoch store must expose its durable current record digest and exact next allowed phase; the approval issuer binds those exact values; the verifier checks the tuple; and the epoch owner atomically consumes/replays that exact continuation against the same record/phase. The owner must make an identical retry idempotent while rejecting nonce reuse with another digest, operation, phase, or envelope. A verifier-only replay cache or caller-selected phase cannot enforce this state transition.

## Evidence and trust responsibilities

**Approval issuer/policy owner** owns operation assignment, the four authorization kinds above, nonce generation and uniqueness/replay policy, signer access and key use, issue/expiry bounds, policy versioning, and issuance/revocation records. It must not conflate plan, global epoch, one-account, and continuation authority.

**Immutable evidence-store owner** owns a bounded reference to one exact immutable version and its authenticated backend namespace/object identity. A reference binds the exact returned bytes by digest. Publication is atomic; exact reads fail closed; retention and read-after-write guarantees, access policy, freshness, and replacement/error behavior are explicit. A mutable “latest” alias, caller bytes, or a bare digest is not an evidence reference.

**Trust/key-registry owner** owns authenticated trust-root provisioning and current per-verification lookup. A result binds authority and immutable key version to the exact public key, validity interval, and revocation state. Unknown, ambiguous, stale, unavailable, revoked, invalid, or rolled-back records deny. Registry update provenance, freshness, rotation, revocation, anti-rollback, and validity at issue and verification time are explicit. This packet chooses no key material or registry product.

**Recovery consumer/epoch-store owner** accepts the private verified-token boundary and checks exact expected fields before provider calls. It owns durable phase state and atomic continuation replay/current-digest enforcement. Its full plan/epoch store remains a separate component; the snapshot pin owner and envelope codec do not acquire approval, READY, or effect authority.

## Four founder decisions

For each decision, name an accountable owner and answer **accept**, **request exact change**, or **no owner / hold**. “No owner / hold” is a valid fail-closed result.

1. **Issuer and policy accountability.** Who owns approval policy and issuance for plan creation, global epoch, one-account activation, and continuation? May the same accountable organization oversee multiple scopes while preserving separate typed bindings and replay domains, or must these scopes be split across owners? Name an independent security reviewer. Confirm or change the candidate v1 Ed25519/domain/wire and explicitly decide whether every approval kind uses the proposed 64-lowercase-hex nonce encoding, shared canonical UTC RFC3339Nano/time limits, and its own nonce uniqueness/replay scope.
2. **Evidence store.** Which existing owner-controlled system can provide exact immutable versions, authenticated namespace/object identity, digest binding, bounded reads, atomic publication, retention, and read-after-write behavior? Name the accountable owner. If none meets the requirements, choose hold; do not substitute a local path or mutable alias.
3. **Trust registry.** Which existing authority owns trust-root provisioning and current `(authority, key_version)` lookup, including authenticated updates, freshness, revocation, rotation, validity, and anti-rollback? Name the owner and independent reviewer. This decides accountability and provenance, not key values.
4. **Recovery consumer and continuation owner.** Who owns acceptance of the expected binding matrix and the durable recovery store that exposes/consumes the current epoch digest and one exact next phase? Name the owner authorized to accept the recovery API boundary. Require an atomic owner-side replay/idempotency rule for continuation; the pin owner and inert envelope store cannot supply it.

Record answers as: `decision ID`, `owner`, `accept/request exact change/no owner`, `changed clause` if any, and `independent reviewer`. No credentials, keys, private infrastructure identifiers, or live evidence belong in the decision record.

## Gates explicitly outside this packet

This approval packet does not resolve authenticated startup identity, durable genesis, old-writer stop/revocation, namespace-wide mutation barriers, successor writer issuance, destination staging limits, expanded repository/database/WAL storage, or physical quota. Those remain separate startup/provider/restore gates. A signed approval or pin-owner success cannot satisfy them.

## Source pins and basis

This packet was corrected against the actual documents in the clean source tree at `c916bcd7ed30592efb065c876455d1bb51509523` (tree `0b026e37226fd8295f2ca1a350a4f5072816a336`), not against the prior packet alone.

- `docs/plans/recovery-plan-approval-verifier-proposed-contract-2026-10-03.md`, SHA-256 `9d0dd8dd7a464ac5e30941e2bb1bb198d4a211165a7526b731fdba0d54bd7b3d`, is explicitly **HOLD** at lines 1–5 and 51–68. Its candidate wire/time/nonce/domain rules are at lines 21–43; its missing issuer, immutable evidence, trust-registry, and recovery owners are at lines 53–60. It says the current signature proposal is not frozen.
- `docs/plans/recovery-planner-and-admission-proposed-contract-2026-10-03.md`, SHA-256 `15b3bbed72eee6a4a4052362838a985999ce9703f472ea4def2ca18779f26cf0`, declares code source baseline `8081632` after PR #349, and states the plan approval binding and distinct identity routing at lines 7–48. The binding type definitions at lines 238–265 include direct `SnapshotSHA256` on `AccountActivationBinding`, exact global actions, and the separate `ContinuationApprovalBinding` with current record digest and exact next phase. The proposed full owner store/factory is at lines 294–332 and 450–476; it remains proposed, not source authorization.
- `docs/plans/authenticated-startup-factory-proposed-contract-2026-10-03.md`, SHA-256 `039da93ef5e62e9e743e3cf89b1aaa8e7cbd72a5f1193211bc0dfeb7e7b054fb`, remains proposal-only at lines 1–3 and describes the separate runtime and durable authority gaps at lines 7–26.
- `docs/plans/recovery-pin-owner-source-freeze-2026-10-03.md`, SHA-256 `67423f326c3107debf2fefdc8841a311a4839dc095009975dbe93ddd4596ae1f`, limits the accepted narrow owner to new `pin_owner*.go` at lines 5–9 and says it cannot persist READY, approve effects, or replace the full store.
- `internal/hosted/backup/SNAPSHOT_INSPECTION_RECEIPT.md`, SHA-256 `99d1e48a635b67dcf2335d3c374767d4f4089167ef2a4d4eec1f8f61b528cfac`, states the raw declared-byte limit is not a filesystem quota or guarantee of temporary Git allocation at lines 12–20.

The prior v1 packet is retained unchanged. This v2 clarifies the candidate policy only; owner acceptance, independent contract review, source freeze, provider evidence, and production qualification remain outstanding.
