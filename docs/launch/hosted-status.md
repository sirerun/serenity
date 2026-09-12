# Hosted launch status

Kept current by the launch-driving session at every task boundary. Exact heads, owners, reusable components, blockers, evidence. No secret values, ever.

## Pins (2026-09-11)

| What | Value |
|---|---|
| `origin/main` | `2d024726a5` |
| Local-only increment | `1dfecc828d` (direction and disposition HTTP wiring), pushed on `origin/feat/blink-still-protocol-routes-20260911`, owner: blink-still session, no PR; landed by T23.2 |
| PR #220 | `4e3c4150f5` keyed memory recovery, open, 12/12 green, unreviewed |
| PR #221 | `3edf6d56b7` writer ownership lock, open, 12/12 green |
| PR #222 | `5ccb30b45b` fresh semantic indexing, open, 12/12 green |
| PR #223 | `86b0929da3` cancellation fencing, open, 12/12 green |
| PR #224 | `57166d6678` exact read (draft), 12/12 green |
| PR #225 | `1c2ac6b0df` credential profiles (draft), 12/12 green |
| Rakazo adapter | elie222/rakazo PR 835, merge `ef63e354`, 2026-09-10 |
| Latest release | v0.1.1 (Homebrew formula working); main has far more; hosted deploy pins a new tag |
| Site | GitHub Pages, `site/`, `serenity.sire.run` CNAME `sirerun.github.io`, HTTPS enforced |
| App origin | `app.serenity.sire.run` (does not resolve yet; T23.15) |
| Plan config | version 1, ratified and to be published (ADR 016) |
| Stripe | Sire Run, Inc. account ready (founder-confirmed); no Serenity products yet; no keys in any repo store |
| Embeddings | `OPENAI_API_KEY`-style key needed by the hosted service (named `EMBEDDINGS_API_KEY` in Secrets Manager); no such key exists in any repo store |

## Reusable components

- `internal/server/mcp` Streamable HTTP handler and session model (reused per brain).
- `internal/server/memory` MEMORY_VERBS tools (filtered to four in hosted mode).
- `internal/writer`, `internal/store`, `internal/index`, `internal/embed` (unchanged).
- `deploy/chat` CloudFormation change-set flow, EMF metrics and alarm pattern.
- `release.yml` linux arm64 archives (pin by tag and sha256).
- `tests/site` Playwright suite and `scripts/site/check.py` for the website.

## Active ownership

| Task | Owner | State |
|---|---|---|
| Groom (this file, hosted-plan.md, ADR 014-016) | planning session | done 2026-09-11 |
| T23.1 | executor | not started |

## Blockers and external items (name, never value)

| Item | Needed by | Owner | State |
|---|---|---|---|
| `RESEND_API_KEY` and Resend domain verification for `serenity.sire.run` | T23.15 | David | not requested yet; executor asks with the exact DNS records prepared |
| `app.serenity.sire.run` A record (foundation PR) | T23.15 | executor opens, seat applies | not opened |
| Stripe test-mode restricted key | T23.16 | David | not requested yet |
| `EMBEDDINGS_API_KEY` for the hosted service | T23.11, T23.17 | David | not requested yet; T23.3 names the pinned model and cost first |
| Stripe live-mode restricted key | T23.38 | David | after the release packet |
| Human walkthrough participants | T23.33 | David | after T23.31 |
| Mini internal disk 5.6 GB free (build floor 20 GB) | every build | executor | preflight before each build; use offload volume |

## Evidence log

- 2026-09-11 Discovery complete (docs/launch/hosted-plan.md section 2). No code written yet.
