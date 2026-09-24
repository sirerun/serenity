# Canonical provisioning integration — recovery qualification pending

Source: 1a59bfe130cc051ae88bb2d575d221291d70a5f1.

Provisioning creates a committed cache-exclusion baseline under exclusive brain ownership before marking an allocating brain ready. It preserves unrelated staged work and rejects unsafe Git/baseline paths. Existing ready brains are not recreated. The runtime commits its model-pinned configuration even when provisioning already created HEAD, including a retry after config was written but not committed. Commits use exact paths rather than incorporating unrelated staged work.

Red-before-green regressions:
- Ready brain without Git HEAD (main).
- Existing unrelated HEAD mistaken for completed baseline (initial draft).
- First runtime open and interrupted config initialization leave config outside HEAD (before runtime integration).

Final combined command: `go test -race -count=1 -json ./internal/hosted/provision ./internal/hosted/pool ./internal/hosted/identity ./internal/hosted/credential` exited 0; 13 top-level tests passed, no failures/skips. Scoped lint of provision/pool exited 0 with zero issues. The combined check held and released the shared build lease.

Runtime ownership was acquired atomically after authenticated transport and current vacant-ref verification; no existing claim was removed or overwritten.

Remaining acceptance: remote PR CI, and explicit treatment of existing ready brains missing Git. No legacy repair, production deployment or complete identity-task acceptance is claimed. Backup-v2 also requires its separately tracked service/journal integrations.

Cross-branch fixture integration: source 866bffe43f820ae38aeb544e65e5e1bb67a632f2 combines backup dfa83cf and provisioning/runtime through 1a59bfe. `go test -race -count=1 -json ./internal/hosted/backup -run TestProvisionedBrainVersion2BackupRestore` exits 0 for untouched and runtime-opened brains, preserving exact HEAD and clean restored trees without embedding calls. Scoped backup lint reports zero issues. Production journal and coordinated service concurrency remain unqualified.

Independent correctness review found and drove a fix for missing persistent hosted Git identity: a real remember/FlushAll regression with global/system Git config disabled previously used machine identity; it now commits as the hosted identity. Test cleanup releases the runtime even on assertion failure. The fixture verifier expects exactly two initialization commits and still rejects an extra history commit.

Final source 0e51058309427858769c9e624166df616dba3ed2: full `go test -race -count=1 ./...` passed in 75 packages (5 without tests) under the shared lease. The only subsequent source change was deferred release in the regression test; that affected test was rerun with race detection and passed. Final scoped lint for provision/pool/fixtureprep reports zero issues. PR CI remains pending.
