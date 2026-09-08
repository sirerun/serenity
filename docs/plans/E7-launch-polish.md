# E7 — Launch polish and adoption loop

Purpose: turn the already deployed Serenity website and adoption chat into a measured, reliable launch surface. Adjacent product-owned release surfaces are tracked in their own private repositories; this epic covers only Serenity's public dependencies and launch work.

Fidelity: executable. Frontier: unblocked after the current Serenity and external dependency release surfaces are inventoried.

## Current fleet state (checked 2026-09-08)

The dated [launch checklist](../launch/checklist.md) records the exact Pages and Lambda revisions, authoritative DNS and certificate state, external dependency status, owners and rollback routes. HTTPS enforcement and chat operational monitoring are verified complete. The live `/healthz` route returns 200; CloudWatch received application latency samples. M4 is complete; M5 is 11/15 with its human release gates preserved. External deployment provenance remains each release owner's responsibility.

## Tasks

- [x] T7.1 Release-surface inventory and coordination  Owner: David  Est: 30m  kind: operations  delivers: [single launch checklist naming the exact Serenity Pages revision, DNS state, Lambda version, adjacent-product dependency status, owners, and rollback links]  deps: []
  - Acceptance: checklist records live URLs and current CI/deployment evidence; external changes remain deferred to their release owner until reviewed and verified in their own repository.
- [x] T7.2 Adoption event contract and browser coverage  Owner: pool  Est: 90m  verifies: [UC-039, infrastructure]  deps: [T7.1]  acc: [Playwright covers landing-page install CTA, docs/get-started navigation, chat happy path, chat error path, and mobile layout; production verification observes the documented events without recording prompt text or secrets]
  - Scope: define a small event vocabulary (`install_cta`, `docs_open`, `chat_started`, `chat_answered`, `chat_failed`) and a privacy-preserving implementation for the static site. Keep the chat transcript out of analytics.
- [x] T7.3 Chat reliability and operations  Owner: pool  Est: 90m  verifies: [UC-039, infrastructure]  deps: [T7.1]  acc: [Lambda health check, allowed-origin request, rejected-origin request, model-outage fallback, and rate-limit behavior are covered in CI; production has an actionable error/latency signal and no API key or prompt transcript is logged]
  - Scope: add CloudWatch alarms or an equivalent existing AWS signal, document thresholds and response steps, and verify the deployed function against the live custom-domain origin.
- [ ] T7.4 Launch content packet  Owner: David  Est: 60m  kind: content  delivers: [launch announcement, short demo script, FAQ answers, and one canonical installation walkthrough linked from Serenity and ndungu.dev]  deps: [T7.1]
  - Acceptance: every claim in the packet links to a live Serenity or repository page; install instructions are copy-pasteable; the packet names the early-access boundary and the chat's privacy behavior.
  - Progress 2026-09-08: [content packet](../launch/content-packet.md) prepared and public source installation verified. The canonical ndungu.dev installation link is draft dndungu/dndungu.github.io PR #6; this task stays open until that cross-site link is deployed.

- [x] T7.5 Domain and security finish  Owner: pool  Est: 30m  verifies: [infrastructure]  deps: [T7.1]  acc: [serenity.sire.run resolves through the authoritative DNS record, HTTPS returns 200 with a valid certificate, GitHub Pages reports the custom domain, and HTTPS enforcement is enabled]
  - Scope: wait for certificate issuance if necessary, then enable enforcement and record `dig`, `curl`, and Pages API evidence. Do not change unrelated DNS records.
- [ ] T7.6 Launch-readiness gate  Owner: David  Est: 30m  kind: human  delivers: [go/no-go launch decision with evidence links and a dated follow-up review]  deps: [T7.2, T7.3, T7.4, T7.5]
  - Acceptance: the decision names the live revision, adoption baseline, known limitations, rollback owner, and the next measurement window.

## Risks and boundaries

- External products are separate release surfaces. Serenity can link to them, but cannot merge or deploy their changes without their repository's normal review gate.
- Analytics must measure adoption without collecting chat contents, credentials, IP addresses, or model prompts.
- The launch packet must preserve the existing early-access and human-control claims; no performance or customer-result claims are introduced without evidence.
