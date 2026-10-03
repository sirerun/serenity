# Verified snapshot lease implementation receipt

## Implementation status

The backup package implementation is in progress. The current draft gives staging
retains the exact inspected manifest and artifact bytes under a private lease
root; candidate extraction and restore read only those retained bytes. Pin
attempts bind the exact owner reservation, lease, digest, attempt, and pin ID.
Restart operations resolve and reopen the durable pin through the lifecycle
authority. Reconciliation repairs interrupted pin and release transitions and
keeps a compact, checksummed release tombstone until owner completion succeeds.

The existing `InspectSnapshot` and `Restore` signatures and their successful
behavior remain available. Verified restore shares the existing validation and
no-overwrite path and performs one retained-artifact verifier copy into the
lease root, protected by a durable, checksummed owner intent. Candidate SQL
preflights byte lengths and row counts before scanning text into Go and
preserves SQL NULL separately from empty text.

`MaxRestoreScratchBytes` limits logical raw verified copies retained under the
private lease root, including Candidate's control database projection and the
single `RestoreVerified` verifier copy. Aggregate retained-artifact and
metadata limits include their durable reservations. This does not bound
unpublished destination staging, expanded Git checkouts/repositories, the
restored database/index/WAL, or filesystem physical quota. Reconcile removes
only exact owner- and inode-verified lease-root scratch. A recovery/applier
owner must independently account, reconcile, and quota-guard destination
outputs before production enables it. The T23.44 physical quota gap remains
open.

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
- A real child-process exit immediately after a durable STAGING, Candidate, or
  RestoreVerified scratch intent
  intent is recovered after reopening; retained budgets account for the intent,
  and Reconcile removes only the inode-verified owned scratch.
- Candidate checksummed private projections are opened through the retained
  descriptor. A SQL-valid in-place change between digest and SQLite open is
  rejected, and Candidate intent metadata consumes aggregate metadata budget.
- Reconcile preserves a nonempty unknown scratch directory with no valid intent.
- Store construction requires the opaque preflighted root/lock identity; zero,
  cross-root and replaced-lock identities fail. A subprocess blocked on the old
  lock descriptor is rejected after the lock pathname is replaced.
- Cancellation requires an exact, one-use proof tied to the active callback,
  identity and held store/lease lock descriptors. Wrong tuple and cross-store
  identity attempts fail; callback-retained aliases expire. Reconcile/Close
  reject unknown/conflicting owner lists and a COMMITTED owner paired with a
  local STAGED record before cleanup.
- Release journal records and RELEASED tombstones retain reservation and
  attempt versions so an absent live lease cannot alias another owner attempt.

Before correction, the focused controls exposed strict-metadata rejection of
all staged records and later exposed fixed-point checksum encoding and fixture
attempt-version issues. Those failed outputs are retained as RED evidence;
corrected focused controls passed.

The current package source fingerprint is
`9e0122ec1429882d4f926d0b8d950613fab88f299fccd1ac6b1f8f75796f2a69`.
Full backup-package tests, package race, vet, lint, and the corrected proof and
identity control set passed at that fingerprint. Two compiled behavioral
mutants failed on their intended assertions: removing proof tuple/lifetime
checks, and removing expected-store identity enforcement. All stages won and
released their exact shared build leases. Full stdout/stderr, source
fingerprints, load samples, commands and release metadata are in the assigned
external evidence bundle.

## Remaining qualification

The backup-owned opaque cancellation proof and mandatory store identity were
added under coordinator-frozen `backup-pin-absence-v1`; the production owner
factory and end-to-end recovery lifecycle remain separate and unimplemented.
Full-module qualification and independent exact-head review are coordinator
owned and still required. The T23.44 destination-output/physical-quota gap
remains open. This receipt is not source acceptance or production lifecycle
authority.
