# Shared agent memory: default-off foundation delivery

Observed 2026-10-16. **The foundation is merged and deployed, with capture off.
The complete shared-memory RFC and authenticated production acceptance remain
incomplete.** This receipt does not qualify physical erasure, managed restore,
real-history import, automatic facts, Zati continuity or all-host plugins.

## Source and release identities

| Item | Exact identity | Evidence boundary |
| --- | --- | --- |
| Authoritative design | PR385, `92b785fe6da569fa537b7ec92fb4a14197c8c3ae` | Design, not runtime completion |
| Foundation | [PR386](https://github.com/sirerun/serenity/pull/386), `8242459bc6a8689d81c96d0b66ed7f751a0de090` | Reviewed head `e0064a646bdea2bf6370d361828b8b57a204d005`; equal landed tree `024eb6efd0b22c363deb2a8698f35e1dd696b990` |
| Reader hardening | [PR387](https://github.com/sirerun/serenity/pull/387), `efdaf3da592e6fea9ddda453d08abc163400bc3c` | Reviewed head `9e48b85320e246123daa2ab2d62d95fc4f4df63f`; equal landed tree `04b4efa627d11df9910372f24c10d876273f27a0` |
| Installed release | `v0.1.14-memory-foundation-8242459bc6a8`, sequence 6/high-water 6 | Foundation source above; reader libraries are not linked into the hosted command |
| Installed executable SHA256 | `67de747230478d7099e4aa986d9a489f72f466d2aa7b5a6e6e1c6c25faa2e50f` | Disk, hosted process and broker process matched |
| Release archive SHA256 | `2bb50aa17b8456a70a2eaa17b6e893a7963527e10f760f0917aee501f42809ef` | Exact signed Linux ARM64 archive |
| Manifest SHA256 | `e6d84041d67fd11378b8d42fe329106d26405def26433093f807de023ac39edc` | Existing signing key and installed root verifier |

The increment adds a bounded episodic protocol on the existing canonical Git/file
store and serialized writer, with bounded canonical projections, exact originals,
retry/deduplication, scoped exclusion fences, checkpoints and bounded context.
Protected queue/policy, a version-pinned Codex text reader and a synthetic TLS
transport are source primitives. They are not installed capture agents.
Nonempty Codex provenance metadata and encrypted-message records are excluded
with content-free gaps; deliberately unsupported shapes remain unsupported.

## Verification and live observations

The foundation passed remote build/vet/full race checks on its recorded candidate,
then affected checks on each reviewed correction. Independent review found and
closed canonical-size admission and queue/wire-boundary defects. Final affected
checks, lint and source vulnerability checks passed. Both exact symbol-retaining
release binaries and matching CGO0 source scans reported no affected
vulnerabilities; two module-only advisories remained explicitly distinguished.
PR387 separately passed three affected race/vet suites, hosted build/dependency
checks, lint with zero issues, vulnerability checks and a scoped secret scan.

GitHub Actions did not start because of account billing. Runs and annotations
were inspected; runner ID was zero and no steps ran. Owner-authorized ADR024
fallback evidence was used for guarded rebase merges. No branch protection,
required approval or ruleset was bypassed. Independent review was recorded for
each exact source revision and the final operational packet.

The operational review closed interruption, durable-publication and postcommit
backup-timer findings. It independently exercised 123 isolated scenarios plus
24 adversarial cases. Disposable tests on the actual systemd252 host qualified
the required unit/guard semantics. These are not physical power-loss or complete
production rollback experiments.

Maintenance began around 13:11 UTC. A retained transient supervisor prevented the
first activation attempt before installation. A separately reviewed, narrowly
scoped rearm step removed only temporary bindings while preserving the durable
hold and permanent startup guards. A disposable target fixture reproduced the
issue and qualified that remedy. Activation then succeeded; public service was
observed healthy at 13:25 UTC. The approximate maintenance interval was 14 minutes.

At 13:29 UTC, the exact release was committed with no pending transition, hold or
closeout marker. Hosted/broker processes and backup timer were active. Control
schema was 10. Configuration bytes were unchanged: archive, WorkOS and billing
were off, with an empty synthetic allowlist. Public health/readiness and OAuth
discovery returned 200, unauthenticated memory/MCP returned 401, and POST
`/episodes/v1` returned 404 with `serenity.episodes/v1` / `archive_disabled`.

Authority version 7 retained existing code, roles, limits and non-build settings,
preserved old/base policies, and added the two exact release identities. Its full
restrictive policy was revision-conditionally written and read back. Direct base
and qualified invocation were denied; concurrency was restored to one. A current
authority lease identifies the installed foundation source. This is authority
admission evidence, not authenticated memory write/recall acceptance.

The post-deployment coherent backup completed at 13:26:44 UTC. Its fresh off-host
completion marker and sibling archive metadata were checked. No customer archive
was downloaded, and no restore acceptance is inferred from that metadata check.

## Rollback and remaining work

The prepared fallback is `v0.1.14-memory-rollback-92b785fe6da5`, source
`92b785fe6da569fa537b7ec92fb4a14197c8c3ae`, sequence 5 with high-water remaining 6.
Its executable SHA256 is
`7137a2aa6f486640b7ddcd3c36f1b059eed444d3acae03d6bc9d1ab1c20691ad`.
The installed root verifier accepted it against actual committed state6 without
changing that state. The prepared manifests/approvals expire at
2026-10-19T12:17:16Z. Actual rollback and authenticated post-rollback recall were
not executed. Preserve schema10 data; the former schema9 executable is not a
rollback target. Re-promotion after rollback requires a sequence greater than6.

The scoped [implementation plan](../plans/shared-agent-memory-implementation-20261016.md)
retains all unfinished work. In particular:

- An authorized synthetic production credential is unavailable. Authenticated
  memory write/recall/idempotency and OAuth/MCP recall/refresh remain open; 200/401
  probes cannot substitute for them.
- The external deletion journal currently understands account/brain subjects,
  not episode fences. Existing recovery/import claims and PR273/PR277 were
  preserved. Compatible external fencing, physical Git/derived/managed-copy
  purge, backup replay and restore must be implemented and qualified before real
  capture. Current exclusion is not physical erasure.
- Real history/tool/attachment import, automatic durable facts, Zati integration,
  cross-agent continuation, installed Cursor/Codex/Claude Code/Antigravity
  adapters, portable export and capacity/soak qualification remain pending.
  Current official host documentation was collected as preparation only.

Private operational receipts, exact packet hashes and recovery commands are in
the task artifact record. No credentials, private host addresses or local
workspace paths are included here. Main-checkout authored material and the
merged design worktree were preserved.
