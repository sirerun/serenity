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
rebuilds its disposable SQLite index. Checkpointed interruption/resume support
is tracked separately by T5.3; this initial command makes no full incremental
update or checkpoint guarantee.
