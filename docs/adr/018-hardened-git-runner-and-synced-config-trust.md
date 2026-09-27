# ADR 018: Hardened git subprocess runner and the synced-config trust boundary

## Status
Accepted

## Date
2026-09-27

## Context
Deep review 001 (`docs/deep-reviews/001-full-codebase.md`, SEC-H05 and SEC-H04) traced two
related weaknesses. First, every git invocation in the codebase (12 production files, among
them `internal/cli/gitx.go`, `internal/connector/gitrepo/gitrepo.go`, `internal/writer/commit.go`,
`internal/hosted/pool/pool.go`) runs `git` with `cmd.Dir` set to a repository but with no
configuration overrides and the inherited environment. Git honors repository-level
configuration such as `core.fsmonitor` and hooks on index-refreshing subcommands
(`ls-files --others`, `status`, `diff`), so a repository whose state an attacker controls can
execute a program on the machine that runs the subcommand. Second, `serenity.yml` is committed
into the brain and synced through the brain remote, yet `config.Load` decodes it without
`KnownFields`, treats `connectors` as an untyped map, passes a `git_repo` connector's `path`
verbatim as a repository root, and returns the daemon bind address unchecked. The trust root
is therefore "anything a brain remote delivers is first-party", which is false for shared or
compromised remotes.

The brain repository itself legitimately relies on one hook: the post-commit auto-push hook
installed by `serenity init` (UC-040). A blanket `core.hooksPath=/dev/null` would break it.

## Decision
1. All git subprocesses run through one package, `internal/gitrun`, and no other production
   package may call `exec.Command` with `git`. A drift test enforces this.
2. `gitrun` distinguishes two trust levels by constructor:
   - `gitrun.Brain(dir)` for the brain repository the process owns: passes
     `-c core.fsmonitor=false -c protocol.ext.allow=never -c core.sshCommand=ssh` and keeps
     repository hooks enabled so the auto-push hook still fires.
   - `gitrun.Foreign(dir)` for repositories the process does not own (connector targets,
     imported brains, restored bundles before validation): additionally passes
     `-c core.hooksPath=/dev/null`, sets `GIT_CONFIG_GLOBAL=/dev/null` and
     `GIT_CONFIG_NOSYSTEM=1`, and never runs a subcommand that writes.
   Both scrub the environment: every `GIT_*` variable is dropped except an explicit allowlist
   (`GIT_SSH_COMMAND` when set by the daemon, `GIT_TERMINAL_PROMPT=0`); `PATH`, `HOME`, `LANG`
   and `TMPDIR` are kept.
3. `config.Load` decodes with `KnownFields(true)` and fails naming the first unknown key.
   Connector `path` values resolve to absolute, cleaned paths and must lie under an allowlisted
   root: the user's home directory by default, extended only by an explicit
   `connectors.roots` list that is validated the same way. The daemon bind address must be a
   loopback address unless `server.allow_lan: true` is set explicitly.
4. The `git_repo` connector reads files with `Lstat`, skips non-regular files, resolves symlinks
   with `EvalSymlinks`, and refuses any path whose resolved form leaves the repository top
   level (SEC-H04).

## Consequences
- Positive: the fsmonitor and hook execution class is closed for every subcommand; a hostile
  connector repository, imported brain or synced configuration cannot run code or read files
  outside its own tree; the review's attack chain AC-2 loses its code-execution leg.
- Positive: one place to audit git invocation, one place to add timeouts and logging.
- Negative: `KnownFields(true)` rejects brains whose `serenity.yml` carries unknown keys; the
  error names the key so the fix is immediate, and `serenity check` reports it.
- Negative: hosted call sites (`pool.go`, `backup.go`, `lifecycle.go`, `provision/canonical.go`)
  live under hosted-completion file claims and migrate through those tasks (T23.45, T23.49,
  T23.48, T23.46); the drift test lands only after they do (E24 T24.30).
- Related: E24 tasks T24.6, T24.7, T24.8, T24.30 in `docs/plans/deep-review-001-remediation.md`.
