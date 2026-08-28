# brain-fixture

A `.dira` ledger for T3.14's CI conformance job (`docs/plans/E3-m3-direction.md`):
proof that the real `dira` binary from `kazi-org/dira`, installed unmodified at
the pin in `internal/dira/PIN`, still runs its `check`, `why`, and `brief`
verbs the way this repo depends on. This is a CLI-conformance test of
upstream dira, not a test of anything Serenity owns — Serenity's own plan
checker (T3.5/T3.7) is a separate matcher over `internal/direction/`.

## Provenance

The nine entries under `.dira/entries/` are a byte-identical copy of
`internal/enforcer/testdata/ledgers/daemon/` from `kazi-org/dira` at the pinned
commit — dira's own fixture for testing its lexical `check` matcher, including
the exact ledger content the `dira check` demo in dira's README and
`.agents/product-marketing.md` §6 is built on. Reusing it here means the
expected outputs below are outputs dira's own test suite already proves
correct at this pin, rather than a second, hand-derived guess at its lexical
matcher's behavior.

`.dira/cache/` (the SQLite read cache `dira` builds on first use) is derived
and gitignored, exactly as it is in a real ledger.

## What running against it proves

Verified by `scripts/verify-dira-cli.sh`, wired into CI as the
`dira-cli-conformance` job:

- `dira check -C testdata/brain-fixture "write the checkpoint file atomically"`
  exits 0 and prints `✓ no conflict with 6 enforced entries` — the plan
  touches nothing any entry here rejects.
- `dira check -C testdata/brain-fixture "add a background daemon to track run state"`
  exits 2 and cites `dec-0060`'s rejected `"a daemon"` alternative verbatim,
  including its `why_not` and `revisit_if`.
- `dira why -C testdata/brain-fixture dec-0060` exits 0 and prints the entry's
  chain unmodified.
- `dira brief -C testdata/brain-fixture` exits 0 and prints the session brief.
