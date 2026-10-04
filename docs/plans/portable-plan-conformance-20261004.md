# PC-SERENITY — Portable plan context and policy conformance

## Context and scope

The user invoked ship for Serenity's bounded assignment in the portable-plan
implementation dispatch dated 2026-10-04. This is ordinary repository delivery,
not enrolled recovery/startup execution. Serenity contributes contextual/policy
examples and only a necessary local read-only compatibility projection. Wazi
owns the shared experimental v0 schema and semantic validator. Its exact frozen
revision and digest must be pinned before adapter implementation or final
conformance. No competing schema, scheduler, memory mutation, authority issuer,
hosted activation or recovery/startup policy assignment is in scope.

Success means offline examples validated against the pinned shared contract,
meaningful negative trust-boundary tests, independent exact-head review, GitHub
rebase merge and verified landed artifacts. Hosted CI observations remain raw;
ADR 024's expressly authorized local source-merge alternative is represented
separately. Context retrieval never satisfies an execution/approval/deployment
dependency. Structural validity never authenticates receipts or policy authority.

## Discovery and deliverables

Two use cases: UC-PC1 consumes scoped memory as context without granting
execution authority; UC-PC2 displays unavailable hosted checks separately from
the scoped owner-approved local validation alternative. Source inventory and
fixture requirements can proceed before contract freeze. Prefer fixtures and
conformance tests over a new production projection unless source mapping proves
one necessary. Do not change canonical memory/protocol or recovery source.

The source baseline is Serenity `e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f`.
Existing recovery owner assignments/holds retain their original owners.

## Capability and preflight record

Session capability selection: baseline, delivery and Go; selector reports its
requirements met. Actual tools: authenticated GitHub CLI and Git/Go/Python;
no callable Ajent MCP or code-review-graph MCP in this session. Use local channels
and source reads; record the Ajent gap in the PR. Kazi CLI exists; canonical
controller stages are not delegated to it. New artifacts/worktrees/caches use
the external SSD. The source checkout's authored untracked files are preserved.
The new lane is isolated on `pc-serenity/conformance-20261004`.

Resource claim: `R-portable-plan-conformance`, held by the comparison coordinator;
exact token recorded in the lane checkpoint. Shared Go build admission requires
fresh one-minute load <=10 and the shared lease, released with its exact token.
The first observed load exceeds that threshold: no build is started. Source/doc
inventory remains independently runnable. No provider or actual brain access is
needed for this offline conformance slice.

## Checkable delivery plan

- [x] PCS.0 Reconcile scope, ownership, tools and source mapping  Owner: pc-serenity-coordinator kind: agent stage: preflight  delivers: [UC-PC1, UC-PC2]  acc: [external SSD writable; isolated clean candidate; existing owners/holds preserved; actual tools and resource gaps recorded]
- [ ] PCS.1 Inventory context/policy surfaces and send consumer fixture requirements  Owner: pc-serenity-inventory kind: agent stage: implement  blocked-by: [PCS.0]  delivers: [source inventory and Wazi counterexample requirements]  acc: [provider-free versus provider-capable reads identified; ADR024 source-merge scope preserved; context is never execution evidence]
- [ ] PCS.2 Pin Wazi's frozen experimental contract  Owner: pc-serenity-coordinator kind: agent stage: preflight  blocked-by: [PCS.1]  delivers: [contract manifest]  acc: [owner-confirmed exact revision/digest, schemas, semantic validator and fixture instructions available; no schema fork]
- [ ] PCS.3 Implement contextual and policy conformance examples  Owner: pc-serenity-coordinator kind: agent stage: implement  blocked-by: [PCS.2]  verifies: [UC-PC1, UC-PC2]  acc: [fixtures preserve context provenance/unavailable states and separate hosted observations from authorized local evaluation; necessary projection is read-only only]
- [ ] PCS.4 Verify behavior and required checks  Owner: pc-serenity-coordinator kind: agent stage: verify  blocked-by: [PCS.3]  verifies: [UC-PC1, UC-PC2]  acc: [pinned validator accepts valid fixtures and rejects invalid trust/mapping cases; targeted tests and formatting/lint pass on exact candidate; hosted CI outcome recorded honestly]
- [ ] PCS.5 Independent exact-head review of PCS.3 and PCS.4  Owner: independent-reviewer kind: agent stage: review  blocked-by: [PCS.4]  verifies: [UC-PC1, UC-PC2]  acc: [reviewer distinct from authors; base/head and stable findings recorded; meaningful negative controls reproduced; all blocking findings resolved]
- [ ] PCS.6 Evaluate checks/holds and GitHub rebase merge  Owner: pc-serenity-coordinator kind: agent stage: merge  blocked-by: [PCS.5]  verifies: [UC-PC1, UC-PC2]  acc: [exact approved head, current policy/required checks and named holds reconciled; rebase merged without protections changes]
- [ ] PCS.7 Verify landed revision and hand off  Owner: pc-serenity-coordinator kind: agent stage: verify-landed  blocked-by: [PCS.6]  verifies: [UC-PC1, UC-PC2]  acc: [landed artifacts match reviewed candidate and pass scoped conformance; PR/review/check/landed evidence shared; claims released exactly]

## Verification and boundaries

Required cases: context-as-authority rejection; unavailable/unmapped context;
provider-capable read excluded from automatic projection; hosted CI unavailable
without alternative; same raw observation with explicitly scoped authorized local
alternative; local failure; stale head/base/policy; cancellation/expiry and late
audit context; alternative source-merge policy does not grant deploy/startup.
Adapt the cases to the single pinned steward contract rather than choosing wire
fields now. Runtime credential/receipt authentication is outside structural
conformance and must remain unqualified. No new HTTP/UI surface is expected.

Review findings get separate stable fix/verify/review task IDs. Required checks
follow the changed scope and ADR024; local success is never hosted-CI success.
Production deployment and release publication are outside the dispatch.

## Status and handoff

Source/doc preflight passed; builds remain resource-held. First runnable task is PCS.1. PCS.2 and
descendants require Wazi's published frozen contract; no such revision has yet
been supplied in the inspected dispatch/channel. Coordination messages are
handoffs, not authenticated execution receipts.
