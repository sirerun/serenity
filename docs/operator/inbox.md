# Review and recover reconciliation decisions

Run `serenity -C /path/to/brain inbox` to review pending or deferred items. Use
`j`/`k` to move, space to accept, `e` to edit a single reconciliation proposal,
`r` to reject with a note, `d` to defer, and `q` to leave. Ordinary terminals
currently require Enter after a key. Grouped items record separate decisions;
an error stops the group before later members are decided.

For reconciliation, accept and edit now publish the accepted claim to canonical
files and commit it before moving to the next item. Fence claims retain the old
row as superseded; shard claims append a replacement and refresh the page's head
rows. Existing prose outside claim and metadata blocks is preserved. Reject and
defer do not change canonical claims.

The recorded decision and its published effect are separate steps. If publication
fails, the decision remains accepted with its original actor and time. The normal
inbox lists unfinished publications even though those decisions are no longer
pending. These commands inspect and retry them explicitly:

```sh
serenity -C /path/to/brain inbox --unapplied
serenity -C /path/to/brain inbox --apply ITEM_ID
```

A retry uses the already recorded verdict; it does not create another decision or
change the reviewer. Publication receipts under `.serenity/reconcile/` retain
before/after bytes until completion. Keep this local runtime directory when
recovering an interrupted review. It can contain private canonical content and
must remain excluded from Git, like the disposition database itself. Deleting
runtime state is not a way to recover an unfinished publication.

## Resolve a stopped publication

An uncommitted edit to a target page or shard pauses publication. Preserve and
review that edit first. Committing prose that leaves the prior claim unchanged
allows the original proposal to be retried. If the prior claim's value, metadata
or lifecycle changed, the stale proposal is refused; generate a fresh proposal
against the current canonical claim instead. A committed deletion is also
refused. The unfinished decision remains visible as history of the attempted
approval; there is no automatic reversal or replacement-decision command.

Once publication starts, every planned file must match its original or intended
bytes. A retry can finish a partial write or failed Git commit, but it will stop
if a human changed those bytes in the meantime. Do not discard those changes to
make a retry pass. Resolve the competing edit deliberately and preserve a copy
of any work that needs to be reconciled separately.

A killed Git process can leave an `index.lock`. Confirm that no Git process is
still operating on that brain before resolving an orphaned lock. Serenity does
not remove Git locks automatically. Its own reconciliation lock is a kernel lock
on macOS and Linux and releases when the process dies.

A completed receipt records a historical application. Repeating `--apply` does
not replay the old approval over a later human edit or retraction.

## Derived index and scope

After new successful publications, the CLI rebuilds the derived index once for
the session, including when a later item fails. This uses the normal rebuild
contract: vectors are regenerated separately by `serenity extract` with the
configured model providers. Inbox publication itself makes no model call. If
index rebuilding fails, the CLI reports that canonical changes were committed
and directs you to `serenity sync`; it does not pretend the publication rolled
back.

This recovery path covers local-owner CLI reconciliation approvals. Other inbox
item kinds retain their existing handlers. Canonical publication through DISPOSITION HTTP and recovery of older
versions' premature `AppliedClaimID` markers are separate surfaces. Receipts do
not provide a multi-file filesystem transaction against arbitrary simultaneous
human writes; byte checks reject observed intervening changes, and each file is
replaced atomically.


Extraction-produced low-confidence items have an explicit typed-assertion and
confirmation flow. They share `--unapplied` / `--apply ID` recovery after a confirmed
human decision. See [extraction review](extraction.md#low-confidence-observations)
for provenance, cancellation and replacement behavior.

## Decision durability

A disposition item's state and its append-only history now commit together.
A failed database write leaves neither half of a new decision behind. Independent
runtime-store handles cannot both win conflicting terminal verdicts, and aging,
resurfacing or result bookkeeping cannot overwrite a newer human decision.
The canonical brain still has one writer under ADR 012; this database guarantee
does not introduce concurrent canonical file writers.

If a process stops before the decision transaction commits, retry the decision.
If it stops after commit but before acknowledging it, the same idempotency key
replays the stored decision. Filesystem publication still uses the separate
`--unapplied` / `--apply` recovery described above.

For callers of the internal capture-routing Store API, new precept-draft routes
carry `route_effect_pending: true` until their follow-on queue item is staged.
Retrying the recorded route recovers that effect once, retaining any subsequent
human review of the child. This marker is optional in the DISPOSITION v1 item
schema. Completed legacy routes without it are preserved rather than inferred
again; old orphan history and incomplete unmarked routes need explicit inspection.
This change adds no capture-routing CLI command.


## Recover paused writes into review

Opening interactive `serenity inbox` imports paused writes before listing review
items. The human and proposed machine versions remain in each dirty-edit item's
payload. Repeating the same conflict does not reset a prior rejection, edited
payload or reviewer; changed conflict evidence creates a separate review item.
The import itself never writes or commits the human's canonical file.

Records being transferred live under `.serenity/pending/.claimed/`. If a process
stops or the review database cannot accept an input, keep that directory and open
interactive inbox again after resolving the failure. Recovery consumes the claimed
record independently of any newer input under the original filename. A malformed
record remains available for inspection rather than being silently discarded.
The incoming filename still represents the producer's latest complete snapshot;
claimed snapshots are immutable until successfully staged.

`inbox --parked` and `inbox --unapplied` remain read-only views. They do not import
pending files, and `--apply` only retries its named canonical publication. These
handoff guarantees concern review staging; importing a dirty-edit item does not
publish either version of its canonical file.
