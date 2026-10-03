# Pure recovery-envelope component delivery

## E23: Recovery-envelope canonical data component

#### Wave 1

- [x] T-ENV.1 — Preflight and freeze pure recovery-envelope-v1. kind: agent stage: preflight deps: none
  acc: Exact contract review CLEAR, source boundaries and hard caps recorded.
- [ ] T-ENV.2 — Implement bounded pure envelope and inventory codecs. kind: agent stage: implement deps: T-ENV.1
  acc: Only new recovery envelope source files and owned receipt; no authority or READY store.
- [ ] T-ENV.3 — Verify canonical bytes, union arms, strict decode, bounds and hashes. kind: agent stage: verify deps: T-ENV.2
  acc: Current source focused checks plus root full-module local validation under real shared leases.
- [ ] T-ENV.4 — Independent review of exact integrated source. kind: agent stage: review deps: T-ENV.3
  acc: Isolated exact-head review with genuine mutation controls and all findings resolved.
- [ ] T-ENV.5 — Merge by normal expected-head GitHub merge. kind: agent stage: merge deps: T-ENV.4
  acc: Fresh holds/base/head/check annotations reviewed under ADR 024; no admin bypass.
- [ ] T-ENV.6 — Verify landed exact tree and preserve original checkout. kind: agent stage: verify-landed deps: T-ENV.5
  acc: Remote landed tree equals reviewed tree, source claim exactly released and authored material preserved.

This component encodes and validates caller-supplied data. It does not authenticate snapshot, approval, journal, writer, provider or pin evidence; persist READY records; authorize effects; implement the recovery coordinator; wire service admission; or establish hosted acceptance. Logical T23.50 and dependent authority owners remain open.
