# Backup transport invalidation correction receipt

## Result

The `S3Storage` context now becomes unusable after failures from public inventory
and ranged-read operations, including failures while parsing or validating a child
response, writing to a caller destination, or removing an owned range spool. The
operation boundary also covers executor setup and preflight failures. The original
exception still propagates. A later `put_if_absent` on that context is rejected
before another child starts.

The existing typed uncertain final `COMPLETE` write remains eligible for bounded,
read-only reconciliation. Any failed inventory/read proof poisons that
reconciliation context; upload cleanup failures continue to poison the context.
Public method signatures, child budgets, proof limits, collision handling, and
no-retry behavior are unchanged.

## Regression evidence

Five public-API tests were added for malformed inventory JSON, mismatched HEAD
version, mismatched range version, destination write failure, and range-spool
cleanup failure. Each performs the failed public operation, then attempts a PUT
through the same context and verifies no `put-object` child is launched.

Additional transport tests preserve the uncertain final `COMPLETE` behavior: a
bounded read remains allowed after that typed failure, while a failed read proof
or final spool cleanup prevents any later read-only proof from starting. An
ambiguous write to any other key still invalidates read access immediately.

Before the transport edit, those five tests produced five expected failures: each
operation raised its expected first error, but the same-context PUT was accepted.
After the edit, all five pass. The exact pre-fix test source snapshot and red log,
the green log, full hosted test log, Ruff log, and SHA-256 manifest are retained
in the external invalidation evidence bundle named
`serenity-backup-transport-final-review-evidence-20261003/invalidation-worktree-20261003/`.

## Verification

- Public-API red: 5 expected failures against the unmodified transport.
- Public-API green: 5 tests passed.
- Hosted suite: 119 tests passed across the hosted unit, supply-chain, backup,
  child-process, transport, publishing, and retention modules.
- Ruff: passed on the transport and its test module.
- `git diff --check`: passed.
- All runtime behavior used a fake AWS CLI and synthetic disk capacity. No real
  AWS request, provider test, deployment, or physical-capacity claim was made.

The coordinator owns independent review, integration, and final verification.
