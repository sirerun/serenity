# Serenity OAuth interoperability repair

Review MCP authorization against the latest published specification and the supported dated protocol. Diagnose Gumloop callback abort and Claude server error without treating either as a proven common cause. Source base: 28010b94. Preserve production authority, privacy, PKCE and exact redirects.

## E1 - Standards and connection repair
fidelity: executable
Acceptance: Reachable defects have regression tests and reviewed fixes; unresolved provider acceptance is explicit.

- [x] T1.1 Inspect discovery, registration, consent, tokens and MCP authorization Owner: oauth-auditor kind: agent stage: preflight acc: [requirement matrix cites official specifications and source; observed failure distinguished from inference]
- [x] T1.2 Implement confirmed interoperability fixes Owner: oauth-coordinator kind: agent stage: implement blocked-by: [T1.1] acc: [minimal secure fixes and failing-before regression tests cover confirmed defects]
- [ ] T1.3 Verify candidate Owner: oauth-coordinator kind: agent stage: verify blocked-by: [T1.2] acc: [affected race tests, full build and vet, lint and plan conformance pass on DGX at exact revision]
- [ ] T1.4 Independently review candidate Owner: oauth-reviewer kind: agent stage: review blocked-by: [T1.3] acc: [exact base and head independently clear with findings resolved]
- [ ] T1.5 Merge and verify landing Owner: oauth-coordinator kind: agent stage: merge blocked-by: [T1.4] acc: [guarded rebase merge and landed tree and affected checks pass]
- [ ] T1.6 Qualify live connection repair Owner: deployment-owner kind: agent stage: verify-landed blocked-by: [T1.5] acc: [existing release owner coordinates any deployment; actual Gumloop and Claude acceptance or exact remaining blocker recorded]

## Batch and ownership

One coherent source batch T1.1-T1.5, coordinator integrates, independent auditor reads source and specifications without editing. Reviewer inherits session model. Shared dispatch admission applies. DGX CPU2, one heavy lane, existing isolated caches and private temporary root. No new spend, production trust changes, protection bypass, or secret logging. Existing production owner controls release and runtime mutations. Local verification applies while Actions billing is blocked.
