# Hosted billing and canonical followup — 2026-10-01

Baseline main: PR327 `b010d7868c41af44aeb6c8acf2f024a886b8e5d3`. Its tree equals reviewed head `afba4e079c7e1bbda84a9b59f56bac52f86f7ef9`. Local validation remains authorized by ADR024; original checkouts, draft PR273 and launch/provider/spend gates are preserved.

## Isolated ownership

The Luna billing lane owns only internal/hosted/billing and its new receipt. Coordinator approved ref-counted context-aware account/event serialization: provider work never holds the lock-map mutex; event lock precedes account lock. Initial webhook subscription fetch is identity discovery only; server-owned customer binding is revalidated and authoritative provider truth refetched under the account lock. The tested lock candidate is035d0de with receiptf4cf7fd, pending independent review. A separate effective-plan followup must align webhook/reconcile eligibility with the meter's status/period/grace rule, preserve exact persisted grace and frozen accounts, and prove rollback for malformed retained grace. Rate limiting and production lifecycle assembly remain open.

The Luna writer lane owns queue.go, commit.go, new core guard/helpers/tests and a separate receipt, in an isolated external worktree. Approved API: Queue.SubmitAndFlush(ctx, root, Job) returns SubmitAndFlushResult with existing Result and a Committed flag indicating a new Git commit; nil Result.Err means flush completed, including a no-op. Shared EnterCommit and exclusive WithCommitFence use a context-aware fair guard separate from runMu. The inline path owns runMu across Render and selective flush, retains dirty paths on failure, and runs local hooks after both guards release. It must not recursively call Flush or wrap its own guard in another shared acquisition. Existing Submit/Result stay compatible and remain explicitly outside this new guard. This is an unwired core seam: production checker activation requires auditing/routing every canonical path, including forget; provider-only indexing jobs must stay outside the commit guard. No memory handler, gateway, service, contracts or module edits are assigned to this lane.

The independent Luna reviewer owns only separate review receipts. Coordinator owns integration, shared records, actual repository-module checks, one full race lane, final source-pinned local validation and ordinary expected-SHA merges. Resource leases are released after writes/banking and before tests; all worktrees/caches/artifacts stay on the external SSD.

## Acceptance boundary

Neither these assignments nor local package tests complete T23.44/T23.47. Canonical marker durability after forget, complete commit fencing, startup/ticker reconciliation, operator resolution, physical staging, checkout limits, resumable production closure/reconciliation and live provider qualification remain open. No provider call, credential/IAM change, deployment, new spending or public launch is authorized by this code assignment.
