# Initial maintenance recovery — E14 source evidence

This increment reconciles an initial absent, zero-byte, partial or full maintenance
gate under a distinct fresh trusted admission. It never infers the earlier actor
from matching prepared copies. Prior intent remains explicitly unattributed.
Capture, production installation and operational authority remain unchanged.

## Boundary

The restricted writer owns the existing exclusive canonical-view and writer
locks, pins the root/runtime/lock/marker identities and appends only missing exact
canonical marker bytes to the same inode. It exposes no Finish or activation.
An absent gate is exclusively created and synchronized before evidence binds its
actual inode. Existing gates also undergo no-append SyncGate file/runtime/root
synchronization before immutable evidence publication; observation alone would
not establish durability after a failed Begin.

The episode-install wrapper checks the complete prepared header and all copies,
entire old canonical/index cohort and fresh ordinary plus initial-recovery
admission. Installation/retained progress and unknown artifacts refuse. The fixed
private recovery record preserves original bytes/metadata, header and identity;
pending exact prefixes may resume before append, but a later longer marker never
becomes a new original. Published evidence is not authority. Complete rechecks
fresh admission before append and after durability, then transfers ownership once.
Ordinary Install/Resume validates any recovery evidence before admitting it.

## Source and qualification

Initial source: 7b955283ca6c86a8318da1e034694ad65b7f1119, base
468f30bf0b5ada74a3fd98303a06cccf3bdee888. Initial archive SHA256:
f243add2b9b1cca8de200f69c6b885ed79c2ac29d3c9324f11c3e55b9a77ef33.
Corrected source: 249ea546f869a0f6f0649ffce53e1478a325a08e; archive SHA256:
5b210862e8ff71cc95052b577239dc34205c9b9ba4f29aefa99c1287e542d597.
Transfer integrity was verified before extraction into isolated existing Linux
ARM64/Go1.26.9 capacity with GOMAXPROCS2, umask077 and private temporary storage.
Checks run serially; no new runner or spending was introduced.

| Check | Observed result |
| --- | --- |
| Full writer race suite | Pass at initial source, 54.933s; entire writer inputs byte-identical at corrected source |
| Focused initial-recovery race tests | Pass at initial source, 94.662s |
| Corrected full episode-install race suite | Pass, 382.318s |
| Vet / lint / scoped vulnerability check | Pass; lint 0 issues, no vulnerabilities found |
| CLI build / Darwin writer cross-compile | Pass; cross-compile does not establish native acceptance |
| Native Linux foreign-owner/safe-metadata fixtures | Pass, 10 subcases with root-only foreign-UID branches actually exercised |

Authored and exercised initial-source coverage includes 11 writer and 15 wrapper
abrupt process-death boundaries, actual cooperating reader/writer refusal,
same-inode prefix completion, file/directory synchronization failures, admission
loss/cancellation, immutable/pending evidence, changed source/index/progress and
actual hosted-pool/Install/Resume integration. Independently prepared A/B copies
cannot identify an empty marker's origin; ordinary ColdAdmission alone does not
satisfy the new role. Retained copies remain explicit after installation.
Foreign-owner cases were additionally executed in isolated root-owned synthetic
scratch data using the exact corrected-source race test binary, SHA256
f9edab24215c26d80814b840be10d902354651b28dd2bf449c99760cc9129c0b.
Both restricted and legacy owners reject a foreign UID; the 10 actual subcases
also cover hardlinks, symlinks, modes, replacement and root/lock movement.
No host user, production path, service or credential was changed. A harness
string-escaping setup error was corrected before this test invocation; it was
not a product-test failure.

## Review and limits

Independent design review accepted the full initial failure class and the later
SyncGate durability correction before qualification. Independent whole-source
review found INIT-R1: creation could leave a persistent gate while an error result
reported marker_held=false. The corrected source conservatively reports that a
gate may remain after any attempted creation, documents uncertainty and tests
precreation rejection without asserting actual presence. Independent re-review
resolved INIT-R1; final evidence/head review remains pending.

Batch ceiling3 used two disjoint implementers and root integration, with a
separate non-author reviewer. All inherited the current selected model/effort;
provider cost is not exposed. No comparative acceleration claim is made.

This is library/source qualification. No production initial-recovery issuer was
invented, no hosted controller was installed, and no deployment, provider freeze,
credential change, enrollment, marker Finish, retained-copy disposal or physical
completion occurred. Full T5.2/T5.3/T1.3 and operational packet/live acceptance
remain open. Hosted CI, guarded merge and landed correspondence remain separate.
