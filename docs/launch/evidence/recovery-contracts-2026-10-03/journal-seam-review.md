Original report SHA-256: `778c6dd857691202ce725f1f298e0b280eab2ed23eb5065cf6823841b51e048a`; paths sanitized and line-end whitespace normalized.

# Serenity startup journal observation seam review

Date: 2026-10-03

## Verdict

**CLEAR** for bounded additive startup journal observation and reserved-writer component specification at exact candidate `6e82e376952aa55b29bfc66cf7451de2f2fb38b6`. This does not approve a startup factory, service/CLI wiring, credentials, provider adapter, source implementation, or hosted readiness.

**CLEAR** for the integrated docs status at exact tip `ce7ba3ad75fcc2a42bc306b75f68223db57e262d` relative to landed docs baseline `db7625cfb6e29e3c7c42531c004d22d72af31c22`. The roadmap correctly reports PR351's exact landed evidence and keeps T24.39 disposition/live evidence open. It labels source workers incomplete, provider authority and physical capacity open, recovery mapping as a source-preflight correction, and startup factory/provider authority as open. The startup proposal at `c97d30089c584d39dd918db209f39fa01d4e92c4` remains bounded design only. The added PR351 verification says docs-only, no Go/test run, billing-blocked Actions and no T24.39 acceptance.

## Seam review

The original `6d561034f5268123790214c5995b17927a420863` candidate was **HOLD**: it required history observation before the first write but did not compare the requested `writerID` against observed historical writer use. A writer ID from G-1 could therefore be reused for G, pass observation before its first G object, and only make the history invalid after writing.

The exact 6e82 amendment closes that finding. Before any PUT, `NewJournalAt` now checks the requested writer against every historical `WriterUse` and refuses cross-generation reuse; the amendment requires a behavioral mutant RED for the pre-write rejection. It also caps observation at 10,000 generations, 100,000 objects, 64 KiB per canonical object, and 64 MiB aggregate canonical bytes; counts seals and entries; refuses over-limit results before decode/projection; checks cancellation between traversal/read/hash/projection steps; rejects non-progress pages; and explicitly leaves transport allocation bounds to each reader adapter.

The exact retry amendment preserves the bound cursor: one pending key/body/predecessor is retained until definitive, retries reuse identical canonical bytes and timestamp, conflicting mutations cannot replace unresolved bytes, and another writer's occupant fences instead of advancing the cursor. Same-instance seal replay returns the original watermark only after exact-byte and no-later-object checks; ambiguous own seal writes retry exact bytes; foreign seals, changed keys, other-generation seals, and post-seal tails fail closed. Initially sealed active positions still require a separately authorized successor. Legacy `NewJournal`/`Seal` behavior is expressly unchanged.

The frozen component remains additive: new reader contracts and deletion-owned journal implementation only, preserving task41 and global packet ownership. Existing legacy `ReadThrough` is unchanged. Position issuance/allocation and historical writer uses still require reconciliation by a later trusted factory; the journal component itself does not authenticate provider identity or prove IAM revocation. Required implementation evidence remains a genuine RED for every behavior guard, focused and integrated verification under the actual build lease, and independent exact-source review before merge.

## Separate source-preflight blocker

The recovery proposal's bounded contract review does not clear full recovery source freeze. `VerifiedLegacyPlan` is described as holding `contracts.RecoveryPlan` and an inner hash, while its constructor relies on `recovery.CreatePlan`/`LoadPlan`; those functions return `recovery.Plan`, a distinct artifact type with `FormatVersion`, string `ProviderObserved`, and `FenceGeneration` fields. `contracts.RecoveryPlan` instead has `ProviderTruthAt time.Time` and `Generation`. The exact validated, lossless mapping and hash preservation between these types must be specified and reviewed before recovery implementation/source freeze. The final roadmap already records this correction as assigned.

## Provenance, privacy, and method

The requested source integration worktree was clean at the reviewed commits. I used an external SSD detached clone at `[external evidence path] pinned respectively to the exact review heads; candidate `6e82e37` is a docs-only amendment after the `ce7ba3a` roadmap update. The tree status was clean and `git diff --check` passed. No source edits, tests, builds, provider actions, push, or merge were performed.

I read the actual repository `AGENTS.md`, `ajent.social`, and `[private local path] No Ajent tools were available and `https://ajent.social` was inaccessible from the web tool, so I could not independently poll that endpoint. The integrated PR351 evidence and local coordination board both disclose this same limitation. A scan of newly added lines found no credentials, customer records, email addresses, home paths, hostnames, or private IPs. One historical evidence line records the generic `[external evidence path] mount and available space; it does not identify a user or host.
