# E18 — Ledger acceptance publication recovery

A real inbox commit failure left precept-draft and child-intent acceptances
recorded but absent from `--unapplied`; `--apply` did not support either kind.
Repeated lower-level application could allocate a second ledger entry.

- [x] T18.1 Recover accepted ledger effects without duplicate entries  Owner: pool  Est: 120m  verifies: [UC-026]  deps: [T17.1]  acc: [invalid drafts or missing/inactive or uncommitted parents stop before interactive disposal; newly recorded ledger acceptances carry recoverable intent; exact entry bytes and dependencies persist before canonical writes; failed commits and actual pre/post-commit SIGKILL recover through --unapplied/--apply without new decisions or duplicate allocations; actor/time, edited child payloads and unrelated human changes survive; legacy unmarked effects are not blindly replayed; executed tests/skips and fault controls are recorded]

Scope: local-owner CLI precept-draft and child-intent acceptances. Other direction
lifecycle APIs, HTTP publication and repair of older unmarked effects remain
separate. A completed receipt is historical and never overwrites a later edit.

Verified 2026-09-08. New ledger acceptances record pending-effect intent with the
same atomic transaction as their human decision. Exact receipts precede canonical
writes and reserve one entry ID. Commit/marker failures recover without allocating
again. Completed receipts preserve later human work. Legacy unmarked acceptances
are surfaced for inspection, since an existing effect cannot safely be guessed.

The full race suite passed 1,736 cases across 58 packages with six explicit skips.
Vet passed and final lint reported zero issues; four affected CLI cases passed
after the final switch-style correction. Six injected faults were detected and
restored. Actual CLI pre/post-commit SIGKILL for both precept drafts and child
intents preserved one decision and one entry, original actor/time/payload, parent
bytes, and idempotent Git history. No live model calls were made.

DISPOSITION's four transcript operations (14 real HTTP cases) were recaptured and
the manifest refreshed. Apart from dynamic IDs/times, the only semantic additions
were two pending-effect fields in the grouped acceptance response. Seeded replay
identities were aligned to the fresh recordings; the comparator was not weakened.
Evidence: `docs/evals/ledger-publication.json`.
