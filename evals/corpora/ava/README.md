# Ava Standardo extraction corpus (RFC 0001 §16; plan T1.14)

The "Ava Standardo" synthetic-persona extraction corpus: labeled (span,
predicate, object) golden data across every predicate family in
`internal/config`'s seeded controlled vocabulary (T0.8), a held-out split,
and embedded contradiction pairs. This is the corpus RFC §16 refers to as
covering "per-connector extraction P/R" -- `internal/eval/direction/`
(T3.13) and `internal/gate/adversarial_corpus_test.go` (T1.20) are its
siblings, covering DIRECTION plan-checking and adversarial ingest
respectively.

`internal/eval/ava_corpus_test.go` is this corpus's consumer of record.

## Layout

- `labels/*.yaml` -- one golden span per file, in `internal/eval.Label`'s
  exact format (ADR-005, unmodified): `span`, `expected: {predicate,
  object, valid_from, valid_to}`, `labeler`, `adjudicated`. Loadable
  directly via `eval.LoadLabels("evals/corpora/ava/labels")` with no
  bespoke wrapper type -- unlike T1.20/T3.13, this corpus needed no new
  `Row`/`Doc` schema because its rows are exactly T1.13's golden-label
  shape.
- `labels/*.yaml` additionally carry two forward-compatible extra fields
  ignored by `eval.LoadLabels` (see `label.go`'s doc comment on
  unrecognized-field tolerance): `contradiction_pair_id` and
  `contradiction_role` (`a` or `b`), embedding which of the 13
  contradiction pairs a span belongs to and which side it argues, directly
  in the label file.
- `checksums.yaml` -- a sha256 manifest over every `*.yaml` file in
  `labels/`, using `internal/eval`'s existing `WriteManifest`/
  `VerifyManifest` (ADR-005) unmodified -- the exact same functions
  T1.13/T1.20/T3.13 all use. **Deviation from T1.20/T3.13's layout:** this
  manifest lives at `evals/corpora/ava/checksums.yaml`, one directory
  *above* `labels/`, not inside it. T1.20 and T3.13 each wrote their own
  bespoke loader (`direction.LoadRows`, `gate.loadAdversarialCorpus`) that
  explicitly excludes a file named `checksums.yaml` from its directory
  scan; `eval.LoadLabels` -- the shared, generic loader this corpus uses
  instead of writing a bespoke one -- has no such exclusion, since no
  corpus had called it against a real, manifest-colocated directory before
  this one. Rather than modify T1.13's shared, already-merged code, the
  manifest is placed one level up, a configuration `WriteManifest`/
  `VerifyManifest` already fully support unmodified via their own
  "does the manifest live inside labelsDir" check
  (`checksum.go`'s `computeManifest`). Regenerate after a deliberate edit
  with `go run evals/corpora/ava/gen_corpus.go` (see below) -- do not call
  `eval.WriteManifest` by hand against a different path.
- `split.yaml` -- the held-out split, in `internal/eval.Split`'s exact
  format (`held_out: [<span text>, ...]`), loadable via
  `eval.LoadSplit`/`Split.Filter` unmodified. 52 of the corpus's 312 spans
  (4 per family) are held out.
- `contradictions.yaml` -- a human-readable index of the 13 contradiction
  pairs (`id`, `family`, `span_a`, `span_b`, `why`), generated from the
  same source data as the label files so the two never drift apart.
- `gen_corpus.go` (`//go:build ignore`) -- the single regeneration
  entrypoint for all four outputs above.

## Why this corpus is generated, not hand-authored per file

T1.20's adversarial corpus and T3.13's DIRECTION corpus are both
hand-authored one YAML file at a time -- appropriate for their scale (16
and 64 rows). This corpus's acceptance floor is >= 20 labeled spans PER
PREDICATE FAMILY across 13 families (260+ spans minimum); hand-authoring
300+ independent files would mean 300+ opportunities for the corpus to
drift from itself (inconsistent entities, dates, or slugs across files)
with no single point of review.

Instead, `gen_corpus.go` embeds one hand-authored, reviewable dataset: a
coherent Ava Standardo timeline (employer and role history, financial and
software accounts, health conditions and medications, preferences,
commitments, deadlines, relationships, project membership, quotes, and
costs), rendered through several sentence templates per predicate family
to simulate how the same underlying fact would actually appear across an
email, a chat message, a formal HR/finance record, and a bio -- exactly
the phrasing diversity a real per-connector extraction P/R eval needs.
Every sentence is real, family-appropriate English; there is no
`"{subject} {predicate} {object}"` placeholder text anywhere in the
output. Regenerating after an edit to the dataset is a single command, and
the manifest, split, and contradiction index are always regenerated
together from the same source, so they cannot silently disagree.

## The persona: a February 2026 upheaval

24 spans per family = 20 regular (5 facts x 4 phrasings) + 4 contradiction
spans (2 restatements of "claim A", 2 of "claim B"). The regular facts
trace Ava's career from Contoso Systems (2019) through Beta LLC, her
accounts, an ongoing set of health conditions and medications, stated
preferences, real commitments and deadlines, her core relationships, her
project history, things she has said in meetings, and recurring costs.

Several of the 13 contradiction pairs are deliberately clustered around
the same month (February/March 2026) as a single plausible incident: a
job change and a promotion rumor land at once, so her employer (`works_at`
-- Acme Corp vs. Beta LLC), title (`has_role` -- Staff Engineer vs.
Engineering Manager), manager (`relates_to` -- Priya Ram vs. Marcus Webb),
and project assignment (`belongs_to_project` -- the Beacon redesign vs.
Project Anchorpoint) are all simultaneously disputed across sources. The
remaining pairs are independent: a bank balance disagreement
(`has_balance`), an account status dispute (`owns_account`), a
differential-diagnosis conflict (`has_condition`), a medication
continuation dispute (`takes_medication`), an editor-preference conflict
(`prefers`), a migration-cutover commitment conflict (`committed_to`), a
deadline-moved-verbally conflict (`deadline_on`), a same-topic
contradictory quote (`said`), and an invoice-vs-ledger cost disagreement
(`costs`).

## What `internal/eval/ava_corpus_test.go` proves

1. **Checksum manifest verifies** (`TestAvaCorpusManifestVerifies`) -- a
   label file edited without regenerating the corpus fails CI, naming the
   file (verified with a real tamper-and-restore pass during development).
2. **Every seeded predicate family has >= 20 labeled spans, and no span
   uses a predicate outside the vocabulary**
   (`TestAvaCorpusCoversSeededVocabularyWithFloor`) -- reads the family
   list from `internal/config.Default()` directly rather than a
   hand-copied duplicate, so the two can never drift.
3. **No duplicate span text** (`TestAvaCorpusNoDuplicateSpans`) -- the
   split file's span-keyed lookup requires span text to be unique.
4. **Every span carries real content** (`TestAvaCorpusContentPopulated`).
5. **The held-out split is well-formed**
   (`TestAvaCorpusSplitFileValid`) -- every `held_out` entry resolves to a
   real label (`Split.Filter` would otherwise silently ignore a typo'd
   entry instead of erroring), and every family has at least one held-out
   span.
6. **>= 10 contradiction pairs exist**
   (`TestAvaCorpusContradictionPairsMeetFloor`) -- this corpus embeds 13,
   one per predicate family.
7. **Every contradiction pair is a genuine conflict, not just a declared
   one** (`TestAvaCorpusContradictionPairsAreGenuineConflicts`) --
   independently re-derives, from the label files themselves, that each
   pair's two spans share the same predicate, assert *different* objects,
   and have *overlapping* validity windows. A pair that merely claims to
   conflict without its underlying labels actually conflicting (matching
   predicate/object, disjoint time windows) fails here -- verified during
   development by flipping one span's object to agree with its pair
   partner and confirming the test catches it.
8. **The inline per-file contradiction tags agree with the human-readable
   index** (`TestAvaCorpusContradictionTagsMatchIndex`) -- exactly two
   spans tagged role `a` and two tagged role `b` per pair id, cross-checked
   against `contradictions.yaml`.

## Scoping note

Like T1.20's adversarial corpus, there is no complete, wired
extraction-to-claim pipeline yet (T1.8/T1.9 are concurrent siblings, not
dependencies of this task). This corpus is golden *label* data in
`internal/eval`'s native format; it does not itself run a real extractor.
`internal/eval.Score` and `internal/eval.ContradictionRecall` (T1.13) are
the scoring functions a future eval workflow will call against this
corpus's labels and a real extractor's `Prediction`/detected-pairs output.
