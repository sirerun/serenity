# Serenity roadmap

Living status for RFC 0001 (docs/rfc/0001-serenity.md). Plan of record: docs/plan.md (epic files under docs/plans/). Update on every merge, lane claim/finish, blocker, and decision.

## Shipped

- 2026 08 27 Plan to code complete (commit 9474e5a): docs/plan.md, docs/plans/E0-E6 (108 tasks), ADRs 001-010, devlog. Owner: David's planning session.
- 2026 08 26 M0 skeleton + substrate invariants (commit 13dc0d2): init, fence + shard writers with round-trip property tests, 10K-claim shard property test, SQLite index with wipe-and-rebuild invariant and the fence/shard disagreement fixture, sync/extract/doctor/status, keychain daemon token, CI (vet/build/test-race, cross-build), goreleaser + brew tap config (never tagged).

## In progress

- none (next: /apply on E0 wave 0a)

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
