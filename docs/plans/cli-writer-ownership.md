# CLI canonical writer ownership

Status: implementation under qualification; not released.
Owner: Codex, supporting the shared-brain prerequisite after keyed recovery.

The released CLI has one command tree but several independent queue and direct
source-write paths. Two stdio servers can currently own the same brain; locking
only their queues would also miss sync, import and cron.

Use a nonblocking OS-held lock at `.serenity/writer.lock`, retained until the
entire mutating CLI command has returned, including deferred flush/cleanup.
Never remove the lock inode. Process exit releases ownership; no PID expiry,
renewal loop, bypass environment variable or inherited ownership grant.

Default commands to ownership. Explicit reviewed canonical readers/control
commands remain usable: check, doctor, status, search, ask, report, protocol and
connect. Derived cache/spend writes and arbitrary external config output are
outside the canonical-memory promise. The plan-check path uses a nil-queue
read-only direction store, as the public read facade already does.

Serve owns its dependencies after recognizing a valid brain, before opening the
index; it re-reads config under ownership. Invalid/missing config retains the
existing transport-only fallback without creating runtime state. Init may create
only the minimal root/lock scaffolding before ownership, then runs normally.

Acceptance: actual process contention and crash release, root aliases,
different-root concurrency, mutating CLI refusal before effects, error cleanup,
plan-check coexistence and no-brain transport behavior. Existing suite and lint
must pass. Default future commands to ownership; exemption changes require review.

Operational effect: a long-lived server excludes sync/import/cron and other
canonical CLI writers until stopped. Multiple clients should use one HTTP server
and queue. There is no supported daemon dispatch path for those CLI jobs yet.
This fences cooperating supported binaries on a local Darwin/Linux filesystem;
older binaries, direct internal callers, Git operations and malicious filesystem
writers are not fenced. It does not provide distributed/NFS ownership or atomic
reader snapshots. No shared brain is deployed by this change.
