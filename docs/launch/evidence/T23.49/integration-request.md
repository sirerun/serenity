# T23.49 integration request

Scope: `internal/hosted/backup/**` only (R-hosted-backup). The changes below
touch `internal/hosted/service/service.go` and `internal/cli/hosted.go`,
both outside this task's write scope (R-hosted-assembly / integrator-owned).
This file is the exact patch request; the integrator applies it.

## 1. `backup.Create` and `backup.Request` gained two required parameters

`Create`'s and `Request`'s signatures changed from:

```go
func Create(ctx context.Context, dataDir, destination string) (err error)
func Request(ctx context.Context, dataDir, destination string) error
```

to:

```go
func Create(ctx context.Context, dataDir, destination, buildSHA string, journal contracts.DeletionJournal) (err error)
func Request(ctx context.Context, dataDir, destination, buildSHA string, journal contracts.DeletionJournal) error
```

Reasons (both required per coordinator review, not optional hardening):

- **buildSHA**: `ManifestV2.Source.BuildSHA` must name a real build identity.
  `internal/hosted/backup` cannot import `internal/cli`'s release-injected
  `Version` (`internal/cli` already imports this package; that would be a
  cycle). Passing `""` is fine and safe: `Create` falls back to the
  toolchain-embedded VCS revision (`runtime/debug.ReadBuildInfo`) and fails
  closed if neither is available. It never records a placeholder like
  `"unknown"`.
- **journal**: interfaces.md decision 3 rule 6 requires the deletion-journal
  watermark to be read *before* any data is copied. `Create` never
  substitutes a fabricated empty watermark for a missing journal -- passing
  `nil` is now a hard error. **Task48's production `DeletionJournal` adapter
  does not exist yet**, so there is currently no legitimate value either
  caller can supply. This is the actual current state, not a testing gap:
  until task48 ships, `Service.Backup` and `serenity hosted backup` cannot
  produce a manifest with a real watermark. Keep this draft unmerged until the
  real adapter and call-site wiring are ready. Do not replace the journal with
  a claimed empty history, and do not disable working production backups merely
  to make this draft compile. A recorded rationale is not journal evidence.

### Suggested call-site diff

`internal/hosted/service/service.go` (`Service.Backup`, line ~329 on base
`055eed446`):

```go
func (s *Service) Backup(ctx context.Context, destination string) error {
	s.Gateway.Maintenance.Lock()
	defer s.Gateway.Maintenance.Unlock()
	if err := s.Pool.FlushAll(); err != nil {
		return err
	}
	return backup.Create(ctx, s.cfg.DataDir, destination, cliVersion, journal)
}
```

where `cliVersion` is whatever release-identity string the service already
carries (empty string is acceptable; `Create` falls back to VCS metadata),
and `journal` is a `contracts.DeletionJournal` -- see the blocker above for
what to pass until task48 exists.

`internal/cli/hosted.go` (`backup`/`restore` subcommands, line ~116 on base
`055eed446`):

```go
if action == "backup" {
	return backup.Request(cmd.Context(), dataDir, snapshot, cliVersion, journal)
}
```

`backup.Restore`'s signature is unchanged (`Restore(ctx, snapshot,
destination string) error`); no caller of `Restore` needs any change.

**Dirty canonical state is refused, not flushed, by `Create` itself.**
Per coordinator review, `Create` never runs `git add`/`git commit` against a
brain's canonical working tree (that would sweep unrelated/unreviewed files
into canonical history). It fails closed with a clear error if any brain's
working tree is dirty when bundling starts. This makes the existing
`Service.Backup` sequencing above load-bearing: `Pool.FlushAll()` must
actually complete (queued writer commits landed) before `backup.Create`
runs, or a pending write will make `Create` fail outright rather than
silently being flushed on its behalf. The offline CLI path has no
equivalent flush step today -- calling `backup.Request`/`Create` against a
data directory with any brain mid-write (an unflushed `writer.Queue`) will
fail for the same reason.

## 2. Provisioning dependency resolved for new brains

PR #275 merged as 06fcce44d83b0e451d599410949cb1c0d46c9ce4 and is included in
this branch. Allocating brains now receive a committed canonical baseline before
ready; runtime opening commits model-pinned configuration, including retries.
`TestProvisionedBrainVersion2BackupRestore` verifies untouched and runtime-opened
brains through real Create/Restore with exact HEAD and clean restored trees.

Existing ready brains missing Git are still corruption. Inventory and explicitly
reconcile those cases before rollout; never infer an untouched brain from an
empty directory or silently recreate lost canonical history.

## Test-only fixtures added

`internal/hosted/backup/backup_test.go` defines `fakeJournal`, a
`contracts.DeletionJournal` fixture that reports exactly the watermark it is
constructed with (including the legitimate all-zero "actually empty" case)
and errors on `AppendDeletion`/`Seal` (never called by `Create`).
`internal/hosted/backup/testdata/backupprobe/main.go` is a real fixture used
only by `internal/hosted/backup/faultbarrier_test.go`'s subprocess tests; it
is not wired into any production path.
