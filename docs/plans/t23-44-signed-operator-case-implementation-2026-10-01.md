# Signed operator-case admission implementation receipt

Source commit: `e55563bf1f62443bf276cb933dfd763d5d935d2d` on branch `hosted/signed-case-worker-20261001`, based on `e862a11` plus frozen development contract commits `849ff76` and `81a70ca`.

This is a development-only verifier. It does not include or activate a production trusted-key registry, signer, approval issuer, Service factory, CLI route, provider, or real approval record. The existing Service interface and operation state machine are unchanged.

`internal/hosted/operatorreview.Admission` structurally implements `service.OperatorReviewAdmission`. Its constructor requires an injected `TrustedKeySource`, expected effective UID, case-age and case-lifetime limits, and clock. Typed-nil or missing sources are rejected. The source's current-key snapshot is the admission linearization point: revocation before the snapshot denies the call; a later revocation does not cancel that already admitted attempt; the next call queries the source again and denies. The only accepted resolution is `committed` with a `fact:` ref containing 64 lowercase hex digits.

The case filename is derived solely from a strict `sha256:` review ref and contains the hash of the entire canonical flat JSON envelope including its signature. Ed25519 verifies the domain-separated canonical payload without the signature field. The verifier checks the signed operation ID, outcome, canonical ref, operator-to-key mapping, validity times, age, expiry, and current revocation state. It returns the existing `ApprovedOperatorReview` value and exposes a fixed denial error on validation failures. It does not write or mutate cases or keys.

The new `internal/hosted/privatefs` helper checks every directory component for canonical nonsymlink paths, current/root ownership, safe writable-ancestor rules, and platform ownership enforcement. It reads only owner-private regular files through `O_NOFOLLOW`, applies a byte ceiling, and rejects changes during the read. Darwin refuses ownership-disabled mounts; Linux uses VFS owner/mode metadata; unsupported platforms fail closed. `admintransport.Listen` delegates containment checks to this helper while peer credential checks, socket creation, and inode-safe close behavior remain in `admintransport`.

The focused Darwin race suites passed separately on the ownership-enabled APFS fixture, with Go build temp/cache on the external SSD:

- `go test -race -exec 'env TMPDIR=<owned APFS fixture tmp>' ./internal/hosted/privatefs -count=1` — pass (final restored run 1.297s).
- `go test -race -exec 'env TMPDIR=<owned APFS fixture tmp>' ./internal/hosted/operatorreview -count=1` — pass (1.451s), including a real authenticated Unix HTTP request and a deterministic post-snapshot revocation barrier.
- `go test -race -exec 'env TMPDIR=<owned APFS fixture tmp>' ./internal/hosted/admintransport -count=1` — pass (1.292s), including existing peer, forged-header, unsafe-ancestor, and inode replacement coverage.

Behavioral mutation controls produced the intended runtime reds and each mutation was restored before the source commit. The evidence is stored outside the repository under `serenity-signed-case-validation-20261001/`:

- `signature-bypass-red.log`: ignoring the Ed25519 verification result made the bad-signature case admit.
- `revocation-red.log`: bypassing the revoked-key check made the second admission admit; the restored test also asserts the key source is called again.
- `strict-parser-boundary-red.log`: removing both recursive duplicate-key validation and canonical-envelope byte equality admitted duplicate and case-aliased fields. This demonstrates the combined strict-parser boundary, not the recursive duplicate scanner in isolation.
- `ancestor-check-red.log`: bypassing the writable-ancestor check accepted the unsafe private directory.
- `nonqualifying-tmpdir-fixture-note.txt`: records the initial test-harness temp-directory misrouting to the ownership-disabled shared volume. That failed setup is not counted as code evidence.

The exact restored file hashes after those controls were `a6badce2b5fccdf5437f0d849f3fee637ee50e1087baa821849646579ca4b0f2` for `admission.go` and `9351c8bfb75af498c360bee8ec74bd2caff91b9b2e7122b8c006fd80de7f87ca` for `privatefs.go`. The code commit was made only after all three focused race suites passed again.

Canonical source claims were won and then released after banking: R-hosted-privatefs (`bdd40c587c9a362bd921d0d0c66c77a347261c88`), R-hosted-operatorreview (`3025b1a8d8176e97a02ba09d44e6204a4138e34f`), and R-hosted-admintransport (`21fa40c425afe6e651c2838c0e4c7331f1a40a64`). Earlier shorthand claims were released by exact SHA after the full frozen names were won.

One initial edit pass was mistakenly made in the coordinator assembly checkout named in the assignment. Work stopped before staging or committing; only this worker's assigned paths were copied into this isolated full clone. The original coordinator checkout and its residue were left untouched for the coordinator to handle. No combined full suite, `go vet`, linter, or Linux ARM64 compile was run in this worker lane; coordinator owns those combined gates and independent review. No broader activation, production trust-source provenance, human approval, Linux runtime, or retention guarantee is claimed.
