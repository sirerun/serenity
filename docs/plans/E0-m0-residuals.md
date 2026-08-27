# E0 -- M0 residuals: substrate invariants the RFC mandates but 13dc0d2 lacks
Acceptance: RFC section 17 M0 AC stays green AND every M0-scoped rule in sections 7, 7.7, and 14 is enforced by a test or CI gate, not by convention; a v0.1.0 tag produces release binaries and a brew formula.
fidelity: executable

Verified at 13dc0d2 (2026 08 27): `go test -race ./...` green (cli, index, store); fence round-trip property, 10K-claim shard property, wipe-and-rebuild + fence/shard disagreement fixtures all present. What is absent: threat model doc, file-first CI gate, writer queue, dirty-tree guard, daemon commits, fence merge test, id-collision tripwire, vocabulary enforcement, compact verb, runtime-state allowlist, linter, an actual release.

### Wave 0a (no deps; 9 agents)

- [x] T0.1 Threat model document (`docs/threat-model.md`) with data-flow diagram and testable invariants  Owner: pool  Est: 90m  verifies: [UC-030, UC-043, UC-046]  lane: agent  acc: [docs/threat-model.md exists and internal/docs test asserts headings for the five RFC section 14 adversaries, the redaction contract, the right-to-forget chain, and the line "no ingest path can create or modify a precept"] (merged PR #2, 2026-08-27)
  - Deps: none. Content: the five adversaries (injected source content, compromised MCP client, model-provider handling, secrets in ingested repos, local attacker on index/keys); mermaid data-flow of what leaves the machine; the deletion chain source -> observations -> claims -> rebuild; keys-in-keychain; loopback-authenticated daemon.
  - Pitfall: one mermaid block inside the doc, no second diagram format.
- [ ] T0.2 File-first CI gate: AST allowlist test over `internal/` for Engine write calls  Owner: pool  Est: 75m  verifies: [UC-002]  acc: [go test ./internal/gate fails when a synthetic file outside the allowlist calls eng.UpsertClaim, passes on the current tree]
  - Scope: `internal/gate/filefirst_test.go` walks `internal/` with go/ast, finds calls to `UpsertEntity|UpsertClaim|InsertChunk|UpsertVector`, and fails unless the calling file is in an allowlist (`internal/index/rebuild.go`, `internal/writer/*`). Include a red-check subtest that writes a temp file with a disallowed call and asserts the walker flags it.
  - Pitfall: an allowlist of files, not taint analysis.
- [ ] T0.3 Writer queue package `internal/writer`: single serialized queue with per-file ordering  Owner: pool  Est: 90m  verifies: [UC-015, UC-016]  acc: [go test -race ./internal/writer property test with 8 goroutines x 200 jobs over overlapping files proves no interleaving per file, total per-file order, and every job landed]
  - Scope: `Queue.Submit(Job{Path, Render func() ([]byte, error)})`, one goroutine drains, per-path sequence numbers asserted in the test via a recording hook; `FenceWriter.WriteEntity` and `ShardStore.Append` gain queue-backed entry points (`writer.Fence`, `writer.Shard`) that all later milestones must use (T0.2's allowlist is the enforcement).
- [ ] T0.6 Fence concurrent-merge determinism property test  Owner: pool  Est: 60m  verifies: [UC-016]  acc: [TestFenceConcurrentMergeDeterministic green: two independently appended copies of one page re-render to the same bytes after row union]
  - Scope: in `internal/store/fence_test.go`; append/supersede-mark edits only (the merge-safe class); assert `git merge-file` style three-way union of rows then RenderEntity is byte-identical regardless of side order.
- [ ] T0.7 Claim-id collision tripwire `store.ErrIDCollision`  Owner: pool  Est: 45m  verifies: [UC-015]  acc: [a unit test forcing two different (subject,predicate,objectKey,validFrom,sourceRef) tuples onto one id gets ErrIDCollision from the writer, never an overwrite]
  - Scope: writer-side registry per file (parse existing rows, compare full tuple on id match); comment documents the widen-hash migration; `DerivedID` gains a width parameter with 8 as default (D2 in ADR 004).
- [ ] T0.8 Controlled-vocabulary enforcement at the writers + seed assertion  Owner: pool  Est: 45m  verifies: [UC-005, UC-016]  acc: [writing a claim with predicate not in serenity.yml families returns store.ErrUnknownPredicate; a config test asserts all 13 RFC predicates are seeded with the RFC tiers]
- [ ] T0.9 `serenity compact` verb (explicit, `--confirm` required until M2 disposition gating)  Owner: pool  Est: 45m  verifies: [UC-033]  acc: [serenity compact without --confirm exits 1; with it the archive shard exists, live shard keeps only heads, and sync after compaction dumps byte-identically to sync before]
  - Extend TestShard10KProperty to run resolve/rebuild over compacted state.
- [ ] T0.10 Runtime-state allowlist in `internal/index`  Owner: pool  Est: 45m  verifies: [UC-002]  acc: [index.RuntimeTables lists {jobs, disposition_items, disposition_history, spend_ledger, caches}; a test seeds a row in each and proves Rebuild/ResetAll leave them intact while all other tables are wiped]
- [ ] T0.12 golangci-lint config + CI step + `make lint`  Owner: pool  Est: 30m  verifies: [infrastructure]  acc: [.golangci.yml exists, ci.yml runs golangci-lint, and the run is green on main]

### Wave 0b (after 0a; 4 agents)

- [ ] T0.4 Dirty-tree guard: pause writes to a human-dirty file and record a pending conflict  Owner: pool  Est: 75m  verifies: [UC-016]  deps: [T0.3]  acc: [test: hand-edit a page, submit a machine write for it; file bytes unchanged, .serenity/pending/<slug>.json holds both sides, clearing it resumes the write]
  - Scope: `git status --porcelain -- <path>` per target file before each job (never whole-repo dirtiness); pending records are runtime state (T0.10) and become disposition items in T2.1.
- [ ] T0.5 Daemon commits (`serenity:` prefix) + doctor last-push age  Owner: pool  Est: 60m  verifies: [UC-040, UC-003]  deps: [T0.3]  acc: [queue flush produces a commit whose subject starts with "serenity:"; doctor against a fixture with a local bare remote reports last-push age and warns when > 24h or never pushed]
  - Pitfall: `git log origin/<branch>` on a never-pushed repo is "never pushed", not an error.
- [ ] T0.11 Cut v0.1.0: verify goreleaser publishes darwin/arm64 + linux/amd64 + linux/arm64 archives and the brew tap formula  Owner: David  Est: 30m  verifies: [infrastructure]  kind: human  deps: [T0.12]  acc: [gh release view v0.1.0 lists three archives + checksums.txt and sirerun/homebrew-tap has Formula/serenity.rb]
  - Needs the `HOMEBREW_TAP_GITHUB_TOKEN` repo secret (founder-held) and a tag push; the release workflow exists but has never fired.
- [ ] T0.13 Wire T0.3 into existing callers + extend the CLI wipe-rebuild test through the queue  Owner: pool  Est: 45m  verifies: [UC-002]  deps: [T0.3, T0.2]  acc: [internal/cli tests write via writer.Fence/writer.Shard and the file-first gate allowlist contains no direct store writes outside internal/writer and internal/index/rebuild.go]

Decision rationale: docs/adr/004-writer-queue-and-pending-records.md (D1 pending records as files under .serenity/pending; D2 hash width 8 with tripwire).
