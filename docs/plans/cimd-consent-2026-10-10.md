# Serenity CIMD and connector consent

Add HTTPS client metadata discovery alongside existing dynamic registration. Diagnose and correct the reported Gumloop consent failure without expanding requested permissions. Source base: 77cdb479ad8646b21e733b1502cc71cf7a9076c5. Scope is Serenity only. Existing startup and signed-release gates remain separate; source checks do not claim deployed acceptance.

## E1 - OAuth compatibility
fidelity: executable
Acceptance: Both registration mechanisms complete authorization securely and consent failures have verified causes.

- [x] T1.0 Inspect source, live discovery and isolated DGX environment Owner: oauth-coordinator kind: agent stage: preflight acc: [source baseline and live DCR metadata recorded; isolated DGX work and scope confirmed]
- [ ] T1.1 Add bounded secure CIMD resolution and discovery Owner: cimd-author kind: agent stage: implement blocked-by: [T1.0] acc: [HTTPS metadata clients authorize and refresh while existing DCR clients retain behavior; SSRF and invalid metadata fail closed]
- [ ] T1.2 Diagnose and correct connector consent Owner: oauth-coordinator kind: agent stage: implement blocked-by: [T1.0] acc: [reported approval error paths are reproduced and corrected with scope, expiry and replay boundaries preserved]
- [ ] T1.3 Verify OAuth compatibility and regressions Owner: oauth-coordinator kind: agent stage: verify blocked-by: [T1.1, T1.2] acc: [real HTTP authorization and token tests cover CIMD and DCR, consent failures and hostile metadata; affected race, build, vet and lint pass]
- [ ] T1.4 Independently review final candidate Owner: oauth-reviewer kind: agent stage: review blocked-by: [T1.3] acc: [exact base and head independently reviewed with no unresolved blocking findings]
- [ ] T1.5 Merge approved candidate Owner: oauth-coordinator kind: agent stage: merge blocked-by: [T1.4] acc: [guarded rebase merge preserves branch protections and coordinator holds]
- [ ] T1.6 Verify landed source Owner: oauth-coordinator kind: agent stage: verify-landed blocked-by: [T1.5] acc: [landed tree matches reviewed tree and affected OAuth checks pass]

## Delivery and constraints

One coherent PR covers T1.0 through T1.6. Worker selection inherits the session. Short dispatch lease and canonical shared cooldown apply. DGX checks use at most two CPU cores, one heavy lane and isolated caches. No credential transfer, paid runners or model fallback. Coordinator owns integration and consent changes; CIMD author owns new resolver and tests. Metadata resolver is integrated through SQLStore client lookup; coordinator owns shared wiring. Existing mcpoauth validation remains authoritative for PKCE, redirects, scopes, token issuance and revocation. Independent review must verify the resolver cannot create a default credential or network bypass.

CIMD documents are untrusted client declarations, not elevated authority. HTTPS-only identifiers, exact client ID match, bounded fetch/cache, public-address-only dialing with DNS rebinding defense and no redirects are required. Preserve DCR and memory scope boundaries. Do not advertise CIMD before the working resolver is wired.

Live Gumloop acceptance requires the actual connector request and user session; a reproduced generic failure is not proof of Gumloop's specific cause. Deployment still requires a runnable admitted hosted factory and verified release signature.
