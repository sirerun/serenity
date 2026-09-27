# T23.49 integration record

The original caller request below has been implemented in the T23.49 working
branch: `Service.Backup` passes the configured journal and build identity;
the service uses the production S3 journal or the explicit local filesystem
journal; and offline CLI backup requires an explicit journal location.

Production deployment wiring remains open. The current stack exposes only the
backup bucket, whose unscoped 30-day lifecycle rule would also expire journal
objects. It also lacks conditional-write enforcement, delete denial, and
`s3:ListBucketVersions`. The hosted config has no journal bucket/region yet.
Do not configure the existing backup bucket as the production journal until
the T23.48 infrastructure request adds a protected journal bucket and supplies
its bucket and region to service config.

The request details below are retained as implementation history and resolved
interface context.

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
  `nil` is now a hard error. This paragraph originally recorded that the
  task48 adapter and callers were missing. T23.48 now provides the S3 adapter
  and service/CLI wiring. Production use still waits on T23.54's dedicated
  protected bucket and delivered config. Do not replace the journal with a
  claimed empty history or point it at the current 30-day Backups bucket.
  A recorded rationale is not journal evidence.

### Original call-site sketch (implemented in the working branch)

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

where `cliVersion` is whatever release-identity string the service carries
(empty string is acceptable; `Create` falls back to VCS metadata), and
`journal` is the configured `contracts.DeletionJournal`.

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
