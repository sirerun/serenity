# E15 authenticated current CLOSED history source

Status: E15 source implementation, qualification, independent review, guarded merge and landed verification complete (T5.101-T5.107). E16 and broader recovery remain open.

The owner explicitly assigned the `R-hosted-authority` source resource to this
batch on 2026-10-19. The previous claim was released by exact compare-and-swap
through the canonical claim primitive, then the resource and T5.102-T5.104 were
acquired by this batch. T5.101-T5.103 are recorded complete in the plan.

The frozen API adds a strictly observational `Kernel.ObserveCurrent`, owner-only
`closed_current`, exact `closed_position` get/list selectors, and a detached
`ClosedSource` that reads full bounded deletion history and performs a fresh
authenticated position recheck. The owner transport sends only those CLOSED
selectors; the operator transport refuses them. `runtimebroker.OpenClosedMaintenance`
constructs the source from the existing authenticated owner client. The seal
transition now clears the generation-one genesis ID when allocating a successor,
matching the journal position contract. For pre-fix CLOSED successor records,
the owner read projects away that redundant valid genesis ID only after the
exact raw state is bound to its witness anchor. A regression reconstructs the
original seal witness shape without adding a revision or sequence and reads its
complete history successfully.

Coverage includes pending state with zero write/advance calls, exact
META/witness binding, observation and complete history after actual table
restore/rebind, malformed and oversized current witness bodies, second-read
revision and TIP drift, owner role restrictions, empty CLOSED history without a
cut, newer entries than an expired historical cut, legacy and current successor
generations, 300 entries across bounded pages, missing/malformed/oversized/extra-
tail journal objects, final position drift, cancellation, malformed wire fields,
and signed owner transport requests with fixture credentials and no-remote-
invocation checks.

The final current Go source diff was fetched from base
`ac1c9cc707db1edf7f092f42f3131cc7c5330e4d` and passed race tests for
`internal/hosted/authority`, `internal/hosted/runtimebroker`, and
`internal/hosted/episodemaintenance`, followed by `go vet` for the same packages.
This ran in the one-CPU existing DGX Spark pod
`serenity-e15-final-20261019` using `golang:1.26.9-bookworm`. The exact source
patch SHA-256 was
`732d86d91424c71ff4a8e64c275ebce967cfff0c76d1935b82f4d2f7af782141`.
`git diff --check` also passed. Independent exact review of the complete PR #414
diff from base `5fe6048e60bfb2c06a29cc8eced5047f0f95cf13` to head
`47ee6e225ab6ac0d73b1bfd993a40e07fb6aba55` covered design, preparation, plan,
source/tests and receipt, with no findings. The source-patch digest above was
unchanged by the documentation commits.

Wazi validation after recording the landed evidence reports 199 tasks,
`schemaValid=true`, `semanticValid=true`, `readerCompatible=true`, and source
digest `sha256:2f8ee1691c6ade740505fb5803456eec64f8af47c291c3965c07c790ef939aa2`.
`authorityAuthenticated=false` is expected and is not authority evidence.

Actions run `39215924845` on reviewed source/review head `47ee6e225ab6ac0d73b1bfd993a40e07fb6aba55` had 14 jobs with zero steps; the test-job annotation says the
account is locked due to a billing issue. This is unavailable hosted CI, not a
pass. Owner-accepted ADR 024 local-validation fallback was used without changing
billing or branch protection.

This is source and synthetic-fixture evidence. No production authority,
provider, IAM policy, enrollment, installation, capture setting or external
service was changed. The source does not establish host coldness, installation
recovery, copy disposal, physical erasure, full T5.2/T5.3 completion or live
capture readiness. Those remain separate plan gates.


## Landed evidence

PR #414 merged by guarded rebase on 2026-10-19 as commit
`3a2a9621917b71f7f32547fde88913546c085b21`. The reviewed tree
`ed4f646f8c05237d35cffb89b41ffc7ac5eedf08` equals the fetched main tree after
merge, and the actual PR base `5fe6048e60bfb2c06a29cc8eced5047f0f95cf13` is
an ancestor. E15 T5.101-T5.107 are complete. No provider, production authority,
installation, capture, physical erasure or controlled-copy disposition is
inferred; E16 and full T5.2/T5.3 remain open.
