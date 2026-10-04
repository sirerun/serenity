# PC-SERENITY source inventory and fixture requirements

Baseline: `e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f` (reviewed at the assigned
worktree HEAD). This inventory maps existing Serenity surfaces for the portable
plan consumer. It does not define a shared schema or claim execution authority.

## Existing interfaces and boundaries

| Surface | Source behavior | Consequence for portable-plan context |
| --- | --- | --- |
| `pkg/serenity.Open` and `Brain` | `Open` constructs a `direction.Store` without a writer queue; the package exposes only read operations and has an AST guard against importing the writer or calling ledger mutators (`pkg/serenity/serenity.go:50-75`, `pkg/serenity/surface_test.go:14-58,90-139`). | This is a local read facade, not a writer, scheduler, approval issuer, or execution receipt source. |
| `Brain.CheckPlan` | Runs the existing DIRECTION constraint matcher and returns its `pass`, `violated`, or `no_applicable_constraints` result (`pkg/serenity/serenity.go:87-140`). | A `pass` is a constraint-check result only. It is not approval, execution, deployment, or evidence that another dependency completed. |
| `Brain.Brief` | Calls DIRECTION's local brief builder against the read-only ledger and returns its packed context (`pkg/serenity/serenity.go:143-160`). | Brief content is context. The method does not create or authenticate a portable-plan receipt. |
| `Brain.Recall` local retrieval | Reads the local derived index, loads the current canonical memory projection, computes retrieval eligibility, and searches with no embedder (`pkg/serenity/serenity.go:259-298`). `internal/cli/search.go:32-84` confirms CLI search also passes a nil embedder and remains full-text-only even when an embedding model is pinned. | These are provider-free retrieval paths when used without the composer branch. Returned hits remain contextual data, not execution authority. |
| `Brain.Recall` composer branch | If a composer router can be built, `Recall` may build an embedding router, call `compose.Ask`, and record provider spend through the SQLite index (`pkg/serenity/serenity.go:299-315`; `internal/providers/providers.go:181-228`; `internal/router/provider.go:22-35`, `internal/router/router.go:219-247,263-279`). Router construction depends on model pins and credentials/configuration and may reach external or configured local model endpoints (`internal/providers/providers.go:87-130`). | `Recall` is provider-capable as a whole. A no-provider example must prove the composer is unavailable or exercise the provider-free search path directly. Provider-capable reads must not be auto-projected as policy or execution evidence. |

The package documentation binds the facade to the same engine as CLI and wire
surfaces, keeps canonical writes with the single writer, and states that the
reader's SQLite index is only as current as its last open; it promises no
bounded freshness (`docs/adr/012-embedded-read-facade-single-writer.md:38-84`).
`Recall` separately refreshes source and retrieval eligibility from canonical
state on each call, but that does not turn its ranking or answer into a
portable-plan authority fact (`pkg/serenity/serenity.go:277-289`).

## Source-policy inputs that affect returned context

`Recall` fails the operation if it cannot load the canonical memory projection
or compute retrieval eligibility. For indexed claims, an absent or mismatched
canonical claim is filtered out; pending claims are ineligible, and provider
egress has stricter visibility and source-attribution checks than local reads
(`internal/index/imported_claims.go:154-184,187-212`). Memory lifecycle records
are never evidence, expired facts are excluded, remote reads are restricted to
world-visible facts, and a durable cancellation fence can outlive its fact
(`internal/store/memoryfact.go:288-350,455-468,530-558`).

An existing CLI regression test shows why context fixtures should model stale
policy explicitly: an expiry written after index construction removes a result
without rebuilding the index (`internal/cli/search_memory_policy_test.go:19-60`).
The embedded-facade test likewise removes canonical evidence after building the
index and expects `Recall` to return no hit (`pkg/serenity/recall_authority_test.go:16-62`).
These are source-level retrieval guarantees; they do not supply a generic
portable-plan status for stale plan heads, policy revisions, or audit facts.

## ADR 024 local source-merge alternative

ADR 024 records that GitHub Actions cannot start jobs while account billing is
locked and authorizes appropriate local checks as the merge gate while hosted
checks are unavailable. It requires the source revision, commands, outcomes,
skips, and material limits to be recorded; substantial changes receive
independent review; reviewer and trusted-coordinator holds remain binding
(`docs/adr/024-local-validation-during-actions-billing-lock.md:6-18`). It
forbids changing branch protections or OAuth restrictions, says blocked Actions
jobs must never be reported as passing, and keeps live deployment and
paid-provider qualification separate (`docs/adr/024-local-validation-during-actions-billing-lock.md:20-30`).

Therefore the policy alternative is a separately scoped local source-check and
review/merge path. It does not rewrite the hosted observation, manufacture a
hosted success, or qualify deployment/provider behavior. It is not a grant for
startup or deployment actions. Any portable example should preserve the raw
hosted state and attach local-policy evaluation as a distinct alternative;
the actual wire names and shape await the shared contract.

## Bounded consumer fixture requirements

The following are semantic counterexamples for the future pinned Wazi contract,
not proposed wire fields. Cover each using that contract's actual validator and
fixture instructions once its exact revision and digest are supplied.

| Case | Required assertion |
| --- | --- |
| Context presented as authority | A valid Serenity brief, recall hit, cited answer, or passing `CheckPlan` remains context/check output and cannot satisfy an execution, approval, startup, or deployment dependency. |
| Missing or unmapped context | Missing local brain, unavailable projection, unknown source mapping, or unavailable context is represented as unavailable/error. It must not become an empty successful context or inherited authority. |
| Provider boundary | Exercise a provider-free read separately from provider-capable `Recall`. A composer/embedding result and its spend record cannot be treated as a local source check, CI observation, or execution receipt. |
| Hosted CI unavailable | Preserve the raw unavailable/blocked hosted-check observation as unavailable; it cannot be represented as passing. |
| ADR 024 alternative | For the same hosted observation, evaluate an explicitly scoped local source-check alternative only when its base/source revision, commands, results, skips, limits, independent review, and holds are represented. Local success is not hosted success. |
| Local alternative failure or missing inputs | A failed local check, absent source revision, stale base, missing required review, or unresolved hold does not qualify the alternative. |
| Stale or revoked source policy | A stale index hit whose canonical source expired, was deleted, became ineligible, or is fenced by cancellation is excluded or reported unavailable. A newer policy input must not silently validate an older snapshot. |
| Late audit context | A late-arriving audit fact may be shown as audit context under the pinned contract, but cannot retroactively change a prior qualification or confer authority. |
| Deployment/startup separation | Passing local checks under ADR 024 does not satisfy a deployment or startup grant; those remain separate, unqualified dependencies. |

## Unavailable contract inputs

The dispatch and coordinator plan state that Wazi owns the shared experimental
contract and that consumers must wait for its exact frozen revision/digest
before implementation or final conformance. No such revision, schema, semantic
validator, or fixture instructions are present in the inspected dispatch and
saved plan. This inventory consequently defines no field names, enums, evidence
objects, or adapter. The current Serenity facade also has no hosted-CI status
reader or portable-plan authority interface.
