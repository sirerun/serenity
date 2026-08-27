# ADR 005: Eval labeling protocol; voice notes move to M2

## Status
Accepted (labeling roles confirmed by David, 2026-08-27)

## Date
2026-08-27

## Context
RFC 0001 section 16 mandates held-out golden sets "labeled by a second labeler
with adjudication" but names no tooling or people. The project has one human
maintainer. Section 10.1 lists the voice-note connector as v1 P0, while the
M1 acceptance criteria in section 17 name only the watcher, IMAP, and repo
crawler; transcription also depends on the router, which lands in M1.

## Decision
- Label files are plain YAML under `evals/corpora/<corpus>/labels/`, one
  record per span: `span`, `expected` (predicate, object, valid window),
  `labeler`, `adjudicated`. Two labelers are two independent frontier-model
  passes from different model families, run blind to each other; every
  disagreement is adjudicated by the maintainer and marked
  `adjudicated: true`. Labels are checksum-pinned; CI fails when a label file
  changes without its manifest entry. No labeling TUI in v1.
- The eval harness never reads the extractor's confidence to decide labels.
  Golden sets used to gate a milestone are frozen before that milestone's
  extractor work starts (the RFC's "never tuned-against in the same
  milestone" rule), enforced by the checksum manifest date.
- The voice-note connector is an M2 task (T2.16), not an M1 gate. The RFC's
  section 10.1 P0 list and section 17 M1 AC are reconciled in favor of the
  AC; an errata line is added to the RFC when M2 plans land.

## Consequences
- Labeling cost is model spend plus adjudication time, not a second human.
- Model-labeled golden sets carry model bias; the second family and human
  adjudication are the mitigation, and the report states the labeling
  provenance.
- M1's connector fixture set is three corpora, not four.
