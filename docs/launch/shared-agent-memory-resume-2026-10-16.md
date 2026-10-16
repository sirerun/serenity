# Shared agent memory: resumed delivery

This records independently landed increments and a default-off hosted release.
RFC 0002/0003 are **not complete**. Physical erasure, managed production restore,
real capture and cross-host continuity remain unqualified.

## Landed source

| PR | Independently accepted revision | Landed revision | Delivered boundary |
| --- | --- | --- | --- |
| [389](https://github.com/sirerun/serenity/pull/389) | `df959ef2299c6ee89a38162bf2d8119be0ea9f21` | `b680354243d70f0d088e1fe7dced95b846174a01` | Atomic encrypted import records, cursor and content-free gaps |
| [390](https://github.com/sirerun/serenity/pull/390) | `c4baa80ae95612f5d7af07b74e945dee04a07227` | `77196ba3f7098985496439ad3b4d3d87d3dce1c7` | Inactive episode-only Git erasure candidates; unsafe or mixed histories refuse |
| [391](https://github.com/sirerun/serenity/pull/391) | `5d0c9d1cbcd36f3fdb8f4b57313ae147d90f28e6` | `5a5dc91f6065acadc08e9a2fc99832d3470159b9` | External episode exclusion intents and startup replay; private synthetic gate and exact-key revocation |
| [392](https://github.com/sirerun/serenity/pull/392) | `95fa53b5bb81869b862d537ec13abf3cb4d9fbb9` | `30919c2fe1581ff2c0e31475dc0b49cb2fd0926a` | Guarded offline fixture operator and service-UID/descriptor custody |
| [393](https://github.com/sirerun/serenity/pull/393) | `3b5b22e0ecbf45f4d20d897398b6c2eb2c4ed3b7` | `f6efd727577728161200a93becf8919a76adca30` | Synthetic Git snapshot, external replay and inactive-candidate qualification |
| [394](https://github.com/sirerun/serenity/pull/394) | `3b90f59a4ba983d5f8259f9c590a7ed758a620f9` | `25d74ce0c64e0032516b38a6902d579c43eb8c1a` | Codex partial-answer phase through protected synthetic import |
| [395](https://github.com/sirerun/serenity/pull/395) | `6c06849c9375796bf7b1b7c74db93592097185bf` | `5dcef0a9e5d3b9091ced4e7ab99fe24c2af63bc0` | Offline recovery of correctly verified, recorded revoked fixture keys |

Each landed tree equals its independently accepted tree and is reachable from
main. Actions jobs did not start because the account is billing-locked.
Owner-authorized ADR024 fallback is source verification, not hosted-CI success.
Current protections and coordination were checked before guarded rebase merges;
no protection bypass or foreign claim takeover occurred. Ajent was unavailable
in the scoped engineering environment and disclosed in the PRs.

## Verification and host limits

Affected race/vet checks ran on existing Linux arm64 capacity with Go 1.26.9,
alongside applicable lint, vulnerability and secret checks. No heavy Mac build
was used. Regressions cover import retry/redaction, malformed Git history,
external journal ambiguity, exclusion replay and dependent checkpoints, plus
22,000-record ordinary-brain startup in normal and archive-disabled builds.
The fixture adapter passed 18 Python models; its actual source 30919 binary
passed 12 isolated Linux UID/GID/descriptor cases. Separate real Unix-peer checks
verified service-owner admission and root/identity refusal.

A controlled installed Codex 0.163.0 session emitted two known visible markers
under the selected runtime settings. Both carried sibling provenance metadata.
A sanitized projection preserving physical positions produced **zero admitted
messages and two explicit provenance gaps** through the landed reader. This is
negative coverage evidence, not intact native history or an installed plugin.
No existing history, hidden reasoning, tool content or attachments were exported.
Matching version strings do not establish source-to-binary build provenance.
Current conservative metadata exclusions remain; PR 394 does not backfill prior
acknowledged unsupported-phase gaps.

## Deployed state and live acceptance

Hosted **sequence 8/high-water 8** uses source
`30919c2fe1581ff2c0e31475dc0b49cb2fd0926a`, executable SHA256
`7e2267ee7d6c91c017d7ea13f30c1ac262e35cddd85e8b247b6c391f9ae93a24`.
The authority runs published version 8 from that source with reviewed policies
and concurrency 1. Newer client/test/operator source is not implicitly installed.

At 21:06 UTC root/public observations, exact release/process checks pass;
health/readiness and public metadata return 200, unauthenticated memory/MCP 401,
and episodes 404 with `archive_disabled` and `no-store`. Capture, WorkOS and
billing remain disabled. The backup timer is active, with no pending release or
maintenance hold. This is not a real-user OAuth-login qualification.

The first full synthetic API run failed at its initial public-disabled check:
the edge returned plain-text 403 to the default Python user agent. It stopped
before append, exclusion or restart. A truthfully identified Serenity client
received the expected JSON. Separately reviewed cleanup verified both reserved
fixture keys return 401, closed the temporary gate, verified exact public 404,
restored the timer and checked the recovery unit stopped-or-absent with no
remaining cgroup. Independent receipt review accepted cleanup. The original
failed journal remains byte-identical; **cleanup does not make the original
full qualification pass**.

Two reserved synthetic accounts/brains and their data remain. Revoked key strings
remain in a private 0600 manifest outside backed-up canonical data; they were not
transferred or exposed and are not described as erased. No real-user credentials,
email assertion, paid infrastructure or destructive customer migration was used.

A subsequent provider refresh confirmed published 8 configuration, code, full
policies and routes unchanged and concurrency 1. Strict comparison identified only
an opaque revision-ID change on the unpublished latest version; this is recorded
without attributing a cause or treating it as a code/configuration change.

## Rollback and next live gate

Compatible archive-disabled safety **sequence 7/high-water 8**, source 30919, has
executable SHA256
`8f181c0091ca02a983b5187fb4163309cd5f7e34aa76dad4c03bd80e6ee73e81`.
The installed verifier accepted it against **actual committed state 8**. Signed
approvals expire **2026-10-20 at 18:49:01 UTC**. An isolated actual-archive check
exercised interrupted 8, recovered 8 and safety 7/high-water 8; production rollback
was not executed. Pending transactions recover forward before safety fallback.
Old release 6/safety 5 and old authority code are incompatible after a new episode
journal subject and are not an assumed rollback path.

Full live append/read/attachment/retry/isolation/checkpoint/exclusion/restart
qualification remains pending; its keys are revoked. A new attempt needs reviewed
current-state cold reissue, explicit compatibility with the existing source-bound
fixture manifest, and exact historical/current credential inventory. PR 395 is
not an installed tool update or permission to retag that manifest. A new packet
must retain TLS, no redirect/proxy, exact pins, bounded cleanup and 401/404 checks.
Quota exhaustion, physical erasure and managed restore remain separate gates.

## Remaining design

T1.3 still needs complete admitted-fence binding to activation, mixed-brain and
derivative handling, controlled canonical/history-copy purge, and managed
backup/export/device custody with honest residual-copy reporting. The inactive
builder cannot authenticate its supplied fences or erase the source.

Automatic protected host import/capture, standing-policy durable facts, Zati's
200-message continuation boundary and installed Cursor/Codex/Claude Code/Google
Antigravity adapters remain open. Queue, decoder and synthetic fixtures do not
establish those end-to-end outcomes. Hidden reasoning and native vendor state
are not recoverable promises.
