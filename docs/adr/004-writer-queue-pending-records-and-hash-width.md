# ADR 004: One writer queue, file-backed pending records, 8-hex claim ids with a collision tripwire

## Status
Accepted

## Date
2026-08-27

## Context
RFC 0001 section 7.7 requires all machine writes to be serialized through one
writer queue with per-file ordering, a dirty-tree check that pauses writes to
a human-edited file and raises a disposition item, and daemon commits with a
`serenity:` prefix. The DISPOSITION queue that would hold that item does not
exist until M2. Section 7.2 sizes claim ids "for the per-entity claim
population" and makes a collision a hard error, without fixing the width; the
tree at 13dc0d2 uses 8 hex characters.

## Decision
- `internal/writer` is the only path that touches canonical files after M0:
  a single drain goroutine, per-path sequence numbers, and a property test
  that proves no interleaving. `FenceWriter` and `ShardStore` stay as pure
  render/parse/append primitives; the file-first CI gate (T0.2) allowlists
  only `internal/writer` and `internal/index/rebuild.go` as engine writers.
- Until M2, a paused write is recorded as `.serenity/pending/<slug>.json`
  holding both sides (human file bytes, machine render). The directory is
  runtime state (derived, never canonical). M2's disposition store imports
  these records as `dirty_edit` items and deletes the files.
- Claim ids stay 8 hex characters (32 bits) with `DerivedID` taking an
  explicit width parameter. The writer compares the full
  (subject, predicate, object key, valid_from, source ref) tuple on an id
  match and returns `ErrIDCollision`; the migration path is to raise the width
  in `serenity.yml` and re-render, never to overwrite.

## Consequences
- Machine-vs-machine contention is impossible by construction, as the RFC
  wants; the cost is that every later milestone must write through the queue
  (enforced by the gate, not by review).
- Pending records survive a crash and are inspectable as JSON; they are not
  in git, which is correct for runtime state.
- A 32-bit id space is ample per entity (thousands of claims); the tripwire
  turns the theoretical collision into a loud migration instead of a silent
  overwrite.
