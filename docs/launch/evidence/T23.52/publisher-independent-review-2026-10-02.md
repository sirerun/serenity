# Serenity T23.52 Publisher/Downloader Independent Source Review

**Verdict: CLEAR for the reviewed source slice. T23.52 remains partial and is not accepted as a complete task.**

## Reviewed scope

- Base: `c3d491b03980310d7d59688d092b8119640e5250`
- Review HEAD: `2ebc9c29e23bd7835d75c7d3a137e6d40d629b96`
- Publisher source commit: `b2b1a7982508d89f23f68f4e77191b247e015f46`
- Source: `deploy/hosted/backup_publish.py`, SHA-256 `30e3ac48150784b5946ef4fe9b92269d2786d3fd8f0c556e15623d529cf6559d`
- Tests: `deploy/hosted/tests/test_backup_publish.py`, SHA-256 `c4c84add463d18643ad5ef7e8b6b09b87d48ad5223daaa62507e0c5bc4ee05e3`
- The diff from base is limited to the publisher, its tests, and its partial source receipt. The isolated full clone was clean at the requested HEAD before and after review.

## Findings

No blocking or non-blocking correctness/security findings in the reviewed source slice.

The publisher requires caller-owned private staging, validates the v2 manifest and exact staged inventory, applies the supported 1,000-brain / 1,001-artifact / 1,003-object envelope, and streams with bounded chunks. It checks for an initially empty remote prefix, writes artifacts and manifest before `COMPLETE`, reads back and hashes remote artifact bytes before the commit object, and reconciles an ambiguous final write only after exact completion readback and complete-object verification. The downloader checks the completion-to-manifest binding, exact remote inventory, transfer bounds, and artifact hashes, then returns a private staged byte tree without granting activation authority.

The source explicitly treats storage as a caller-supplied interface. Complete provider version/delete-marker enumeration and atomic put-if-absent semantics are adapter obligations, with versioned S3 qualification still required. No production transport, runner, retention authority, purge, install, activation, or live qualification is present in this source milestone. Those remain open and prevent whole-task acceptance.

## Verification

Using the required owner-only SSD-backed fixture at `/Volumes/SerenityPrivateFixture20261001/tmp`, with `PYTHONDONTWRITEBYTECODE=1` and `ResourceWarning` promoted to errors:

- `deploy/hosted/tests/test_backup_completion.py -v`: 20 tests passed.
- `deploy/hosted/tests/test_backup_publish.py -v`: 18 tests passed.
- `git diff --check c3d491b03980310d7d59688d092b8119640e5250...HEAD`: passed.
- Three temporary mutation controls went red as intended: early `COMPLETE` commit left a completion object after readback failure; omitted pre-commit artifact hash comparison left `COMPLETE` after corrupt remote bytes; relaxing the 500 GB ceiling admitted a 501 GB configured bound. Each mutation was restored in the isolated clone, and the publisher source hash returned to the reviewed SHA above.

Test and control logs are under `/Volumes/BuildOffload/serenity-publisher-independent-review-20261002/`. SHA-256 values:

- `completion-tests.log`: `b62d0b5cbcb4e33432ba7aebbb2b4d041dc79993e0f3ba742b3d6d9416d75411`
- `publisher-tests.log`: `5b635cbee2045f1b54146d966d4e357b9c1d7505900a7ca5a81a8aea1a39c787`
- `red-premature-complete.log`: `3237970b3b3a5fb4b9336688a7e7629aa147c067a3377a773acf25316619ae1f`
- `red-omitted-precommit-hash.log`: `5d3a0e1f82d15e783051a5e67143a916913658d80dfc8b57d120b8bd2691956c`
- `red-relaxed-snapshot-bound.log`: `af9d00a13040854bc9f4f3ea153c2bb50be509b40d87296b85739d95f364f334`

Ajent tooling was unavailable; the local project feed and Serenity coordination file were checked read-only.
