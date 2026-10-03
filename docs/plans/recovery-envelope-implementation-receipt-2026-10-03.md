# Recovery envelope codec implementation receipt — 2026-10-03

Implemented the frozen pure v1 recovery-envelope codec at base `c27acc70fc69e4da5c58f2e17febd0f399a74621`. The implementation is limited to the new `internal/hosted/recovery/envelope*.go` files and this receipt.

The codec provides the closed `ELIGIBLE` / `FROZEN_ONLY` tagged union, ordered canonical JSON, domain-separated integrity hashes, bounded context-aware validation, deep-copy support, strict duplicate/unknown/trailing/noncanonical JSON rejection, summary inventory digest recomputation, and full-row brain/inventory digest helpers. Literal golden vectors cover both envelope arms and nested brain/inventory hash domains. Frozen dispositions exist only on `FROZEN_ONLY`; eligible account IDs are validated against the allowlist and inventory. Journal cut, old writer generation, and last-seal positions remain distinct under the frozen constraints.

All values and hashes are inert integrity data. The codec does not mint inspection, approval, provider, journal, pin, or writer authority and cannot make a plan ready. Decoding summary hashes cannot reconstruct full brain rows or produce a verified inspection; owner-controlled evidence validation remains outside this component.

Focused package checks on source fingerprint `8e6b2aa5a646267cde6ecc337e172cba604c7e5b2510a8ed53dc3e63ac59d1d6` passed:

- `go test -race ./internal/hosted/recovery -count=1` with the assigned fixture environment
- `go vet ./internal/hosted/recovery`
- `golangci-lint run ./internal/hosted/recovery`

The three behavioral mutant controls compiled and failed their targeted assertions as expected: disabling summary inventory-hash comparison, allowing both union arms, and disabling duplicate-key plus canonical-byte rejection. Their evidence records are under `/Volumes/BuildOffload/serenity-recovery-envelope-implementation-evidence-20261003/mutant-{hash,union,duplicate}-01`. Every mutation was restored before final checks. Each build stage used the durable runner and released its exact shared lease.

The root full-module verification and independent exact-head review remain root-owned and are not claimed by this receipt.
