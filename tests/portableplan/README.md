# Serenity portable-plan conformance

This offline example slice consumes Wazi's frozen experimental contract 0.0.1.
It changes no memory engine, protocol, scheduler, policy authority or service.
No compatibility projection is needed to demonstrate these contextual/policy
boundaries: the examples are explicit synthetic interchange records.

## Pinned inputs

`pin.json` identifies the owner source freeze at
`16b66e5eedf20d52e72928bb56a0c19e391e8ce9`, digest
`sha256:7582512f122d2f2a9c4461facc7541c9887053f137260d6ebe9c6dea611d039d`.
`testdata/upstream` is an unchanged copy of the 66 manifest-listed paths and
manifest. It includes the owner's 60 neutral conformance cases. The Go test
verifies individual bytes, unique canonical paths and the aggregate digest,
then loads the pinned Draft 2020-12 resources without an external loader.
This is consumption of one shared schema, not a Serenity schema fork.

The source freeze initially existed locally in the steward's repository; do
not infer public retrieval or runtime qualification from its Git object ID.
The steward's Go implementation and operational instructions have a separate
source/artifact qualification from the frozen schema/fixture digest.

## Cases and authority

The Serenity corpus preserves scoped context and truthful unavailable states.
It represents hosted raw failure/non-start separately from an approved local
source-merge alternative, with missing/revoked/failed/stale and scope-escalation
counterexamples. Context and passing constraint checks never issue execution,
review, landing, startup or deployment authority. Later observations remain
audit-only when their attempt is terminal.

All producer, policy, grant and verified-success assertions in these examples
are synthetic. Schema/coherence checks do not authenticate them. The real
Serenity merge policy is separately grounded in
[ADR 024](../../docs/adr/024-local-validation-during-actions-billing-lock.md).
No fixture claims a current candidate passed hosted CI, connected to a brain,
read private memory or executed a provider. Available contextual metadata is an
example envelope, not evidence that the richer scoped Serenity interface ships.

## Run explicitly

The package uses the `portableplan` build tag so an ordinary source test does
not silently depend on an external validator or call another service. Use the
owner-qualified offline `wazi-contract` binary against this exact digest.
The steward documents these operations; they do not start a service:

```sh
wazi-contract version
wazi-contract fixtures --contract-digest sha256:7582512f122d2f2a9c4461facc7541c9887053f137260d6ebe9c6dea611d039d
wazi-contract validate --contract-digest sha256:7582512f122d2f2a9c4461facc7541c9887053f137260d6ebe9c6dea611d039d tests/portableplan/testdata/serenity/EXAMPLE.json
GOWORK=off go test -tags portableplan ./tests/portableplan
```

Choose an actual catalog file in place of `EXAMPLE.json`. The validator's
qualified source revision, artifact digest, exact commands and outcomes must be
recorded with candidate review evidence. The source-freeze receipt alone is not
a usable validator. Full acceptance requires the steward's semantic checker for
all neutral and Serenity cases; structural validation by itself is insufficient.

Shared machine load/build admission and external-SSD cache/temp rules apply.
No checks have passed merely because their command is documented here.
