# E16 — Reviewed human edit publication

A real inbox approval recorded an accepted dirty edit while leaving the canonical
shard unchanged. The former application helper also committed the current page
without verifying that it still matched the captured human copy.

- [x] T16.1 Publish explicitly reviewed human edits with crash recovery  Owner: pool  Est: 120m  verifies: [UC-012, UC-026]  deps: [T15.1]  acc: [interactive acceptance previews the captured human content and requires confirmation; changed shard-head objects become human assertions and survive rebuild; prose and unrelated staged files remain intact; newer, ambiguous or unsafe inputs stop before disposal/publication; accepted effects remain visible and retryable across commit failure and actual pre/post-commit SIGKILL without duplicate history or claims; executed suite counts, skips and fault controls are recorded]

Scope: exact reviewed entity pages, fence-tier truth, and object corrections to
existing shard heads. Direct edits to shard JSONL, added/removed shard rows, and
changes to shard metadata require explicit resolution and are not guessed at.

Verified 2026-09-08. The real CLI now previews the captured human page and
requires confirmation before recording acceptance. Publication uses exact-byte
plans and a durable receipt; changed shard objects become human assertions while
prose and unrelated staged files survive. Stale, ambiguous and unsafe inputs stop
before review or application. Both the CLI and legacy internal helper use the
same recovery protocol. Completed effects receive an optional publication marker.

The full race suite passed 1,690 cases across 58 packages with six explicit skips.
Vet passed; final lint reported zero issues after a test-only switch-style fix.
The affected nine-case regression was rerun after that style correction. Five
injected faults failed as intended and were restored byte-for-byte. Real CLI
SIGKILL before and after Git commit recovered one decision and one human shard
correction, preserving prose; the correction remained searchable after wiping
and rebuilding the derived index with `serenity sync`. No live model call was made.

Evidence: `docs/evals/dirty-edit-publication.json`.
