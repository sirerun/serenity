# Signed operator-case admission design

Baseline: assembled candidate `8fdd32ffdcee5fa069625d33207f0b36a914b9a5`. This is a read-only design receipt. No source, frozen contract, approval case, signing key, provider, or CLI/factory wiring was changed or exercised.

## Existing boundary

`internal/hosted/service/operator_review.go` defines the optional `service.OperatorReviewAdmission.Authorize(ctx, request, operationID, reviewRef)` seam and the result fields the resolver consumes. The handler authenticates admission before ledger lookup or runtime fence, then requires the admitted operation/ref/outcome and exact canonical reference to match the fenced checker result. It only commits a matching `CanonicalLanded`; there is no release action. A missing admission fails closed. `admintransport.AuthenticatedUID` exposes only the peer UID inserted from the package-private accepted Unix connection; `Listener.HTTPServer` rejects requests without that verified context. `TestAdminOperatorReviewOverAuthenticatedUnixSocket` exercises that context through a real local socket, while the public handler has no resolve route.

Repository search finds no existing Ed25519 operator-key registry, revocation source, signed case store, or human approval issuer. The test admission callback is a fixture and cannot serve as production trust. The Unix transport authenticates the service effective UID, not an individual human; the signed case/key identity must supply that distinction.

## Recommended admission shape

Implement a read-only `FileOperatorReviewAdmission` in a service-owned file such as `internal/hosted/service/operator_review_ed25519.go`. It structurally implements the existing interface; it does not change the frozen operation state machine or the `OperatorReviewAdmission` method signature. Require explicit constructor inputs for the case directory, current key source, expected peer UID, and policy bounds. No default key, generated key, implicit path, or service-side signing function is allowed. If any dependency is absent, invalid, stale, revoked, unreadable, or unverifiable, return an error; the existing handler responds with a fixed denial before lookup or fencing.

Admission order:

1. Require a live context and a request whose context contains `admintransport.AuthenticatedUID`; compare it to the explicitly configured admin peer UID. Do this before opening either trust or case files. Never accept UID/operator identity from headers, body fields, or a caller-selected context value.
2. Parse `reviewRef` as exactly `sha256:` plus 64 lowercase hexadecimal characters. Derive one fixed basename beneath the configured case directory; never join caller-controlled path components. Read the current trust snapshot and the case through the same hardened read-only filesystem helper.
3. Verify that SHA-256 over the exact complete signed-case bytes, including the signature, equals the requested `reviewRef`. The case document has no self-referential `review_ref` field.
4. Strictly parse the case, verify its signature with the current non-revoked Ed25519 public key, derive `OperatorID` from the key registry, and require exact equality for requested operation ID, fixed `committed` outcome, and signed expected canonical reference. Return the existing `ApprovedOperatorReview` fields unchanged only after all checks pass.

Use a domain-separated deterministic payload for Ed25519, for example `serenity.operator-review.v1\0` followed by canonical serialization of version, key ID, operator ID, operation ID, outcome, expected canonical reference, approval time, and expiry. The case must bind the exact `operationID` and exact `CanonicalVerdict.Ref`; it contains no fact text, customer payload, client key, fingerprint, or caller-provided actor role. Use ASCII bounded opaque identifiers and canonical UTC timestamps to avoid Unicode and time-parse aliases. `keyID` should be derived from the raw 32-byte public key (for example `ed25519-sha256:<lowercase SHA-256>`), with exactly one current registry record mapping that key to one operator identity. The 64-byte Ed25519 signature must use one documented base64 encoding and have no alternate accepted form.

The current key source should be a linearizable `Current(ctx)` snapshot, consulted on every `Authorize` call rather than an unbounded startup cache. A local file implementation is reasonable only if it is explicitly injected from the trusted startup owner, atomically replaced, freshly read, strict-parsed, and not writable by the service UID. Place it and the case directory under a dedicated operator-control root, separate from `DataDir` and customer brain trees. Require a root/policy-owner-controlled, service-readable but service-nonwritable tree (for example root-owned directories with read/execute access for the service group and no group/other write bits), regular read-only case/key files, no symlink at any component, owner-aware storage, descriptor-based `O_NOFOLLOW`/`fstat`, and a hard byte ceiling with `stat` plus `LimitReader(max+1)`. Do not trust a private leaf directory if any ancestor is replaceable. The shared BuildOffload mount is not suitable; it has ownership disabled and writable ancestors.

To avoid two independent implementations of the ancestor, symlink, mount-ownership, and bounded-file checks, request a small `internal/hosted/privatefs` read-only helper shared by admintransport and this admission. It should expose a narrow policy for allowed UID(s), allowed read mode, no-follow regular-file read, maximum byte count, and platform ownership enforcement. Reuse the corrected ancestor policy (`root` or expected owner, non-writable unless sticky-protected, Darwin refuses `MNT_IGNORE_OWNERSHIP`, unsupported platforms fail closed). The helper must not create, chmod, or replace files. Because admintransport has a separate ownership claim, get coordinator approval for that cross-package extraction before assigning implementation; do not copy its security checks into an unreviewed second helper.

Suggested bounded records are a flat versioned case envelope (case maximum 8 KiB) and a trust snapshot containing at most 128 key entries (maximum 64 KiB). These are proposed parser ceilings, not measured deployment capacity. Use a schema-aware token parser that rejects duplicate names at every object depth, exact-case aliases, unknown fields, trailing documents, excessive nesting, invalid UTF-8, oversized growth, and noncanonical timestamp/base64/hex encodings. `json.Decoder.DisallowUnknownFields` alone is inadequate because Go JSON field matching accepts case-insensitive aliases and duplicate keys overwrite earlier values. Reject, do not truncate, any oversized file. The case reference is the content address `sha256:<lowercase hex of the full canonical signed envelope>`; signature verification is over the separately defined canonical payload.

The file-backed key registry contains explicit active/revoked state, key ID, operator ID, raw public key, and validity timestamps. On every admission, a missing, malformed, duplicate, revoked, not-yet-valid, or expired key denies authorization. Atomic replacement provides one complete current snapshot per request. This read-only verifier must never mint, import, rotate, revoke, or persist keys or cases.

## Review and freeze decisions

Freeze the following before implementation:

- Who is the root of trust for the key registry and case repository, where they live, who may install/revoke keys, and how registry updates are approved. Root ownership and filesystem permissions are the proposed local trust root; the service UID must not be able to add its own signing key.
- Whether a revocation blocks every uncommitted case immediately or only new admissions, and whether an already-authorized request may finish if revocation occurs while it waits for the canonical fence. The simplest linearization point is successful `Authorize`; stronger last-moment revocation needs a second current-snapshot check immediately before the ledger transition (with an explicit race/locking contract).
- Approval validity: maximum case age, signed expiry, permitted future clock skew, clock source, and whether a case may be re-used after a prior `pending_review` response. The current result type exposes `ApprovedAt` but not expiry; the verifier can enforce expiry internally without changing the frozen state machine.
- Human identity mapping and approval quorum: one key per operator versus shared/team keys, and whether one or two distinct operators must approve. `OperatorID` must come from the trusted key record, not from an unsigned HTTP field.
- Case retention/immutability: content addressing detects edits but does not prevent deletion or rollback. The external issuer/store must guarantee put-if-absent and retain the exact signed bytes for at least as long as ledger evidence can reference them. The service admission implementation must remain read-only.
- Exact canonical-ref format and issuer workflow. The current service compares `ExpectedCanonicalRef` byte-for-byte with the runtime checker result; no fallback to a Git HEAD, user fact text, or `EvidenceAbsent` is permitted.
- Parser caps and file permission policy, including whether the service is allowed group-read access to root-owned files and which non-Darwin filesystems are trusted.

## Required implementation controls

Use fixed non-production Ed25519 test vectors only; do not create real private keys or approval records. Tests should prove:

- a valid signature with an active key returns all expected fields; wrong signer/key-to-operator mapping, bad signature, revoked key, expired/not-yet-valid case, wrong peer, or missing peer fails before ledger lookup/fence;
- content hash mismatch, path traversal/uppercase digest, file replacement, symlinked ancestors/files, wrong owner, writable ancestor, ownership-disabled mount, non-regular file, hard size overflow, and cancellation all fail closed;
- duplicate/case-aliased/unknown keys at every object depth, trailing JSON, wrong types, malformed UTF-8/base64/hex/time, and excessive nesting are rejected; an old standard-library JSON decoder mutation is genuinely red;
- the key snapshot is re-read after an atomic revocation update, and the chosen in-flight-revocation policy is exercised with deterministic barriers;
- an actual owner-only Unix socket produces the accepted `AuthenticatedUID`, while `httptest`, forged headers, public TCP, and arbitrary direct contexts are rejected before filesystem reads;
- a valid approved case binds only one exact operation ID and expected `CanonicalRef`; changing either, outcome, or reviewRef fails, and repeated exact resolution does not change original quota deltas.

No Go/build tests, signer tooling, key generation, real approval case, provider call, CLI/factory activation, or service mount was performed for this design receipt.
