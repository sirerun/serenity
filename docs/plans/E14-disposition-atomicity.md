# E14 — Disposition state atomicity

An audit on 2026-09-08 forced a real SQLite state-write failure. The item stayed
pending but its history claimed an accepted decision; retrying that key could
then replay a decision that never committed. A second regression synchronized
two independent store handles after reading one pending item. Both returned
successful conflicting verdicts. The existing race test shares one Store mutex,
so it does not cover this boundary.

- [x] T14.1 Make disposition transitions atomic and preserve competing decisions  Owner: pool  Est: 90m  verifies: [UC-012, UC-026]  deps: [T10.2, T11.3]  acc: [a real SQLite failure leaves both item and history unchanged and a retry can succeed; independent store handles yield one winning terminal verdict and one matching conflict response; queue aging and result bookkeeping cannot overwrite a newer human decision; route metadata is recorded with its decision; regression tests exercise real database transactions and meaningful negative controls with named execution counts and skips]

This task hardens runtime disposition state. It does not authorize multiple
canonical brain writers or change ADR 012's single-writer contract. Filesystem
publication remains governed by the existing inbox journal and writer lock.

Verified 2026-09-08. Item snapshots and append-only decision history commit in
one SQLite transaction. A lost snapshot cannot overwrite another handle's
winning verdict. Queue aging, resurfacing and applied-result bookkeeping use the
same guarded update primitive. Capture route metadata commits with its verdict;
new precept-draft routes retain a pending-effect marker until deterministic
follow-on staging completes. Legacy completed routes are not duplicated.

The final race suite executed 1,655 passing cases in 58 packages, six explicit
skipped cases; lint reported zero issues. Real SQLite triggers tested failure
of either side of the transaction and later follow-on/completion failures.
Real SIGKILL inside the item transaction left pending state with zero history;
SIGKILL after commit left one disposed decision and one history event. Retrying
both cases resulted in exactly one decision and preserved original evidence.
Three deliberate faults were detected: removing snapshot checks, splitting
history/state writes, and disabling follow-on recovery on replay.

Evidence: `docs/evals/disposition-atomicity.json`. The optional
`route_effect_pending` field is documented in the DISPOSITION v1 item schema.
Capture routing remains an internal Store API; no new CLI routing surface is
claimed. Existing legacy orphan history and incomplete unmarked legacy routes
are not automatically reconstructed or rewritten.
