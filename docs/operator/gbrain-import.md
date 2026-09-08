# Import a gbrain checkout

Initialize a separate Serenity brain, then import its gbrain source checkout:

```sh
serenity -C ./my-serenity-brain init
serenity -C ./my-serenity-brain import --from-gbrain ./my-gbrain-checkout
```

The importer reads markdown files with YAML frontmatter directly. Run gbrain's
own fence backfill first if facts exist only in its derived database. There are
no model calls, and neither source files nor `.dira` precepts are modified.

Each facts/takes row becomes one claim. The source reference is
`gbrain:<page-path-without-extension>#<row>`. Row numbers are scoped to their
fence: facts #1 and takes #1 have different content-derived IDs and retain the
fence name in provenance. Claims keep their source text, validity, visibility,
supersession/retraction, and every original cell in provenance metadata.
Confidence/weight is capped at 0.90. All predicate/confidence interpretations
carry `review: true`; importing them does not approve a precept or effect.

Pages preserve aliases, frontmatter (including external IDs), original body,
timeline entries, and typed `links_to` graph edges in their canonical markdown.
The supplementary `serenity:metadata` block carries fields the visible claim
table cannot express. Text, state, and validity in the table remain authoritative
when edited; the source copy is historical provenance. Graph edges are persisted
for consumers; this command does not add a graph traversal API.

The initial importer supports fence-tier mappings (`prefers`, `committed_to`,
`said`, `relates_to`). It fails if the target config moves these families to
shards. Unknown extension columns are retained verbatim in provenance, not
interpreted. Malformed tables, invalid scores/visibility, missing or cyclic
supersession targets, symlinks, and colliding target slugs fail explicitly.

A source `people/ava.md` maps to `brain/entities/<frontmatter-type>/ava.md`.
Two source pages with the same basename are rejected, since Serenity's entity
identity is currently slug-global. Source and destination directories must be
separate. An existing destination page is accepted only if its bytes exactly
match the import; different content, including untracked human work, is never
overwritten. Repeat imports of unchanged pages create no duplicate claims.
The importer publishes and git-commits one complete page at a time, then the CLI
rebuilds its disposable SQLite index.

## Resume an interrupted import

Rerun the same command against the same source snapshot and target. The importer
records each committed page and its fence-qualified row IDs in
`.serenity/import/gbrain.json`. The checkpoint is local runtime state and is
never committed with canonical pages. It advances only after the page commit;
a killed process can leave an uncommitted page or an unpublished checkpoint temp
file, both recovered on the next run. Completed pages create no extra commits.

Before skipping a page, Serenity verifies its source hash, mapping, canonical
bytes, and committed Git bytes. Copied or stale checkpoints cannot skip
uncommitted work. Changed source snapshots, corrupted checkpoint files, and
human edits or deletions fail explicitly. Preserve the original snapshot to
resume; use a fresh target for a changed migration. This is interruption recovery,
not replacement of previously imported pages with newer versions.

A kernel file lock rejects concurrent imports into the same target and releases
automatically when a process exits or is killed. It coordinates importer processes;
stop other writers while migrating. Checkpoint directories, files, and lock files
must not be symlinks. The supported macOS/Linux builds include this locking path.
The checkpoint is disposable: if removed while no import is running, an unchanged
source can rebuild it from matching canonical pages and their Git commits.

## Field-level report

```sh
serenity -C ./my-serenity-brain import --from-gbrain ./my-gbrain-checkout --json
```

JSON output includes page/claim counts and `audit.rows`, `audit.fields`, and
`audit.unmapped_fields`. The synthetic fixture reports 8 rows, 74 source cells,
and an empty unmapped-field list. The importer audits claims parsed back from
the rendered canonical page before publishing any pages. A missing/different
source cell, source reference, claim text, validity window, visibility, take
actor, or review flag aborts the import; JSON mode returns the field diagnostics
alongside the nonzero exit. Diagnostics contain source values and should be
kept local when importing private material.

`TestGBrainFieldRoundTrip` separately checks all fixture rows against hand-written
expectations, including lifecycle, superseded pointers, confidence and who/weight.
Extended typed-fact and resolved-take columns are retained verbatim and covered
by a second fixture test. Predicate inference and calibration are still semantic
interpretations requiring review; an empty report does not certify their truth.
