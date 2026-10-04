# Startup authority owner decision packet

**Status: proposed founder assignment only.** This packet asks the founder to name accountable owners for the authority contracts required before an authenticated hosted startup factory can be considered. It selects no provider, trust root, key, credential source, or human owner; it grants no source, service, provider, or deployment authority. Until the required contracts and evidence exist, hosted startup remains fail-closed.

## Decision requested

For each row below, assign an accountable owner who can identify the real authority and evidence source, or record that no such owner/source is presently available. If any required authority has no owner or cannot produce its evidence, keep startup unavailable and do not freeze or implement the factory seam. These assignments are prerequisites for a later owner-contract review; they are not permission to invent local substitutes or start provider operations.

| Required owner contract | Founder assignment needed | Minimum owner deliverable before any source freeze | Fail-closed choice if unavailable |
|---|---|---|---|
| Authenticated runtime and control-plane identity | Name the owner of the trusted workload identity channel and the authority that validates it. | An owner-reviewed identity binding for deployment, exact journal namespace, unique instance and startup-attempt IDs, expiry, nonce, issuer, and audience; a statement of how the channel authenticates the running workload and how stale/replayed attempts are rejected. Bind the runtime source in the trusted composition root; do not accept identity from CLI flags, environment, hostnames, local SQL, or a caller-provided struct. | Do not enter restart preparation or issue journal credentials. |
| Genesis trust and durable issuance/epoch authority | Name the owner accountable for permit trust and the durable authority record. | Define the authenticated first-issuance permit and trust policy, including deployment/namespace audience, generation 1, expiry, nonce, signed-payload digest, and authority-assigned operation ID. Specify durable atomic single-use consumption, idempotent resolution after ambiguous responses/crash, and durable records for the one active writer epoch and each successor allocation. Empty storage is never genesis evidence. | Do not infer genesis from an empty namespace or mint a new writer. |
| Complete old-writer credential, principal, and session inventory | Name the security/access owner who can enumerate every effective mutation path. | An owner-controlled, complete inventory across writer principals, credentials and in-flight sessions; workload/base credentials, role/delegation chains, renewals, metadata/default chains, operators and break-glass access, CI/automation, replication/maintenance, policy administrators, delete/version/lifecycle controls, and any identity able to grant mutation. Include exact resource scope, mutation verbs, issuer/delegation route, maximum lifetime/renewal, and the enforcement point. Do not use a caller completeness boolean or only enumerate the last observed WriterID. | Do not claim the old writer is fenced; do not expose a read-capable prepared startup. |
| Enforceable namespace-wide fence and revocation | Name the owner of the journal namespace access policy and the evidence proving it is effective. | A provider-enforced reservation/deny barrier covering all inventory routes and policy/credential changes that could bypass it; explicit in-flight request/session treatment; exact old-credential revocation/denial; read-after-change evidence and policy epoch/version; and a bounded, fail-closed resolution path when stop/revoke outcomes are ambiguous. The barrier must remain effective through observation, writer activation, service replay, and publication, then permit only the new lease. | Keep the namespace write-fenced and startup unresolved; never treat timeout, process stop, local flock, conditional-write collision, or a boolean receipt as proof. |
| Unique writer lease, journal ancestry, and exact head reservation | Name the owner of durable single-writer epoch allocation and journal/history reconciliation. | A durable exclusive reservation and CAS protocol bound to the exact namespace, genesis/successor issuance, complete history digest, every entry-and-seal WriterUse, positive JournalPosition, observed head/predecessor seal, inventory digest, and policy epoch. Reconcile every historical writer ID against durable issuance records. Activate one globally unique writer lease at the exact reserved position; preserve it through first append, service replay, and publication; close/revoke it on shutdown only after workers are joined. | Do not activate a writer or construct a service from an unauthenticated position. |
| Stable journal observation and provider object adapter | Name the storage/journal owner able to prove that the frozen namespace can be observed and conditionally mutated as required. | An owner-controlled read adapter that bounds transport allocation and provides a complete, stable observation while the external reservation is held; qualification of immutable canonical objects, atomic conditional create, version/delete-marker behavior, no lifecycle deletion, policy enforcement, and ambiguous-write readback. Prove that policy and history cannot be replaced while the fence is held. | Fail the observation/admission; do not use an arbitrary store or default SDK credential chain as a production writer. |
| Service lifecycle and trusted composition root | Name service and binary-composition owners for the eventual typed admission/writer-lease handoff. | An owner-reviewed mutually exclusive startup-versus-recovery dependency contract: authenticated `StartupProof`, read-only journal reader during proof, and exact `StartupLease` for the activated writer. No handler or worker publication before proof and replay. Service shutdown joins workers, then revokes the exact lease with errors retained as unresolved state. CLI wiring may only call the trusted factory after all authority dependencies are bound. | Preserve existing recovery semantics and keep `hosted serve` unavailable. |

The founder may assign the accountable owners separately or to an existing accountable group, but each owner must accept the exact boundary and evidence obligation. A source interface, mock, local file, self-signed test fixture, hard-coded key, arbitrary injected adapter, or operator assertion is not a substitute for an owner and its authority.

## Current source and contract boundary

Source was read from clean commit `c916bcd7ed30592efb065c876455d1bb51509523`, tree `0b026e37226fd8295f2ca1a350a4f5072816a336`. The authenticated startup factory document is a held proposal; it describes genesis and ordinary restart as separate authenticated paths and explicitly says no production factory, trusted credential issuer, provider authority, or startup readiness is established. Its source basis predates this packet, so the current source baseline above is the controlling source snapshot.

The accepted `startup-journal-observation-v1` amendment supplies a bounded, read-only history observation and an exact-position writer constructor. `JournalObservation` reports the complete canonical history digest and first-use writer provenance across entries and seals. Its documented precondition is an externally established stable namespace reservation. `JournalPosition` carries positive active generation and genesis/successor identity, but the reader and `NewJournalAt` validate structure only; they do not authenticate the supplied authority IDs, provider namespace, stable read reservation, writer issuance, or lease. The observation component explicitly grants no startup factory, service admission, CLI wiring, provider calls, or credentials.

Current code remains fail-closed at the service edge: `LifecycleDependencies` accepts an injected writable `DeletionJournal`, a nonempty `BuildSHA` string, `RecoveryAdmission`, and operator-review admission; validation does not establish who authenticated those values. `AssembleWithDependencies` calls the recovery-specific admission and requires both replay reads to end at the same sealed watermark. `hosted serve` returns `ErrStartupUnavailable` pending an admitted journal dependency factory. `Service.Close` stops and joins its existing workers and closes gateway/pool/store, but there is no startup writer lease to revoke. The recovery-only `FenceReceipt` has plan-specific fields and boolean stop/revocation claims; it is not generic restart authority.

The existing AWS journal constructor uses the SDK default credential chain and comments that production must configure an independently scoped credential source. Conditional create and journal hashes protect object positions and content; they do not prove that only one authorized live writer exists, that alternate principals cannot mutate, or that a stable namespace-wide reservation is held. The source has no authenticated runtime identity adapter, genesis issuer/ledger, writer-lease authority, complete mutation inventory, or provider-qualified fence evidence.

## Required order and stop conditions

1. Founder assigns accountable owners or explicitly records that a required authority is unavailable.
2. Each assigned owner produces its bounded authority contract and identifies its real evidence source; storage and security owners jointly reconcile the complete mutation inventory and fence.
3. Journal and service owners review the proof-to-lease boundary, including exact head reservation, first append, service replay, handler publication, and shutdown revocation. Recovery remains a separate path.
4. Independent review checks that the proposed seam cannot accept caller-selected positions, fake proof booleans, default credentials, partial writer inventories, stale sessions, or an unowned namespace.
5. Only after all owners and reviewers agree may the coordinator consider an exact startup-factory source freeze and later implementation assignment.

If any owner cannot demonstrate its evidence source, the result is `startup unavailable`; there is no partial readiness state and no fallback to local process identity, empty history, `RecoveryAdmission`, SQL, a filesystem lock, or a test fake. No provider experiment, key creation, credential issuance, deployment, or acceptance is authorized by this packet.

## Source pointers and digest manifest

At the stated baseline, these files ground the current-state findings:

| Source | Relevant lines | SHA-256 |
|---|---:|---|
| `internal/hosted/service/service.go` | 63–99, 269–281, 334–370, 470–483 | `3688c0f755d41a85997dc88accaaa0a0ce2d2c6ef24320d0473246fdf5a751c3` |
| `internal/cli/hosted.go` | 21–34 | `f4467c762ed9403cc188e38b55eaa52e0c43b4b2486fbd2677eed5c607628db0` |
| `internal/hosted/contracts/deletion_reader.go` | 18–40 | `6e69d1e07fe3d02389f887ca0e9154f455ea2ef44071225daef0efc24d9729fb` |
| `internal/hosted/deletion/journal_observation.go` | 26–42, 94–105 and observation limits | `42abd0a58022d494dc59a77d27594fb8fd9a85c3ad787e4f39d5afb965d86e8f` |
| `internal/hosted/deletion/journal.go` | 47–79, 252–309, 440–490 | `c8be00925c0b282053a0dbe5d34697421ef16bac89022dba5f9ae96b13abb48f` |
| `internal/hosted/deletion/config.go` | 26–51 | `282db17ccca0d04b041eaecf38a282c8ac8d9fa1b71383c386c09e1e67878a63` |
| `internal/hosted/contracts/fence.go` | 11–80 | `31f70544acf61ec74448fda29e672aa7421ad49911dab105f3ee3b2f4d1ac8a1` |

Contract/evidence documents read at the same baseline:

| Document | SHA-256 |
|---|---|
| `docs/plans/authenticated-startup-factory-proposed-contract-2026-10-03.md` | `039da93ef5e62e9e743e3cf89b1aaa8e7cbd72a5f1193211bc0dfeb7e7b054fb` |
| `docs/plans/startup-journal-observation-freeze-2026-10-03.md` | `978b83d875f840ccb3124c0f9645671f74d36b1c929189193d433c5c5d977b0e` |
| `docs/plans/startup-journal-observation-delivery-2026-10-03.md` | `796882f420a6f4adc5a635b4cb007ca0d6c9c8c4128810f80166e0efe2c74061` |
| `docs/plans/startup-factory-contract-assignment-2026-10-03.md` | `ebd91e6955111b2c25f2c928d00ead1b143264bfae9bf235e97b75deb83c158b` |
| `docs/launch/evidence/recovery-contracts-2026-10-03/startup-final-clear.md` | `6da86798a03005765aaa73b6c2c02e134801fb0656eab2facb561a6d252f3d7f` |

The contract source and evidence receipts are design/input boundaries, not proof that an authority adapter, provider policy, credentials, deployment, or startup factory exists. This packet does not alter those boundaries.
