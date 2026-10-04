# Experimental contract 0.0.1

This headless interchange has three separate records: `definition` is authored
intent, optional `snapshot` is an observed current execution, and `evidence` plus
`evaluations` record observations and policy-conditioned judgments. The bundle
uses `contractVersion: "0.0.1"`. JSON Schema Draft 2020-12 validates shape; the Go
validator checks the coherence rules below. Neither check authenticates receipts,
proves current grants, executes dependencies or supplies canonical admission.
All `verified`, `complete` and `satisfied` values are producer assertions until a
consumer independently qualifies them. A browser import never authenticates them.

## Identity and authored authority

- Plan/task/requirement/unit/evidence/evaluation IDs are opaque, case-sensitive,
  stable logical identifiers. Plan and task IDs must be qualified (contain `:`),
  such as `example:plan` and `example:plan:T1.1`. Task IDs are unique in a bundle;
  all references resolve within it. IDs are never inferred from filesystem hashes.
  Consumers retain their authority namespace across revisions; qualified IDs in
  different authorities do not collide.
- Each definition declares one source: `markdown` or `native`, an `authorityId`,
  retrievable `ref`, source `revision`, SHA256 digest and normalization
  `adapterVersion`. Definition revision/digest equal that source revision/digest.
  Digest means SHA256 of the exact authoritative source bytes, including newlines;
  it is not a self-hash of normalized JSON. Consumers must retrieve and hash the
  source separately to authenticate it. One revision cannot have two writable
  masters; portable JSON and Markdown views of native plans are not authorities.
- Markdown task source requires a positive line and the same `ref` as the
  definition source. Native task source requires `canonicalId` and the same `ref`.
  `source.raw` and `metadata` retain opaque authored text and unmapped fields.
  Acceptance is a nonempty opaque string: normalization must preserve authored
  wording/multiline content, and cannot manufacture missing acceptance to satisfy
  validation. Missing acceptance is an out-of-band normalization diagnostic;
  emit no fabricated valid task or completion.
- `authoredStatus` only describes checkbox/author assertion; it never fulfills
  a dependency, approves review, proves landing or proves deployment. Dirty input
  is represented by `definition.metadata.sourceState: "dirty"`; no satisfied
  current evaluation may be accepted from that snapshot. Unknown cleanliness is
  not proof of clean input and still needs consumer source qualification.

## Logical gates and canonical execution

- Stages are nonempty open strings. Preserve `author`, `fix`, `rereview`, `deploy`,
  `verify-deployed` and unsupported tokens verbatim. UI grouping is descriptive;
  it cannot change readiness or turn an unknown stage into Build/implementation.
- Dependency predicates are exactly `execution-complete`, `checks-satisfied`,
  `independent-approved`, `landing-verified`, `deployment-verified`, and
  `domain-accepted`. `execution-complete` may omit `requirementId`. All other
  predicates require a requirement on the dependency's referenced task with the
  same predicate. Dependencies cannot self-reference or form cycles.
- Requirement IDs are unique, reference a task and pin a named policy revision
  and subject. Domain acceptance also requires an explicit namespaced domain
  (containing `:`); it cannot stand in for review/landing/deployment.
- Checks and review can run in parallel against the same head/base. Merge joins
  their separate gates. A negative review remains unsatisfied. A bounded fix can
  depend on execution completion of the review, without depending on successful
  approval; re-review subsequently addresses that fix. Stable delivery gate IDs
  stay unchanged, with current evaluations bound to the new candidate/attempt.
- A canonical execution unit has exactly one authority/canonical ID and explicit
  task/stage coverage. A task can belong to at most one unit; its unit reference
  and the unit's task list agree; every covered task's stage is listed. Duplicate
  `(authority, canonicalId)` units are rejected. Only that service admits and
  advances enrolled work; no portable task row authorizes a second dispatch.

## Revision-bound observations and evaluations

- Snapshot plan identity/revision/digest matches definition. One current execution
  per task identifies its attempt, state and optional matching execution unit.
  Historical attempts belong in audit evidence, not duplicated current rows.
- Evidence IDs/evaluation IDs are unique. Evaluations also have a unique logical
  key `(requirementId, taskId, attemptId, planRevision, policyRevision)`; conflicting
  current judgments cannot coexist under different IDs. Historical evaluations
  must use a different bound revision/attempt and remain nonsatisfying.
- task/requirement/evidence references
  resolve. Non-audit evidence matches the current definition and current task
  attempt. Evidence may retain old revisions/attempts only with `auditOnly: true`.
  Canceled/expired current execution evidence is audit-only; an observation must
  not revive terminal execution. Audit evidence cannot satisfy a current gate.
- Every evaluation matches its task's requirement. A satisfied evaluation requires
  `qualification: "verified"`, exact plan revision/digest, current nonterminal
  attempt, exact requirement policy, and at least one qualifying evidence item.
  Its requirement subject's every field matches both snapshot subject and evidence
  subject. Review/landing/check predicates require head and base; deployment
  requires artifact and environment; domain requires artifact and namespaced
  domain. Execution completion requires the current state `complete`.
- Qualifying evidence is non-audit, `trust: "verified"`, `result: "passed"`,
  same task/attempt/revision/digest/policy/subject, and correct kind: execution,
  check, review, landing, deployment or domain respectively. Context cannot supply
  execution authority. Missing/unverified/stale/rejected/unknown/canceled/expired/
  unsupported/unavailable observations stay explicit and cannot qualify success.
- Satisfied deployment additionally names `approvalEvidenceId` referencing a
  verified/passed policy-approval item included in its evidence IDs, with metadata
  `scope: "deployment"` and the same task/attempt/revision/policy/subject. It never
  inherits source-merge permission.
- Independent approval requires review evidence with an explicit `independence`:
  `contributors` mode claims the complete author/fix contributor set, nonempty,
  excluding the verifier. `authority-attestation` mode requires named authority
  and retrievable attestation provenance and also excludes any listed verifier.
  `singular` mode cannot establish all-contributor independence. A validator
  checks the claimed relationships only; authenticating the complete contributor
  set or authority attestation is still a consumer requirement.
- An unavailable hosted check can coexist with satisfied checks only when the
  requirement explicitly `allowLocalAlternative: true`, the evaluation references
  both unavailable hosted and passed local observations, and `alternative` names
  a referenced verified/passed policy-approval evidence item for the same subject,
  task, attempt, plan and policy. Scope is exactly `source-merge`. This retains
  unavailable CI rather than fabricating its result, and never grants deployment
  or weakens protected-branch policy. Alternative objects on other predicates
  are rejected. Policy approval must itself be authenticated by the consumer.
- Other outcomes retain truthfully reported failure/unknown/unavailable/stale/
  unsupported judgments and provenance. They cannot satisfy readiness. Unknown
  schema fields are rejected outside explicit `metadata`; unknown contract
  versions fail closed. Extension metadata cannot override core fields.

## Source mappings and trust boundary

Ordinary Markdown maps source task IDs into a persistent authority namespace;
row line/FNV display IDs are retained as migration metadata, never universal IDs.
Moving a file needs an explicit authority mapping, not a silent identity change.
Experimental 0.0.1 supports one authoritative Markdown file per definition.
Split root/include plans cannot be flattened with invented root source locations
or digests: normalization reports unsupported split-source input. Multi-file
authority/digest rules require a later separately versioned contract. Missing
stage/acceptance similarly produce diagnostics instead of invented defaults.
UUID/canonical service task IDs remain opaque. A native adapter uses the actual
service source revision/digest and canonical IDs; it does not parse a rendered
Markdown view as another master. Negative review/fix, checks/review joins and
compound enrollment stay service-owned. AMOS narrative ACCEPTED/REVIEWED belongs
in metadata/authored status with unknown/unverified judgments until receipts are
qualified. Serenity context is context evidence, never execution authority.

The neutral fixture catalog exercises mapping coherence and trust boundaries;
passing it is not runtime interoperability, receipt verification, policy approval,
CI availability, accepted deployment or release evidence. Consumer owners must
pin the exact revision, validate first, then perform their own trust checks.

## Portable reference safety

Core source/receipt/attestation references use logical URNs, repository-relative
paths without traversal, or HTTPS URLs without credentials. Absolute local paths,
file URLs, loopback/private-address URLs and URL user information are rejected.
This does not inspect arbitrary opaque metadata or authenticate remote content;
consumers must scrub private metadata before any export/publication and verify
retrievability separately. Native request/sequence/lifecycle qualifiers belong in
namespaced metadata and require the native consumer's own exact binding checks.
