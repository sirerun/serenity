# T23.48 integration request — dedicated hosted deletion journal

**Owner:** T23.54 infrastructure integration, coordinated with T23.52 retention.

The runtime S3 journal adapter and service configuration fields are implemented
in the T23.48/T23.49 working branch. Production assembly now requires
`deletion_journal_bucket` and `deletion_journal_region`; generation defaults to
1 and can be set with `deletion_journal_generation`. The hosted stack and
example config do not provide these yet, so deploying this service code without
the integration will fail startup.

Add a dedicated, stack-managed journal bucket. Do not reuse the current Backups
bucket: its unscoped lifecycle rule expires current and noncurrent objects
after 30 days, which can remove the append-only journal history.

The stack integration must:

- Enable versioning, encryption, block public access, and TLS-only access; retain
  the bucket on stack deletion/replacement.
- Avoid lifecycle expiration for journal objects. T23.52 must coordinate any
  minimal-metadata retention with proof that all snapshots protected by the
  journal have been permanently purged.
- Enforce conditional object creation (`If-None-Match: *`) for journal writes
  and deny object/version deletion for the `deletion-journal/*` prefix.
- Grant the hosted role only the required bucket listing/version-list and
  object read/conditional-write permissions on this bucket/prefix. Do not grant
  journal delete permission or broad account-wide S3 access.
- Populate `deletion_journal_bucket`, `deletion_journal_region`, and
  `deletion_journal_generation` in the delivered service configuration before
  starting the production service.
- Add template tests for the bucket, policy, IAM resources and delivered config;
  qualify the deployed policy and service startup with task66's approved
  disposable-resource rehearsal.

The disposable S3 qualification at `s3-qualification.json` passed conditional
create, policy enforcement, versioning, concurrent-writer fencing, journal
roundtrip/seal, delete-marker visibility, and cleanup. It does not qualify the
currently deployed stack or its IAM/configuration.
