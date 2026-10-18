# Same-release maintenance owner — T5.34–T5.36 source evidence

The owner assigned source reconciliation and autonomous delivery. The versioned
owner is now `deploy/hosted/maintenance`: adopted generic mechanisms replace the
formerly unversioned source boundary without creating a second active controller.
This is source delivery authority, not production maintenance, provider freeze or
restore, credential reissue, capture enrollment or a deployment permission.

## Current identity and provenance

Read-only existing-host observation at2026-10-18T12:04:14Z found installed/highwater8,
service source30919c2fe1581ff2c0e31475dc0b49cb2fd0926a, expected executable/broker/
config identities, normal hosted/broker and timer active, backup inactive, and no
hold/pending state. Archive/workos/billing are false; synthetic enrollment is
empty. Observation held the release lock shared and used no SQLite, provider
mutation or service control. Private inputs/results remain outside Git. This is
identity evidence, not drained custody, current CLOSED authority or approval.

Generic donor filenames/hashes are in `maintenance/provenance.json`; originals
remain preserved. Rendered old6-to8 packets, credentials, approvals, saved provider
observations and private principal/host constants were excluded. CONTRACT.md
freezes exact same-release identity, finite trusted owner admission, typed unit
custody, authenticated read-only authority collection and a distinct v3 proof.
Existing v1/v2 admission remains strict. Operational root code never queries
SQLite or reads fixture credentials; those actions retain the service UID.

## Qualified source

Code source: **cec1f80dddf30a18587aa6859401398a2fffc01e** on reviewed base
378efd187d4c0871f063096bf2ed15b016036fa4. Exact Git archive SHA256:
a2c8c0466261d7762e9d0f582896b7f6c2ce12f1b023a92ec031d3651951d8d5.
Transfer integrity was checked before extracting into an isolated Linux workspace.
Builds use existing Linux ARM64/Go1.26.9 capacity, GOMAXPROCS2, umask077 and private
temporary/cache storage. No new runner or paid capacity was provisioned.

| Check | Observed result |
| --- | --- |
| Operator package race suite |45.896s, pass at c6e83342; operator source/tests byte-identical at cec1f80d |
| Maintenance Python discovery |50 tests pass at cec1f80d,19.791s |
| Legacy/operator Python discovery |24 tests pass at cec1f80d,0.101s |
| Vet |Operator and synthetic helper pass |
| Lint |0 issues after equivalent helper tagged-switch cleanup; exact helper bytes match final source |
| Scoped vulnerability check |No vulnerabilities found |
| CLI build |Pass; product Go inputs unchanged by continuation fixes |
| Darwin arm64 operator cross-build |Pass; compilation only |
| Real Linux service-UID/FD harness |16 synthetic cases pass against exact source-stamped operator |
| Systemd transport |Read-only existing harmless service/timer on systemd255 qualified direct typed property shapes, Service.ControlGroup, Conditions ABI and manager envelopes; affected transport code unchanged |

Exact source-stamped operator SHA256:
182aa76e483868c03f48b1388c4b8193ce9fe10546d588286d3a914a633a601a.
Separate synthetic helper SHA256:
e59fd661806ed5f380092eb762dfbb684c1782b686f6fe6bdd4515916ac5ef0f.
Harness SHA256:3a32b4898faed36eddc8c3067d91689f526867c9a73c01ca792efb455b2b3e74.
The helper is never linked into the operational child.

The actual UID1800/GID1800/no-supplementary-group/six-FD harness uses the real
pure producer, strict Python proof validator and Go child with synthetic stores.
It checks wrong source/expiry/lock/UID/schema/hash/boot refusals with unchanged
data/private trees; prepare/replay, metadata snapshots and revoke/replay; active
legacy adoption refused unchanged; then externally revoked-with-retained-custody
legacy adoption, reissue and revoke. It verifies exact credential states, retained
account/brain identities and parent lock ownership. External revocation is modeled
only inside new synthetic scratch data; operational adoption checks are unchanged.
Scratch removal is not a secure-erasure claim. Provider and real systemd wrapper
qualification remain false in the receipt. The earlier working-tree build used
explicit synthetic e40 identity; final qualification uses the actual code commit.

## Independent review and recovery findings

Non-author review accepted the contract before dispatch, then reviewed complete
candidate/base diffs. MAINTOWN-R1–R4 are resolved at cec1f80d: verified no-child
abort covers interrupted hold/readiness publication; late closeout reconstructs
exact original hold and held startup after supervisor containment; zero/partial
progress, completion, guard and permit writes reconcile under fresh authority;
reserved-domain custody includes exactly the two manifest accounts.

Hash-bound progress staging has immutable admission/completion dependencies.
Readiness prefixes remain inactive. Old guard prefixes never cause an old
supervisor reload. Permit contexts precede SHA-bound stages; pending grants are
revoked, never promoted. Unknown artifacts remain untouched/refused, while
containment still stops owned units. Expired grants never authorize new fixtures.
R1–R3 continuation tests include real partial writes, renewal, supervisor-exit
containment, unlink/directory-sync interruption and preserved unknown bytes.

Source re-review cec1f80d and final evidence/plan review
d75cc3d168012b2c62fc4f10d32debbc89ca16ab on378efd18 are clean. T5.38/T5.39 are not pre-marked; exact guarded merge and landed tree/ancestry
verification will be recorded in PR412 and the subsequent plan closeout.

## CI and remaining operational gates

PR412 Actions run39130459408 at c6e83342 had14 jobs with zero executed steps
(13 failed, one cancelled). Test job117684385489 reports: “The job was not started
because your account is locked due to a billing issue.” Main protection returns404;
rulesets are empty. Existing ADR024 owner-authorized scoped local/Linux fallback
applies without retries, paid capacity or policy changes. This is unavailable
hosted CI, not a hosted pass. Ajent tools are unavailable; GitHub and available
repository coordination records were checked, and trusted holds remain blocking.

Actual current host/systemd/service-UID/provider qualification and an exact fresh
independently reviewed finite private operational packet remain required for
T5.40/T3.14. No old approval, synthetic source stamp, local flag or hash is current
authority. No production operation occurred. Capture stays off; old binary/data
is never restored as rollback. Full memory erasure/integration and the wider
automatic-memory plan remain open.
