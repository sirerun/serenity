# Governed memory integration candidate — 2026-10-12

## Source and evidence

Production candidate and acceptance-fixture source: `7773ed132d155638316925f72b79f7db39f0abed`.
The final commit adds only the opt-in hosted process fixture; production code is unchanged from `b1ef2fa195e2e25e268972e72c953aefc723e918`.
This receipt is a documentation-only follow-up, not a new production protocol revision.

On Linux ARM64, with Go from the task toolchain, `GOMAXPROCS=2`, an isolated Go cache, and a private fixture directory:

- `go test -race -p 1 ./...` passed at the candidate revision. Both `TMPDIR` and the repository-supported `SERENITY_RECOVERY_TEST_TMPDIR` pointed at the private fixture directory. An earlier run without the latter failed existing backup/runtime-broker private-directory checks; no source checks were weakened.
- `golangci-lint run` over `internal/store/...`, `internal/writer/...`, `internal/index/...`, `internal/server/memory/...`, `internal/hosted/gateway/...`, `internal/hosted/pool/...`, and `internal/hosted/service/...` reported zero issues at the candidate revision.
- `go build -p 1 -o <task-artifact> ./cmd/serenity` passed at `b1ef2fa`; this is identical production source, before the test-only final commit. This is not a signed release archive.

These are local source checks, not hosted CI or deployment evidence. Cross-repository Zati acceptance is recorded by its integration owner; it must remain tied to both exact source revisions.

## Delivered semantics

See [the protocol contract](GOVERNED_MEMORY_v1.md). Sessionless authenticated commands support remember, lexical recall, inspect and retract, with durable command identity and read-only reconciliation. Canonical claims carry producer-created UUIDs, versions, actual observation timestamps and source references. Unknown confidence is omitted, never replaced by an invented score. Legacy raw memories are not automatically upgraded into governed claims.

Active governed statements project into ordinary Serenity memory reads; command envelopes and queries are excluded from generic extraction and provider input. Retraction removes active recall visibility while retaining command history. This is not historical erasure. One active-claim retraction remains possible at full quota; repeated unique no-op tombstones are refused. Promotion and governed export are explicitly unsupported in this revision.

The fixture exercises real authenticated HTTP service assembly with synthetic lifecycle dependencies. Production CLI startup still requires the managed root broker, signed authority/journal lifecycle and instance identity. The fixture does not qualify those production dependencies, and no bypass was added. Service startup may perform an embedding readiness probe; the governed command path itself does not require a provider request.

## Activation handoff — not executed

Existing procedure: [hosted deployment](../../deploy/hosted/README.md), [deployment script](../../deploy/hosted/deploy.sh), and [release verification](../operator/release-verification.md). Previous deployment receipts describe historical state only; current production state and release sequence must be read again before activation.

1. Obtain explicit authorization to publish a release and activate the existing hosted service. This candidate has not been pushed, merged, tagged, signed or deployed by the producer lane. Review the integrated Zati adapter pin and protocol contract together.
2. Reconcile the existing instance, architecture, persistent data mount, backup health, release state and current managed lifecycle enrollment through the established AWS CLI/SSM procedure. Confirm broker configuration, journal authority, instance identity and readiness dependencies. Do not provision a replacement stack merely to deploy this candidate.
3. Produce the reviewed Linux ARM64 archive and checksum. Use the existing approved signature policy: matching GitHub release-workflow Sigstore identity, or the explicitly enrolled owner KMS manifest policy with its approved manifest, monotonic sequence, signature, root-owned verifier/policy/approval files and locked release state. Signing authority, release publication access and the exact approval packet are prerequisites; this receipt supplies none of them.
4. Publish matching authenticated release assets, then run the reviewed `deploy/hosted/deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET final` through the established root SSM path. Preserve existing configuration and enrollment. The script verifies before extraction, prepares the release, selects the binary with its broker units, checks local readiness and commits only after readiness. On an uncommitted activation failure it stops the service/broker and retains state for exact owner recovery.
5. Verify authenticated memory commands, reconciliation after restart, current-token revocation, ordinary MCP continuity, public health/readiness, backup completion and Zati connection custody against the deployed revision. Record startup provider probes separately from governed operations. Public smoke and integrated runtime acceptance remain separate from the script's local readiness result.

No production reads, production mutations, release signing, provider spending or deployment were performed while preparing this receipt. Runtime identities, resource names, credentials and machine-local paths are intentionally excluded.
