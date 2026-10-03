# Verified snapshot lease producer freeze v1

Coordinator freezes `verified-snapshot-lease-v1` from exact producer proposal `6a013b49b94e7f9286b782ad2827441d2e5bd737` after independent CLEAR (banked producer report). Consumer `331df6469fbf4f884b06ed4e70bc5d2819bcaa4f` review confirms the matching pin protocol and closes those handshake findings; its separate cyclic/ambiguous plan-hash finding remains HOLD for recovery coordinator implementation. This scoped dependency separation enables the complete backup component without declaring the whole recovery contract or hosted startup ready. The original E24/hosted objective is unchanged.

## Source authorization

Canonical coordinator claim is `R-backup-verified-snapshot-lease` at `183472c8e52a438ebd83be5ac39038341d79c1e9`. One isolated worker owns only `internal/hosted/backup/snapshot_inspect.go` shared verifier extraction, `internal/hosted/backup/backup.go` private restore-core extraction/error propagation, new `internal/hosted/backup/snapshot_lease*.go` implementation/platform helpers/tests, and one new implementation receipt. Platform helpers may support Darwin/Linux with explicit refusal elsewhere; no unsupported success or module change. Preserve public Inspector/Restore signatures and successful data behavior. All exported lease API shapes, exact cancellation ticket/version, durable owner handshake, bounded candidate projection, pin lifecycle and scratch semantics follow frozen v1; changes need coordinator amendment and affected review.

No contracts, recovery/billing/service/CLI source, schema, registry or installed-script edit is authorized in this producer lane. Production lifecycle authority is supplied by the later reviewed recovery owner; test fakes stay in test files. No transient-only substitute for durable pins, restart repair or authoritative release.

## Preflight evidence and verification

Main source baseline is `db7625cfb6e29e3c7c42531c004d22d72af31c22`; source checkout is isolated on mounted writable external storage with more than 20 GiB free. Owned APFS fixture is mounted with global permissions enabled and current-UID 0700 scratch. It is 8 GiB, not 500 GB acceptance. Build caches/artifacts remain on external storage. Before any build, check one-minute load at most 10 and acquire/verify/exactly release the shared build lease. Existing other-project lease must be respected. Worker runs focused behavior/mutation checks and formatting/lint; coordinator alone runs required full-module race/vet/lint/Linux ARM64 compilation and exact independent source review before merge. All skipped/held checks are disclosed.

The complete producer implementation, its verification, independent review, normal expected-head merge and landed proof remain VSL-IMPL through VSL-LANDED. Recovery coordinator, genesis/restart, provider authority wiring, hard physical quota, capacity, deployment, purge, spend and hosted acceptance remain open and separately gated.

## Subsequent consumer review

Recovery proposal `a50c876f06682d42530d32e46a2ffc60aa0485f4` independently clears the prior `331df646` hash-domain HOLD. The earlier paragraph records the original producer-freeze decision and remains historical. This new proposal separates the legacy inner plan hash from the outer eligible/frozen envelope and epoch/account record hashes; the banked final report clears only the bounded coordinator contract. Recovery source implementation and all live gates remain open.
