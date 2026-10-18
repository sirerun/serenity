# Authority-bound inactive preparation — T5.68–T5.73

Base e81590013e0bda743eef4f105c513e024c552d2f. Production source checked at
69768f007d68b4d887cf8f5e970675b67fc756cb; archive SHA256
38680a7a79ae6534bcbb19f3174a3b5782cd125e0b45c122998959d2ee069a3f
matched on the existing Linux ARM64 runner. Commit 6de7c9fd adds only an
intentional nil-context negative-test lint annotation; the exact revised test
was transferred and rechecked (SHA256
a090b8a1de39d0c6c64cc9da360016b52a5d28433a25063f9b157a73efcde07f).

An internal service operation now prepares an inactive hosted erasure candidate
using the existing authenticated authority client and lease. Complete cut
observations bind exact position, expiry, epoch and local fence inventory in
both directions. Fresh authority and source checks surround preparation. The
caller cannot supply an authority snapshot or destination. CLI dependency wiring
uses the existing broker; there is no HTTP/model operation or active installation.

The operation owns a private operation container. Any failure after ownership
reports possible inactive residuals, including failed unpublished stages. Retry
cannot silently adopt them. Existing account/brain admission, service maintenance
exclusion, pool closure and stable writer ownership apply. Cancellation-aware
queue and pool closure retain runtime/index/writer resources until draining is
actually complete, refuse new acquisition while closing, and permit explicit
retry. Preparation closes its target before flushing other runtimes, avoiding a
retry trap after cancellation.

Independent review accepted AUTH-R1 (unattributed failed staging copies) and
AUTH-R2 (background pool shutdown could outlive the operation deadline). Owned
containers and cancellation-aware lifecycle APIs resolve them. The non-author
reviewer re-reviewed the aggregate source at 69768f0 with no remaining findings.
Final documentation/test-annotation review and guarded landing are recorded in
the PR rather than anticipated here.

Observed Go1.26.9, GOMAXPROCS2, private temporary storage, serial bounded commands:

- Full writer/pool/episodemaintenance/maintenancegate race suites pass:
  50.362s / 6.360s / 1.536s / 2.722s.
- Focused service preparation race tests pass 10.027s, then 10.052s after
  the test annotation; hosted CLI race tests pass 1.050s.
- Earlier selected service source/episode/preparation/authority/current suite
  passed 332.149s on the initial source, before review corrections. This is
  supplementary evidence only; corrected paths have fresh focused coverage.
- Affected five-package vet passes; lint reports zero issues.
- Scoped govulncheck reports zero called vulnerabilities; two module-only
  advisories outside called code remain. CLI binary build passes.
- Actual authority kernel/server/client fixtures, actual service HTTP episode
  append/checkpoint/exclusion and actual Git/pool/provision paths are exercised.
  Fixtures use local authority storage, not live provider custody.

Source/synthetic checks do not establish installed/provider/production behavior,
power-loss durability, complete mixed derivative closure, active installation or
physical-copy disposal. T5.2/T5.3/T1.3 and automatic capture remain open/off.
Batch ceiling3, two implementation outcomes with separate authority/service
owners and independent review. Session model/effort inherited; token/cost
telemetry unavailable. Ajent MCP unavailable; repository/GitHub coordination is
checked. ADR024 proportional source fallback applies only to confirmed billing
blocked Actions without changing protections.
