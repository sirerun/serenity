# Governed memory integration candidate — 2026-10-12

## Source and evidence

Original qualified production candidate and acceptance-fixture source: `7773ed132d155638316925f72b79f7db39f0abed`.
The final commit adds only the opt-in hosted process fixture; production code is unchanged from `b1ef2fa195e2e25e268972e72c953aefc723e918`.
Final qualified source and Zati adapter pin: `7a5f647a37214c619300b8873974872a5ee30860`. It adds the official-issuer fixture and documentation, with no production code change. The full `go test -race -p 1 ./...` suite passed again at this exact revision using the private fixture directory settings below; the hosted service package reran in 22.700 seconds. The earlier lint and build results below retain their original revisions.

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

## Hosted activation

The integration coordinator deployed source `972d3692dc1b8fb9284fb9c68c850b14ae28da65` as signed sequence3 on 2026-10-12. Production behavior is equivalent to the retained adapter pin `7a5f647a`; subsequent changes before deployment were tests and documentation. Exact release hashes, independently inspected postflight evidence and public checks are recorded in the [activation receipt](GOVERNED_MEMORY_ACTIVATION_HANDOFF.md).

The existing managed broker/authority and root-verifier transaction were used successfully, with prior builds preserved and no IAM widening or new infrastructure. Public governed memory now refuses unauthenticated requests with401. Real account consent and customer-memory acceptance against production remain unverified; synthetic fixture qualification is not upgraded into that claim.

## Controller renewal fixture follow-up

Producer fixture revision `2f659651` passed the full race suite with the private recovery directory settings above. Its production code remains equivalent to adapter pin `7a5f647a`; no adapter re-pin was needed. Hosted activation occurred subsequently as recorded above.

An explicitly gated test-only wrapper shortens the first authorization-code access token to four minutes in both its persisted token record and response. It preserves the original request body for the real OAuth handler. Refresh tokens still produce the normal one-hour access lifetime. This synthetic lifetime permits immediate renewal qualification without changing clocks or production configuration.

The independent assembled acceptance at Zati `bb9459f` against producer `2f659651`, both race-enabled, passed in 21.97 seconds: one registration, two token exchanges, two binding reads, unchanged opaque credential reference and connection version, a completed governed recall using that reference after renewal, no refresh loop, and revocation preventing the next due refresh. Governed operations added no embedding requests. The browser/session, initial token lifetime and lifecycle dependencies are synthetic; this does not establish managed deployment or native browser acceptance. Owned fixture processes and private manifests were cleaned up.

## Scheduled backup follow-up

Read-only production status confirmed the first scheduled post-activation backup completed successfully at 2026-10-12 13:01:21 UTC, with exit status0. Hosted service, broker and backup timer remained active; the running executable digest was unchanged, sequence3 remained installed and no release transaction was pending. This does not qualify a production restore. The release manifest's 15:55 UTC expiration limits installation admission, not ongoing service operation or normal broker-authorized restart.
