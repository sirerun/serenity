# Serenity spatial inspector, Book Binder brand and public release

Status: source contracts frozen; brand and API implementation integrated at0515d8d0. Verification, independent review, merge and release gates remain open.
Contract: ordinary repository delivery, not an enrolled lifecycle or a second service scheduler.
Owner: root coordinator; brand_ship and api_ship author lanes, release_preflight independent rollout/review lane.

## Outcome and authorization

Ship an authenticated, read-only spatial view of real Serenity memories, and update https://serenity.sire.run, its logo and brand to the selected rose Book Binder identity. User explicitly requests production release. Execution belongs to /ship; this plan does not deploy. Preserve existing recovery/startup authority holds and unrelated source work. Release readiness for this change is separate from broad hosted-launch acceptance.

Public website and public demo contain no private memories. Proposed routes: /explore/ for a clearly labeled synthetic demo and /dashboard/explore for the authenticated inspector; preflight may adjust routes to the actual serving topology without changing privacy boundaries. First release navigates by added/captured year, not historical knowledge reconstruction. Unknown dates stay explicit; entity dates derive from eligible linked records. No write, deletion, ingestion, billing or recovery changes are introduced by the inspector.

## Evidence and discovery

- Planning baseline e2b5dd17. Prototype commit bed37f58 on design/inspector-graph-20261003 contains the accepted identity, Three.js graph, tests and prior independent CLEAR. This is prototype evidence only; it has39 synthetic records and is not on main.
- site/ holds the public pages and assets; site/brand/index.html still describes the old open-ring S mark and older light palette. Rebrand every existing surface, rather than replace its product content with prototype copy.
- .github/workflows/pages.yml deploys site/ through GitHub Pages. site/embed.go also embeds the website in the hosted binary and defines CSP. internal/hosted/dashboard/dashboard.go serves website.Handler alongside session-based dashboard routes. The authoritative live origin must therefore be confirmed before packaging/deployment.
- Existing dashboard has /dashboard/memories and POST /memories, but no inspected graph endpoint. Reuse its session, account and brain ownership checks; do not put MCP credentials in browser storage.
- internal/server/memory/recall.go is an existing retrieval surface, not a paginated complete graph. internal/store/source.go exposes source metadata/bytes; adapters must preserve policy and tenancy and avoid raw byte/URI leakage. Inventory facts, claims, entity identity and validity fields before freezing DTOs.
- Graph detect-changes reports9 changed files,0 changed functions and0 affected flows at baseline; graph MCP architecture/flow tools are not exposed, so source/route scans were used. No graph refresh or source mutation performed.
- Live website web-fetch failed in this invocation; older palette/browser evidence is a design reference, not current hosting proof. Preflight must obtain fresh browser/provider evidence.
- docs/plan.md, docs/roadmap.md, ADR014–025 and current coordination feed are preserved. docs/design.md is absent; established architecture lives in RFC/ADRs and docs/design/serenity/. No new ADR is ratified by planning.

## Use cases

| ID | Outcome | Interface | Priority |
|---|---|---|---|
| UC-INS-01 | Owner sees their authorized brain as a graph and accessible list | authenticated inspector + read API | P0 |
| UC-INS-02 | Owner searches/filters by type, scope and added/captured year | graph/list/year rail | P0 |
| UC-INS-03 | Owner inspects provenance and available supersession links | detail/source API | P0 |
| UC-INS-04 | Visitor recognizes Book Binder and explores an explicitly synthetic demo | public site/demo/brand downloads | P0 |
| UC-INS-05 | Operator releases and rolls back a verified, matching site/API revision | existing deployment/runbook | P0 |

## Capabilities and execution rules

Profile helper selected baseline, delivery, design-browser and go; requirements_met=true, no installs activated. Actual bindings: git and gh CLI for repository delivery, Go/Node and existing test suites for validation, CUA for manual Chrome QA, AWS CLI for existing AWS deployment. Kazi CLI is available: acc lines below are acceptance contracts, not predicates executed during planning. Stage tasks stay coordinator tasks; draft predicates just in time. Cloudflare MCP and Ajent tools are unavailable in this environment: Cloudflare changes are blocked until configured tools are available; do not substitute unauthorized CLI/browser mutations. Deployment that requires no Cloudflare change can use existing authorized AWS/GitHub tooling after topology verification. A helper's generic composio requirement is not relevant to this repository and is not activated.

All future task/worker worktrees, caches and generated artifacts use mounted writable external SSD. Parallel GPT-6-Luna workers may own brand/site and read-API files after a shared contract freeze; coordinator owns integration and gates. Claim files/scopes before source changes; heavy builds obey load<=10 and the shared build lease. Every coding PR has independent exact-base/head review, ordinary reviewed merge and landed verification. ADR024 local-validation fallback requires actual evidence if Actions remains billing-blocked; it does not itself make Pages deployment runnable.

## Reachable wave: contracts, brand and read API

- [x] INS-00 Confirm live topology, authority and source contracts  kind: agent stage: preflight  verifies: [UC-INS-01, UC-INS-04, UC-INS-05]  blocked-by: []  acc: [record current main/prototype pins, trusted holds, origin serving the domain, runtime route ownership, session/brain authorization, DTO date/scope/validity semantics, expected datasets and paging limits; qualify tool access, rollout/rollback paths and Actions status; assign disjoint file ownership]

### Brand/site candidate

- [x] INS-B1 Implement Book Binder brand across existing website  kind: agent stage: implement lane: agent  verifies: [UC-INS-04]  blocked-by: [INS-00]  acc: [selected silhouette and rose/charcoal tokens appear on home/product/pricing/get-started/chat/docs/brand and login/dashboard surfaces as applicable; favicon, wordmark, downloadable light/mono assets, metadata/social image and theme colors match; existing copy, accessibility and links preserved; all old S descriptions replaced; asset quality at small sizes checked]
- [ ] INS-B2 Verify brand/site candidate  kind: agent stage: verify  verifies: [UC-INS-04]  blocked-by: [INS-B1]  acc: [scripts/site/check.py, strict docs build, existing site tests and changed embed/CSP tests pass; browser tests cover primary CTA plus a narrow-screen edge case; desktop/phone visual QA covers all changed templates, theme contrast, favicon and downloads; lint/format checks pass at candidate SHA]
- [ ] INS-B3 Independent code and visual review of INS-B1  kind: agent stage: review  blocked-by: [INS-B2]  acc: [named independent reviewer records base/head SHA and all site/asset/CSP changes; blocking findings fixed, affected checks repeated and re-review CLEAR]
- [ ] INS-B4 Merge exact reviewed brand/site candidate  kind: agent stage: merge  blocked-by: [INS-B3]  acc: [trusted holds checked; reviewed head merged normally by authorized coordinator; landed SHA and tree proof recorded; automatic site deployment behavior accounted for]
- [ ] INS-B5 Verify landed brand/site revision  kind: agent stage: verify-landed  blocked-by: [INS-B4]  acc: [landed site/assets and relevant tests match reviewed tree; any automatic production update checked and reported honestly; no live proof inferred from a merge]

### Authenticated read-API candidate

- [x] INS-A1 Implement bounded owner-only graph and detail reads  kind: agent stage: implement lane: agent  verifies: [UC-INS-01, UC-INS-02, UC-INS-03]  blocked-by: [INS-00]  acc: [session-authenticated per-brain GET graph/detail endpoints expose stable IDs, eligible facts/claims/entities, relationships and permitted source metadata; pagination has explicit cursor/limits/truncation; scope/year/search counts have defined dataset semantics; absent dates explicit; deleted/expired records and source disclosure follow current policy; no brain mutations, credentials, private local paths or raw source bytes leak; responses no-store]
- [ ] INS-A2 Verify read API and privacy boundaries  kind: agent stage: verify  verifies: [UC-INS-01, UC-INS-02, UC-INS-03]  blocked-by: [INS-A1]  acc: [real HTTP request tests assert status/body for authorized brain, signed-out401, other-account denial, guessed source IDs, invalid cursor and bounded paging; fixtures cover missing sources, unknown dates, supersession/expiry and deletion; retrieval creates no brain writes/model calls; focused race tests, format/vet/lint and applicable required suite pass at candidate SHA]
- [ ] INS-A3 Independent review of INS-A1 read/auth contract  kind: agent stage: review  blocked-by: [INS-A2]  acc: [independent reviewer records base/head SHA, traces route/session/brain/source ownership and pagination; accepted blocking findings resolved with affected verification and re-review]
- [ ] INS-A4 Merge exact reviewed read-API candidate  kind: agent stage: merge  blocked-by: [INS-A3]  acc: [trusted holds checked; authorized normal merge of exact reviewed head; landed SHA/tree recorded]
- [ ] INS-A5 Verify landed read API  kind: agent stage: verify-landed  blocked-by: [INS-A4]  acc: [landed HTTP/auth/source regressions pass and contract version recorded; no production API readiness inferred]

## Later wave outlines

### Inspector integration

Outcome: real-memory graph/list with time navigation, detail provenance and responsive, accessible operation; public synthetic demo distinctly labeled. Integrate selected prototype modules without silently cherry-picking unrelated historical docs. Add build output to actual site/embed asset pipeline; satisfy existing CSP without broad unsafe script allowances. Bounded graph rendering, progressive loading, loading/error/empty states, no-WebGL/list fallback, reduced motion and privacy-safe URL/history state. Performance budgets must be measured on representative10K-record fixtures, not render every record unbounded; preserve global vs loaded counts explicitly. Browser acceptance includes sign-in/brain switching, filtering/year reset, source inspection, back navigation, session expiry, phone pinch and keyboard operation. Existing connections/billing/settings unaffected.

- [ ] INS-F0 PLAN expand frontend integration into preflight -> implementation -> verification -> independent review -> merge -> landed tasks  kind: agent stage: preflight  blocked-by: [INS-A5, INS-B5]  acc: [use landed API/brand contract, frozen route/build/CSP integration and measured data shape to allocate executable task IDs and browser/performance budgets; include exact-review coverage and fix/re-review paths before coding]

### Production qualification and release

Outcome: inspector and matching brand live at https://serenity.sire.run with authenticated owner data and public synthetic demo, at verified deployed revision. Package the actual serving path: Pages assets when applicable, embedded hosted binary/API when applicable; retain prior artifacts and rollback procedure. Use existing deployment tooling; no new hosting migration, pricing or spend is implied. Release contract must resolve current hosted security/recovery prerequisites with the trusted lead rather than claim their broad completion. If Pages Actions is billing-blocked, use only an established authorized alternative or record a real release blocker. Canary with dedicated non-customer test account/brain; verify cross-account denials, source privacy, session expiry, static asset/CSP behavior and browser flows over public HTTPS. Verify health before/after rollout, exact artifact SHA/build identity, live website/brand links and rollback evidence. Public release notes contain no private memory data.

- [ ] INS-R0 PLAN expand production release after inspector landed verification  kind: agent stage: preflight  blocked-by: [INS-F0]  acc: [replace this provisional dependency with the concrete frontend verify-landed ID when INS-F0 expands; only become executable after that task is complete; create separate release preflight, qualification, independent release review, deployment and public-HTTPS verification tasks, with rollback and provider blockers named]

## Open decisions and risks

Safe defaults: existing host and session authentication; added/captured-year timeline; owner-only reads; public synthetic demo; website-wide Book Binder; no record writes. These are proposed implementation contracts to freeze at INS-00, not a newly ratified ADR. If multi-brain selection, counts or date meaning cannot be resolved from current contracts, record the concrete decision rather than guess. Historical as-of reconstruction is deferred. Production topology/tool access and recovery-related release clearance remain unverified, not assumed ready.

## Evidence routing

Task receipts: docs/tasks/inspector-release/ when execution begins. Accepted architecture decisions: existing ADR series after checking duplicates. Events and failures: append docs/devlog.md; delivery status: existing roadmap conventions. Independent reviews: docs/reviews/. Final release receipt records public URL, artifact/commit, deployment identifier, browser/provider observations and rollback target. Never replace historical prototype or fleet evidence with broader claims.

## INS-00 contract freeze (source lanes)

Brand/source lanes GO at e2b5dd17 with disjoint SSD worktrees. Heavy checks remain held for load>10; rollout preflight remains separate. API routes: GET /api/inspector/v1/brains; GET /api/inspector/v1/brains/{brainID}/graph; GET /api/inspector/v1/brains/{brainID}/nodes/{nodeID}. Session401 JSON, every brain/account ownership checked, guessed unavailable metadata404. Graph shape version1,brainId,nodes,edges,totalMatching,nextCursor; max100/default50 records/page, stable typed IDs and cursor bound to normalized filters. Detail contains node, relatedNodes, edges, truncated (bounded100). Nodes keep prototype type/id/kind/label/text/entityId/scope/status/sourceIds/createdAt/capturedAt/observedAt/validUntil/confidence/supersession fields. Facts use actual CreatedAt; claims observedAt remains separate from missing createdAt; sources expose OccurredAt as observedAt, not an invented capture timestamp. Entity date is earliest eligible linked fact only, explicitly dateKind=earliest-linked-memory. No remote local-only/secret disclosure; eligibility follows current erased/expired/visibility policy. Source metadata allowlisted, no URI/meta/rawbytes. Existing owner brains only, no provisioning writes or modelcalls; existing authenticated-session expiry renewal is allowed.

User selects native parallel GPT-6-Luna authors. INS-A1 uses explicit lane:agent: Kazi CLI1.297.1 exposes only claude/opencode harnesses and no qualified Luna/Codex binding. No goal reference supplied; check-only gate N/A per apply PHASES, required independent acceptance checks remain unchanged.

INS-00 seam clarifications: read-only means no memory/brain writes, not replacement of existing identity semantics. Reuse identity.Service.Session, including its ordinary session-expiry renewal; do not duplicate authentication SQL. The API author may add a minimal pool unloaded-vs-capacity distinction with tests, preserving ErrCapacity compatibility for existing callers. New gateway fenced-read helper and tests are in API review scope. No startup/recovery behavior change is authorized.

Release topology verified: the hosted binary embeds site/ and serves the public domain; Pages is a mirror, so Pages-only publication cannot satisfy this release. Effective existing environment AWS identity reads the stack and host. Read-only SSM reports active hosted/caddy/backup services and version0.1.13-hosted-partner-candidate. Signed deployment qualification is blocked: latest candidate release lacks required Sigstore bundle, host lacks Cosign, Actions jobs fail without executed steps. No signature gate waiver or paid-launch/billing authorization is inferred. These block deployment, not independent local source implementation.
