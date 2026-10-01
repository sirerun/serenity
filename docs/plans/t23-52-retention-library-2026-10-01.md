# T23.52 retention library receipt — 2026-10-01

Implemented on main #332 baseline `230234c969058807236af76d91da0967357a37cd`, following the planner design in `5396048f478c419d2fe8080cd0921819b6dd3c5c`. This is an unused local library candidate only. It does not wire a purge worker, timer, CLI, IAM policy, lifecycle rule, upload path, or production backup service. No AWS calls or external object operations were made.

`deploy/hosted/backup_retention.py` defines a storage adapter seam plus complete inventory, canonical plan serialization/loading, and an explicit apply operation. The fixed namespace is `snapshots/`; every returned key is checked as exactly `snapshots/YYYYMMDDTHHMMSSZ/...`. One timezone-aware UTC clock establishes the 29-day snapshot cutoff and 1-day incomplete-multipart cutoff. The planner traverses every versions/delete-markers page and every multipart page with both continuation markers, rejects malformed or repeated tokens and foreign/malformed keys, and applies bounded page, item, part, and serialized-plan limits by failing rather than truncating.

The frozen plan binds bucket, expected owner, prefix, generation time, both cutoffs, every exact version/delete-marker identity and multipart upload identity, timestamps, and a SHA-256 digest. `apply` requires the reviewed digest and matching bucket/owner, the exact still-current cutoffs, and a fresh complete inventory identical to the approved inventory. It rechecks eligibility, deletes exact version IDs in batches of at most 1,000, refuses any deletion error or incomplete/unexpected response, aborts only uploads initiated at least one day earlier, checks that their parts are gone, then takes another full inventory and errors if eligible versions, markers, or uploads remain. The returned report includes deletion counts and oldest surviving snapshot/LastModified timestamps and ages.

Validation passed locally with a fake adapter:

- `PYTHONDONTWRITEBYTECODE=1 TMPDIR=/Volumes/BuildOffload/tmp python3 -m unittest tests/test_backup_retention.py -v` — 16 tests passed.
- `ruff check deploy/hosted/backup_retention.py tests/test_backup_retention.py` — passed.
- `git diff --check` — passed.

Tests cover dry-run behavior; versions plus delete markers; exact 1,000-item batching; pagination and malformed/repeated markers; strict scope and timestamps; item/resource limits; frozen-plan roundtrip, digest, and cutoff approval; changed inventory; per-item failure; post-delete proof; and multipart part verification. The fake adapter is not an AWS qualification.

The caller must provide an adapter that sends the expected-owner constraint on every operation, caps response page sizes, normalizes genuinely empty list fields to empty arrays, stores reviewed plan bytes privately with restrictive permissions, and controls concurrent snapshot writers during apply. No production evidence establishes these caller properties. The existing backup shell/uploader remains unchanged; the 31-day backup-expiry objective and T23.52 acceptance remain open until scheduled production purge behavior and operational reporting are integrated and qualified. The deletion-journal prefix is not read or modified.
