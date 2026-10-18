# Canonical reader/maintenance gate — T5.74–T5.79

Base2529d034025886093dbd2ad076eac63bb91237ea. Revised production source
887834098f5ccd89f8a0eece30e1715ebf023921; archive SHA256
d5ecf3dee83bcd2766873882d2c4751451e6696cc9e2d95134d87a18c1f3ad9c
matched on the existing Linux ARM64 runner. Earlier component archives were
also integrity checked; component correspondence is stated below.

A permanent canonical-view.lock now admits shared cooperating readers and
exclusive maintenance. Normal writer ownership alone initializes the empty,
private, regular, single-link, user-owned inode and synchronizes it. Readers
open existing state without creation and check the persistent marker after
shared ownership. Maintenance acquires view-exclusive before writer-exclusive,
retaining both through Finish until Close. Neither stable lock is unlinked.
Unsafe/missing state, held maintenance and surviving markers refuse disclosure.
Readers are excluded from maintenance, not given ordinary-write snapshots.

Actual CLI check/doctor/status/search/ask/report entry points, hosted cold/warm
inspector, per-brain inventory, governed lookup and pre-runtime episode replay
hold the shared lease through canonical use. Static connector status remains
filesystem-free. Public inventory takes maintenance before account ownership;
internal callers avoid recursive maintenance locks. Replay keeps its shared view
through transition to pool writer ownership.

Compatibility corrections were tested rather than hiding failures:

- Initial full writer race passed51.700s; gateway failed two actual restored
  brain cases because restored runtime state is intentionally absent. Admitted
  recovery is already a canonical writer: only an exactly missing view lock
  permits explicit AcquireBrain/Close before retrying the shared lease. Unsafe,
  busy or pending state cannot be repaired. Ordinary readers remain strict.
- Initial CLI suite exposed manually initialized fixtures lacking actual writer
  setup; shared fixtures now perform real owner initialization. Subsequent CLI
  checks exposed unrelated hook no-op and blocking-exit contracts. Hook input
  parsing stays filesystem-free; only a real ExitPlanMode check obtains a read
  lease and maps failure through the existing blocking exit2 contract.
- Source audit caught serve decoding config before ownership. Existence-only
  admission now precedes ownership/decode; malformed named config may create
  minimal lock metadata, never hooks. Pending maintenance exposes neither keys
  nor false transport fallback. Generic transport creates no runtime state and
  never performs a second unowned HTTP config read.

Observed bounded Go1.26.9/GOMAXPROCS2/private-temporary-storage checks:

- Full writer race51.700s at ee6a96cc; subsequent writer production delta is
  static diagnostic text only.
- Full gateway race40.227s at ad83332f; gateway source is unchanged at88783409.
- Full CLI race52.877s and full hosted-service race357.626s at88783409.
- Four-package vet passes; lint reports zero issues; scoped govulncheck finds
  zero called vulnerabilities (two required-module-only advisories remain);
  CLI binary build passes. Final gitleaks and full Wazi checks pass.
- Actual cross-process read/maintenance exclusion, process death, surviving
  marker, stable-inode retention and error-path lock release are exercised.
  CLI execution and real gateway/pool/Git/restore fixtures cover joined paths.

Independent non-author aggregate reviews at ee6a96cc/ad83332f and final hook
source delta88783409 found no remaining findings. Final evidence/document review
and exact guarded landing are tracked separately in the PR. Operator guidance
corrects stale export wording from the already-landed disclosure fence.

This is source/synthetic evidence. No installed/native/provider/power-loss or
physical-purge acceptance, active installation or capture activation is claimed.
Old binaries and direct filesystem/private-helper callers remain unqualified
readers and must be drained or admitted before installation. Full T5.2/T5.3/T1.3
remain open, including runtime-artifact classification and fresh cold authority.
Batch ceiling3, two implementation outcomes with disjoint authors and independent
review; inherited session selection, token/cost telemetry unavailable. Ajent MCP
unavailable; repository/GitHub coordination checked. ADR024 fallback applies only
to confirmed billing-blocked Actions without modifying protections.
