# Hosted AWS source implementation evidence

## Scope

Baseline: dc7a7629f9bc289aeb420b34120384e4e072b460 (including PR368 and PR369).
Implementation source: b4510adb947f8e0cc5a69101f9fdb8c32d72c937, plus documentation-only qualification closeout.
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
