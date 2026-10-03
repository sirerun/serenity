# Corrected backup transport independent review

**Disposition: CLEAR on source semantics for exact commit `d5152fef04f29645a45a388141c5e3c932d38a0e`, subject to a separate mechanical Ruff-format correction/check.** This clears the transport invalidation finding for this exact candidate. Formatting remains an explicit outstanding validation item below; this report does not claim Ruff format passed.

The prior initial CLEAR on `6bab4ce` was superseded by the supplementary HOLD after public malformed-response calls were shown to permit a subsequent same-context PUT. See `review-report.md` and `supplementary-invalidation-hold.md` in this evidence directory. The corrected `d5152fef` source fixes the reported behavior; this report applies only to that exact corrected source commit.

## Independent behavioral review

Reviewed the corrected commit and its assignment/implementation receipt. The commit's authored diff is confined to `deploy/hosted/backup_s3_transport.py`, its transport tests, and the handoff receipt; there is no caller wiring in the authored diff. The public `list_prefix` and `read_to` methods now poison the context for operation failures raised during response parsing/validation, destination writes, and owned range cleanup. `put_if_absent` also includes operation initialization/preflight in its failure boundary. Existing argument validation and fail-closed usability checks remain outside that boundary.

I reran each of the five public invalidation tests: malformed inventory, wrong HEAD version, wrong range version, destination write failure, and range cleanup failure. Each asserts a subsequent same-context public PUT refuses and that no fake `put-object` child was called. All five passed. I also reran controls for ambiguous non-COMPLETE write invalidation, ambiguous COMPLETE read-only reconciliation, failed reconciliation proof poisoning, final COMPLETE spool-cleanup failure, normal publish/download verification, and the publisher's exact ambiguous-final readback proof. All passed (11 focused tests total).

For genuine baseline REDs, I temporarily substituted the production transport bytes from `6bab4ce` (`8606ae4fdfface1da33d7def5147593507fb9531297909c543ab950c76985170`) in this dedicated review clone and ran the five new public invalidation tests. All five failed because the follow-up public PUT was not rejected. I restored the corrected transport file byte-for-byte afterward. Captured output is `corrected-baseline-five-reds.txt`.

## Validation

- Full hosted suite: **119 passed**, `ResourceWarning` treated as an error. Captured in `corrected-119-hosted-tests.txt`.
- Ruff check on all six allowed Python files: passed (`corrected-ruff-check.txt`).
- `git diff --check 4b2fd8e..HEAD`: passed (`corrected-diff-check.txt`).
- `ruff format --check` on the four transport/child owned files: **not clean**. It reports two formatting changes in `deploy/hosted/tests/test_backup_s3_transport.py` (the helper at line 164 and assertion at line 674); three of the four files are already formatted. Captured in `corrected-ruff-format-check.txt`. Coordinator advised that integration will apply the formatter only to this owned test file, preserving all semantic source bytes, and will check the formatted candidate separately. This review therefore gives semantic CLEAR for d515 while leaving that mechanical formatting check explicitly pending.

The review clone is clean at d515. No provider request, credential access, live test, Go test, or author-checkout edit occurred. No claim is made here about T23.52 acceptance or provider/deployment/recovery qualification.
