# Recovery pin-owner component delivery

## E23: Durable snapshot pin owner core

#### Wave 1

- [x] T-PNO.1 — Freeze narrow owner contract and ownership. kind: agent stage: preflight deps: []
  acc: Exact6588 contract and independent CLEAR banked, single-owner/factory publication boundaries recorded, writable SSD and narrow claim verified.
- [ ] T-PNO.2 — Implement exact durable pin owner core. kind: agent stage: implement deps: [T-PNO.1]
  acc: Only owned new pin_owner files implement all reviewed events, methods, exact tuple/CAS/tombstones and bounded crash replay; READY/effects/committed-restore release remain rejected.
- [ ] T-PNO.3 — Verify current owner and real producer pair behavior. kind: agent stage: verify deps: [T-PNO.2, T-PNOFIX.4, T-SPS.3]
  acc: Current-byte real producer-pair lifecycle, crash, identity, bounds, stale tuple and proof tests pass; compiled mutation RED/restored PASS, focused race/vet/lint and root full-module race/vet/lint/Linux checks recorded under real exact-released leases.
- [ ] T-PNO.4 — Independently review exact source and mutation controls. kind: agent stage: review deps: [T-PNO.3, T-SPS.4]
  acc: Independent isolated exact-head reviewer checks all lifecycle/wire/factory assumptions and reproduces decisive behavioral RED controls; all source findings resolved and renewed CLEAR recorded.
- [ ] T-PNO.5 — Merge normally after fresh holds and head checks. kind: agent stage: merge deps: [T-PNO.4]
  acc: Fresh exact head/base/discussions/check annotations and trusted feed/board checked; expected-head normal merge under ADR024 without admin/protection/billing bypass.
- [ ] T-PNO.6 — Verify landed tree and exact claim release. kind: agent stage: verify-landed deps: [T-PNO.5]
  acc: Remote merge/main tree equals final reviewed tree, source claim exactly released, fourteen original authored files preserved and landed receipt persisted.

Logical T23.50, full READY/coordinator/factory authority, post-restore release, runtime/provider and physical quota remain open.

#### Wave 2 — Accepted review findings and producer restart controls

- [x] T-PNOAUD.1 — Record independent baseline static HOLD. kind: agent stage: review deps: [T-PNO.1]
  acc: Exact ea318 review records three stable findings plus retained producer primitive gap; all WIP checks are evidence only, not source acceptance.
- [ ] T-PNOFIX.1 — Correct unsupported future record version handling. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Unsupported versions return Unavailable before current-version strict field decode and preserve disk bytes with no mutation; malformed current records remain corrupt.
- [ ] T-PNOFIX.2 — Reject mismatched pin tuples before PinKeep. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: RESERVED and PIN_PENDING reconciliation checks exact digest/pin tuple without authorizing deletion.
- [ ] T-PNOFIX.3 — Exercise durable cancel publication crash prefix. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Actual child after-event:PIN_CANCEL crash reopens permanent tombstone, rejects old proof retry and permits only correctly fresh N+1.
- [ ] T-PNOFIX.4 — Verify all three corrections at current bytes. kind: agent stage: verify deps: [T-PNOFIX.1, T-PNOFIX.2, T-PNOFIX.3, T-PNOFIX.5]
  acc: Meaningful regressions and compiled mutation controls, restored passing focused race/vet/lint with exact build lease releases. T-PNO.4 is the independent renewed review of these findings.
- [x] T-SPS.1 — Adopt reviewed producer crash and restart rule. kind: agent stage: preflight deps: [T-PNO.1]
  acc: Proposal096 design CLEAR and coordinator source freeze e762c9c banked; exact source claim c20ad and disjoint backup/testhooks ownership; source acceptance remains HOLD.
- [ ] T-SPS.2 — Implement producer primitive controls and authorized restart. kind: agent stage: implement deps: [T-SPS.1]
  acc: Fresh exact owner authority precedes resumed deletion/new tombstone; completed tombstone uses exact terminal acknowledgement only after live lease absence; real tagged primitive controls and anchored reachable partial states preserve production no-op and RemoveAll. Adopted terminal amendment retains exact receipts with bounded4096 journal and complete peak metadata reservations.
- [ ] T-SPS.3 — Verify producer current source and controls. kind: agent stage: verify deps: [T-SPS.2]
  acc: Ordinary/tagged child crash matrix, authority denial, cancellation, tuple/root/inode mismatch, live-present, repeated terminal real-pair reconciliation, bounded journal cap/overflow and peak metadata regressions; compiled RED/restored PASS and focused checks under exact released leases. Full integrated checks occur in T-PNO.3.
- [ ] T-SPS.4 — Independently review producer source and reachable prefixes. kind: agent stage: review deps: [T-SPS.3]
  acc: Exact-head independent source review verifies canonical serializer/path/identity equivalence, real production recovery, authority before mutation, no production triggers, and accurate evidence limits.
- [ ] T-SPS.5 — Merge producer with qualified owner delivery. kind: agent stage: merge deps: [T-PNO.4, T-SPS.4]
  acc: Same normal exact-head integrated PR as T-PNO.5; neither component merges past a HOLD.
- [ ] T-SPS.6 — Verify producer landed and release its claim. kind: agent stage: verify-landed deps: [T-SPS.5]
  acc: Same reviewed/landed tree proof as T-PNO.6, with exact producer source claim release and preserved authored material.

- [ ] T-PNOFIX.5 — Refuse owner bootstrap over retained or unknown history. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Before creating/rebinding lock, reservations or superblock, bounded no-follow scans reject retained history without its superblock, a superblock without its bound lock and unknown entries; bytes/inodes/inventory remain unchanged. Fresh empty and exact known empty bootstrap prefixes remain supported. Current focused checks/compiled control are required under T-PNOFIX.4.
