# Hosted AWS source implementation evidence

## Scope

Baseline: dc7a7629f9bc289aeb420b34120384e4e072b460 (including PR368 and PR369).
Implementation source: b4510adb947f8e0cc5a69101f9fdb8c32d72c937; security toolchain amendment ce1d409 raises the minimum to Go 1.26.9. Documentation-only closeout follows.
This receipt qualifies source integration only. No new AWS resources, trust
policies, release signatures, installed binaries or production data changed.

The source now contains the DynamoDB transaction authority, independent S3
witness lineage, owner inspection and disaster rebind, Linux measured-process
broker and startup lifecycle, witnessed backup cuts and frozen restore, strict
KMS release tooling and explicitly authorized pending-release recovery.
Infrastructure renders a pinned Lambda version with default concurrency zero.
The runtime cannot directly access journal storage or signing authority.

## Local verification

DGX Linux, bounded two-lane verification, Go parallelism one and GOMAXPROCS two:

- Full `go test -p 1 ./...`, `go vet -p 1 ./...`, and
  `golangci-lint run --concurrency 2 ./...` passed at e354049. The final
  b4510ad transport delta passed the actual GNU archive regression, all 125
  deployment tests, and backup vet/lint.
- Authority and backup race suites passed after integrated fixes. Runtime
  broker, service and CLI race suites passed in the runtime lane with the same
  final component source and authority paging changes.
- A race-instrumented releaseauth test binary ran as root and passed the
  trusted-file, credential, persistent-lock, pending/crash and recovery tests.
  These tests were actually executed rather than skipped as an ordinary user.
- All 125 deployment Python tests passed. The real nonempty snapshot producer
  also passed Go create -> tar -> unprivileged Python unpack -> authorized
  frozen restore under the race detector.
- Static Linux ARM64 builds passed for serenity, serenity-authority and
  serenity-release at e354049. Darwin AMD64 CLI cross-compilation passed; this does not
  qualify an Apple runtime.
- The synthetic 10,000-object startup case passed under the race detector in
  6.946 seconds with 234 gateway calls and zero per-object GETs. It uses
  deterministic stores and does not measure AWS latency or cost.
- AWS CloudFormation `validate-template` accepted the infrastructure template
  through a read-only API call. This is syntax evidence, not deployed policy
  or provider behavior evidence.

Hosted CI is unavailable because of account billing. Local evidence is used
under the existing repository delivery policy; no CI status was fabricated.

## Independent review coverage

Authority source through 2cfd3eb was independently reviewed by the coordinator.
The authority worker independently cleared runtime commits d6b1547 and
6752cb3. Release source a41cdad/e0d1f9c/93aa8fa was independently reviewed by the
coordinator. The release worker independently cleared coordinator source at b4510ad: backup,
Lambda/deployment/infrastructure composition and mechanical authority lint
changes. Each reviewer excludes their own authored implementation from their
clearance. External exact-head receipts are retained with the task evidence. The final
GNU root-header regression failed before the extractor correction and passed
afterward; the reviewer independently repeated the real transport roundtrip.

Findings resolved include persistent fresh-data checks after failed enrollment,
post-network peer revalidation, stale-writer/renewal handling, credential-file
ownership and descriptor binding, exact archive layout, root upload isolation,
exclusive Lambda invocation policy, and incompatible binary-only rollback.
Both deployment policies stop the application and broker after activation
failure; compatible units/configuration/authority/data recovery must be
qualified before production. Expired pending release recovery requires a fresh
strict approval bound to exact pending, previous state and installed artifact;
ordinary verification cannot use it to bypass expiry.

## Gates deliberately still open

- Exact live trust-policy and bounded-spend approval, with current identity
  and provider negative tests; the installed AWS CLI must support the required
  full Lambda resource policy API before enabling concurrency.
- Existing-data migration: current production data cannot pass fresh-enrollment
  checks. Preserve a verified quiescent checkpoint and resolve deletion-history
  provenance before any explicit adoption or separate-environment decision.
- Real provider fencing, disaster restore, backup upload and cross-generation
  recovery; source fixtures do not satisfy these acceptance gates.
- Signed release publication, latest-code deployment, authenticated owner
  explorer acceptance and operational rollback.

See hosted-aws-activation.md for the unexecuted infrastructure/policy sequence,
cost scenario and migration boundary, and hosted-release-recovery-amendment.md
for the narrowly scoped release recovery contract. T-HU.6 through T-HU.9 remain
open; source-only landing does not claim the deployment objective is complete.

## Final security and browser qualification supplement

The pinned govulncheck v1.1.4 found reachable standard-library vulnerabilities
with Go 1.26.5. The minimum is now Go 1.26.9; the same scanner reports zero
reachable vulnerabilities and zero affected imported packages. It still reports
two vulnerabilities in required modules whose affected packages are not called.
Gitleaks v8.30.0 scanned the implementation commit range with redaction and found
no leaks. Immutable workflow action-pin validation passed.

All six dedicated explorer browser tests passed against the real HTTP
non-customer `_test.go` fixture in desktop, mobile and dark configurations.
They exercise paging/facets/provenance, inert untrusted text, foreign-brain
isolation and expired-session cleanup. This fixture is not production startup.

The separate full hosted browser invocation did NOT pass: three signup cases
failed before readiness, six explorer cases were skipped without their optional
fixture binary, and three cases did not run. The unchanged signup harness
invokes ordinary hosted serve without the now-required broker socket. Baseline
dc7a762 also unconditionally refused hosted startup without admitted authority.
No development bypass or production fake was added to make that harness pass.
Full broker-backed signup/runtime browser acceptance remains open under T-HU.6;
the separate successful explorer fixture does not close it.

Root-executed release filesystem/locking/recovery race tests also passed with
Go 1.26.9. Linux AMD64 and Darwin ARM64 application cross-builds passed on that
toolchain; cross-compilation does not establish native runtime acceptance.

Final Go 1.26.9 verification completed on the ce1d409 source: full repository
`go test -race -p 1 ./...` and `go vet -p 1 ./...` passed; full golangci-lint
reported zero issues. Linux ARM64 static builds of the application, authority
Lambda and release verifier passed. These supersede the earlier toolchain's
source checks without changing the open provider/runtime acceptance gates.
