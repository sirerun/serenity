# Consolidating agent memory

Run `serenity -C <brain> cron consolidate` after ingest or on a nightly
schedule. The command regenerates entity summaries from active canonical
claims, refreshes shard heads, commits its queued page changes, refreshes
the search index, and embeds changed or missing chunks under the configured
model pin. A successful job updates `.serenity/cron/consolidate.json`.

Summaries are deterministic bullet lists. Their freshness banner names the
latest dated active evidence; after 30 days it states that nothing new has
arrived since that date. Evidence without a date is explicitly undated.
Banners contain no run timestamp, so repeated runs with unchanged evidence
and the same freshness category leave canonical bytes unchanged.

Committed edits inside the DERIVED summary are replaced. Committed
fence-tier claims remain authoritative. Prose, frontmatter and other content
outside the summary, claims and claims-detail fences are preserved. An
uncommitted target-page edit pauses its machine write through the existing
writer guard and saves both versions in `.serenity/pending/`; commit or
resolve the human edit before retrying. Consolidation does not bypass this
guard to overwrite an uncommitted summary.

Shard JSONL remains canonical and is never rewritten by consolidation.
Old, unmodified head rows can advance to their current resolved heads.
A committed head row that differs from its original shard claim stops the
pass with an explicit disposition-required error before any pages are
written. This preserves the human edit until the shard-head disposition
workflow can append its intent to the shard; consolidation itself does not
accept or silently discard that edit. Entities present only in shards get
an entity page in the ingestion default entity bucket.

The search projection is rebuilt from canonical files. Existing vectors are
retained only for identical chunk references and exact text, separately for
each model pin. Edited and deleted chunks lose old vectors; unchanged chunks
incur no embedding calls. Runtime queues and disposition history survive the
refresh. An interrupted index refresh can be repaired by rerunning it;
canonical files remain the source of truth.

With `models.embedding: none@v0` (the initialization default), consolidation
explicitly operates with full-text search only. A pinned model requires a
working configured embeddings provider; missing credentials or provider
errors fail the job and do not advance its success record. Canonical page
commits may already have completed before an embedding failure. Retry fills
missing vectors while preserving successful ones.

The same job function accepts an injected clock for scheduled execution and
testing. Callers that embed the consolidation package must supply their
shared writer queue, configuration and index handle. Daily briefing rendering
is a separate capability.
