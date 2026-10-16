# Visible history reader qualification

The initial `internal/episodehost` reader is a preparation component. It does
not install a plugin, discover history files, enroll sessions, queue content or
upload anything. Full host capture, import and cross-agent continuation remain
unqualified. Apply the shared capture policy before any durable staging.

## Codex 0.163.0 snapshot reader

The decoder is pinned to the public
[0.163.0 protocol source](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/protocol/src/protocol.rs),
[response models](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/protocol/src/models.rs)
and [rollout recorder](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/rollout/src/recorder.rs).
These are schema evidence; the synthetic tests are not installed-host acceptance.
The [official hooks reference](https://developers.openai.com/codex/hooks)
explicitly describes transcript format as unstable.

A trusted caller supplies the exact enrolled session identity and an explicit
visible-session decision. The reader cannot detect host private mode. Unknown
sessions, different versions, forks requiring inherited history and rewritten
acknowledged prefixes fail closed. No filesystem path in a transcript is opened.

Covered records are user/assistant text in response-item messages. Original
text bytes and recorded timestamps are retained; the occurrence identity binds
the enrolled session and physical record position. Identical text at different
positions remains separate evidence. Event messages are not imported a second
time. Reasoning, nonvisible roles/channels and compaction-derived summaries are
excluded. Tools, attachment content and unknown variants produce content-free
coverage gaps. This subset does not meet the full capture requirement yet.

Input is limited to16 MiB, individual JSONL lines to1 MiB, message text to64 KiB,
and each page to64 messages or256 scanned new lines. These are parser limits,
not measured service capacity. An incomplete final line remains unacknowledged.
The caller saves the returned prefix cursor only after every returned message
is safely admitted; retries preserve occurrence identity. The cursor detects
truncation, edits and rotation instead of silently inventing a new stream.

## Other requested hosts

The current official [Claude Code hooks](https://code.claude.com/docs/en/hooks),
[Cursor hooks](https://cursor.com/docs/hooks), and
[Antigravity hooks](https://www.antigravity.google/docs/hooks) document lifecycle
and transcript-related surfaces. Each still needs its own supported-version
reader, installation/rollback and actual execution fixtures. No common hook
name or transcript-path field establishes identical coverage across hosts.
Zati remains the required first runtime integration; this decoder does not
change its context admission or the 200-message boundary.

The synthetic transport reports `tool_identity_and_pairing_not_archived` for
normalized tool events: its first protocol projection preserves visible text,
role and kind but does not qualify native tool-call pairing or tool identity.
No actual host tool coverage is claimed.

Codex persists harness provenance as a sibling `metadata` field on response
items. This reader conservatively excludes every nonempty metadata object with
`host_provenance_metadata_unsupported`, including compaction, inherited context,
retained evidence, delivery markers and otherwise neutral host metadata. Absent,
null and empty metadata remain eligible. This deliberately narrows coverage until
individual provenance shapes have version-bound fixtures; it never labels a
compaction result or copied parent context as a new original occurrence.
Messages with non-null `encrypted_content` are also excluded as a whole, even
if they contain plaintext blocks. Gap records contain no source text.

The pinned source evidence is Codex commit
`83086188ae0614bd882818ff1aa23f3194962012`,
[`CodexHarnessMetadata`](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/history/src/lib.rs)
and [`RolloutItemWire`](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/history/src/rollout_payload.rs).
No host installation, real-history import or production capture is enabled.
