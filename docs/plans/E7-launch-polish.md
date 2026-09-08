# E7 — Launch polish and adoption loop

Purpose: turn the already deployed Serenity website and adoption chat into a measured, reliable launch surface. This epic coordinates Serenity with the Ajent work that owns the sign-in destination; it does not reopen the shipped visual direction or rewrite the product roadmap.

Fidelity: executable. Frontier: unblocked after the current Ajent and Serenity release surfaces are inventoried.

## Current fleet state (checked 2026-09-08)

- Serenity has no open GitHub PRs; the Pages site and Lambda endpoint are live at `serenity.sire.run`.
- Ajent PR #24 (browser-login logo) is open with its Go/PostgreSQL checks green. Ajent PR #25 (consulting footer) is a draft with checks green. Ajent PR #15's contrast revision is merged, but its live deployment is not claimed in the shared feed. Ajent PRs #14, #9, and #8 remain open maintenance/infrastructure work.
- Serenity's core roadmap still has the founder-only T4.17 M4 exit and later E5 release-gate work; E7 is the public-surface polish lane and must not silently claim those product milestones.

## Tasks

- [ ] T7.1 Release-surface inventory and coordination  Owner: David  Est: 30m  kind: operations  delivers: [single launch checklist naming the exact Serenity Pages revision, DNS state, Lambda version, Ajent logo revision, owners, and rollback links]  deps: []
  - Acceptance: checklist records live URLs and current CI/deployment evidence; Ajent PRs #15, #24, and #25 are classified as merge, defer, or reject with one named owner for each.
- [ ] T7.2 Adoption event contract and browser coverage  Owner: pool  Est: 90m  verifies: [UC-039, infrastructure]  deps: [T7.1]  acc: [Playwright covers landing-page install CTA, docs/get-started navigation, chat happy path, chat error path, and mobile layout; production verification observes the documented events without recording prompt text or secrets]
  - Scope: define a small event vocabulary (`install_cta`, `docs_open`, `chat_started`, `chat_answered`, `chat_failed`) and a privacy-preserving implementation for the static site. Keep the chat transcript out of analytics.
- [ ] T7.3 Chat reliability and operations  Owner: pool  Est: 90m  verifies: [UC-039, infrastructure]  deps: [T7.1]  acc: [Lambda health check, allowed-origin request, rejected-origin request, model-outage fallback, and rate-limit behavior are covered in CI; production has an actionable error/latency signal and no API key or prompt transcript is logged]
  - Scope: add CloudWatch alarms or an equivalent existing AWS signal, document thresholds and response steps, and verify the deployed function against the live custom-domain origin.
- [ ] T7.4 Launch content packet  Owner: David  Est: 60m  kind: content  delivers: [launch announcement, short demo script, FAQ answers, and one canonical installation walkthrough linked from Serenity and ndungu.dev]  deps: [T7.1]
  - Acceptance: every claim in the packet links to a live Serenity or repository page; install instructions are copy-pasteable; the packet names the early-access boundary and the chat's privacy behavior.
- [ ] T7.5 Domain and security finish  Owner: pool  Est: 30m  verifies: [infrastructure]  deps: [T7.1]  acc: [serenity.sire.run resolves through the authoritative DNS record, HTTPS returns 200 with a valid certificate, GitHub Pages reports the custom domain, and HTTPS enforcement is enabled]
  - Scope: wait for certificate issuance if necessary, then enable enforcement and record `dig`, `curl`, and Pages API evidence. Do not change unrelated DNS records.
- [ ] T7.6 Launch-readiness gate  Owner: David  Est: 30m  kind: human  delivers: [go/no-go launch decision with evidence links and a dated follow-up review]  deps: [T7.2, T7.3, T7.4, T7.5]
  - Acceptance: the decision names the live revision, adoption baseline, known limitations, rollback owner, and the next measurement window.

## Risks and boundaries

- Ajent is a separate release surface. Serenity can link to it, but cannot merge or deploy Ajent changes without that repository's normal review gate.
- Analytics must measure adoption without collecting chat contents, credentials, IP addresses, or model prompts.
- The launch packet must preserve the existing early-access and human-control claims; no performance or customer-result claims are introduced without evidence.
