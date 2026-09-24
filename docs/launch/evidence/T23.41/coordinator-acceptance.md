# T23.41 — Coordinator acceptance

This record is the coordinator's acceptance of T23.41, distinct from ADR 017
(`docs/adr/017-hosted-operation-and-recovery-contracts.md`), which records
chief-architect's design approval only. Per T23.41's own acceptance criteria,
design approval and coordinator acceptance are separate steps; neither is
inferred from the other.

## Acceptance

- Status: **ACCEPTED for downstream implementation dispatch**.
- Reviewer / date (UTC): David Ndungu, 2026-09-21.
- Reviewed revision: `c61ab91bb48099dfbfcfec2ee86ea2cd590439c4` (PR #236, "Hosted T23.41: propose recovery, accounting and storage contracts").
- Basis: ADR 017's four approved decisions (storage admission, operation accounting, deletion journal, restore fencing) and `docs/launch/evidence/T23.41/architecture-review-request.md`.

## Scope of this acceptance

- Unblocks downstream implementation dispatch of the six tasks that depend on T23.41: T23.42, T23.44, T23.46, T23.47, T23.49, T23.51 (per `docs/launch/hosted-status.md`).
- **Storage remains conditional**, unchanged from ADR 017: storage admission stays BLOCKED until task44 supplies the OS-enforced staging limit, allocation-based accounting (including external Git/index writes), and a measured `MaxMutationStageBytes`. This acceptance does not lift that condition.
- **This acceptance does not qualify live S3 or storage admission.** No SQL migration, production code path, cloud resource, provider call, or launch action is authorized by this record — same scope limit as ADR 017's own "Consequences" section. Task48's live S3 conditional-write qualification and task44's staging-limit/accounting work remain separately required before storage admission or the deletion-journal substrate is operationally accepted.

## References

- `docs/adr/017-hosted-operation-and-recovery-contracts.md`
- `docs/launch/evidence/T23.41/architecture-review-request.md`
- `docs/launch/hosted-status.md`
