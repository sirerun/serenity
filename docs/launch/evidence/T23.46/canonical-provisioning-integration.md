# Canonical provisioning integration — recovery qualification pending

Source: 1a59bfe130cc051ae88bb2d575d221291d70a5f1.

Provisioning creates a committed cache-exclusion baseline under exclusive brain ownership before marking an allocating brain ready. It preserves unrelated staged work and rejects unsafe Git/baseline paths. Existing ready brains are not recreated. The runtime commits its model-pinned configuration even when provisioning already created HEAD, including a retry after config was written but not committed. Commits use exact paths rather than incorporating unrelated staged work.

Red-before-green regressions:
- Ready brain without Git HEAD (main).
- Existing unrelated HEAD mistaken for completed baseline (initial draft).
- First runtime open and interrupted config initialization leave config outside HEAD (before runtime integration).

Final combined command: `go test -race -count=1 -json ./internal/hosted/provision ./internal/hosted/pool ./internal/hosted/identity ./internal/hosted/credential` exited 0; 13 top-level tests passed, no failures/skips. Scoped lint of provision/pool exited 0 with zero issues. The combined check held and released the shared build lease.

Runtime ownership was acquired atomically after authenticated transport and current vacant-ref verification; no existing claim was removed or overwritten.

Remaining acceptance: version-2 create/restore of untouched and first-open brains, full repository CI/review, and explicit treatment of existing ready brains missing Git. No legacy repair, production deployment or complete identity-task acceptance is claimed. Backup-v2 also requires its separately tracked service/journal integrations.
