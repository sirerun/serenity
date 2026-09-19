# T23.41 architecture review request

**Requested reviewer:** chief-architect (per `docs/launch/hosted-plan.md`
task41 required steps: "With chief-architect, resolve and publish the four
bounded design decisions").

**Requester:** headless Claude Code Sonnet worker, worktree
`/Volumes/BuildOffload/wt/serenity-launch-T23.41-20260918`, branch
`hosted/T23.41`, base `810349ba7c1ed2c05fe34e3892de764a26a4633c`.

**Why this is a request, not a receipt:** this session is non-interactive
with no synchronous reviewer available. Per the worker protocol
(`docs/launch/hosted-completion/worker-prompt.md`: "prepare concrete changes
before escalating a genuinely missing approval") and this task's own
instruction ("If architecture approval is missing, prepare concrete
proposals and reviewed-ready implementation but report PARTIAL; never
invent approval"), the four decisions below are drafted to the point a
reviewer can approve, amend or reject them in one pass, but none is marked
approved anywhere in this task's output.

## What needs a decision

`docs/launch/hosted-completion/interfaces.md` "Proposed decisions — pending
chief-architect review" (added this pass) contains the full technical detail
for each. Summary of what a reviewer must rule on:

1. **Storage admission (physical headroom).** Is the proposed conservative
   growth-envelope-plus-empirical-fixture approach (§1) the right shape, or
   does it require the "separately approved staged-write design" interfaces.md
   names as the alternative? No default value is asked for; the mechanism
   itself needs approval before task44 can even design its fixture.
2. **Crash-safe operation accounting.** Approve or amend the proposed
   `operations` table shape and phase state machine (§2) — this determines
   the schema migration task41/57 will need to apply before task44/47/48/50
   can build against it.
3. **Independently durable deletion journal.** Approve or amend the proposed
   S3 versioned + Object Lock substrate, key format and IAM prefix scoping
   (§3).
4. **Restore eligibility / activation barrier.** Approve or amend the
   proposed generation-fence mechanism (§4) — specifically whether fencing
   via the existing `writer.AcquireBrain` lock-file liveness check is
   sufficient, or whether an explicit `serenity hosted recovery fence`
   operator command is required before any account can unfreeze.

## What is already concrete and does not need this review

`internal/hosted/contracts/**` (billing truth/closure, backup manifest v2,
recovery CLI shape, telemetry, provider pin, accounting units, registration
mode) and `internal/hosted/testhooks/**` (fault barrier) are frozen,
compiled, `go vet`/lint-clean, and covered by real tests in this worktree —
these seams do not depend on the four decisions above and can be reviewed
as ordinary code, not an architecture question.

## Requested outcome

A reviewer reply (recorded in this file's own follow-up, in
`ajent.social`, or as a PR review comment once this branch is published)
naming, per decision: approved as proposed / approved with the following
amendment / rejected with the following required alternative. Until that
reply exists, task44/47/48/50 must treat all four as blocked, per
`docs/launch/hosted-plan.md`: "Unresolved decision means dependent task is
blocked, not an agent guess."
