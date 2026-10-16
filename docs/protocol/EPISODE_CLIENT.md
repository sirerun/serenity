# Offline episode client primitives

`internal/episodeclient` is T2.0 preparation for RFC0002/0003 and ADR027–029.
It has no HTTP client, provider call, host reader, enrollment, credential lookup,
installation, background worker, summary extraction or archive activation.
No production enrollment is permitted until archive physical purge and restore
fences are qualified. The package and fixtures do not qualify any actual host.

## Trust and exactness

A trusted adapter supplies a versioned `Policy`, explicit `Mapping` entries,
opaque `Binding{Brain, Policy, Producer}`, and source classifications. Never
populate policy or classifications from conversation text, model instructions,
repository settings or a guessed directory name. Role and tool fields remain
attributed evidence, not approval or execution authority. The producer identity
must be stable across restarts and separately enrolled from other installations.
IDs are 1–128 ASCII letters/digits/dot/underscore/hyphen, never paths or secrets.

Each input needs exactly one workspace mapping to the queue's entire binding.
Missing/duplicate/conflicting mappings fail closed. An explicitly mapped
`personal` workspace is supported but is never a fallback. Restricted brains
need separate queues and current grants. Binding changes cannot decrypt/rebind
an existing queue; there is no reroute or automatic migration API.

Only `Privacy="public"`, `Class="visible"` and
`HostMode="normalized-visible-v1"` can be admitted. Missing/unknown privacy,
credentials, secret/uncertain files, hidden reasoning, memory output and injected
recall are excluded **before any durable staging**. Known Serenity/memory tool
names are excluded as a second defense; adapters must classify all of their own
memory tools/aliases as `memory` or `recall`. Unknown host modes are reported as
unsupported. A privacy transition must call `Pause` for the covered scope; simply
submitting an excluded event does not erase previously queued or published work.

Best-effort text redaction covers common authorization/password/token/API-key
forms and private-key blocks. It is not a complete secret detector. Classify
secret files and credential sources as excluded rather than relying on regexes.
Images are opaque bytes: no OCR, metadata removal or visual secret detection is
claimed. A trusted adapter must attest that exposed attachment bytes are clear;
unknown secret status is excluded. No file or URL is opened from captured text.

Admitted text preserves whitespace and UTF-8 exactly, except explicitly reported
redactions. Exposed `text/plain`, `image/png`, and `image/jpeg` attachment bytes
are copied and hashed; only text/plain undergoes text redaction. Media types
here describe byte custody, not image decoding or validation. Other media types,
missing bytes, uncertain/secret sources and invalid text encoding produce fixed,
content-free gaps. No unsupported bytes or attachment names/paths enter gaps.
A gap does not claim omitted data was preserved. Post-redaction limits also apply.

## Public Go APIs

| API | Contract |
| --- | --- |
| `OpenQueue(Options) (*Queue, error)` | Opens a dedicated queue with caller-supplied 32-byte AES key, immutable policy/binding and explicit item/byte limits. Linux and Darwin only. |
| `(*Queue).Capture(Input) (Decision, error)` | Filters, copies and atomically stages one bounded normalized event. Identical pending retries are no-ops; changed event content or conflicting sequence is rejected. |
| `(*Queue).Stats() (QueueStats, error)` | Reports item count, serialized record bytes and paused-scope count without content. |
| `(*Queue).Pause(Scope) (int, error)` | Atomically persists a local pause and removes covered unpublished items. Empty scope means all; nonempty workspace/conversation fields are conjunctive. The count is confirmed only when error is nil. |
| `(*Queue).Resume(Scope) error` | Explicitly removes this exact pause only. Overlapping broader pauses remain. Cannot override a server fence, revoked grant, private state or changed policy. |
| `(*Queue).Drain(ctx, Reconciler, Publisher, max) (DrainResult, error)` | At most 64 items, with fresh current binding/fences before each publication and exact complete receipt before local removal. Transport is a separate trusted adapter. |
| `(*Queue).Close() error` | Releases the handle; does not remove queue state or promise cryptographic key zeroization. |
| `Import(io.Reader, LogSource, Cursor, emit) (ImportResult, error)` | Bounded generic JSONL import; normally use `Queue.Capture` as `emit`. Returns safe progress and content-free gap counts. |
| `DeterministicEventID(LogSource, line) string` | Stable fallback identity from a logical log ID, incarnation and physical line number. Invalid arguments return empty. |
| `DigestRecord(Record) string` | SHA-256 of this package's JSON record; useful for synthetic receipts. This is not the archive wire digest or a signature. |

`Input` and `Record` fields and JSON tags are the exact local schema in
`policy.go`. `Input` preserves source and observed times supplied by the adapter;
no timestamp is fabricated. Required role is user/assistant/tool/system, and
kind is message/tool_request/tool_result. The caller supplies stable conversation
and event IDs and positive sequence. Attached bytes use ordinary JSON base64.
`Record` includes the exact immutable binding, workspace, identity, times,
attributed role/kind/tool, admitted text/bytes/digests and fixed gap codes.

`Decision.Admitted` means a durable local write or an identical pending retry
completed successfully, not remote publication. A write/fsync failure is an
ambiguous local outcome: retry the **same** identity. `DrainResult.Published`
counts exact receipts followed by successful durable local removal. Neither
status absence nor a transport error proves remote nonexecution.

## Queue storage, limits and concurrency

The queue uses stdlib AES-256-GCM with a fresh random nonce per snapshot.
Authenticated associated data binds the format and brain/policy/producer tuple.
The entire state, including pauses, identities and generation, is encrypted;
there are no plaintext event filenames, indexes or sidecars. The caller owns key
provisioning/recovery; no credential stores are read or copied.

The caller supplies an absolute, clean, symlink-free directory path beneath
existing trusted parents. Only the final directory is created. Ancestors must
be root/current-user owned and not group/world writable (sticky temporary parents
are permitted). Queue directories require mode 0700; data/lock/pending files
require mode 0600, current ownership and a single hard link for regular files.
Unsafe existing paths/permissions fail closed rather than being repaired.
`os.Root`, no-follow opens, bounded reads and repeated inode/permission checks
protect managed file access. POSIX modes do not qualify arbitrary ACLs, network
filesystems, hostile root/same-user processes or rollback of a whole directory.
The actual filesystem must qualify locking, atomic rename and directory fsync
before use. The library is not secure deletion or forensic disk erasure.

Every operation takes a handle mutex and an OS file lock shared across handles
and processes. Drain holds it through reconciliation, publish and durable
acknowledgment. Callbacks must not reenter the queue and must honor bounded
contexts. Waiting for an OS lock itself is blocking. A pause waits for an
already-running drain call; publications completed before that pause are already
published and require a separate server deletion action. Once Pause succeeds,
covered remaining items are removed and further covered capture is blocked.
Use small drain batches/deadlines to keep privacy controls responsive.

Writes create an exclusive encrypted `queue.pending`, fsync it, close it,
atomically rename it to `queue.bin`, then fsync the directory. Directory creation
also syncs the parent. A stale pending file is unacknowledged crash residue and
is discarded under the lock; committed state is never replaced with that residue.
Only admitted data reaches either file. The dedicated directory is owned by this
API; do not put authored material in its reserved filenames. On failure, an old
snapshot or encrypted pending residue can remain; no success is claimed.

Limits are deliberately conservative and are not measured capacity claims:

| Limit | Value |
| --- | --- |
| Text per input and admitted record | 64 KiB |
| Attachment bytes | 32 KiB each, at most 8 per input |
| Serialized admitted record | 512 KiB |
| Pending records | Caller limit, 1–1024 |
| Pending serialized record bytes | Caller limit, 1 byte–32 MiB |
| Pause scopes | 1024 |
| Complete opaque fence set | 4096 conversations |
| Drain batch | 1–64 records |
| Snapshot metadata/encryption allowance | At most 1 MiB above caller record-byte cap |

Atomic replacement temporarily needs two snapshots, so provision up to twice
`MaxBytes + 1 MiB`, plus filesystem/lock overhead. No FIFO or age eviction exists.
Limits stop admission visibly. Pausing/removing remains possible when the item
or record-byte cap is full (subject to metadata/disk capacity). Storage failure
cannot guarantee that a requested pause persisted; surface the error and stop
capture/drain in the host until resolved. Snapshots are rewritten, so this is a
bounded correctness primitive, not a high-throughput queue. Only pending event
identities are retained locally; permanent retry/conflict protection belongs to
the canonical server. The server must reject changed-content reuse after local
acknowledgment. Queue order is conversation ID then sequence, with no invented
cross-conversation causality. Live/import sequencing still needs adapter-level
coordination with published ranges.

## Replay and transport integration boundary

`Reconciler.Reconcile(ctx, ReplayKey)` receives metadata only. The trusted
integration must resolve current workspace enrollment, policy, privacy and grant;
verify the current authenticated brain; then fetch the **complete**
server fence set for the brain, including other producers. Set `Allowed` only after those checks. A
partial/reset-required/failed response cannot set `Full=true`. The generation
must be explicitly known (zero can be known); timestamps must be fresh, not in
the future, with expiry at most one minute after sync. The queue rejects expired,
missing, stale, older-than-observed or mismatched verification. Highest observed
generation and complete fence set are persisted, including when publication is
blocked. Known fenced conversations are also rejected at subsequent local
admission. A same-generation set change or disappearance of an observed fence
fails closed; intentional recapture is unsupported. Offline admission before the
first synchronization is only local staging, never permission to publish.

The `Conversations` map uses lowercase SHA-256 keys from
`ConversationFenceKey(producer, conversation)`: the hash of the JSON array of
those two strings, matching the archive protocol. Keep the complete opaque set;
filtering it to the currently queued conversation would lose observed fences
after a restart or when draining a different conversation. The synthetic
`internal/episodecapture` adapter verifies and retains this complete set.
Fenced records are retained blocked; explicit `Pause` purges covered unpublished
copies and stops admission. Policy/brain remaps and revocation similarly remain
blocked, never rerouted. A blocked first record stops the batch visibly.

`Publisher.Publish` must send the validated generation and stable identity,
check it again at server admission **and canonical publication**, and translate
an exact canonical receipt into `Receipt{Key, Digest, Complete}`. `Complete`
requires custody of every admitted text/attachment component. The interfaces
are trusted seams, not security tokens; setting booleans in a fake reconciler
or publisher provides no real authorization. This lane supplies only test fakes.
A deletion after client synchronization must still be rejected by the server.
No production use is authorized by the existence of these interfaces.

## Generic normalized import

`testdata/normalized-visible.jsonl` is explicitly synthetic package input, not
an observed Cursor, Codex, Claude Code, Antigravity or Zati schema. No host reader
or fictional lifecycle hook is shipped. Host formats, private-mode detection,
attachments, installation and native continuation remain unqualified.

Import consumes at most 8 MiB + one overflow byte, 4096 complete records and
768 KiB per line, with a maximum JSON nesting depth of 16. Unknown fields,
duplicate/case-alias keys, invalid UTF-8, unpaired UTF-16 escapes, extra JSON
values and unknown schema fail visibly. A final incomplete line returns
`ErrPartial` after admitting only complete preceding lines. No partial bytes are
staged. An over-limit source requires an explicitly designed segmentation path;
there is no automatic truncation or renumbering.

With absent event IDs/sequences, the fallback ID is `log-` plus SHA-256 of
`episodeclient.log.v1`, NUL, logical source ID, NUL, incarnation, NUL, decimal
one-based physical line number. Sequence becomes that line number. Identical
text on different lines remains distinct. Existing IDs/sequences are preserved.
Hooks and import must reuse the same mapping. Never regenerate producer IDs,
log IDs or incarnations merely to bypass a conflict or fence.

The cursor stores source/incarnation, complete-record count and SHA-256 of the
exact consumed JSONL byte prefix (including newlines). Resume verifies the prefix
**before** any callback. Rotation, prefix rewrite/reordering and truncation fail
with `ErrRotation`; explicit reconciliation is required, not silent reset.
An emit error leaves that record unconsumed, except explicit policy exclusion or
unsupported host mode, which advances with an exclusion count. Persist only the
returned cursor in protected adapter state. A crash between queue write and
cursor persistence repeats the same identity; this is safe only with canonical
server idempotency after queue acknowledgment. The cursor is not a host log
reader, encrypted adapter state store or server publication watermark.

## Verification and remaining gates

Focused tests cover exact bytes, source exclusions, redaction, mapping failures,
unknown modes/formats, parser bounds, prefix/rotation conflicts, stable import
retry, encryption/restart/binding, capacity without eviction, pause scope,
permissions/symlinks/hard links, interrupted staging, stale/revoked/fenced replay,
incomplete/ambiguous receipts, and concurrent handles/pause serialization.
These test sources require execution and independent review; their presence is
not a passing test receipt. Parent-owned qualified Linux/Spark commands:

```sh
GOMAXPROCS=2 go test -count=1 -p=1 ./internal/episodeclient
GOMAXPROCS=2 go test -race -count=1 -p=1 ./internal/episodeclient
```

Run on the exact candidate revision with the repository Go toolchain. No Mac
build/test/vet was run by this lane. Formatting and `git diff --check` are source
checks only. Remaining gates include remote tests, independent review, final wire
adapter and real server integration, fault/crash/power-loss qualification,
filesystem/ACL/key custody, durable host progress, actual host capability evidence,
physical archive purge/restore, and separately authorized enrollment. Parent owns
all merge, integration and deployment decisions.
