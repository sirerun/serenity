# Recovery envelope codec implementation receipt — 2026-10-03

Implemented the frozen pure v1 recovery-envelope codec at base `c27acc70fc69e4da5c58f2e17febd0f399a74621`; this bounded-preflight follow-up is based on implementation commit `58cd3102ba9ad1349d3921953d8aa3d35f4cf420`. Changes remain limited to new `internal/hosted/recovery/envelope*.go` files and this receipt. The final Go-source fingerprint is `825acea13712878ade494da27106157f35aab501d4634a63908eb3e534473cf6`.

The codec provides the frozen `ELIGIBLE` / `FROZEN_ONLY` tagged union, ordered canonical JSON, domain-separated integrity hashes, context-aware validation, deep-copy support, summary inventory digest recomputation, and full-row brain/inventory digest helpers. The preflight scanner now runs before typed decoding and enforces the recognized schema, required common/nested fields, arm presence/absence by kind, duplicate and unknown key rejection, per-field byte ceilings, account/artifact/journal integer ceilings and overflow, array counts (100 eligible/allowlist/plan IDs; 10,000 full accounts/dispositions), input size and nesting caps, and context cancellation at token boundaries. It rejects wrong scalar/container kinds and nulls before typed decoding. The inactive union arm remains absent as specified by the frozen contract; no wire-format or golden-vector change was made.

Literal golden vectors cover both union arms and nested brain/inventory hash domains. Adversarial preflight tests cover 101-item eligible lists, 10,001 dispositions, 2,000 unique unknown object members, oversized keys/strings/references/account IDs, integer overflow, required-field omissions, wrong types, null arrays/arms, missing and extra union arms, and cancellation during scanning. The tests call the preflight stage directly to verify those limits before typed envelope construction.

Final-source focused checks passed under the durable runner with a fresh shared lease each time and exact release:

- `fix-stage-013-focused-test`: package tests passed.
- `fix-stage-014-focused-race`: package race tests passed.
- `fix-stage-015-focused-vet`: vet passed.
- `fix-stage-016-focused-lint`: golangci-lint passed.

Compiled behavioral mutants produced RED for array-count, string-size, required-key, wrong-type/null, context-cancellation, inventory-hash, union-arm, and duplicate-key/canonical-byte guards. The corresponding records are under `/Volumes/BuildOffload/serenity-recovery-envelope-implementation-evidence-20261003/preflight-final-mutant-{array,string,required,types,context,hash,union,duplicate}-01`. All mutations were restored before final source checks; exact source fingerprints matched the clean checks. Earlier runner attempts that reported load/lease holds did not start commands. One early bare focused `go test` was run without the durable runner; it is unqualified and is not counted as verification.

All public values and hashes remain inert integrity data. The codec does not mint inspection, approval, provider, journal, pin, or writer authority and cannot make a plan ready. Decoding summary hashes cannot reconstruct full brain rows or produce a verified inspection; owner-controlled evidence validation remains outside this component. Root-owned full-module verification and independent exact-head review remain outstanding.

## Public Decode context-sentinel follow-up

On base `fb5e71c2298467cdb71b2d0f24a333ead1ca9bc5`, `DecodeRecoveryEnvelopeV1` now returns preflight size and context errors unchanged. A public-API cancellation test verifies that cancellation during scanning preserves both `ErrRecoveryEnvelopeContext` and `context.Canceled` and returns a zero envelope and empty hash. This does not change canonical bytes or hash domains. Final Go-source fingerprint for this delta is `c43bcb84696f4f04da545c7f17b390c498d2ad26760cf9a1531bdab0f4a5082f`.

Runner-backed checks passed on that fingerprint: `context-stage-002-focused-test`, `context-stage-003-focused-race`, `context-stage-004-focused-vet`, `context-stage-005-focused-lint`, and restored public regression pass `context-stage-006-restored-test`. The compiled context-sentinel-loss mutant failed the public test as expected at `context-mutant-public-api-01`; its lease released successfully and the source was restored before the final test.
