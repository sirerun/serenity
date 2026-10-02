# Independent source review: T23.52 publisher collision correction

Date: 2026-10-02

## Disposition

**CLEAR for source commit `68557003d6cbf7b4ec8e01217967793d5743856a` only**, against baseline `f4738933649bf399f098949430ce6def3699ee8b`. The explicit `ObjectExists` branch preserves the distinction between a definite completion-key collision and an ambiguous final-write response. Independent tests confirmed that the collision returns `ObjectExists` without recording an owned COMPLETE put, while the existing ambiguous-write exact-readback success remains green.

This is not full T23.52 acceptance, transport/provider qualification, wiring/admission, production activation, or authority to purge/deploy. I made no source changes, Go calls, provider calls, or claim operations. The reviewed checkout was clean at the exact SHA after verification.

## Checkout and scope

I verified `/Volumes/BuildOffload` mounted writable with available space, then created a fresh full clone at `/Volumes/BuildOffload/worktrees/serenity-publisher-collision-review-20261002-1108` from the candidate author clone and detached it at the exact requested SHA. Diff against qualified main contains only:

- three lines in `deploy/hosted/backup_publish.py`;
- 25 lines in `deploy/hosted/tests/test_backup_publish.py`;
- the 15-line source-only receipt `docs/launch/evidence/T23.52/publisher-definite-collision-correction-2026-10-02.md`.

The correction catches `ObjectExists` and rethrows it before the broad handler that reconciles ambiguous write failures by exact remote readback. It leaves all other exceptions on the previous reconciliation path. The new test simulates a competing writer installing the exact intended COMPLETE bytes, then makes this attempt receive definite `ObjectExists`. This is the critical case: broad readback reconciliation would otherwise accept those bytes and return success. Its assertions require propagation and confirm the test attempt did not record its own COMPLETE put.

## Independent controls

All test processes used `TMPDIR=/Volumes/SerenityPrivateFixture20261001/tmp`, `SERENITY_RECOVERY_TEST_TMPDIR=/Volumes/SerenityPrivateFixture20261001/tmp`, and `PYTHONDONTWRITEBYTECODE=1`. The fixture parent was owned by the current user, mode 0700, and writable.

- Focused collision and ambiguous-success controls: **2 passed**.
- Full hosted Python suite with `ResourceWarning` treated as errors: **87 passed** in 9.376 seconds.
- Ruff on both changed Python files: **All checks passed**.
- `git diff --check f473893..HEAD`: passed.
- Mutant control: in a disposable copy under the private fixture, I removed exactly the three-line `ObjectExists` catch and ran the new collision test. It exited 1 with the expected real assertion, `AssertionError: ObjectExists not raised`. I restored the copy byte-for-byte; source SHA-256 before mutation and after restoration was `c3fa6c7a88d58fb83ec25565d2f2c00d9bb6da21b9d9a040530fbad2ee32cb87`. The reviewed clone's production file had the same hash before and after and was never edited.
- Mutation log: `/Volumes/BuildOffload/serenity-publisher-collision-review-red-20261002.log`, SHA-256 `4775e024ec0193c51a92495757ccac790dbd8dd4cc2d43cac9072c9470a6d29f`.

The private fixture root contained an empty owned directory `tmpyjlkdu4d` born on 2026-10-01, before this review. I left it untouched. All test-created temporary directories from this review were automatically removed.

## Limits

The tests use the repository's fake storage adapter; no real transport or provider behavior was exercised. The candidate fixes definite-collision semantics at the publisher boundary only. The source receipt correctly keeps the publisher unwired and full T23.52 open. No claim is made about remote inventory, production admission, retention authority, physical quotas, or launch readiness.
