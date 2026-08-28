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

## L-0002: A task's `task/<id>` branch may already be checked out in an idle sibling worktree

**Tags:** #git #worktree #gotcha
**Date:** 2026-08-27
**Repo:** sirerun/serenity

**Rule:** Never `git checkout -b task/<id>` for a goal's branch inside a
kazi-worktree without first checking `git branch -vv` for that name. If
it is already checked out elsewhere (`+` prefix in the listing), commit
on the current local branch instead and push straight to the remote ref:
`git push origin HEAD:refs/heads/task/<id>`. This satisfies a `landed`
predicate that fetches and compares `origin/task/<id>` without needing a
same-named local branch, so it never collides with the other worktree.
**Why:** Wave 0a's dispatch (docs/roadmap.md, 2026 08 27) created one
long-lived worktree per task under `/Users/dndungu/Code/sirerun/wt-T0.<n>`
*and* separately routed the actual work order for at least one of those
tasks (T0.3) to an ephemeral `kazi-worktrees/p-*` worktree on a different
local branch. Both worktrees have a local branch named
`task/t0-3-writer-queue`; git refuses `checkout -b` on a name that already
exists, and would refuse a plain `checkout` too since the branch was
checked out in the other worktree. The sibling `wt-T0.<n>` worktree was
confirmed idle (clean tree, `+0 -0` vs `origin/main`, no commits) before
working around it this way -- if it is not idle, that is a live duplicate
claim on the same task and needs a human/coordinator decision, not a
git workaround.
**Trigger:** A kazi goal whose `landed` predicate checks
`origin/task/<id>` while the executing worktree's local branch has a
different name than `task/<id>`, and `git branch -vv` shows `task/<id>`
already checked out (`+`) in another worktree on this machine.

## L-0003: A kazi `landed` predicate must not hardcode the push branch name -- the scheduler-owned worktree runs on its own `kazi-partition/p-<hash>` branch

**Tags:** #kazi #git #worktree #gotcha
**Date:** 2026-08-27
**Repo:** sirerun/serenity

**Rule:** When authoring a kazi JIT proposal's TASK BRIEF (apply/KAZI-EXEC.md
step 1), never instruct the grind harness to `git push -u origin
task/<goal-id>` by literal name. Either omit the branch name (`git push -u
origin HEAD`) or, if a `landed` predicate needs a stable ref to check,
check `HEAD == @{u}` only (no hardcoded remote branch name) and separately
verify the *content* landed (e.g. grep for the new file/symbol on
`origin/main` post-merge) rather than asserting a specific branch exists.
If a goal reports `stuck` with only a `landed`-shaped predicate in the
failing set while every code/behavior predicate is green, treat it as this
landmine before escalating the model ladder: `git branch -a` /
`git ls-remote origin` for a `kazi-partition/p-<hash>` ref is very likely
where the real, converged commit already landed and is recoverable by
`git fetch origin <that-ref>` + `git cherry-pick`.
**Why:** kazi >=1.27x (confirmed on 1.275.0) executes every goal --
serial or `--parallel`, single-partition or not -- in a scheduler-owned
worktree it derives from `--workspace`, checked out on its own
`kazi-partition/p-<partition-hash>` branch, not a branch literally named
`task/<goal-id>`. `--workspace` is a *source* the scheduler copies from,
never the actual execution environment. T0.2's JIT brief hardcoded `git
push -u origin task/t0-2-file-first-gate`; the harness (Claude Sonnet 5,
following the brief faithfully) had no local branch by that name in the
scheduler's own worktree, so the `landed` predicate's `HEAD == @{u}`
check could never be satisfied -- the goal cycled `stuck` with the exact
same single failing predicate for 3 iterations before kazi escalated to
a human, even though `cap-gate-package-green`, both named subtests,
`guard-full-suite`, `guard-vet-clean`, and `guard-repo-identity` all
converged genuinely (verified independently after recovery: `go test
./internal/gate -v`, `go vet ./...`, `go test ./... -race`, `golangci-lint
run ./...` all clean on the recovered commit). The real work was never
lost -- kazi had already pushed it to `origin/kazi-partition/p-16b82378e-
8787989-790523c6-1039f1b4` -- but nothing in the stuck-verdict JSON says
"your code is fine, only your bookkeeping predicate is broken"; that
diagnosis takes reading the full predicate vector (kazi_status --json)
and noticing every OTHER predicate already reads `pass`.
**Trigger:** A kazi goal's terminal verdict is `stuck` (not `error`), the
persistent failing-predicate set is exactly one id and that id is your
own process/landed predicate (not a `cap-`/`guard-` predicate tied to real
code or tests), and every other predicate in the last observed vector
reads `pass`.

## L-0004: The local pre-commit hook lints staged files as an isolated pseudo-package, producing false "undefined" errors

**Tags:** #git #hooks #golangci-lint #gotcha
**Date:** 2026-08-28
**Repo:** sirerun/serenity

**Rule:** If `git commit` fails with golangci-lint `undefined: <symbol>`
(typecheck) errors for symbols that are genuinely defined elsewhere in the
same package, do not assume the new code is broken -- verify with a
whole-repo `golangci-lint run` (no path arguments) before touching the new
file, and commit with `--no-verify` once that whole-repo run is clean.
**Why:** This checkout's local `.git/hooks/pre-commit` (untracked -- no
source under version control installs it; confirmed by a repo-wide grep for
"pre-commit" and hook-related filenames, nothing found) runs
`git diff --cached --name-only | xargs golangci-lint run --fix`. Passing
explicit file-path arguments makes golangci-lint build a synthetic,
single-file pseudo-package instead of resolving the real package directory
from disk, so any symbol defined in an unstaged sibling file in the same
package is invisible to the typecheck. Hit while shipping T3.12 (PR #14):
adding `internal/gate/precept_immutability_test.go`, which reuses
`violation`/`joinViolations`/`writeFixture`/`repoRoot` from the unstaged
`internal/gate/filefirst_test.go`, produced 10 false `undefined` errors and
blocked the commit. A whole-repo `golangci-lint run` on the same tree
reported 0 issues, and `go build`/`go vet`/`go test -race ./...` were all
clean -- confirming the hook, not the code, was wrong.
**Trigger:** Any commit that stages a new file into an existing multi-file
package without also staging (or having already committed) every sibling
file the new one depends on -- a near-certainty whenever a task adds one
new `_test.go` file to a package like `internal/gate` that already has
shared test helpers.

## L-0005: The pre-commit hook's full `go test ./...` can commit fixture junk onto a task branch under concurrent pool load

**Tags:** #git #hooks #writer #concurrency #critical
**Date:** 2026-08-28
**Repo:** sirerun/serenity

**Rule:** After any `git commit` that ran the local pre-commit hook (it
shells `go test ./...` for every staged `.go`/`go.mod` change), run
`git log --oneline -5` on the resulting branch before pushing or opening a
PR and check for commits you did not author -- especially any message that
looks like a test fixture seed (e.g. "seed <entity> page/shard"). If found,
do not try to hand-repair the tree: `git reset --hard` back to the branch's
correct base inside the (isolated, disposable) task worktree, re-verify
`go build`/`go vet`/`gofmt -l`/`go test -race ./...`/`golangci-lint run`
clean, then recommit (`--no-verify` is justified here -- the hook itself is
the corruption vector).
**Why:** During T1.6's `git commit`, `internal/writer`'s git-fixture tests
(`TestFenceEntryPoint`, `TestDirtyTreeGuardResumeAfterClear`,
`TestQueueFlushCommitsWithSerenityPrefix`,
`TestQueueFlushNoopWhenNothingTouched`,
`TestQueueFlushScopesToTouchedPaths`) both failed AND, worse, landed three
commits directly on the task branch that the task never authored ("seed
alice-tan page", "seed checking-acct shard", "seed alice-tan page", same
second). The first of those commits deleted `go.mod`, `cmd/`, `docs/`,
`LICENSE`, `Makefile`, `README.md`, `.github/`, `.gitignore`,
`.golangci.yml`, `.goreleaser.yaml` from git's tracked tree (the files
stayed on disk, git just stopped tracking them) and added a
`brain/entities/person/alice-tan.md` fixture -- i.e. some git-fixture test
helper operated on the real worktree instead of an isolated `t.TempDir()`
repo. Re-running the exact same `internal/writer` suite immediately
afterward, in isolation with no concurrent sibling load, passed cleanly
with zero corruption -- this points to load-contention (7+ concurrent
kazi/test/lint processes hammering the same machine during a pool wave)
rather than a bug that fires on every invocation. Root cause not yet
isolated to a specific line in `internal/writer`.
**Trigger:** Committing Go changes (which fires the hook's full
`go test ./...`) while several other pool agents are concurrently running
their own `git commit`/`kazi apply`/`golangci-lint` on the same host --
the exact condition of an `/apply --pool` wave with many parallel kazi-lane
tasks.

## L-0007: L-0005/L-0006's stated root cause (GIT_DIR/GIT_WORK_TREE env inheritance) was tested and does NOT reproduce -- the real mechanism is still unknown

**Tags:** #git #hooks #gotcha #unconfirmed

**Rule:** Treat L-0005 and L-0006's "Why:" sections as an unconfirmed
working theory, not a settled root cause. Do not implement a fix (e.g.
clearing GIT_DIR/GIT_WORK_TREE/GIT_INDEX_FILE in internal/writer's
git-fixture test helpers) on the assumption that theory is correct without
first re-confirming it independently -- it already failed one clean-room
repro attempt.

**Why:** L-0005/L-0006 state that the shared `.git/hooks/pre-commit`'s
`go test ./...` step lets `internal/writer`'s git-fixture tests (which
`git init`/`add`/`commit` inside a `t.TempDir()` via `cmd.Dir`) inherit
GIT_DIR/GIT_WORK_TREE/GIT_INDEX_FILE from the hook's own process, causing
the "isolated" fixture commits to land on the real repo instead. A
follow-up clean-room repro (single process, zero pool contention: a
minimal repo, the same hook, a test doing exactly what the fixture helpers
do) did NOT reproduce this -- GIT_DIR/GIT_WORK_TREE were confirmed unset in
the hook's own environment (only a relative GIT_INDEX_FILE=.git/index was
present), and the nested fixture commit landed correctly in its own
tempdir every time.

The corruption itself is real and repeatedly observed during this wave
(multiple task branches and the primary checkout all hit variants of it)
-- only the explanation is unconfirmed. The leading alternative theory is
concurrent pool-load contention: this incident occurred with 7+ agents
running their own `git commit`/`kazi apply`/`go test`/`golangci-lint`
processes against worktrees of the same repo on the same host
simultaneously, which is a meaningfully different (and much less
tractable) failure class than a deterministic env-var leak -- possible
culprits include races on the shared `.git/objects`/`.git/index` under
concurrent access, OS-level file-descriptor or process-limit pressure, or
something not yet identified.

**Trigger:** Before spending effort on a fix targeting the GIT_DIR/
GIT_WORK_TREE theory specifically, re-run the clean-room repro described
above (single process, no concurrent pool load) to confirm it actually
reproduces in this repo's exact test helpers first. If it doesn't, the fix
effort should go toward isolating/serializing git operations under
concurrent load instead.
