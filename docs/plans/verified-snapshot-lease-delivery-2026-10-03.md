# Verified snapshot lease delivery

Status: producer API frozen and complete source implementation in progress; recovery coordinator proposal independently CLEAR at `a50c876f06682d42530d32e46a2ffc60aa0485f4`. This is the next implementation wave toward production recovery, not hosted startup acceptance. The producer API and consumer pin handoff agree. The prior coordinator hash-domain HOLD is resolved by that exact amended proposal; its separate source-owner freezes, implementation and qualification remain required. Existing public snapshot inspection and restore behavior must remain compatible.

## Ownership and scope

Coordinator retains the backup lease claim. One isolated backup worker owns shared verifier extraction in `internal/hosted/backup/snapshot_inspect.go`, private restore-core extraction in `internal/hosted/backup/backup.go`, new `snapshot_lease*.go` implementation, platform helpers and tests, and one implementation receipt. No shared contracts, billing/service/CLI source, schema, module, registry, installed script or provider changes belong to this wave. Full recovered startup, genesis and ordinary restart remain separate deliverables.

## E23: VSL source delivery

#### Wave 1

- [x] T-VSL.1 — VSL-PRE — Freeze the corrected backup API and matching consumer pin handoff. kind: agent stage: preflight deps: [] Acceptance: producer `6a013b49b94e7f9286b782ad2827441d2e5bd737` independently CLEAR; consumer `331df6469fbf4f884b06ed4e70bc5d2819bcaa4f` independently confirms the matching pin handoff while retaining a separate coordinator hash-domain HOLD. Agreed pin identity/lifecycle/order, checkout nullability, byte/count limits, explicit file ownership, mounted writable SSD and canonical source claim are recorded in the producer freeze. Only the backup producer is authorized by this completed preflight.
- [ ] T-VSL.2 — VSL-IMPL — Implement the complete reviewed backup-owned lease API. kind: agent stage: implement deps: [T-VSL.1] Acceptance: stage and restore consume the same verified private bytes; durable pin/reopen and authorized terminal/abandon cleanup work across restart; projections retain pending checkout state without exposing authority or secrets; context, borrower, identity and accounting invariants match the frozen contract. No stub success or reduced transient-only substitute.
- [ ] T-VSL.3 — VSL-VERIFY — Verify affected behavior and module compatibility. kind: agent stage: verify deps: [T-VSL.2] Acceptance: meaningful hostile mutation controls demonstrate genuine RED before correction; focused race tests include same-byte handoff, forged/ref mismatches, tamper, pin/persist/release crash boundaries, borrower cancellation, limits, checkout nullability and public Inspector/Restore regression. Full local `go test -race ./...`, `go vet ./...`, lint and Linux ARM64 compile pass under the shared build lease and explicit owned fixture; all skips and limitations recorded. Formatting and diff checks pass.
- [ ] T-VSL.4 — VSL-REVIEW — Independently review the exact implementation head. kind: agent stage: review deps: [T-VSL.3] Acceptance: separate reviewer records base/head and contract coverage, independently reproduces decisive mutation REDs and reports CLEAR; accepted findings receive fixes, affected checks and new exact-head review. Review covers the full lease lifecycle, not only the verifier extraction.
- [ ] T-VSL.5 — VSL-MERGE — Merge the qualified source PR normally. kind: agent stage: merge deps: [T-VSL.4] Acceptance: fresh PR head/base/discussions/check annotations and actual coordination feed/board show no applicable hold; expected-head normal merge follows ADR024 local qualification without bypassing protections or changing billing.
- [ ] T-VSL.6 — VSL-LANDED — Verify source delivery on main. kind: agent stage: verify-landed deps: [T-VSL.5] Acceptance: remote main/PR merge state and landed tree match the exact reviewed candidate, source claim is exactly released, roadmap and landed receipt record the result. No provider, physical quota, recovery activation or hosted acceptance is inferred.

## Follow-on gates

Recovery consumes the implemented lease only after an independently reviewed owner handshake. Production admission still needs trusted writer fencing, lineage and credential authority. First startup and ordinary restart need authenticated factories. Physical quota and 500 GB qualification remain T23.44 work; cooperative lease accounting does not meet that requirement. Deployment, provider actions, purge, spend and launch remain separately gated.

## Current review correction

The VSL-PRE row records the producer handoff at its original freeze. Subsequent full consumer review clears `a50c876f06682d42530d32e46a2ffc60aa0485f4`, without changing producer v1. The proposal's historical line saying producer review is in progress is superseded by the banked producer `6a013b4` CLEAR and this current status. Source implementation is actively assigned at `e8a5c38fd68ac93873e2973a18f6ec6aea1ddb82`; no source checks or completion are yet claimed.

Machine-readable task IDs above retain the original VSL stage names as aliases; earlier assignments and evidence keep their original stage labels. Dependency order and acceptance are unchanged.
