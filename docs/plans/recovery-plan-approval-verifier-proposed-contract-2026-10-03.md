# Recovery plan-approval verifier: proposed contract and source-freeze hold

**Status:** source-grounded proposal only. The verification interface and approval semantics are specified by the recovery planner/admission proposal, but production source freeze is **HOLD** until immutable evidence-reference and current trust-root/key-registry owners publish and independently clear their contracts. This document creates no verifier, key, signer, evidence reader, or authority.

**Source pin:** `main` `c2d5a5439675fb7a964aadedb1a0a461f0fce386`, tree `0ec437d42305603f90ca671998ae174fc92cc10a`.

## What the accepted planner contract requires

[`recovery-planner-and-admission-proposed-contract-2026-10-03.md`](recovery-planner-and-admission-proposed-contract-2026-10-03.md) defines `RecoveryApprovalVerifier.VerifyPlan(ctx, ref, expectedSnapshotSHA256) (VerifiedPlanApproval, error)`. Its plan approval binding is exactly:

- expected raw source-manifest SHA-256;
- sorted, unique, opaque account allowlist, which may be empty;
- one recovery `operation_id`;
- a cryptographically unique `approval_nonce`;
- expiration time.

The proposal requires `PlanRecovery` to compare the verified digest and scope before provider calls, query only the sorted intersection of the allowlist and complete snapshot inventory, and retain noneligible inventory accounts as frozen. The signed approval is for plan creation only. It is distinct from the later global epoch approval, which signs the already finalized `RecoveryEnvelopeHash` and its exact restore/fencing scope, and from each one-account activation approval. It is not an approval of an envelope hash that does not exist yet.

The inert envelope wire records `PlanApprovalRef`, digest, nonce, expiry, allowlist and operation ID as integrity-bound data. The envelope codec/hash and plan store do not verify a signature or confer approval authority. `RecoveryEnvelopeHash`, `LegacyPlanArtifactHash`, and `RecoveryContractPlanHash` remain distinct; none substitutes for the signed approval or verified token.

## Candidate signed plan-case v1 wire

The following is a proposed exact format to make the abstract contract implementable. It is **not yet frozen** because no owner has accepted the authority/key identity, immutable ref schema, versioning policy, or signing domain. No code may infer approval from a document that merely resembles this format.

The immutable evidence object is UTF-8 JSON, at most 16 KiB, with exactly these members in the listed order, no whitespace or alternate escaping, no duplicate/unknown/trailing data, and canonical bytes equal to re-marshaling the parsed envelope:

```json
{"version":1,"authority":"<authority-id>","key_version":"<immutable-key-version>","issued_at":"<UTC-RFC3339Nano>","expires_at":"<UTC-RFC3339Nano>","snapshot_sha256":"<64-lowercase-hex>","account_ids":["<sorted-unique-account-id>"],"operation_id":"<recovery-operation-id>","approval_nonce":"<unique-random-nonce>","signature":"<canonical-base64-ed25519-signature>"}
```

`account_ids` may be empty. IDs are 1–64 ASCII bytes matching `[A-Za-z0-9][A-Za-z0-9._:-]{0,63}`, sorted by raw byte order and unique. `operation_id` is 1–128 ASCII bytes using the same safe character set and must match the operation assigned to this recovery attempt. `approval_nonce` is 32 bytes from a cryptographic random source, represented as exactly 64 lowercase hex characters; nonce issuance and uniqueness tracking must be enforced by the approval issuer, not by this verifier. `authority` and `key_version` are nonempty ASCII identifiers of at most 128 bytes, matching `[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}`. `snapshot_sha256` is exactly 64 lowercase hex. Times must be UTC RFC3339Nano strings whose parse-and-format round trip is byte-identical; `issued_at <= now < expires_at`, expiry must be after issue, and configured max age/lifetime bounds apply. Signature uses canonical padded standard Base64 and decodes to exactly 64 Ed25519 bytes. The verifier caps the account array at 100 before allocation; all string lengths, total input bytes, nesting depth, duplicate keys, UTF-8, JSON types, integer syntax/range and required members are checked before typed decode.

The signed payload is the same JSON object without `signature`, in the same field order and with the same tags. The candidate domain separator is the exact byte sequence `serenity.recovery.plan-approval.v1\x00`; the signature input is `domain || canonicalPayload`. The domain and field schema require explicit approval by the recovery contract owner and security/key authority owner before freeze. Do not reuse the `serenity.operator-review.v1\x00` signature domain.

The verified binding returned to recovery must deep-copy `account_ids`, preserve the exact canonical evidence digest and return a private-field `VerifiedPlanApproval` minted only inside the production verifier after all checks. It records `authority`, immutable `key_version`, verifier identity/version, evidence digest, canonical binding and an internal verification timestamp. Caller-constructed/decoded values, hashes, booleans, database rows, or a reference alone cannot create this token. The planner must compare the returned snapshot digest to its expected raw manifest digest and validate the operation and allowlist before any provider call.

## Evidence reference and current-trust requirements

The planner proposal names `EvidenceRef` and `ImmutableEvidenceReader.ReadExactVersion(ctx, ref, maxBytes)` abstractly. At this source pin, there is no concrete `contracts.EvidenceRef` type and no production immutable approval evidence reader or current key/trust registry for recovery plan cases. The candidate owner-local reference should be a locator for one immutable, exact object version, with bounded scalar identifier lengths and an independently authenticated backend namespace/object identity. The reader must return those exact version bytes or fail closed; it may not follow a mutable “latest” pointer, accept caller-provided bytes, or treat a digest-only reference as content. The resolved bytes must hash to the digest committed by the owner-controlled reference record. The exact ref fields, storage owner, object-version semantics, digest binding, freshness/retention guarantees, backend identity and authority to publish/revoke objects remain an unimplemented owner contract; no new shared `contracts` type is proposed here.

The trust policy must resolve `(authority, key_version)` from a current owner-controlled trust registry on every verification. It must return a uniquely bound Ed25519 public key plus immutable authority/key-version identity, validity interval, and revocation state. Unknown, ambiguous, stale/cached, revoked, malformed, or unavailable entries deny. The key must be valid at `issued_at` and at verification time under an explicit policy. Trust roots and registry updates require authenticated provenance, revocation semantics, anti-rollback/version rules, and independent owner review. A configured raw public key, self-signed case, caller-selected key, local test key, or `TrustedKeySource` implementation without a production trust contract does not establish an approved root.

The recovery production factory must construct/bind both evidence reader and trust policy from reviewed owner-controlled implementations. Missing or typed-nil production sources must fail construction/verification. Tests may use fakes only inside `_test.go`; they establish parser/signature/state behavior, not production evidence, key enrollment, human review, or approval issuance.

## Why `ApprovedOperatorReview` is not this verifier

`internal/hosted/service/operator_review.go` returns `ApprovedOperatorReview` for a private service operation-resolution flow. The concrete `internal/hosted/operatorreview.Admission` authenticates a local admin transport peer, reads a locally addressed case file, verifies the `serenity.operator-review.v1\x00` Ed25519 signature using an injected `TrustedKeySource`, and returns operation ID, review-ref digest, operator ID, committed outcome, expected canonical fact reference and approval time.

That shape does not bind a recovery snapshot SHA-256, sorted account allowlist, recovery plan nonce, immutable plan-evidence object version, recovery-specific authority/key version, or the recovery approval lifecycle. It only accepts the operator-resolution committed outcome and applies its own request/ref/key/time policy. Its existing `TrustedKeySource` is an adapter seam, not proof that a current production recovery trust registry exists. Do not convert its `ReviewRef`, `ExpectedCanonicalRef`, result boolean, or case signature into a `VerifiedPlanApproval`; do not change or broaden that service API in this lane. Plan creation, global epoch approval and account activation each need their own exact signed binding and verifier path.

## Fail-closed source-freeze blockers

Source freeze remains held until all of the following have an exact reviewed owner contract and owner:

1. **Approval issuer/policy owner:** approves the recovery-only canonical wire, exact domain bytes, operation assignment, nonce uniqueness/retention, allowable account-ID grammar, expiry bounds, and signing/revocation lifecycle.
2. **Immutable evidence-store owner:** defines `EvidenceRef`, immutable version lookup, reference-to-bytes digest binding, namespace/store identity, atomic publication, retention and read-after-write guarantees, access control and failure semantics.
3. **Trust/key-registry owner:** defines production trust-root provisioning, authority-to-key-version mapping, freshness, revocation/rotation, validity periods, anti-rollback, and authenticated registry provenance. No roots or private signing keys are supplied by this proposal.
4. **Recovery owner:** accepts the owner-local evidence-reference and private verified-token boundaries and the expected operation/snapshot comparison sequence, without widening the envelope codec/store into an authority source.

Until those are resolved, `VerifyPlan` is a design signature only; it must not be implemented with a self-signed fixture, local file path, process config key, caller boolean, embedded public key, or arbitrary interface adapter promoted as trusted. The source freeze must explicitly name each owner and exact immutable dependency contract. This proposal does not grant the root coordinator, epoch verifier, store, CLI, provider adapter or `ApprovedOperatorReview` authority to fill the gaps by assumption.

## Required verifier tests after a separate source freeze

A later implementation needs independent literal canonical-wire and signature vectors, not values produced by the implementation under test. Cover exact field order/domain and signature bytes; empty and nonempty sorted scopes; raw manifest digest and operation mismatch; duplicate/unknown/missing/trailing/noncanonical JSON; invalid UTF-8/types/oversized members/deep nesting/over-limit account arrays; UTC timestamp roundtrip, expiry, future issue, max-age/lifetime; malformed signature/key sizes/base64; unknown, ambiguous, stale, revoked, rotated and not-yet-valid key versions; missing reader/registry; mutable-version substitution, digest mismatch, wrong store identity and unavailable reader; context cancellation at each read/lookup/verify boundary; deep-copy mutation of account slices; and zero provider calls when verification or binding comparison fails. Include behavioral mutant controls for signature-domain omission, key-version substitution, scope omission and digest comparison removal after compiling. Fakes prove local behavior only. No test or source result constitutes provider/live acceptance.

## Review decision

This document recommends retaining the abstract planner interface and current inert envelope exactly as designed while holding verifier source. Independent reviewers should either accept the candidate wire and assign the missing trust/evidence owners, or return exact corrections and owners. Do not mark the recovery coordinator production-ready until these owner-controlled sources exist, are independently reviewed, and are wired through the only reviewed production factory.
