# T23.49 backup completion library independent review — 2026-10-01

Reviewed commit: `b054456f6e7fdc1ae4aa56ae0ffaf09cd89f2d60` (`deploy/hosted/backup_completion.py` and `deploy/hosted/tests/test_backup_completion.py`). Review checkout is based on that exact commit. Verdict: **useful bounded library candidate, with one reproducible crash-retry blocker; not integrated or sufficient for T23.49 acceptance.**

## Review result

The helper correctly binds a v2 manifest's raw SHA-256 and exact snapshot prefix into `COMPLETE`, rechecks every listed artifact's length and digest, rejects unlisted/missing files, validates timestamped prefixes, rejects duplicate JSON keys and case-folded duplicate/reserved names, and reads files with `O_NOFOLLOW | O_NONBLOCK` plus regular-file and mutation checks. Staging is required to be a private directory. Completion uses a same-directory hard link, so an existing `COMPLETE` cannot be overwritten, and it fsyncs the marker and directory on the normal path.

**Blocker — crash leaves self-generated staging debris and makes retry fail.** `create_completion` writes `.completion-*` inside the snapshot directory, then fsyncs and links it to `COMPLETE` (`backup_completion.py:140-155`). The exact inventory check rejects any such filename on a retry (`:121-134`). I reproduced process death immediately after the temp-file fsync and before `os.link`: the directory retained `.completion-*`, and a subsequent `create_completion` failed with `VerificationError: snapshot contains missing or unexpected artifacts`. I also injected process death immediately after `os.link`: both `COMPLETE` and `.completion-*` remained, and `verify_completed` rejected the marker with the same unexpected-artifact error. Use a same-filesystem temporary outside the inventory directory, or add narrowly validated cleanup/recovery for this helper's own temp names before inventory checking; cover both crash windows with fault-injection tests. Preserve no-overwrite semantics and fsync the final directory entry.

## Validation performed

- `TMPDIR=/Volumes/BuildOffload/tmp/backup-completion-review PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/hosted/tests -p 'test_backup_completion.py' -v`: all 15 tests passed.
- Isolated subprocess crash injection after temporary-file fsync reproduced the retry failure; a second injection after the hard link reproduced verification failure with `COMPLETE` present. The injected processes and all temporary data were confined to the review scratch directory; the candidate source was unchanged.

The existing tests cover normal round-trip and mode, tampered manifest/artifact, wrong prefix, duplicate/unsafe paths and brain inventory, symlink and FIFO, unexpected artifact, malformed version/duplicate keys, truncated/extended marker, and marker no-overwrite. The crash window is not covered.

## Integration and contract boundary

This is an unused local-library helper. `deploy/hosted/backup.sh` still uploads the snapshot recursively and then writes/uploads a plain timestamp marker; it does not call this helper. The helper itself explicitly does not establish authenticity, manifest/SQLite/Git/schema correctness, journal authority, or upload success. It verifies local bytes only. The caller must validate backup semantics and journal completeness independently, keep the staged tree immutable across verification and transfer, upload the bound manifest/artifacts, and publish the exact `COMPLETE` bytes last. The current helper alone proves none of those remote properties.

Do not treat this commit as T23.49 complete or as a restore verifier. Require the crash-retry correction and integration wiring before relying on its marker. T23.48 real deletion-journal writers, T23.50 restore adoption/fencing, and T23.54 scoped object-store credentials/IAM remain separate requirements; no cloud/provider call or deployment qualification was performed here.
