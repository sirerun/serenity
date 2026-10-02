# Snapshot inspection implementation receipt

This change adds `backup.InspectSnapshot` as a bounded, read-only inspection
of a private local snapshot. It compares the raw manifest bytes with the
caller-approved SHA-256 before parsing or opening artifacts, validates copied
control-database bytes through a read-only SQLite URI, verifies bounded
account and brain inventories, and copies bundles into private scratch to
check their Git heads. It returns the source build reference and embedded
journal watermark as data; it does not authenticate their origin or prove
that either value is authoritative.

Inspection does not call `store.Open`, run migrations, publish a restore tree,
change snapshot files, or decide account eligibility. In particular,
`restore_pending`, `deleting`, and `deleted` are preserved as snapshot metadata.
The manifest digest is a caller-provided byte binding, not a signature or
proof of operator approval. The local declared-byte limit is a refusal bound;
it is not a filesystem quota or a guarantee about temporary Git allocation.
As with the existing private-filesystem trust model, concurrent tampering by
the same effective UID is outside the boundary. Cleanup is rooted in the
validated scratch parent and refuses to recursively remove a replaced scratch
directory.

`Restore` now uses the same context-aware manifest/artifact copy and read-only
database verification helpers. It passes the supplied context through these
steps and bounds the ready-brain inventory query before restore actions.

Validation on the isolated SSD worktree, with Go cache and temporary files on
`/Volumes/BuildOffload/tmp/t23-50-snapshot-inspection-20261001`:

- `go test -race ./internal/hosted/backup -count=1` passed (22.911s).
- Deterministic mutations went red when bypassing the approved manifest digest,
  the artifact checksum, nonblocking FIFO open, pre-open context cancellation,
  and scratch-directory identity check. Mutations were restored byte-for-byte
  before final validation.
- The scratch replacement test renames the invocation's directory, puts
  caller data at the old path, and verifies cleanup refuses it and preserves
  the replacement.

Mutation logs and saved source copies are in
`/Volumes/BuildOffload/tmp/t23-50-snapshot-inspection-20261001/controls/`.
This is a local component implementation only. It does not establish hosted
startup admission, snapshot authenticity, journal generation adoption,
provider closure, lifecycle eligibility, or deployment readiness.
