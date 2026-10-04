# Snapshot pin-owner static preflight

Verdict: **PRELIMINARY STATIC CLEAR for the narrow superblock-version correction at `b0a4c822f930b06b5444db51d7dc1c427f82655d` (tree `501b1a1f8b8d4bcace25eb59fb457ef534f6439a`).** This is not a source qualification or full contract acceptance. No Go commands were run in this review; author lint and compiled-control work remains outside this report.

## Finding resolved

**Historical exact-head verdict: HOLD at `69c4a22ee831a4e061b27f23320f0c6987c8857e`.** There, `pinOwnerDecodeSuper` strictly decoded the version-1 struct before classifying its version. A well-formed future superblock carrying a new field was therefore classified as corrupt instead of unsupported. This was a real static HOLD finding for that exact head.

**Correction appendix, exact integrated head `b0a4c822f930b06b5444db51d7dc1c427f82655d`:** the reviewed correction checks `pinOwnerWireVersionHint` before strict version-1 decoding in `internal/hosted/recovery/pin_owner.go:1197-1213`. The hint parser (`:1219-1284`) bounds input by the superblock/record limit, parses the complete top-level object, rejects duplicate top-level keys and malformed/trailing JSON, and accepts only a positive canonical unsigned integer version. A valid future version now returns `ErrPinOwnerUnavailable`; missing, null, zero, non-integer, duplicate, and overflowing versions remain `ErrPinOwnerCorrupt`. Version 1 still goes through strict known-field decoding, canonical re-encoding, checksum, and StoreID checks.

The new `TestPinOwnerFutureSuperblockUnavailablePreservesBytes` covers a future version with an extension field, `MaxUint64`, overflow, duplicate, null, missing version, and oversized decode. The filesystem cases reopen through the public constructor and verify error class, unchanged superblock bytes and inode, and unchanged owner-root entry names. This is useful regression coverage for the defect and the no-bootstrap-mutation boundary; it is source inspection only here, not a claim that the tests were executed.

## Remaining boundaries

This review clears only the identified decoder issue. The earlier broader owner-source qualification HOLD remains: producer crash-boundary coverage and the combined contract matrix still need their separately assigned review and qualified checks. The current author package/race/vet results are not independently reproduced here; lint and compiled behavioral controls were reported as pending. No READY, provider, startup-factory, full recovery-owner, physical-quota, or production acceptance follows from this review.

## Review provenance

- Exact reviewed commit: `b0a4c822f930b06b5444db51d7dc1c427f82655d`
- Exact tree: `501b1a1f8b8d4bcace25eb59fb457ef534f6439a`
- Detached review clone was clean at the reviewed head.
- `git diff --check` across the correction was clean.
- Review action: static source and test inspection only; no source edits and no builds/tests.

Coordinator evidence note: source changes invalidate the older606 focused check receipts as current qualification. No fresh60/b0 package, race, vet or lint result is claimed. A subsequent active-owner superblock revalidation concern is separately under independent review; this report clears only constructor decoder classification. External original report SHA256: `c53015340ea5213dcd82caff89e2fc2fa143739ae396dc42ba41af67337823a3`.
