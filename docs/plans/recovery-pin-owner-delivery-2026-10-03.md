# Recovery pin-owner component delivery

## E23: Durable snapshot pin owner core

#### Wave 1

- [x] T-PNO.1 — Freeze narrow owner contract and ownership. kind: agent stage: preflight deps: []
  acc: Exact6588 contract and independent CLEAR banked, single-owner/factory publication boundaries recorded, writable SSD and narrow claim verified.
- [x] T-PNO.2 — Implement exact durable pin owner core. kind: agent stage: implement deps: [T-PNO.1]
  acc: Only owned new pin_owner files implement all reviewed events, methods, exact tuple/CAS/tombstones and bounded crash replay; READY/effects/committed-restore release remain rejected.
- [x] T-PNO.3 — Verify current owner and real producer pair behavior. kind: agent stage: verify deps: [T-PNO.2, T-PNOFIX.4, T-SPS.3]
  acc: Current-byte real producer-pair lifecycle, crash, identity, bounds, stale tuple and proof tests pass; compiled mutation RED/restored PASS, focused race/vet/lint and root full-module race/vet/lint/Linux checks recorded under real exact-released leases.
- [x] T-PNO.4 — Independently review exact source and mutation controls. kind: agent stage: review deps: [T-PNO.3, T-SPS.4]
  acc: Independent isolated exact-head reviewer checks all lifecycle/wire/factory assumptions and reproduces decisive behavioral RED controls; all source findings resolved and renewed CLEAR recorded.
- [ ] T-PNO.5 — Merge normally after fresh holds and head checks. kind: agent stage: merge deps: [T-PNO.4]
  acc: Fresh exact head/base/discussions/check annotations and trusted feed/board checked; expected-head normal merge under ADR024 without admin/protection/billing bypass.
- [ ] T-PNO.6 — Verify landed tree and exact claim release. kind: agent stage: verify-landed deps: [T-PNO.5]
  acc: Remote merge/main tree equals final reviewed tree, source claim exactly released, fourteen original authored files preserved and landed receipt persisted.

Logical T23.50, full READY/coordinator/factory authority, post-restore release, runtime/provider and physical quota remain open.

#### Wave 2 — Accepted review findings and producer restart controls

- [x] T-PNOAUD.1 — Record independent baseline static HOLD. kind: agent stage: review deps: [T-PNO.1]
  acc: Exact ea318 review records three stable findings plus retained producer primitive gap; all WIP checks are evidence only, not source acceptance.
- [x] T-PNOFIX.1 — Correct unsupported future record version handling. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Unsupported versions return Unavailable before current-version strict field decode and preserve disk bytes with no mutation; malformed current records remain corrupt.
- [x] T-PNOFIX.2 — Reject mismatched pin tuples before PinKeep. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: RESERVED and PIN_PENDING reconciliation checks exact digest/pin tuple without authorizing deletion.
- [x] T-PNOFIX.3 — Exercise durable cancel publication crash prefix. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Actual child after-event:PIN_CANCEL crash reopens permanent tombstone, rejects old proof retry and permits only correctly fresh N+1.
- [x] T-PNOFIX.4 — Verify all three corrections at current bytes. kind: agent stage: verify deps: [T-PNOFIX.1, T-PNOFIX.2, T-PNOFIX.3, T-PNOFIX.5, T-PNOFIX.6, T-PNOFIX.7]
  acc: Meaningful regressions and compiled mutation controls, restored passing focused race/vet/lint with exact build lease releases. T-PNO.4 is the independent renewed review of these findings.
- [x] T-SPS.1 — Adopt reviewed producer crash and restart rule. kind: agent stage: preflight deps: [T-PNO.1]
  acc: Proposal096 design CLEAR and coordinator source freeze e762c9c banked; exact source claim c20ad and disjoint backup/testhooks ownership; source acceptance remains HOLD.
- [x] T-SPS.2 — Implement producer primitive controls and authorized restart. kind: agent stage: implement deps: [T-SPS.1]
  acc: Fresh exact owner authority precedes resumed deletion/new tombstone; completed tombstone uses exact terminal acknowledgement only after live lease absence; real tagged primitive controls and anchored reachable partial states preserve production no-op and RemoveAll. Adopted terminal amendment retains exact receipts with bounded4096 journal and complete peak metadata reservations.
- [x] T-SPS.3 — Verify producer current source and controls. kind: agent stage: verify deps: [T-SPS.2, T-SPSFIX.1, T-SPSFIX.3]
  acc: Ordinary/tagged child crash matrix, authority denial, cancellation, tuple/root/inode mismatch, live-present, repeated terminal real-pair reconciliation, bounded journal cap/overflow and peak metadata regressions; compiled RED/restored PASS and focused checks under exact released leases. Full integrated checks occur in T-PNO.3.
- [x] T-SPS.4 — Independently review producer source and reachable prefixes. kind: agent stage: review deps: [T-SPS.3]
  acc: Exact-head independent source review verifies canonical serializer/path/identity equivalence, real production recovery, authority before mutation, no production triggers, and accurate evidence limits.
- [ ] T-SPS.5 — Merge producer with qualified owner delivery. kind: agent stage: merge deps: [T-PNO.4, T-SPS.4]
  acc: Same normal exact-head integrated PR as T-PNO.5; neither component merges past a HOLD.
- [ ] T-SPS.6 — Verify producer landed and release its claim. kind: agent stage: verify-landed deps: [T-SPS.5]
  acc: Same reviewed/landed tree proof as T-PNO.6, with exact producer source claim release and preserved authored material.

- [x] T-PNOFIX.5 — Refuse owner bootstrap over retained or unknown history. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Before creating/rebinding lock, reservations or superblock, bounded no-follow scans reject retained history without its superblock, a superblock without its bound lock and unknown entries; bytes/inodes/inventory remain unchanged. Fresh empty and exact known empty bootstrap prefixes remain supported. Current focused checks/compiled control are required under T-PNOFIX.4.

2026-10-03 current source update: the four accepted owner implementation findings are corrected in author60600440, integrated69c4a22 with identical six owned Go files. Package, race and vet stages passed on author current bytes. Lint, compiled behavioral controls, combined producer matrix and final independent source acceptance remain pending; verification and merge tasks stay open. Earlier full-module baseline is historical after these source changes.

- [x] T-PNOFIX.6 — Classify unsupported future superblock wire before v1 decode. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Bounded syntactic version parsing precedes strict v1 superblock decoding; positive unsupported uint64 versions with future extension fields return Unavailable without mutation, while missing, zero, duplicate, malformed and overflowing versions remain corrupt. Active operations and reopening preserve superblock, lock and history bytes/inodes on refusal.

- [x] T-PNOFIX.7 — Revalidate the current superblock for active owner operations. kind: agent stage: implement deps: [T-PNOAUD.1]
  acc: Under the acquired owner lock, every active read and mutation checks supported canonical checksummed superblock, exact StoreID and all captured root/lock identities. Future/corrupt/missing or changed identity refuses with zero output and no history mutation; genuine-pair active API regressions and compiled control distinguish this boundary from constructor-only rejection.

- [x] T-SPSFIX.1 — Reserve complete release capacity before requesting owner release authority. kind: agent stage: implement deps: [T-SPS.1]
  acc: Conservative actual-encoder footprint includes retained manifest-inclusive metadata and terminal/temp peak before any owner RELEASE_BEGIN or producer release mutation. Unknown future owner authorization uses a worst-width encoding bound for capacity only, never authority. Fresh exact owner authorization and the later exact capacity check remain mandatory; a real-pair budget-gap regression preserves owner history and producer bytes on refusal; actual-encoder storage-only fixtures verify the journal cap without fabricating authority-backed receipts.

2026-10-03 implementation checkpoint: integrated38f08ae contains the current owner core, active-superblock revalidation, complete pre-authority release capacity reservation, retained terminal receipts and actual canonical tagged crash fixtures. Independent preliminary static reviews clear the accepted source findings narrowly. Implementation rows are complete; verification rows remain open until current producer/combined checks, decisive compiled controls and final independent exact-head source review are recorded. The owner author revision a3526dc has focused package/race/vet/lint passes, which do not substitute for combined-source qualification.

Current owner-focused verification is complete at a352: package/race/vet/lint, three compiled assertion REDs and restored combined targeted PASS have exact released leases. The current-focused-controls receipt records their scope; combined producer/full-module qualification and final independent review remain open.

Current producer verification is complete at c754/root e050 with identical Go fingerprint bfd24dc: tagged race, vet, lint, four current compiled behavioral REDs and restored full tagged race PASS. Full-module qualification, final independent review and merge remain open.

#### Wave 3 — Final review and full-module findings

- [x] T-SPSFIX.2 — Enforce tombstone fixture lifecycle and distinct phase inventory. kind: agent stage: fix deps: [T-SPS.2]
  acc: A captured RELEASED tombstone prefix requires absent live lease; earlier prefixes retain live-directory identity checks. The distinct phase-name test lists each existing wire phase exactly once, with constants and actual hook calls unchanged.
- [x] T-SPSFIX.3 — Qualify final test corrections at current bytes. kind: agent stage: verify deps: [T-SPSFIX.2]
  acc: Current ordinary/tagged checks cover the phase catalog and stricter tombstone fixture; compiled actual writer-order regression is detected, existing authority/receipt/capacity controls remain compiled RED, and restored current source passes. Root full-module qualification then completes T-PNO.3.

First full-module race snapshot at 1c9 failed the distinct-phase-name test because the same phase was listed twice. It recorded 3159 passing test/subtest events and 83 passing packages, command exit 1, unchanged source and exact released build lease fdafdf38ec21684ab714abebd32bd9cb3af5133b. This is a failed snapshot, not full qualification. Author f1d9 (including reviewer tombstone fixture correction 11f4) is integrated ad3d129; current producer verification is reopened, and final combined checks/review/merge remain pending.

Current-source qualification: integration c916bcd7ed30592efb065c876455d1bb51509523 / Go fingerprint 5176070520a9496599cbf0aabe44cfcc2daae34fa26681c13b231f6c4d1c0425 passed the three-package tagged race/vet/lint, all five compiled assertion controls, restored tagged race, and full-module race (3160 passing test/subtest events, 84 passing packages), vet, lint and Linux ARM64 build. All command/release exits were 0 except the intentional compiled assertion controls (command 1, exact release 0). See the current tagged matrix and full local qualification receipt. Independent exact-head review and merge/landed stages remain open; no READY, provider, startup or physical quota acceptance is granted.

Final narrow independent source review CLEAR at PR358 head8321d6ee229ce01879158bb8ea76b784d0c3f26d/tree1e9e2d260f89361dc2569ca3e3d13cf2738189b0 and basebee4790cf9e53e858802b059f3e6f0af38153e41. Four independent compiled assertion REDs and restored three-package tagged race PASS were inspected. Narrative report SHA-256 d38d0ff95f800ee48d6db6c800ec0e25aecfafc5ce89300b1f14a7628bb7594a is copied exactly in the final source review. Earlier skipped/setup attempts are explicitly excluded. This bank changes documentation only; final documentation review, normal merge and landed proof remain required.
