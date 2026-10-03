# Recovery pin-owner component delivery

## E23: Durable snapshot pin owner core

#### Wave 1

- [x] T-PNO.1 — Freeze narrow owner contract and ownership. kind: agent stage: preflight deps: []
  acc: Exact6588 contract and independent CLEAR banked, single-owner/factory publication boundaries recorded, writable SSD and narrow claim verified.
- [ ] T-PNO.2 — Implement exact durable pin owner core. kind: agent stage: implement deps: [T-PNO.1]
  acc: Only owned new pin_owner files implement all reviewed events, methods, exact tuple/CAS/tombstones and bounded crash replay; READY/effects/committed-restore release remain rejected.
- [ ] T-PNO.3 — Verify current owner and real producer pair behavior. kind: agent stage: verify deps: [T-PNO.2]
  acc: Current-byte real producer-pair lifecycle, crash, identity, bounds, stale tuple and proof tests pass; compiled mutation RED/restored PASS, focused race/vet/lint and root full-module race/vet/lint/Linux checks recorded under real exact-released leases.
- [ ] T-PNO.4 — Independently review exact source and mutation controls. kind: agent stage: review deps: [T-PNO.3]
  acc: Independent isolated exact-head reviewer checks all lifecycle/wire/factory assumptions and reproduces decisive behavioral RED controls; all source findings resolved and renewed CLEAR recorded.
- [ ] T-PNO.5 — Merge normally after fresh holds and head checks. kind: agent stage: merge deps: [T-PNO.4]
  acc: Fresh exact head/base/discussions/check annotations and trusted feed/board checked; expected-head normal merge under ADR024 without admin/protection/billing bypass.
- [ ] T-PNO.6 — Verify landed tree and exact claim release. kind: agent stage: verify-landed deps: [T-PNO.5]
  acc: Remote merge/main tree equals final reviewed tree, source claim exactly released, fourteen original authored files preserved and landed receipt persisted.

Logical T23.50, full READY/coordinator/factory authority, post-restore release, runtime/provider and physical quota remain open.
