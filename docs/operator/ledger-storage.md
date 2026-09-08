# Decision ledger storage

The decision ledger lives in real directories under `.dira/entries/`. Entry names
must use Dira IDs such as `dec-0001`, and the ID inside a file must match its name.
Reads and writes reject symlinked ledger directories or entry files. An unsafe
ledger produces an explicit error in `serenity check`; it is not treated as an
empty set of constraints or a successful check.

Each new or replacement entry is encoded and synced before publication. Creating
an existing ID reports a conflict; replacing an entry preserves its permissions.
Readers see a complete old or new entry, with the version taken from the same
open file descriptor as its bytes. Deletion also syncs the containing directory.
Only the direction subsystem writes ledger entries, as before.

A killed writer can leave `.serenity-*.tmp` staging files inside the entries
directory. These are unpublished artifacts and never appear as ledger entries.
Preserve them while inspecting an interrupted operation. Resolve any cleanup only
after confirming that no writer is using the ledger. Canonical entries remain
complete; an interrupted create may have published no new entry at all.

These guarantees apply to individual file operations. A staged-to-accepted
transition, supersession involving multiple entries, and Git commit are separate
steps. File atomicity alone does not claim that a whole decision workflow or its
inbox bookkeeping completed. ADR 012 still requires a single canonical writer.
