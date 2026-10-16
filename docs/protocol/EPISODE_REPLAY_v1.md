# Episode exclusion and external replay v1

This adds external replay safety to the synthetic-only
[episode API](EPISODES_v1.md). It does not enable real-history capture, enroll a
host, qualify managed restore as a product, or certify physical erasure. The
API continues to report `physical_purge: "unsupported"` and
`managed_restore: "unsupported"`; capabilities retain `physical_purge: false`.
T1.3 remains open. Provider and deployment qualification are separate gates.

## Existing transport and authority

Episode exclusion uses the existing independently durable deletion journal,
immutable object layout, conditional-create transport, hash chain, generation
reservation and startup authority. There is no additional journal, bucket,
control-database completeness marker or production enablement flag.

An entry uses `subject_type: "episode_conversation"` and `outcome: "requested"`.
`purged` is invalid for this subject. Its `subject_id` is exactly 262 ASCII bytes:

```text
v1:<brain-sha256>:<conversation-sha256>:<command-sha256>:<request-digest>
```

Every hash is exactly 64 lowercase hexadecimal characters. The brain hash is
SHA-256 of the UTF-8 bytes `serenity:episode-deletion:brain:v1`, one NUL byte,
and the authenticated opaque brain ID. The other hashes are the existing
`EpisodeConversationKey`, `EpisodeCommandKey` and normalized exclusion
`EpisodeHash` values. Command and digest preserve idempotent receipts without
retaining originals. The timestamp is the canonical exclusion's `recorded_at`.
No original conversation/producer/command IDs, paths, transcript, attachments,
reason text or credentials enter this subject. These hashes remain protected
metadata; hashing is not a claim of anonymization.

The journal object's fields, key layout and encoding are unchanged. Account
and brain entries retain byte-identical encoding. Old validators only admit
`account` and `brain`; an old reader rejects the explicit new subject while
validating the chain. An episode is never disguised as a brain deletion.
**After any new entry exists, rollback requires an episode-aware binary.**
Do not deploy an old reader against that history or remove new entries to make
an old reader start.

## Exclude acknowledgment and failure

Authentication, exact brain binding, write scope, synthetic qualification,
account maintenance and credential rechecks remain required. Under the
maintenance/account/runtime locks, the serialized writer validates current
canonical identity and generation, then attempts the external intent before
publishing the content-free canonical fence. HTTP 200 `status: "excluded"`
requires both matching independent intent and successful canonical publication.
Write/storage quotas do not consume the reserved exclusion path.

A failed or ambiguous append returns 503 `outcome_unknown`. The writer still
attempts to persist the content-free local fence as quarantine. Such a fence
alone is not evidence of restore protection. Other episode requests fail with
503 `exclusion_replay_pending` until reconciliation; unpublished canonical
state also fails closed (`publication_pending`). If external intent succeeds
but canonical publication fails, the journal preserves the exclusion across
restart and older-snapshot recovery. The request is never acknowledged merely
because either one of these writes succeeded.

An identical exclusion retry can repair its own pending boundary, using the
same opaque identities and timestamp. It may use the generation from the
failed request even when its own quarantine fence incremented generation.
It cannot bypass another pending exclusion or change the request digest.
After successful publication, the normal current-generation requirement
applies again. Repeated immutable intents with identical identity and time
are harmless and replay once. Conflicting identities or times fail closed.
No unsealed tail read clears an uncertain outcome.

The finite source-store fence allowance remains enforced. If it prevents
installation after durable intent, exclusion returns failure and the brain
remains closed; startup also fails until canonical reconciliation is possible.
Neither a full quota nor a fence limit is permission to acknowledge absent
durability. If both independent append and local quarantine persistence fail,
there is no successful exclusion receipt: runtime remains closed and restart
requires fresh authority-admitted full history to resolve any landed intent.

## Startup and old snapshots

Service assembly uses its existing authority boundary before opening handlers:

- Ordinary startup consumes the full history from `Observe`, after the
  existing startup authority verifies its reservation, position and history.
  An unsealed observation alone is not proof; the external authority is required.
- Recovery obtains the existing admitted cut, reads from the beginning as well
  as the cut, and requires both reads to end at the same sealed boundary.
  Pre-snapshot intents cannot be discarded as an incremental optimization.

All entries are validated and episode entries are separated before legacy
`ReplayDeletions`. Brain hashes resolve only against current brain identities
or explicit whole-brain deletion disposition in the admitted history. A
stronger account/brain deletion is left to legacy replay, which must finish
before handlers open. This avoids reopening deleted data or permanently
blocking startup merely because a legitimately deleted brain is now absent.
Unresolved identities, malformed versions, conflicting receipts, wrong-brain
canonical records, missing journal evidence, incomplete history, cancellation
and publication failure all stop startup.

Missing canonical fences are rebuilt with the existing serialized writer,
without original identifiers or payloads. Every canonical exclusion must match
an admitted external intent, and every admitted active-brain intent must have
a published canonical exclusion. Generation is reconstructed from canonical
fences, not a restored local database. Existing eligibility checks exclude
originals and dependent checkpoints; stale queued append/checkpoint attempts
cannot republish them. Sibling brains retain their independent scope.

## Foundation exclusions without journal evidence

The earlier foundation could acknowledge local fences without external intent.
Those fences cannot silently acquire a restore guarantee from an upgrade.
A live, authorized identical-exclude retry can append the matching journal
intent and reconcile the fence; unrelated operations remain closed meanwhile.
At startup, any local fence missing from admitted full history is a hard
`ErrEpisodeReplayPending` failure. Resolve that migration explicitly before
exposing handlers. Do not infer an empty external history from the local DB,
delete the fence, or label old snapshots protected merely because a local
fence once existed. A snapshot that lost an unjournaled foundation fence
cannot prove that exclusion ever happened.

## Verification scope

Contract fixtures pin strict identity validation, legacy object bytes and
old-reader subject rejection. Writer and HTTP fixtures cover ordering,
ambiguous failures, canonical failure, persistent quarantine, retry, finite
fence capacity, quota bypass for exclusion and authorization. Service fixtures
exercise an actual pre-delete Git snapshot against current admitted journal
history, checkpoint exclusion, sibling isolation, ordinary authority admission,
sealed recovery, invalid history and legitimate whole-brain deletion.
These are synthetic verification fixtures, not provider or production receipts.
