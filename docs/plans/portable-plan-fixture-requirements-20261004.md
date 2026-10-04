# PC-SERENITY consumer fixture requirements

These are semantic counterexamples for Wazi's steward-owned experimental
contract, not a proposed competing wire format. Convert them to the exact pinned
contract only after the owner freeze. All example identities and data are neutral
and synthetic; structural conformance does not authenticate their producer.

| Case | Observation and expected outcome |
| --- | --- |
| SCTX-01 context is not authority | A scoped eligible remembered fact or authored decision is available as context. It does not satisfy an execution-complete, independent-review, source-landed, deployment or approval requirement merely because its text says the action succeeded or is allowed. |
| SCTX-02 honest unavailable | Project mapping, audience eligibility, record provenance or a supported scoped read is missing. Preserve unavailable/unmapped/unknown instead of empty-success, broader-brain fallback or invented references. |
| SCTX-03 non-executing read policy | Automatic contextual projection cannot call the Go facade Recall or query-bearing MCP recall under a provider-free policy: those paths may call configured providers. Brief relevance ranking is not selected-project isolation. No canonical-file fallback or memory mutation. |
| SPOL-01 hosted unavailable, no alternative | Hosted job raw conclusion is failure and its non-start annotation reports billing unavailability. Keep both raw facts. With no qualified approved alternative, the relevant check requirement remains unsatisfied/unavailable; do not produce hosted success. |
| SPOL-02 explicit scoped alternative | Keep the same hosted observation. A separately qualified owner-approved local policy revision permits appropriate passing local checks and independent review for source merge. Record its policy/evidence bindings separately; only that scoped evaluation may be satisfied. Hosted CI is still unavailable. |
| SPOL-03 unqualified policy text | A memory note, checkbox, schema-shaped policy record, arbitrary label or claimed issuer is not authenticated alternative-policy authority. Without the consumer's qualified policy/grant validation, no source-merge requirement is satisfied. |
| SPOL-04 stale or negative alternative | Local failure, mismatched source head/base, changed policy revision, stale/unknown/expired evidence or revoked policy cannot fulfill the current candidate's check requirement. Keep raw observations rather than replacing them. |
| SPOL-05 no scope escalation | A local source-merge alternative does not fulfill deployment, release, recovery approval or startup authority. A source-landed observation also does not prove deployed acceptance. |
| SAUD-01 terminal audit | Late contextual or local-check facts after cancellation/expiry can remain audit-only. They cannot reopen terminal execution or satisfy the current delivery dependency. |

## Actual policy source and observed provider result

At Serenity source `e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f`,
[ADR 024](../adr/024-local-validation-during-actions-billing-lock.md) is expressly
owner-approved. It permits local checks appropriate to the change plus
independent review for source merges during the billing lock, preserves holds,
forbids calling blocked hosted jobs passing, and leaves deployment/provider
qualification separate. It grants no new recovery/startup policy assignment.

Read-only GitHub observation on 2026-10-04: CI run `37175767570`, source
`e2b5dd17c889219ad50a1dbaa3b5c932c0dd930f`, test check `111357928363` had raw
conclusion `failure`; its failure annotation said the job was not started because
the account was locked due to billing. This observed non-start reason qualifies
the unavailable classification while preserving the raw failure. It is historical
provider metadata, not validation of this new candidate and not a synthetic
fixture to be presented as a current receipt. The fixture corpus must use neutral
IDs and label synthetic evidence explicitly.

## Contract requirements to resolve with the steward

The pinned interchange must represent raw observations separately from named
requirement evaluations, bind source/policy/scope/provenance, preserve unavailable
and audit-only state, and distinguish contextual records from execution evidence.
An authenticated policy alternative is a consumer-owned qualification step;
shape validation alone cannot establish it. If the experimental contract lacks
one of these distinctions, report the concrete counterexample to Wazi instead
of adding a private extension with different semantics.

Conformance should include valid representations of unavailable/unsatisfied
states, not only invalid schema files. Add invalid mapping cases where context
or unavailable hosted evidence falsely satisfies a delivery gate. Positive
alternative-policy fixtures retain the raw hosted failure/non-start observation
and require explicit local evidence/current candidate/policy mapping.
