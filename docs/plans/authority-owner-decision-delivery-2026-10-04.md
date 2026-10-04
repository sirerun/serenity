# Authority-owner decision packet delivery

## E24: Proposed recovery approval and startup authority-owner decisions

#### Wave 1

- [x] T-AOD.1 — Verify the four pinned decision artifacts and review relationships. kind: agent stage: preflight deps: []
  acc: Approval packet v2 SHA-256 `cb3918b3f97bd2a4bb72b49f2f4d65ff6474f5cb53b5e9e71487f5a2477cfdda` is paired with review SHA-256 `315c7035dacd09bed1b4e78d6e0fc2c555c746522e939be4f21ba54d86b28df5`; startup packet SHA-256 `fa0d9614970ce4e0011d37bd910b18104df739a2f3aa52966f7465cdc95bb24b` is paired with corrected review SHA-256 `bcc13991d18f5133b679f7d3d17cbbae5e5681625ae75d4999bb8e66b2ea935f`. The approval review retains its prior HOLD history; the startup report is the corrected review artifact. Source-contract context remains pinned to historical source snapshot `c916bcd7ed30592efb065c876455d1bb51509523`, while this docs-only delivery is based on integration commit `8321d6ee229ce01879158bb8ea76b784d0c3f26d`.
- [x] T-AOD.2 — Deliver exact proposed packet and review bytes. kind: agent stage: implement deps: [T-AOD.1]
  acc: Four repository copies preserve their source bytes and hashes. They ask for founder owner decisions only; all choices remain proposed and confer no owner assignment, source freeze, implementation, provider, deployment, or runtime authority.
- [x] T-AOD.3 — Verify the five-file docs-only change and plan structure. kind: agent stage: verify deps: [T-AOD.2]
  acc: Exact source/destination hashes, privacy scan, whitespace check, base/HEAD/tree, and six-stage dependency structure are recorded in the external delivery receipt. Only these four copied documents and this delivery plan are owned by this task; source code and existing authored files are unchanged.
- [ ] T-AOD.4 — Independently review the exact five-file head and receipts. kind: agent stage: review deps: [T-AOD.3]
  acc: Reviewer checks exact-head equality, packet/review hashes, proposal-only status, privacy, and plan parse. Resolve findings on a new reviewed head if required.
- [ ] T-AOD.5 — Integrate after PR358 and fresh coordination checks. kind: agent stage: merge deps: [T-AOD.4]
  acc: Root performs normal expected-head integration only after PR358 landed verification and current trusted holds/base/head checks. This worker does not push, create a PR, or merge.
- [ ] T-AOD.6 — Verify landed documentation and preserve source provenance. kind: agent stage: verify-landed deps: [T-AOD.5]
  acc: Verify the landed tree equals the reviewed five-file tree, retain exact source hashes and historical source pins, and record remaining owner-decision gaps.

## Provenance and decision boundary

The four copied artifacts are the approval decision packet v2 and its v2 review, plus the startup owner-decision packet and its corrected review. Their source evidence files are preserved unchanged. Repository filenames provide dated discoverability; the packet contents remain byte-identical to those source artifacts.

Both packets remain proposed founder decision requests. No founder choice, accountable-owner assignment, owner contract, source freeze, production factory, READY state, trust root, key, credential source, provider operation, deployment, or spend is asserted or authorized by this delivery. The historical `c916bcd7ed30592efb065c876455d1bb51509523` source snapshot is not replaced by the docs-only base and must not be read as current implementation acceptance.
