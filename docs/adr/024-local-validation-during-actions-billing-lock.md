# ADR 024: Local validation during the GitHub Actions billing lock

## Status
Accepted by David, 2026-10-01.

## Context

GitHub Actions cannot start jobs while the account is locked for billing.
David explicitly instructed the coordinator to use local validation, continue
autonomously, and review and merge the work. Restoring paid Actions is not a
prerequisite for development or a request to incur new spending.

## Decision

Use local checks appropriate to each change as the merge gate while Actions is
unavailable. Record the source revision, commands, outcomes, skips, and material
limits in the PR and project evidence. Substantial implementation changes receive
independent review. Honor explicit reviewer and trusted coordinator holds.

Keep the shared build lease, external-SSD artifact policy, and repository review
rules. Do not change branch protections or OAuth restrictions to enable a merge.
Never report blocked Actions jobs as passing. Live deployment and paid-provider
qualification require their own evidence; local checks do not establish them.

## Consequences

Development and reviewed merges continue without GitHub billing expenditure.
T24.41 remains an optional owner action for restoring hosted CI, rather than a
blocker on locally validated work. Existing source-pinned local receipts remain
reviewable; missing live qualification remains visible.
