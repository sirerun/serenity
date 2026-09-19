# T23.41 architecture review request

**Requested reviewer:** chief-architect (per `docs/launch/hosted-plan.md`
task41 required steps: "With chief-architect, resolve and publish the four
bounded design decisions").

**Requester:** headless Claude Code Sonnet worker, branch
`hosted/t23.41-20260918`, base `810349ba7c1ed2c05fe34e3892de764a26a4633c`.

**Why this is a request, not a receipt:** this session is non-interactive
with no synchronous reviewer available. Per the worker protocol
(`docs/launch/hosted-completion/worker-prompt.md`: "prepare concrete changes
before escalating a genuinely missing approval") and this task's own
instruction ("If architecture approval is missing, prepare concrete
proposals and reviewed-ready implementation but report PARTIAL; never
invent approval"), the four decisions below are drafted to the point a
reviewer can approve, amend or reject them in one pass, but none is marked
approved anywhere in this task's output.

## Revision note

An independent code review found concrete defects in the first draft of
decisions 2, 3 and 4 (not just missing approval — the proposed mechanisms
did not actually satisfy the invariant each is supposed to close) and a
real concurrency bug (data race plus a lost-wakeup hang) in the
`internal/hosted/testhooks` fault-barrier package. All four are fixed in
this revision; `docs/launch/hosted-completion/interfaces.md`'s
"Proposed decisions" section states what was wrong in each prior draft and
what changed. None of this moves any decision from "not approved" to
"approved" — it only makes the proposal worth reviewing.

## What needs a decision

`docs/launch/hosted-completion/interfaces.md` "Proposed decisions — pending
chief-architect review" contains the full technical detail for each.
Summary of what a reviewer must rule on:

1. **Storage admission (physical headroom).** Explicitly **BLOCKED with no
   adopted mechanism** — the original growth-factor-multiplier proposal
   (p95 growth ratio × 1.5) was withdrawn as unsound (a statistical
   estimate, not a hard ceiling). §1 now proposes a staged-write mechanism
   instead: stage a mutation in isolation, measure its real physical growth,
   admit against that measured value, then atomically publish. The reviewer
   rules on whether this staged-write shape is the right mechanism (or
   requires a proven mathematical bound instead) before task44 can start
   any implementation at all.
2. **Crash-safe operation accounting.** §2's `OperationRecord` now carries a
   `Deltas []OperationDelta` slice (not a single metric/unit pair), so one
   logical mutation's multiple counters — e.g. a `remember` call's
   `"writes"` and `"input_tokens"` deltas, currently two independent
   `meter.Reserve`/`Finish` cycles — finalize together in one transaction,
   plus a `CanonicalRef` field, a `LeaseExpiresAt` field aligned with the
   proposed SQL column, and a fourth `pending_review` phase so
   `ReconcilePending` never silently resolves an unprovable row to
   committed or released. Approve, amend or reject the table shape and
   phase state machine.
3. **Independently durable deletion journal.** The watermark mechanism was
   replaced: entries are now addressed by a monotonic `SequenceID` assigned
   by the control DB's single fenced writer (paired with a `Generation`
   from decision 4), not an S3 `ListObjectsV2` continuation token — the
   prior design could permanently miss entries whose opaque subject ID
   sorted lexicographically before an already-consumed key. Object Lock /
   compliance-mode retention is withdrawn as a proposed mechanism (real
   SPEND-gate cost/operational consequences); this proposal commits only to
   bucket versioning as the durability floor pending a SPEND-gate decision
   on retention. Approve, amend or reject the sequence/generation design,
   IAM prefix scoping and the open retention question.
4. **Restore eligibility / activation barrier.** The local-lock-file
   fencing option is dropped entirely for cross-host restore: reading
   `internal/writer/ownership_unix.go` in full confirmed `AcquireBrain`
   never writes a PID and is a kernel-local `flock()` that cannot observe a
   different host at all — restore's own primary scenario. §4 now requires
   both an AWS-API-verified old-instance stop and explicit revocation of
   every credential/session (including the old instance's live IAM
   role/session, not just its database access) the old instance could still
   use, offered alongside a fully specified distributed generation barrier
   as the still-open alternative. Approve, amend or reject which mechanism
   task50 builds against.

## What is already concrete and does not need this review

`internal/hosted/contracts/**` (billing truth/closure, backup manifest v2,
recovery CLI shape, telemetry, provider pin, accounting units, registration
mode) and `internal/hosted/testhooks/**` (fault barrier, including this
pass's concurrency fix) are frozen, compiled, `go vet`/lint-clean, and
covered by real tests — these seams do not depend on the four decisions
above and can be reviewed as ordinary code, not an architecture question.

## Requested outcome

A reviewer reply (recorded in this file's own follow-up, in
`ajent.social`, or as a PR review comment once this branch is published)
naming, per decision: approved as proposed / approved with the following
amendment / rejected with the following required alternative. Until that
reply exists, task44/47/48/50 must treat all four as blocked, per
`docs/launch/hosted-plan.md`: "Unresolved decision means dependent task is
blocked, not an agent guess."
