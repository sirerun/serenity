# Inspector API and brand preliminary source review

Status: **BLOCKED; source review only. No formal CLEAR.**

Reviewed base `e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f` and code candidate `0515d8d0` (`Add owner-scoped read-only inspector API`). The later coordinator head `b5bcb98cd74963054f8f65544af23361f30308b2` changes only plan/checkpoint/preflight documentation relative to that code candidate; it was not included in the code review. Read-only review of the integrated API and brand candidate; no files outside this receipt were changed.

## Findings on original code candidate `0515d8d0` (superseded by follow-up dispositions)

### F01 — Claim visibility can be downgraded while merging page and shard copies

`internal/hosted/dashboard/inspector.go:466-470` merges page claims into shard claims. `mergeInspectorClaim` at `:699-705` replaces the existing visibility whenever the page copy is anything other than `private`, including empty. `inspectorClaimEligible` at `:745-755` treats empty visibility as eligible. A stale/default-empty or shared entity-page copy can therefore turn a private shard claim into an eligible response. No test covers conflicting visibility values between the two representations. Confirm the canonical visibility authority and fail closed on conflicting/private copies; add a regression fixture.

### F02 — A stale page claim can undo a retraction or other newer lifecycle state

Shard lines are folded at `internal/hosted/dashboard/inspector.go:668-696`, then the entity-page copy is merged afterward at `:466-470`. `mergeInspectorClaim` at `:706-707` lets any nonempty page state overwrite the folded shard state. A stale `active` page row can undo a `retracted` shard line; the resulting active row passes `inspectorClaimEligible` at `:745-755`. `internal/store/shard.go:401-415` defines retracted IDs as dead in `ResolveHeadLines`. Resolve the established shard/page authority and prevent stale page rows from reviving retracted claims; add conflict tests for retracted, superseded, and active states.

### F03 — Claim validity dates fail open for supported formats and malformed values

`internal/hosted/dashboard/inspector.go:749-753` only rejects validity bounds when `claimDate` returns a date. The parser at `:776-787` accepts RFC3339 and `YYYY-MM-DD`, then returns nil otherwise. `internal/domain/claim_time.go:8-27` also accepts `YYYY-MM` and `YYYY`, and rejects malformed nonempty bounds. Thus future `ValidFrom` values using year/month forms, expired `ValidTo` values in those forms, and malformed nonempty dates are treated as undated by the inspector and remain eligible. Use the domain validity predicate or equivalent fail-closed parsing; cover supported formats and invalid strings.

### F04 — Unlinked entity-page labels are returned without an eligible record or claim

`readInspectorEntities` loads entity pages at `internal/hosted/dashboard/inspector.go:623-657`. Later, `:574-578` adds every entity page not already marked restricted, and `:580-590` emits it as a graph node even if no eligible world fact or claim links to it. This may expose an orphan/stale label left after an associated record is erased or unlinked, and does not match the contract for eligible entities. Emit only entities reached from eligible facts/claims (or otherwise establish a documented, tested rule for standalone entities). This is a same-account privacy/eligibility concern; no cross-account exposure was found.

## Checks and bounded observations

- **Build, tests, race, vet, and formal acceptance are unverified.** No Go checks were run; the coordinator reported the shared build lease is held by another lane. This review does not claim compilation or test success.
- Source inspection confirmed session lookup requires an active account (`internal/hosted/identity/identity.go:289-303`). Brain ownership, ready state, ID/path-key equality, and canonical root checks are in `internal/hosted/gateway/inspector_read.go:53-102`.
- A cold-open race was investigated and not substantiated: the only normal `Pool.Acquire` path found is `Gateway.bootstrapTools`, under the same maintenance/account locks (`internal/hosted/gateway/gateway.go:219-244`). Startup reconciliation that may open cold brains finishes before handler assignment and worker startup (`internal/hosted/service/service.go:402-433`); periodic reconciliation is existing-runtime-only. Warm reads pin the runtime and take its mutation lock.
- The suspected zero `CreatedAt` date is not a finding: canonical memory-fact decoding rejects zero timestamps (`internal/store/memoryfact.go:218-220`).
- Read bounds are explicit (four global inspector slots, node/file/byte limits); pagination still reconstructs and filters the full bounded graph for each page (`inspectorGraph`, `inspector.go:190-220`). That is bounded but may repeat filesystem work for large graphs; representative load/performance checks remain unverified.
- Brand source copy clearly labels the original PNG and raster-in-SVG files; the assets use embedded `data:` payloads without external resource references. The original PNG is 297,499 bytes. At the original `0515d8d0` candidate, each SVG was about 794 KB because the base64 image was duplicated into `href` and `xlink:href`; this duplication is removed at `6fabfe5a`, where each SVG is about 397 KB. The recurring navigation logo remains a large asset, and no browser performance acceptance was run.
- The author-reported Pages Python unit test and link check passed. Paired strict MkDocs baseline and candidate runs both failed with the same seven existing missing-link warnings. These are not new candidate failures. The author retains paired logs outside this public repository; this reviewer did not rerun the checks.
- A separate independent release preflight record exists; current production qualification remains separate from this source review.

## Follow-up review at integrated head `6fabfe5a19d826ee84bab2dcf17cad16f4513893`

I re-reviewed the integrated candidate against the same base `e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f`, including API, context, brand assets, documentation, and tests. The earlier F01–F04 source findings are resolved at this head: private visibility and retraction are sticky across either merge order; claim-date eligibility uses `Claim.CurrentAt`; and standalone entity pages are emitted only when an eligible fact or claim seeds an entity. The new unit tests cover date precision/malformed dates, both retraction merge directions, context ordering/cap/induced edges, and empty-page context. The HTTP tests cover authenticated owner routes, cross-owner and guessed-source denial, cursor paging/filter binding, no-store headers, filesystem nonmutation, and absence of embedding/model calls.

### F05 — Unused import blocks Go compilation

`internal/hosted/dashboard/inspector_http_test.go` imports `github.com/sirerun/serenity/internal/embed`, but the package name is never referenced. The test's `f.embed` occurrences refer to a fixture field, not that imported package. This is a compile-time failure and must be removed or used before the candidate can pass Go checks. I did not run Go commands because the shared build lease remains held by another lane.

### Remaining qualification notes

- This remains a source-only review, not formal CLEAR: Go compilation/tests, race, vet, lint, and integrated frontend checks are unverified. The test fixture has real HTTP ownership checks, but no end-to-end fixtures for private facts/claims, expiry, deletion/tombstones, and supersession; helper tests cover only portions of those policies.
- Paging and context are deterministic and bounded in source: the graph is rebuilt from the eligible projection for each page; primary counts/cursors exclude context, context is capped at 100, and context edges are induced and ID-sorted. This source review does not establish representative latency or memory behavior.
- Per-request configured raw-input ceilings can sum to roughly 224 MiB (source bodies 64 MiB, source metadata 32 MiB, claims tree 64 MiB, entity tree 64 MiB). Four global read slots permit about 896 MiB of raw-cap equivalents concurrently before decoded object/map overhead. These are hard source bounds, not observed heap use; on the 2 GiB host, peak memory under maximum-sized data and four concurrent reads is unqualified. Require a representative high-water memory/performance measurement or tighter aggregate limits/admission before production.
- The duplicate SVG payload is fixed: at `6fabfe5a`, the PNG is 297,499 bytes and each SVG is about 397 KB, with one embedded `href` image and no external resources. This is roughly half the prior SVG payload, though the recurring navigation logo remains a large asset; no browser performance acceptance was run.
- Brand copy remains explicit that the original is PNG artwork and the SVG variants contain raster artwork; no old S-mark copy was found in the reviewed site text. The author-reported generator preservation test and link check passed; strict MkDocs baseline and candidate produced the same seven preexisting warnings.

No formal CLEAR is issued. F05 must be corrected, then the exact new head must compile and pass the required checks and remaining integrated acceptance.

## Final source-only disposition at `d53fecf828b194b35f7d46d9146ccfcc11aba5f4`

Compared the complete candidate to base `e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f`. This includes the API, gateway/pool read path, context projection, brand/site changes, generator, and added tests. `git diff --check` passed. Source review did not run Go or site builds.

F01–F04 are resolved in the final source: claim visibility remains private if either merged representation is private; retraction remains sticky across both merge orders and shard revisions; eligibility delegates date-window checks to `Claim.CurrentAt`, which recognizes year/month/day/RFC3339 formats and rejects invalid bounds; and entity labels are emitted only for entities seeded by eligible facts or claims. F05 is resolved: the unused `internal/embed` test import is absent from `inspector_http_test.go`.

The new canonical projection fixture covers public and expired/private facts, a private claim, missing source provenance, claim retraction, source tombstoning, a true old/new supersession pair, and an orphan entity page. It asserts excluded nodes stay out, eligible labels and supersession remain, and JSON contains none of the raw source body, URI, metadata marker, or tombstoned-source bytes/URI. HTTP fixtures now exercise private, expired, and forgotten facts through graph responses and guessed detail IDs. These source-level assertions address the earlier fixture gap; execution remains unverified.

No additional confirmed source blocker was found in this pass. This is **BLOCKED / source review only, not formal CLEAR**: Go compile, focused and package tests, race, vet, lint, and integrated frontend checks have not run. Existing production capacity qualification also remains open: configured raw-input caps total about 224 MiB per read, and four read slots allow about 896 MiB of raw-cap equivalents before decoded-object overhead. Treat representative peak-heap/latency testing (including concurrency) or tighter aggregate admission as a required production performance gate, not as a demonstrated OOM defect. Browser asset performance remains unmeasured; SVG image payloads are embedded once and the duplicate-reference issue is fixed.
