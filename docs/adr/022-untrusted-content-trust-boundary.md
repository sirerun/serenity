# ADR 022: Untrusted-content trust boundary -- connector-kind trust, inbox gating, principal-derived actor, output neutralization

## Status
Accepted

## Date
2026-09-27

## Context
Serenity turns untrusted content into claims, prompts and files. Deep review 001 found four
places where that boundary is thinner than the RFC intends: a non-conflicting claim extracted
from an ingested email is activated with actor `machine` and no human step, and the composer
cites it as fact with no trust marker (AI-01); the local HTTP API accepts a client-supplied
`actor` on `/disposition/dispose`, so an agent holding the daemon token can dispose a
`precept_draft` as `human:david` and `inbox --apply` publishes it without showing the payload
(AI-03); composer output is returned verbatim to MCP clients and terminals, so a planted
markdown image URL exfiltrates co-resident claims and ANSI escapes can rewrite the operator's
terminal (AI-05, SEC-M05); and the free-text plan classifier returns
`no_applicable_constraints` or `pass` on empty, low-confidence or out-of-set output (AI-06).

On 2026-09-27 David chose inbox gating of first-seen claims from untrusted connector kinds over
prompt-only markers or gating every machine claim.

## Decision
1. Connector-kind trust. Each connector carries `trust: trusted|untrusted` in `serenity.yml`;
   defaults are `imap` and `git_repo` untrusted and `file` trusted. A first-seen,
   non-conflicting machine claim from an untrusted connector is written `State: pending` and
   surfaces in the inbox as a `claim_candidate` item; accepting it activates the claim with the
   disposing human as actor. Conflicting claims keep the existing supersession review path.
   The composer prompt renders `actor` and trust for every claim, and citations expose them.
2. Actor from principal. The DISPOSITION and DIRECTION HTTP handlers ignore any actor on the
   wire and derive it from the authenticated transport principal (`agent:<credential-id>` for
   the bearer, `human:<user>` only for the CLI channel). Publication of a `precept_draft`
   requires a `human:` actor and therefore the CLI; `inbox --apply` prints the full payload
   before publishing a precept and refuses precept acceptances that did not originate in the
   CLI. Provider error bodies are never returned to any caller; `check_plan` maps them to a
   status code. The protocol documents record the wire `actor` field as advisory.
3. Output neutralization. One helper strips C0/C1 control characters and ESC sequences; it is
   applied at every CLI print of ingested or model text (`ask`, `search`, `inbox`) and at the
   MCP `synthesize` output. The composer removes any URL that did not appear in the retrieved
   evidence before returning an answer.
4. Classifier fails closed. An empty action list, a confidence below threshold, an action
   outside the constraint set, or cited evidence not found in the plan text yields
   `unverified`, never `no_applicable_constraints` or `pass`.

## Consequences
- Positive: a planted claim needs a human accept before it can be cited; a forged human
  approval cannot enter the directive ledger from the bearer transport; an exfiltration URL
  or a terminal escape never reaches a renderer; the classifier cannot be talked into a pass.
- Negative: people with busy mailboxes see more inbox items; the trust default is per kind and
  can be flipped per connector, and bulk-defer (UC-013) applies.
- Negative: DISPOSITION v1 clients that relied on sending `actor` see it ignored; the change is
  recorded in `docs/protocol/DISPOSITION_v1.md` and no shipped client depends on it.
- Related: E24 tasks T24.14, T24.15, T24.16, T24.17 in `docs/plans/deep-review-001-remediation.md`.
