# Verified snapshot lease implementation receipt

## Result

The backup package now has a durable verified snapshot lease producer. Staging
retains the exact inspected manifest and artifact bytes under a private lease
root; candidate extraction and restore read only those retained bytes. Pin
attempts bind the exact owner reservation, lease, digest, attempt, and pin ID.
Restart operations resolve and reopen the durable pin through the lifecycle
authority. Reconciliation repairs interrupted pin and release transitions and
keeps a compact, checksummed release tombstone until owner completion succeeds.

The existing `InspectSnapshot` and `Restore` signatures and their successful
behavior remain available. Restore shares the existing validation and
no-overwrite path, while verified restore uses a bounded private scratch copy
of the retained artifacts. Candidate SQL preflights byte lengths and row counts
before scanning text into Go and preserves SQL NULL separately from empty text.

## Behavioral evidence

The focused public lease controls pass with real owned APFS-backed fixtures:

- Staging survives removal of the original snapshot source and restores the
  exact retained bytes.
- A changed retained artifact is rejected before destination creation.
- Replacing the store root is rejected.
- Pin, close, restart, resolve, reopen, and find preserve the exact pin.
- An interrupted `RELEASING` record is recovered through the durable release
  tombstone and owner completion path.
- Retrying canceled attempt N leaves pending attempt N+1 unchanged, while
  cancellation of the exact durable pin is refused.

Before correction, the focused controls exposed strict-metadata rejection of
all staged records and later exposed fixed-point checksum encoding and fixture
attempt-version issues. Those failed outputs are retained as RED evidence;
corrected focused controls passed.

Focused lease controls passed using the root Python build-stage runner with
exact lease `90322d7cf4d0467c03de6468f80ba903f30e7f73`, released by the runner.
The full backup package tests also passed with lease
`fad31e1d8eb634c0dd890699f5b9d7d445af486e`, released by the runner. Their full
stdout, stderr, source fingerprints, load samples, and release metadata are in
the assigned external evidence bundle.

## Remaining qualification

Focused race, vet, and lint checks remain pending, along with independent exact-
head review and coordinator-owned full-module qualification. No source
acceptance, integration, hosted-startup, provider, deployment, physical
capacity, or production lifecycle authority is claimed by this receipt.
