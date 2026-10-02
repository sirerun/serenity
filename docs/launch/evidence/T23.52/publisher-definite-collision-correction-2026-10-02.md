# Publisher definite completion collision correction

Source-only coordinator correction against main f4738933649bf399f098949430ce6def3699ee8b. Canonical T23.52 claim00ba7bb354207539fbdc341d9ee1f46272985a22 and R-hosted-backup-ops9513e31eb68779012d272341f87ac35ebf8a6071 own backup_publish.py, its existing test file and this receipt. Existing installed backup scripts, units, retention helpers and Go files remain unchanged.

The publisher previously caught every final COMPLETE write exception and attempted exact readback reconciliation. If a competing writer installed the exact intended marker and our conditional write returned definite ObjectExists, publication returned success. A definite occupied key is not an ambiguous lost-response write.

The new public publisher test simulates that collision with an otherwise exact verified remote payload. Before changing production source it exited1 with “ObjectExists not raised.” The three-line correction propagates ObjectExists before the existing ambiguous-write handler. The new test passes, and the existing lost-response success and refusal controls remain green. This distinguishes a definite collision from unknown write outcome without weakening byte verification or COMPLETE-last ordering.

Coordinator validation: all87 hosted Python tests pass in8.974s with ResourceWarning treated as errors and the explicitly owned private fixture; Ruff and diff checks pass. Go/module files are byte-unchanged from qualified main f473893. Independent review is still required before merge. No AWS request, provider adapter installation, purge, activation or spend action occurred; fullT23.52 remains open.

Evidence SHA-256:
- root-publisher-definite-collision-red-20261002.log: b6ae8d6753f504c692999f88c58a548b608997c912ddb47e5200a1f9931fbd54
- root-publisher-collision-tests-20261002.log: 51197272cc7e6b6949f901ad158056f38667c409adecbfa6d58544edf26e6605
- root-publisher-collision-ruff-20261002.log: 82b3e6a6c090a57601d22943bd23fca9218d1031dbe5a7b754092f9a156b4f18
- root-publisher-collision-diff-20261002.log: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855

Independent review now clears exact source68557003d6cbf7b4ec8e01217967793d5743856a after87 hosted tests, the focused collision/lost-response pair and a genuine handler-removal mutation RED. The review copy was restored byte-exact. Final added review documentation does not change qualified source. Full task and production gates stay open.
