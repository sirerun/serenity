# Hosted launch status

Kept current by the launch-driving session at every task boundary. Exact heads, owners, reusable components, blockers, evidence. No secret values, ever.

## Pins (2026-09-12)

| What | Value |
|---|---|
| `origin/main` | `799d6747c7bf90d7590ed1470ce43b30e257ae02` |
| Local-only increment | `1dfecc828d` (direction and disposition HTTP wiring), pushed on `origin/feat/blink-still-protocol-routes-20260911`, owner: blink-still session, no PR; landed by T23.2 |
| PR #220 | `c5db4520ec7070ad487d8a257b5d8dc8ec97059f` keyed memory recovery, merged |
| PR #221 | `d7cfe81d94c905979f1d9bd501e79e41c5834989` writer ownership lock, merged |
| PR #222 | `1d88adfe9a588fa0d3b54c2d55b82af937d2a6de` fresh semantic indexing, merged |
| PR #223 | `69e4a127395c94f2d5d8aa8f99bc0ee7d3e10d44` cancellation fencing, merged |
| PR #224 | `1fad19ca78a72af024d17f2f09f78d005bf87650` exact read, merged |
| PR #225 | `799d6747c7bf90d7590ed1470ce43b30e257ae02` credential profiles, merged |
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
| T23.1 | executor | complete 2026-09-12; receipt below |

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
- 2026-09-12 T23.1 complete: PRs #220-#225 merged in order; final main is
  `799d6747c7bf90d7590ed1470ce43b30e257ae02`; [post-merge CI run
  34681426437](https://github.com/sirerun/serenity/actions/runs/34681426437)
  passed all jobs, including vet, build, race tests and lint.
- The contract-required local gates also passed in an isolated worktree at the
  final SHA under the shared build lease: `go vet ./...`, `go build ./...`,
  `go test -race -timeout 600s ./...`, and `golangci-lint run --timeout=10m`
  (`0 issues`). The lease was released with a matching CAS. The
  pre-existing local `LAUNCH-PROMPT.md` remains untouched.
- The final diff contains the expected `cancel_memory_operation`,
  `read_memory_fact`, and `--credential-profile` surfaces. T23.2 onward remain
  open; no hosted deployment or launch-readiness claim is made.
