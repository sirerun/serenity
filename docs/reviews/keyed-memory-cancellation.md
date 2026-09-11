# Keyed cancellation qualification record

In progress, 2026-09-10. No release or deployment.

The initial implementation extended forget's selector. Store, writer and index
race suites passed, but three pinned protocol checks correctly rejected removal
of forget's required id. Corrected by keeping the five pinned tools unchanged
and adding cancel_memory_operation as an explicitly discoverable extension.

Read-only source review found no blocking semantic defect and requested
merged-history, codec compatibility and actual MCP/restart/index proof. Those
checks now pass. The real pinned CLI/MCP process proof has 34 passing assertions,
including cancellation-before-creation, late delivery after crash, key-only
cancellation of an existing fact, schema rejection, sync/restart and an older
reader failing closed on the v2 source. No direct canonical file edits.

Final local race suite: 1,825 test/subtest passes, zero failures, six disclosed
opt-in/helper/facade skips. Lint reports zero issues; vet/build pass. All twelve
hosted CI checks pass, including external gbrain conformance and crossbuilds.
The shared build lease was observed and released.

One earlier full local run failed an unchanged disposition importer concurrency
test with Darwin renameat EINVAL (1,824 passes, one failure, six skips). There is
no diff in internal/disposition. The isolated test then passed twenty repetitions
on the parent and twenty on this branch; the full final run also passed. The
cause is unresolved, not fixed by this PR. Retain this occurrence as a separate
qualification follow-up; do not erase it from the evidence or claim EINVAL is a
safe contention outcome. Owner Codex: reproduce on the unchanged importer at the
next Darwin concurrency qualification, before changing its error handling.

This remains logical withdrawal, not physical purge. Cancellation v2 brains must
not be downgraded to an older writer; failure to open/read is intentional.
