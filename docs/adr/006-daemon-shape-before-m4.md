# ADR 006: `serenity cron` runs scheduled work until `serenityd` lands in M4

## Status
Accepted

## Date
2026-08-27

## Context
M2 needs scheduled work: the expiry sweeper (auto-defer, park), the weekly
decay and alias sweep, nightly consolidate, and queue SLO computation. The
long-running daemon (`serve`) is an M4 deliverable. Building the daemon early
would pull M4's auth and transport into M2; leaving the sweeps unscheduled
would make M2's acceptance criteria depend on a process that does not exist.

## Decision
`serenity cron <job>` runs one scheduled job to completion and exits
(`sweep`, `consolidate`, `decay`, `slo`). It is idempotent and safe to run
concurrently with the CLI because every write goes through the writer queue
(ADR 004) inside the process and the working-tree check guards cross-process
edits. The operator's manual ships launchd and systemd timer units. In M4,
`serenityd` embeds the same job functions on an internal ticker; `serenity
cron` remains as the manual and test entry point.

## Consequences
- M2 ships without a daemon; every scheduled behavior is testable as a
  function call with an injected clock.
- Two cron jobs racing on the same file resolve through the dirty-tree
  guard, not through a lock file; the manual documents "one timer per brain".
- The inbox TUI uses `golang.org/x/term` raw mode with hand-rolled list
  rendering; a TUI framework is not adopted for one screen.
