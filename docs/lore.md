# Project Lore

Append-only log of gotchas, invariants, and landmines. Unlike
docs/devlog.md (per-session investigation records), entries here
describe rules that must always hold or things that must never
happen again. Entries are never reordered and never pruned.

Retrieval: grep by tag, e.g. `grep -n "#build" docs/lore.md`. New
entries are appended at the end and receive a stable L-NNNN ID.
See lore/SKILL.md for the entry format.

---

## L-0001: Go tooling in this repo must run with GOWORK=off (or from an offloaded worktree)

**Tags:** #build #go #worktree #gotcha
**Date:** 2026-08-27
**Repo:** sirerun/serenity

**Rule:** Run every `go build`/`go vet`/`go test` inside this repo with
`GOWORK=off`, unless the working directory is on a path with no ancestor
`go.work` (for example a per-task worktree under `/Volumes/BuildOffload`,
per docs/plan.md's own worktree convention).
**Why:** `/Users/dndungu/Code/sirerun/go.work` declares `use (./api ./gist
./mint)`, but `./api` and `./gist` no longer exist at that top-level path
(reorganized into `legacy/`/`incubating/` elsewhere in the monorepo tree).
Go's workspace auto-detection walks up from the current directory looking
for `go.work`, so any Go command run from inside `serenity/` -- or from a
sibling worktree created directly under `sirerun/` (the default recipe in
apply/KAZI-EXEC.md is `git worktree add ../wt-<task-id> ...`, which lands
exactly there; the stray `/Users/dndungu/Code/sirerun/serenity-wt-m1`
worktree is an existing example) -- picks up this broken parent go.work
instead of serenity's own `go.mod`, and fails with `cannot load module
../api listed in go.work file: open ../api/go.mod: no such file or
directory`, even though serenity's own build and tests are fully green in
isolation. Already independently discovered and worked around in
docs/devlog.md (2026 08 27 entry).
**Trigger:** Any `go build`/`go vet`/`go test` invocation -- by a human, a
subagent, or a kazi-converged predicate's `custom_script`/build guard --
run from `serenity/` or a worktree parented under `sirerun/` without
`GOWORK=off` set. A predicate that reports a module-not-found error
mentioning `../api` or `../gist` is this landmine, not a real build break.
