# Shared agent memory: resumed delivery

This receipt covers the resumed October 16 implementation attempt. It does not
claim completion of RFC 0002/0003, physical erasure, managed restore or native-host
capture. Real capture remains disabled.

## Landed source

| PR | Independently accepted revision | Landed revision | Delivered boundary |
| --- | --- | --- | --- |
| [389](https://github.com/sirerun/serenity/pull/389) | `df959ef2299c6ee89a38162bf2d8119be0ea9f21` | `b680354243d70f0d088e1fe7dced95b846174a01` | Atomic encrypted import records, cursor and content-free gaps; duplicate retry preserves state |
| [390](https://github.com/sirerun/serenity/pull/390) | `c4baa80ae95612f5d7af07b74e945dee04a07227` | `77196ba3f7098985496439ad3b4d3d87d3dce1c7` | Inactive episode-only Git rewrite candidate; malformed or mixed histories refuse |
| [391](https://github.com/sirerun/serenity/pull/391) | `5d0c9d1cbcd36f3fdb8f4b57313ae147d90f28e6` | `5a5dc91f6065acadc08e9a2fc99832d3470159b9` | Independently durable conversation-exclusion intent and startup replay; bounded private synthetic gate and credential revocation |
| [392](https://github.com/sirerun/serenity/pull/392) | `95fa53b5bb81869b862d537ec13abf3cb4d9fbb9` | `30919c2fe1581ff2c0e31475dc0b49cb2fd0926a` | Separate guarded offline synthetic fixture operator; existing provisioning, no email assertion or public authentication bypass |

Each landed tree equals its independently accepted tree and is reachable from
main. GitHub Actions jobs did not start because the account is billing-locked.
Proportional owner-authorized ADR024 fallback was used; this is not hosted-CI
success. Current protection and coordination checks were performed before each
guarded rebase merge. No protection bypass or foreign claim takeover occurred.
Ajent tooling was unavailable in the scoped engineering environment and disclosed
in the PRs.

## Verification

Affected Go race/vet checks ran on qualified existing Linux arm64 capacity with
Go 1.26.9, alongside applicable static, vulnerability and secret checks. No heavy
Mac build was used. Meaningful regressions cover duplicate retry, malformed Git
history, snapshot-before-exclusion replay, external journal ambiguity and the
22,000-record ordinary-brain startup case in normal and archive-disabled builds.

The fixture adapter passed 18 Python custody models. The actual source30919
release fixture binary passed 12 isolated Linux cases with a real nonroot UID/GID,
empty supplementary groups, six inherited descriptors and retained parent lock.
Failures included source, executable, boot, schema, lock and expiry mismatch.
These are synthetic checks, not production provisioning or host acceptance.

## Deployment boundary

The new feature and compatible archive-disabled safety binaries are built from
source30919. Signed sequence8 and conditional sequence7/high-water8 artifacts
expire October 20 at 18:49:01 UTC. A real archive-byte isolated recovery check
passed interrupted8, recovered8 and safety7/high-water8. This is not an executed
production rollback.

At the latest recorded pre-staging observation, production remains sequence6,
schema10, source8242459, with capture, WorkOS and billing disabled. New code and
signed files have been staged under root custody but not activated. The first
installed-verifier wrapper failed before invoking the verifier because the target
Python lacks `hashlib.file_digest`; its streaming-hash correction requires exact
re-review and a fresh successful verifier receipt.

Authority rollout, cold fixture provisioning, host activation and authenticated
API/restart/cleanup acceptance remain pending in this checkpoint. The current
source does not grant physical deletion acceptance. After the first new episode
journal subject, old authority code and old release6/safety5 are incompatible;
only the reviewed episode-aware recovery path is eligible. Safety7 approval is
conditional on committed state8, never a pending transaction.

## Remaining scope

Full T1.3 remains open: live purge activation, mixed-brain histories, all dependent
copies, managed backups/exports/devices and qualified restore. Retained fixture
rows, canonical bytes, backups or revoked credential strings must not be called
erased. Protected host enrollment/import, automatic durable facts, Zati's
200-message continuation boundary, and actual Cursor/Codex/Claude Code/Antigravity
integration still require their original plan gates. Existing source readers and
synthetic fixtures are not installed-host acceptance.
