# Prepared episode installation and forward recovery — T5.86–T5.93

Base7f93325b7f911a2fd4653a23ccb91a32e56f79dd. Initial source candidate
a16dad04d16f837189d03e43f313a1ddca3e75fc passed focused normal-path race checks
in29.414s on the existing bounded Linux ARM64 runner. Independent review found
three issues before merge: unbounded directory enumeration (INST-R1), torn final
transaction records (INST-R2), and caller-owned slice reuse (INST-R3). All were
accepted and corrected. Revised source dcba668e746d581b46c68137ce867f64db5d6cd6
passed the full race and process-death suite in 287.614s. Its archive SHA256
25a905a4c2da7d436c5d3e064f42e8de8caef2ded3422da34be35c1d2cebd03b matched
before extraction. Independent non-author review of that exact source is clean.

The initial static run passed vet but found unchecked reader-close calls and
import formatting. Commit6360709e makes those explicit and checks test-reader
cleanup, without changing preparation/transaction control flow. Its fresh exact
archive SHA256f22a333b11d121994c20914e083bb1a760f3ff48f44d6af15619d49e9253c086
matched on the runner. Fresh affected helper/installed-reader race checks pass
in3.594s; vet passes, lint reports zero issues, scoped govulncheck reports no
vulnerabilities and the CLI build passes. The full crash suite is reused from
dcba668e because the correction changes no transaction behavior. All Go checks
use Go1.26.9, GOMAXPROCS2, private temporary storage and bounded serial execution.
This is scoped dependency evidence, not a whole-project vulnerability audit.
Final aggregate review, secret scan and guarded landing are recorded in the PR.

The internal episodeinstall library adds inactive durable preparation and a
fixed forward transaction. It has no HTTP/model capability and is not wired to
production activation. A trusted cold owner must authenticate actual account,
brain, root custody and complete current relevant exclusions on every admission;
a live preparation cut, local manifest or historical approval cannot do that.
Cold admission must work while canonical components are partially installed.

Preparation acquires ordinary writer ownership, requires the supported empty
control-state cohort and exact source inventory, and creates an exclusive private
operation. Original Git/canonical proof, original index, mutable inspection copy,
bare candidate, nonbare replacement and new lexical index have distinct fixed
locations and complete inventories. It rejects malformed pages before index
rebuild can log them, invokes no providers, closes all database handles, syncs
all copies and pins a canonical header. Private routes are hashed in that header
and omitted from exported references. Preparation never installs or purges.

The trusted owner subsequently acquires the persistent maintenance marker.
Install revalidates the entire old source after that handoff. It moves only the
fixed canonical components and index family; root/runtime directories and stable
lock inodes remain. Exclusive Linux/Darwin renames use pinned parent descriptors
and cannot replace unexpected destinations; other platforms refuse. Every move
is covered by a durable complete intent and parent-directory syncing. Resume
uses actual old/retained/staged inventories, explicit absence and fresh admission,
never an old-state rollback or mutable phase flag. Completion leaves marker and
ownership held for independent owner closeout. All old copies remain sensitive.

Record publication uses two fixed content-free pending files. A pending record
must be an exact private owned regular single-link byte prefix of its expected
canonical record. After fresh admission/slot checks, recovery appends missing
expected bytes, syncs and exclusively publishes it. Unknown bytes, duplicate
pending/final state and premature completion refuse. This closes the initial
reviewer's process-death gap after file creation or partial writing.

The preparation caller's fences and preservation pins are decoded into a fresh
request before sorting. Tests mutate original caller slices during preparation
and verify immutable captured identity/header values. Directory walks use
bounded batches with contextual and aggregate limits, including fixed-layout
entry caps. Rooted copy reads cannot escape the source through substituted
symlinks. No global logger, umask, credentials or runtime configuration changes.

Actual synthetic fixtures use hosted provisioning, pool open/drop, writer source,
entity and episode operations, complete native exclusions, independent sources
and an episode-dependent checkpoint. Qualification must prove retained bytes,
lexical lookup, held-reader/pool refusal, stable lock inodes, reopened hosted use
only after explicit fixture owner closeout, and excluded event/checkpoint absence
from every new Git object. Real process-death tests exercise component rename and
both-parent sync boundaries, plus creation/partial writing/file sync/publication/
directory sync of both intent and completion. This is Linux process-death evidence,
not host power-loss or installed production qualification.

Remaining gates: unknown partial Begin marker reconciliation, actual cold-owner
integration, nonempty database controls and broader runtime cohorts, ambiguous
legacy derivatives, all controlled-copy disposition, host/power-loss acceptance,
authenticated production acceptance and capture. T5.2/T5.3/T1.3 remain open and
capture stays off. Three coherent outcomes under default ceiling3 retain current
session selection; token/cost telemetry unavailable. Ajent MCP is unavailable;
repository/GitHub coordination and merge gates still apply. ADR024's confirmed
billing-blocked Actions fallback never changes protections or buys capacity.
