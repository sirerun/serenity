# Owner-approved automatic-memory integration takeover

Source base: `eef5abf6ca389a7b45abc4c9149c72e691ef66e6`.
The owner assigns the prior integration lanes and approves autonomous work for
16hours; renewed window ends2026-10-19T01:43:56Z. Current attended Codex
engineering executor and all helpers inherit current model/effort/account.
Default implementation batch ceiling3 remains; source author and independent
reviewer are separate. No production release/custody admission is invented.

## Preservation and reconciliation

Six dirty/untracked backup files were preserved in an external archive with
SHA256 `5f635273c7945356607864a9ced4daed3232bd98b4d73025ff6a08e2afc3c6b1`.
The retained manifest records each exact path/size/digest. Originals remain
untouched. Three test/helper files equal main; the other implementation/test
changes are superseded by explicit build identity, bounded manifest parsing,
atomic publication, authority cuts and later cancellation/custody hardening.
A separate read-only reviewer found no old behavior that should be copied back.
The old build-identity fallback test would restore a deliberately retired path.

The old maintenance source62550e9a is not an ancestor of this base. Its related
landed correction is8b8d7767/PR335, with a different patch identity. This is a
behavior reconciliation, not an ancestry/equality claim. PR273 remains retained
for its historical provenance, rather than being merged over newer interfaces.

After explicit owner assignment and preservation, the exact observed claim refs
T23.48/T23.49 and R-hosted-assembly/R-shared-memory-20261016 were CAS-released.
T5.2/T5.3 and replacement resource claims now bind this integration session.
Other claims and all old worktrees remain unchanged. Current process inspection
found no referenced old-lane command; that snapshot is not proof no other worker
exists. Ajent MCP is unavailable; repository/GitHub coordination must still be
checked at boundaries. No external coordination messages were sent by this audit.

## Implementation findings

Existing backup admission uses the current service maintenance owner and
broker-backed immutable authority cut. Integrity does not imply current episode
eligibility or purge. Hosted canonical repositories contain .gitignore,
serenity.yml, entities/claims and .dira records beyond raw episode pairs.
Freeform legacy fact provenance cannot establish a complete typed support graph;
ambiguous dependencies must refuse rather than preserve or discard everything.

The stable writer lock is inside .serenity and must never move during canonical
replacement. First source increment adds persistent maintenance admission shared
by cooperating CLI and hosted writers. It grants no activation, freshness or
physical-purge authority. Later installation must consume current authenticated
journal cuts, retain this lock, account for old/index/stage/export/backup copies,
and remain unavailable across interrupted/unknown phases.

## Verification environment and limits

Mac load23.01 exceeds the existing10threshold, so no heavy local build is used.
The existing Linux ARM64 runner has cached Go1.26.9 and ample available memory;
checks use owned source snapshots,2CPU workers, private temporary directories,
serial heavy lanes and bounded watchdogs. No credentials or private operational
inputs are transferred. Current149-task Wazi reader/schema/semantic gate passed
before dispatch. This is source work; native UI/provider/installed qualification
and full-repository evidence remain distinct. Token/cost telemetry is unavailable.

## First qualification and review findings

Initial source474151870907edbc61f20353c5fa462be4100834 archive
f8d040dce79798ad3a39eb69162d1e71ced7cec90193f552ce9e83d548624687
passed writer race checks (50.587s). Pool checks exposed an assertion mismatch:
AcquireExisting refuses cold runtimes with ErrCapacity before ownership; its
expected result is corrected while preserving exact no-mutation checks.

Independent review accepted two blocking findings before merge: MAINT-R1 requires
brain-parent directory synchronization so a newly created .serenity entry is
durable; MAINT-R2 requires Resume to synchronize marker and directory rather
than trust readable page-cache bytes after a failed Begin sync. Both require
affected fault-injection checks and exact revised-source re-review. The initial
source is not approved or landed. Finish post-unlink sync failure remains an
unknown completion that may conservatively resurrect a gate on restart.

## Corrected source qualification

Revised production sourceb13720a1c506605c64922ade5cec08f2997c5c29 archive
3c6f5a3180a6620da1ace355cb32de9fe45b65ec7277515d08d98f91187609f8
passes `go test -race -count=1 -p=1 -timeout=360s` for internal/writer,
internal/hosted/pool and internal/gate (50.023s,5.637s,2.806s), plus
`go vet -p=1 ./internal/writer ./internal/hosted/pool`.

Lint first found five unchecked test cleanup returns. Test-only correction
2c61c0231d0beea624d3a273434a2ac282fd7604 was transferred by exact file hash
485fdfbdf2108f99e2befa1af3f1c08905a86cb232f3e85d08b0175bb90bb644;
production source is unchanged. Focused maintenance race passes (4.377s),
affected lint reports zero issues, and scoped govulncheck finds zero called
vulnerabilities. Two module-level advisories are outside called code; this is
not a claim every required module is vulnerability-free. Formatting/diff pass.

Separate reviewer `/root/review_maintenance` accepts MAINT-R1/R2 fixes at exact
b13720a1 after checking all changed source and callers. Final test-only cleanup
and documentation delta still needs exact-head review. Full repository suite,
physical power-loss experiments, native host acceptance and production are not
qualified. The per-call injected fsync failures verify refusal/retry behavior;
they do not reproduce a physical power cut.
