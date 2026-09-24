# CLI writer ownership review record

Local qualification, 2026-09-10; builds use the shared machine lease.

- Full race suite: 1,802 test/subtest passes, zero failures, six disclosed skips
  (the same opt-in/helper/facade skips recorded for keyed recovery).
- Vet, build and lint pass; lint reports zero issues. An unused pre-lock config
  binding was corrected before the final run.
- Actual rebuilt CLI/MCP probe: concurrent server and symlink-alias refusal;
  sync, extract, import, cron, compact, inbox, init, config and capture refused
  during server ownership; plan-check succeeded; crash/restart and keyed
  withdrawal recovery remained correct.
- Tests independently exercise a live subprocess lock contender followed by
  SIGKILL/reacquisition, distinct roots, stable lock inode, unsafe symlink lock,
  default command error cleanup, no-brain transport, and held ownership while a
  real Git pre-commit hook blocks shutdown flush.
- Source review found the informational `connectors status` command needed an
  exemption. Added it and verified output both beside a writer and outside a
  brain without creating runtime state. Strengthened plan-check verification to
  require successful execution and the expected parsed verdict.

Scope is cooperating supported CLI binaries on local Darwin/Linux filesystems.
A long-lived daemon excludes other canonical CLI mutations; readers and the
plan-check hook can continue. This is not a lock against arbitrary Git/OS writers,
a cross-machine lease or an atomic read snapshot. No shared brain deployment.
