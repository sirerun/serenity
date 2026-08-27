# Serenity roadmap

Living status for RFC 0001 (docs/rfc/0001-serenity.md). Plan of record: docs/plan.md (epic files under docs/plans/). Update on every merge, lane claim/finish, blocker, and decision.

## Shipped

- 2026 08 27 T0.3 writer queue `internal/writer` (`Queue`: one drain goroutine, per-path sequence numbers assigned under lock so submission order == execution order; `writer.Fence`/`writer.Shard` queue-backed entry points wrapping `store.FenceWriter.WriteEntity`/`store.ShardStore.Append` -- both stay pure primitives per ADR 004; `TestQueueOrderingProperty` 8 goroutines x 200 jobs over 3 overlapping paths, race-clean, proves no interleaving + total per-file order + all 1600 jobs landed) -- PR #5, merge commit `be8d2bc`. Owner: kazi lane (goal `t0-3-writer-queue`, converged rung 1 claude-sonnet-5, first-pass rate 0.6). Independently re-verified (fresh fetch + `go test -race -v ./internal/writer/...`, `go vet`, `gofmt -l`, full suite) before merge, not just kazi's self-report. Deviation (non-blocking): `Submit`/`Fence`/`Shard` return richer tuples (`Result{Seq,Bytes,Err}` / `(path, bytes, err)`) instead of a bare `error` as scoped -- keeps the four mandated names, and the extra return values are what T0.4's dirty-tree guard will need (rendered bytes for pending records). **`internal/writer` is now available** -- unblocks wave 0b (T0.4 dirty-tree guard, T0.5 daemon commits, T0.13 wiring + gate extension).
- 2026 08 27 T0.12 golangci-lint CI gate (`.golangci.yml` v2: govet, staticcheck, errcheck, unused / gofmt, goimports; `make lint`; CI `lint` job; the 26 pre-existing errcheck findings it surfaced fixed for real, incl. two genuine bugs -- `ShardStore.Append` swallowing a flush error and `ShardStore.Compact` swallowing a tmp-file write error) -- PR #4, merge commit `2ee0a14`. Owner: kazi lane (goal `t0-12-golangci-lint-ci`, converged on claude-sonnet-5 rung 1, no escalation needed). Unblocks T0.11 (v0.1.0 release cut, David).
- 2026 08 27 T0.1 threat model doc `docs/threat-model.md` (RFC 0001 §14: five adversaries, one mermaid data-flow diagram, redaction contract, keys-in-keychain, loopback-authenticated daemon, precept-integrity invariant, right-to-forget deletion chain) plus `internal/docs/threat_model_test.go`, the file-first heading gate that keeps the doc honest -- PR #2. Owner: `/apply --pool` session af341d66 (subagent lane).
- 2026 08 27 docs/lore.md L-0001 (GOWORK=off landmine: the parent sirerun/go.work references removed ./api and ./gist, breaking `go build`/`test` for any Go command run from serenity/ or a worktree parented under sirerun/) -- PR #1. Owner: a third concurrent `/apply --pool` session on this shared checkout.
- 2026 08 27 Podcast script docs/content/podcast-serenity-and-gbrain.md (two voices, ~13 min) and its ElevenLabs Studio render (Turbo v2.5, Alice as HOST, Adam as MAINTAINER, MP3). Owner: David's planning session.
- 2026 08 27 Plan to code complete (commit 9474e5a): docs/plan.md, docs/plans/E0-E6 (108 tasks), ADRs 001-010, devlog. Owner: David's planning session.
- 2026 08 26 M0 skeleton + substrate invariants (commit 13dc0d2): init, fence + shard writers with round-trip property tests, 10K-claim shard property test, SQLite index with wipe-and-rebuild invariant and the fence/shard disagreement fixture, sync/extract/doctor/status, keychain daemon token, CI (vet/build/test-race, cross-build), goreleaser + brew tap config (never tagged).

## In progress

- 2026 08 27 E0 wave 0a (T0.1, T0.2, T0.3, T0.6, T0.7, T0.8, T0.9, T0.10, T0.12) re-claimed and dispatched via `/apply --pool` (session af341d66, `/loop` tick). At claim time `git ls-remote origin refs/claims/*` was empty and `gh pr list --state all` showed no PRs beyond #1 (the lore fix) -- the prior "claimed and in flight" line this superseded never reached a PR, so its claims were either released or went stale and were pruned by another session; no duplicate work found (no `wt-T0.*` worktrees, no `task/*` branches on origin). T0.1 -> subagent lane (isolated worktree, docs+test) -- **done, see Shipped**. T0.12 -> kazi lane (JIT proposal from acc:, converged rung 1 claude-sonnet-5, merged PR #4) -- **done, see Shipped**. T0.2/T0.3/T0.6/T0.7/T0.8/T0.9/T0.10 -> kazi lane (JIT proposal from acc:, converge in `../wt-T0.<n>`, GOWORK=off per L-0001), still in flight. Other concurrent sessions observed on this machine (serenity-a/b/c/d, sire-a, seat, blink-oxalpha) hold no active claims on these 9 tasks as of dispatch. Wave 0b (T0.4, T0.5, T0.13) stays blocked on T0.2/T0.3 landing.

## In flight (PRs open)

- none

## Planned

- E0 M0 residuals (threat model, file-first gate, writer queue, dirty-tree guard, daemon commits, fence merge test, id tripwire, vocabulary enforcement, compact verb, runtime allowlist, lint, v0.1.0 tag)
- E1 M1 ingest spine + honest evals (watcher, Gmail IMAP, repo crawler, chunker, router, extraction, claims write path, embeddings, hybrid search, ask, eval harness, Ava corpus, BrainBench trend)
- E2 M2 reconcile + entities + disposition queue + ladder calibration
- E3 M3 direction (dira vendored, interview, check_plan, orphan detector)
- E4 M4 serve + protocols (MCP/HTTP auth, MEMORY_VERBS conformance, DISPOSITION v1, DIRECTION v1, spend ceiling, connect claude)
- E5 M5 migration + launch (gbrain import, docs site, adversarial gate, name decision, install-time AC)
- E6 M6 hardening soak (outline; starts after code complete, ADR 002)

## Blocked

- T0.11 v0.1.0 release: needs the HOMEBREW_TAP_GITHUB_TOKEN repo secret and a tag push (founder). FOUNDER: David.

## Decisions

- 2026 08 27 ADR 001 Gmail app-password IMAP certified (David).
- 2026 08 27 ADR 002 code complete = M0-M5 ACs green; M6 is a post-code-complete soak (David).
- 2026 08 27 ADR 003 third-party deps only at connector/provider edge (fsnotify, go-imap/v2, net/http adapters).
- 2026 08 27 ADR 004 one writer queue, file-backed pending records, 8-hex ids with collision tripwire.
- 2026 08 27 ADR 005 eval labeling protocol (two model families + David adjudicates); voice notes move to M2 (David).
- 2026 08 27 ADR 006 `serenity cron` until serverd in M4; x/term inbox TUI.
- 2026 08 27 ADR 007 reconcile constants (0.85 candidates, corroboration formula, parked resurfacing, read-time decay).
- 2026 08 27 ADR 008 precepts are unmodified dira entries; applies_when in the body; Serenity owns the matcher.
- 2026 08 27 ADR 009 gbrain import reads markdown fences; field mapping fixed.
- 2026 08 27 ADR 010 check exit codes per RFC, one event log per transport, token rotation, mkdocs-material, name decision at M4 exit (David).
