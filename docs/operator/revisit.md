# Revisit recorded decisions

Run a review sweep against a brain:

```sh
serenity -C /path/to/brain cron revisit
```

A successful run prints `cron revisit: ok (created=N unsupported=N)` and
exits zero. The counts distinguish new cards from prose conditions the sweep
could not evaluate.

The sweep evaluates machine-readable `revisit_if` conditions on active
precepts and creates review cards in the disposition queue. It does not
change a decision, accept an alternative, or execute an action. The original
decision and alternative remain in the Dira ledger unchanged.

Use a weekly timer from the [scheduling guide](scheduling.md), substituting
`revisit` as the job name. The command itself runs once and exits.

## Conditions

`revisit_if` belongs to an alternative in a Dira entry. It remains a string,
so existing entries stay compatible with Dira. The sweep recognizes two
explicit forms:

| Form | When a review becomes due |
| --- | --- |
| `after:2026-12-01T00:00:00Z` | At or after the RFC 3339 timestamp. |
| `claim_state_changed:project-a/deadline_on/2026-12-01` | When the states or identities of claims at this exact key change after the first observed baseline. |

The claim key is `subject_slug/predicate/normalized_object_key`. Percent-encode
each component separately if it contains a slash or other reserved character;
the two separator slashes remain literal. Use the stored normalized object key,
not an arbitrary display label. Changes to unrelated claim keys do not trigger
the condition.

The first observation of a claim key establishes a baseline without creating
a review. Subsequent sweeps compare against that baseline. A change that
occurs and reverses entirely between sweeps cannot be detected.

Ordinary prose conditions remain available for human interpretation. The
sweep reports them as skipped rather than guessing their meaning. A malformed
condition beginning with a recognized prefix is an error. New conditions on
an accepted precept follow the normal human-confirmed supersession workflow;
the sweep never edits the precept to add them.

## Duplicate prevention and persistence

Each condition has a persisted checkpoint, including `last_revisited_at`.
The checkpoint and its review card are saved in one transaction. Concurrent
sweeps and restarts therefore do not create duplicate cards for the same
observation.

There is a rolling seven-day cooldown after a review fires. An outstanding
review suppresses another card, including after the cooldown expires. A key
change during the cooldown remains eligible for a later sweep. Once a review
is disposed, a still-elapsed deadline can produce a new review after the
cooldown; disposing a review does not supersede its original condition.

Checkpoints and review cards are runtime state in the brain's local SQLite
database. An ordinary index rebuild preserves them. Deleting the runtime
database also deletes this history: deadline conditions can fire again, and
claim conditions establish fresh baselines. Preserve runtime state when
backing up the review queue.

The sweep makes no model calls. It checks the claims currently available in
the brain's index; ingest and reconcile new evidence before expecting it to
affect a review condition.
