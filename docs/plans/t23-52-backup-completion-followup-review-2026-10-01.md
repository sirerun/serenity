# T23.52 backup completion helper follow-up review — 2026-10-01

Reviewed candidate: `ceccd429eeef029cebb456dda9d61dd3d1816b29`. This follow-up is specifically scoped to T23.52's backup completion helper. The prior receipt `docs/plans/t23-49-backup-completion-independent-review-2026-10-01.md` in commit `fbaaf5434ab5ae404c2d04a1f339f57f3dd94016` remains the review of the original `b054456` behavior and its crash-retry defect; it is not the verdict on this corrected commit.

## Verdict

The `ceccd429` correction addresses the prior blocker. It writes temporary completion bytes beside the snapshot, outside the exact artifact inventory, and retries an existing completion only after `verify_completed` confirms the exact requested prefix, current manifest, and all artifacts. A valid marker is not overwritten; directory sync is retried. Invalid or wrong-prefix existing completion remains an error. I found no new defect in the bounded completion-library behavior reviewed here.

## Validation

- `TMPDIR=/Volumes/BuildOffload/tmp/backup-completion-review PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/hosted/tests -p 'test_backup_completion.py' -v`: all 19 tests passed.
- Process death injected after temporary-file fsync but before link left a sibling temporary file; retry created and verified the exact marker successfully.
- Process death injected immediately after the hard link left the marker plus a sibling temporary file; retry verified and returned the same marker successfully.
- A manifest with an incorrect control database SHA-256 failed with `VerificationError: artifact checksum or length mismatch` and did not create `COMPLETE`.
- All probes used isolated temporary directories under `/Volumes/BuildOffload/tmp/backup-completion-review`; the candidate source was unchanged.

## Limits remain

This library is still not wired into `deploy/hosted/backup.sh` or a restore/downloader/retention path. It verifies local manifest/artifact bytes and a marker binding only; it does not establish authenticity, SQLite/Git/schema validity, deletion-journal authority, or successful/complete object-store upload. T23.52 integration must publish the exact marker only after the exact verified artifacts and manifest are uploaded, and preserve the explicit immutability/caller-owned outer-staging assumptions. This review made no cloud calls and gives no deployment or remote-store qualification.
