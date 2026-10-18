# CLOSED history and installation admission — proposed v1

Status: API/source design independently reviewed on 2026-10-18; ownership
coordination and complete Wazi admission precede implementation. No operational permission is granted.

## Required outcome and staged delivery

Full T5.2/T5.3 needs actual current external exclusions and an actual caller, not
another caller-supplied assertion. E15 supplies authenticated complete current
CLOSED history using the existing authority owner protocol. Its dependent E16
must connect the existing root maintenance controller to a service-UID installer
child with fresh per-phase root custody/history checks. Neither outcome activates
capture, closes the maintenance marker, disposes of copies or establishes live
provider/installed-host acceptance. Controlled-copy disposition remains separate.

Existing cuts are historical capabilities. A current CLOSED namespace may have
no cut; appends after an earlier cut make that cut incomplete. Owner reads of
expired cuts remain historical reads. Existing live-lease episodemaintenance.Source
cannot be used as cold authority. Do not restrict the feature to convenient
existing-cut cases. Freshly ENROLLED is not CLOSED; an empty-history CLOSED test
must actually prepare/close or otherwise reach CLOSED through the real protocol.

## E15 observational kernel

Add Kernel.ObserveCurrent(context.Context) (Record, error), without changing
Current's existing mutating reconciliation behavior. ObserveCurrent validates
context/configuration and performs only bounded reads:

1. Strongly Load META; require found, no Pending, valid revision/tip and configured
   lineage/table identity. Unknown, missing or unfinished state refuses.
2. Read the independent witness TIP; require exact equality with META's tip.
3. Read that exact immutable witness. Enforce the existing 4*MaxBodyBytes bound
   independently of the provider; verify body SHA, strict witnessRecord decoding,
   version/sequence/lineage/table/revision and byte-exact stored state. Reuse the
   existing predecessor sequence/cumulative-byte/bootstrap consistency rules;
   do not invent restrictions that reject a legitimate restored table lineage.
4. Strongly Load META and read witness TIP again. Require no Pending and the same
   revision/state/tip, with identical witness TIP. Return a detached record only
   after cancellation checks. Do not silently retry against a newer anchor.

Never call Current, Resolve, Apply, receipt recovery, witness Advance, Store
Complete/Commit/Import, or any provider mutation on this observational path.
Existing owner Inspect remains potentially mutating through Current; it is not a
substitute. Pending observations refuse without completing their transaction.

## E15 owner wire and client

Use existing authenticated owner principal, transport and configuration. A
position pin is a comparison value, never authentication or continuing authority.

```go
type ClosedPosition struct {
    Position contracts.JournalPosition `json:"position"`
    Epoch uint64 `json:"epoch"`
    Anchor WitnessTip `json:"anchor"`
}
func (*Client) ObserveClosed(context.Context) (ClosedPosition, error)
func (*Client) ClosedReader(ClosedPosition) contracts.JournalObjectReader
```

Request/Response add optional ClosedPosition pointer (`closed_position`). New
owner-only `closed_current` accepts no extra fields. Existing get/list accepts
exactly one selector: lease, reservation, cut OR closed_position. Strictly validate
nested pins and retain all old field validation and role restrictions.

Route closed_current and closed-position get/list before the existing receipt
replay and Kernel.Current paths. Require actual state Phase=CLOSED, Lease=nil,
Prepared=nil and a valid journal position. ACTIVE with an expired lease still
refuses, as do ENROLLED/PREPARING and mismatched owner identity.

ObserveClosed uses ObserveCurrent, checks CLOSED, then repeats the observation
and compares the complete position/epoch/anchor before responding. Get/list
requires exact equality with expected pins before reading and again before any
response, using boundedReader strict=true. Drift returns no partial page/body.
Unknown fields, combined selectors, missing/malformed pins and extra mutation
fields refuse. No persistent CUT, receipt, new bearer token or signing system.

The client validates expected/returned pins and preserves the existing bounded
reader page/body checks. Cached immutable page bodies are historical data; every
completed history observation and every future installer admission requires a
fresh authenticated ObserveClosed equality check. Preserve existing APIs and
historical cut semantics; runtime principals cannot invoke the new owner route.

The episodemaintenance API adds a distinct CLOSED source, preserving live Source:

```go
type ClosedAuthority interface {
    ObserveClosed(context.Context) (authority.ClosedPosition, error)
    ClosedReader(authority.ClosedPosition) contracts.JournalObjectReader
}
type ClosedSource interface { Observe(context.Context) (ClosedSnapshot, error) }
type ClosedSnapshot struct {
    Observation contracts.JournalObservation
    Position authority.ClosedPosition
    Recheck func(context.Context) error
}
func NewClosedSource(ClosedAuthority) (ClosedSource, error)
```

The existing trusted factory supplies the authenticated capability; this internal
interface does not authenticate an arbitrary implementation. Nil/typed-nil inputs,
cancelled contexts and malformed closed positions refuse. Snapshot data is detached
from the private position used by Recheck; no historical CutID is invented.

The actual runtimebroker.OpenOwner path must also support these calls. Extend
ownerTransport only for closed_current and get/list with a nonnil closed_position
and no lease/reservation/cut selector. Preserve its refusal of historical get/list
and other runtime writer actions. operatorTransport must explicitly refuse the
new closed selector/action, preserving its historical-reader-only contract even
if accidentally configured with owner credentials. Server principal checks repeat
these gates. Test the actual transport/signed-request path with private synthetic
configuration and fixture credentials, plus no-remote-invocation refusal counters;
this does not qualify deployed IAM or provider permission.

A trusted episodemaintenance CLOSED source factory uses the authenticated client,
ClosedReader and deletion.NewJournalReader(...).Observe to verify ALL current
history, followed by fresh equality recheck. Its detached snapshot supplies an
explicit Recheck closure. No arbitrary caller can replace the authenticated
position/history with a JSON assertion. Recheck never implies host coldness.
Existing bounds remain: 10,000 generations, 100,000 objects, 64MiB total, 64KiB
objects and existing bounded pages. Preserve generation seals, empty successors,
writer identity, full chain/head validation and complete relevant exclusions;
do not require every active generation to be sealed.

## E16 dependent actual caller contract (not yet API-frozen)

The existing root controller retains authenticated owner configuration and all
credentials. The service-UID child receives no owner credentials. Existing six
fixture FDs contain documents/lock probes, not a freshness RPC: never repurpose a
probe FD. Design a distinct versioned installation-child control channel using
actual parent lifetime, retained release lock, finite cold custody and authenticated
E15 history observations. Every ColdAdmission and InitialRecoveryAdmission phase
must request a fresh parent check, not replay the starting packet.

Bind actual root/runtime/unit/release/config and account/brain/source route,
operation/header, source/candidate heads, relevant fence history and original/current
marker observations. Compare complete relevant account/brain exclusions using
existing two-way matching rules; whole-account/brain deletion refuses. Initial
recovery additionally needs explicitly authorized reconciliation of unattributed
intent and fencing of prior workers. Timeouts, cancellation, parent death, channel
substitution, stale journal/custody or unqualified namespace return no usable
installation boundary. Keep marker/residual copies on uncertainty.

E16 freezes the exact channel API/caller/installation lifecycle only after E15
lands. Its acceptance must include actual authenticated server/client, real root
controller and real UID child across Recover/Install/Resume; callback-only fixtures
cannot close it. Production installation, finite operational packet/provider
qualification, physical copy disposition and capture remain separately gated.

## Acceptance and ownership

E15 tests exercise real server/client and deletion reader: CLOSED with no cut and
empty history; newer exclusions after old cuts; multiple generations; pending
state with ZERO mutation calls; wrong/missing/restored META/TIP/witness bodies;
strict witness-state binding; owner/runtime role separation; all non-CLOSED states;
page/object/final recheck drift; malformed/missing/extra-tail/oversized objects;
cancellation and detached responses. Existing live/cut/restore tests regress.

The observed R-hosted-authority claim is preserved. Protocol source edits require
verified resource ownership or explicit nonoverlap/handoff from its trusted owner;
a stale-looking timestamp or completed unrelated PR is not permission to steal it.
Root owns the current plan/T5.2/T5.3 preflight; no authority implementation is
admitted by this document. E15 source scope is exact authority kernel/wire/client
plus trusted CLOSED history observer and joined tests. E16 actual caller and
controlled-copy work remain explicit downstream outcomes.

## E16 mechanism preflight (not implementation)

An actual isolated Linux root/fork/socketpair probe on 2026-10-18 confirmed:
root-created SOCK_SEQPACKET peer credentials retain creation identity, while
SO_PASSCRED/SCM_CREDENTIALS identify the actual UID/GID-dropped child with empty
supplementary groups. Forged root credentials refuse; unexpected SCM_RIGHTS are
identified and closed; truncation is checked; pidfd lifetime and EOF distinguish
live/dead peers, and the owned child is reaped. No service, user, credential,
canonical path or database was changed. This validates only the mechanism; the
versioned seventh-FD API, complete root/child admission and installed-host behavior
are not implemented or qualified. Existing six fixture FDs remain unchanged.
