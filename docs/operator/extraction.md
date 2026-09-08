# Extract sources and review contradictions

Run `serenity --root /path/to/brain extract all` after sources have been stored.
Extraction uses the model pinned in `serenity.yml`; the command may make provider
calls. Sources marked `index_only` are excluded from extraction.

One command prepares ready observations from all sources as a batch. Existing
entity types, shard segments and human prose are preserved. A dirty or malformed
canonical target stops publication instead of replacing the operator's content.
Commit or resolve your edits, then rerun extraction. An exact repeated observation
adds no canonical claim.

## Conflicting facts

A ready observation is compared with current canonical claims, including safe
additions earlier in the same batch. A contradiction or overlapping replacement
is staged as a reconciliation item. The proposal is not made active before review.
The prior claim must still be present and committed when the item is staged.
Candidate selection reads canonical files; a stale search index is not authority
for this decision.

Run `serenity --root /path/to/brain inbox` to review:

- Space accepts the proposal and commits its canonical effect.
- `e` followed by the reviewed replacement and Enter accepts an edited value.
- `r` followed by a note and Enter rejects the proposal.
- `d` defers the item.

See [inbox recovery](inbox.md) if an accepted effect could not finish publishing.
An acceptance is recorded separately from successful canonical publication;
`inbox --unapplied` and `inbox --apply ID` expose and retry unfinished effects.

Repeating extraction of the same source assertion preserves an existing pending
or deferred review. A prior terminal human decision continues to govern that
source, subject, predicate, normalized value and valid-from date, including a
human-edited acceptance. Changes in extraction attempt time, confidence or model
label alone do not erase that decision. New source content is new evidence and
may produce another review. Existing canonical historical identities are not
reactivated merely because the source is extracted again.

## Limits of this verification

This is the local-owner CLI workflow and retains the one-writer-per-brain
convention. Keyed queue insertion is atomic across SQLite handles; this does not
claim general cross-process arbitration of all review actions. Canonical batch
publication preflights dependencies but is not a multi-file filesystem transaction.
If source publication commits and queue staging then fails, the command reports
that boundary and asks for a rerun; unchanged queue inputs are deduplicated.

The verification fixtures use deterministic local HTTP responses and invented
data, not live model-quality measurements. The synthetic 10K timing harness
measures pipeline components and writes its golden observations; it does not
assert that the CLI would automatically accept every potentially conflicting
observation in that corpus.

## Low-confidence observations

Observations below confidence 0.6 are retained as distill inbox items. Their
original source digest, span, pinned model, confidence and text remain unchanged
through review. Repeated extraction keeps one item and its decision. Even a later
model response above the threshold cannot bypass the retained review.

In `serenity inbox`, a low-confidence item displays its assertion and source
provenance. `d` defers and `r` rejects it without changing canonical claims. Space
leaves it pending and explains that a typed human assertion is required.

To publish a fact you have personally verified:

1. Press `e`, type the value you assert, and press Enter.
2. Read the proposed subject, predicate and value. If a canonical prior would be
   superseded, its ID and value are shown before confirmation.
3. Type `y` and Enter to record the decision and commit its canonical effect.
   Any other response cancels and leaves the item pending.

This creates an explicit human assertion at confidence 1, attributed to the
recorded human actor. It does not relabel the original model response as certain.
The new claim links back to the distill item; the queue retains the original
machine provenance. A dirty, malformed or ambiguous target, or multiple conflicting
priors, requires resolution before the decision can be recorded. A later change
to the confirmed prior is preserved rather than overwritten.

Publication uses the same durable receipt and recovery path as reconciliation.
If the commit fails or the process stops, the accepted decision remains visible
in `inbox --unapplied`; retry with `inbox --apply ID` after resolving the reported
problem. Repeating a completed application does not create another claim or commit.
This repair covers extraction-produced distill items, not arbitrary captured text
or other distill producers. HTTP disposition calls still record decisions; the
supported canonical approval path here is the local CLI.

Keep the runtime database and publication receipts when recovering an interrupted
review. Rebuilding the search index preserves inbox rows; deleting `.serenity`
removes runtime review history and is not an inbox-recovery procedure.
