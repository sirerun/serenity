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
  produce a manifest with a real watermark. Recommended interim options, in
  order of preference:
  1. Block `Service.Backup` / the CLI `backup` action from running until
     task48's adapter is wired (return a clear "backup not yet available"
     error at the call site) -- keeps this explicitly `BLOCKED`, matches the
     dependency graph (`K[49] --> P[52]`, `D[48] --> P[52]`) already showing
     48 gates the later purge/backup-ops task.
  2. If David/coordinator wants `Create` exercisable in production before
     task48 lands, an explicit, clearly-labeled "no deletions have ever run
     on this host" journal implementation could be wired *by the integrator*
     with a recorded, reviewed rationale -- this worker will not author that
     implementation itself, since silently normalizing "no journal" to
     "empty journal" from inside this package is exactly the fabrication the
     coordinator's review flagged.

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

## 2. Cross-owner provisioning gap (not a file this task can patch)

`internal/hosted/provision/provision.go`'s `Provisioner.finish` marks a brain
`ready` right after creating its (empty) directory. `internal/hosted/pool`'s
`Acquire` only initializes the canonical `.git` repository and its baseline
commit the first time a brain is actually opened (lazily, on first gateway
request). T23.49's accepted contract requires a ready brain missing its Git
repository to fail backup as corruption, with no exception for an
apparently-untouched directory (an emptied directory is not proof a brain
was never used). This package now enforces that strictly
(`buildBrainArtifact` in `internal/hosted/backup/backup.go`), which means:
**a brand-new signup that has never connected/used its default brain will
fail `backup.Create` today**, until provisioning is changed to create the
canonical baseline eagerly (as part of the allocating-to-ready transition,
under brain ownership) rather than lazily on first pool access.

This worker does not own `internal/hosted/provision/**` or
`internal/hosted/pool/**` (R-hosted-identity / R-hosted-runtime) and has made
no change there. Per the coordinator's own team-board plan (2026-09-24
13:25Z/13:32Z entries), the fix belongs to the identity/provisioning owner
(T23.46) with a shared baseline helper reused by `pool.Acquire`, plus a
read-only legacy-brain inventory pass before any reconciliation. Recording
here only so backup-v2 rollout is not scheduled ahead of that fix.

## Test-only fixtures added

`internal/hosted/backup/backup_test.go` defines `fakeJournal`, a
`contracts.DeletionJournal` fixture that reports exactly the watermark it is
constructed with (including the legitimate all-zero "actually empty" case)
and errors on `AppendDeletion`/`Seal` (never called by `Create`).
`internal/hosted/backup/testdata/backupprobe/main.go` is a real fixture used
only by `internal/hosted/backup/faultbarrier_test.go`'s subprocess tests; it
is not wired into any production path.
