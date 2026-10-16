# Protected import progress

`episodecapture.StageCodexDurable` is a synthetic-only integration of the pinned
Codex reader and protected queue. It reads only a caller-supplied stream. It does
not discover histories, enroll a host, read credentials or upload records.

Each page atomically commits redacted events, the verified prefix cursor and
cumulative fixed-code gaps into one encrypted queue snapshot. Failure from
capacity, policy, pause, exclusion fences or conflicting progress commits
neither the page nor its cursor. The caller can drain acknowledged queue items
and retry the same page with stable identities. A page larger than the queue's
configured item limit remains blocked; it is not silently truncated.

The cursor binds host/parser version, logical source, workspace and conversation
to the queue's existing brain/policy/producer authenticated encryption. It holds
no original transcript bytes or filesystem path. Progress updates use a
compare-and-swap under the existing process and filesystem locks. A stale reader
must reload and verify the prefix again. Restart resumes from the committed
prefix; rewritten or truncated history is refused, never reset automatically.

Partial final lines are not consumed. Their pending status is persisted without
repeatedly counting the same incomplete line as a new omission. Hidden reasoning,
unsupported tools/attachments and other reader gaps retain their fixed reason
counts. Redaction gaps also remain on their queued event records. No gap means
that unavailable bytes were captured.

Queue format 2 adds bounded progress for at most 128 logical sources and 32 gap
codes per source. Format 1 queues remain readable and become format 2 on the next
successful write. Earlier queue binaries reject format 2 instead of dropping
progress. A rollback therefore requires a format-2-aware client. This is a
client-library format; the hosted service does not link this queue implementation.

This primitive does not establish host capability, physical erasure, managed
restore or real-history enrollment. Those gates in the implementation plan remain
open. Losing the encryption key still makes the local queue unrecoverable.
