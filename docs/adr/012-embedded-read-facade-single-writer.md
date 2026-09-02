# ADR 012: Sire embeds Serenity's read paths through an exported facade; writes stay single-process

## Status
Accepted

## Date
2026-09-02

## Context
David ruled on 2026-09-02 (recorded in sirerun/sire `docs/improvement-register.md`
§2 T8) that Sire consumes Serenity as a Go library: the read paths a governed
agent runtime needs at every step -- DIRECTION `check_plan`, `recall`/search,
and `brief` -- run in-process inside Sire's API pods, and every write to a
brain goes through one brain-writer process per tenant.

Three existing commitments shape how that can be done without a fork:

- RFC 0001 §13.1: CLI and protocol surfaces are thin wrappers over one engine,
  drift tests assert identical results, and the daemon has no privileged
  internal path. An embedded consumer must be held to the same rule, or it
  becomes the privileged path the RFC forbids.
- ADR 004: `internal/writer` is the only path that touches canonical files
  after M0 -- one drain goroutine, per-path sequence numbers, the file-first
  CI gate. Per-path ordering is a single-process guarantee; two processes
  each holding a writer queue over one brain have no ordering between them.
- ADR 003: core packages are stdlib-only (third-party deps live at the
  connector/provider edge), so embedding the read paths costs a consumer no
  transitive dependency surface beyond Go's standard library and the SQLite
  index driver.

Sire's API tier is many pods. Embedding makes each pod a reader of one
brain; nothing about that changes which process may write. RFC §3 says
Serenity's roadmap is never set by Sire's needs and that any affordance Sire
wants arrives through the public surface. A facade that re-exports the
functions the CLI already calls is packaging, not a new affordance: it adds
no verb, no field, and no behavior the CLI does not have.

The functions the CLI calls today, and that the facade must call and nothing
else: `check.New(store, nil)` + `Matcher.Match` (internal/cli/check.go:86,
check.go:100) with a `direction.NewStore(root, queue)` ledger handle
(internal/cli/check.go:77); `search.Search(ctx, eng, nil, query, limit,
search.Options{})` (internal/cli/search.go:55); `compose.New` +
`Composer.Ask` (internal/cli/ask.go:64-65). `brief` has no shipped source yet; it
lands with the DIRECTION server in T4.6.

## Decision
1. Serenity exports one consumer package, `pkg/serenity`. T4.18 (E4 wave
   4a, dispatchable now) ships `Open(brainPath string, opts ...Option)
   (*Brain, error)`, `(*Brain).CheckPlan(ctx, actions []Action) (Verdict,
   error)` and `(*Brain).Recall(ctx, q string, budget Budget) (RecallResult,
   error)`; T4.19 (wave 4c, after the DIRECTION server's brief packer,
   T4.6) adds `(*Brain).Brief(ctx, budget Budget) (Brief, error)`. Beyond
   the wire types those signatures name, `go doc` of the package lists
   nothing else at either point.
2. The facade adds no logic. Each method calls the same internal function
   the corresponding CLI verb calls, with the same arguments; a drift test
   deep-equals `CheckPlan` against `serenity check --json --actions` over the
   brain fixture (T4.18), extended to the T4.13 DIRECTION transcripts and
   to `Brief` against the wire object (T4.19). When the CLI's wiring
   changes, the facade changes in the same PR or the drift test goes red.
   This is how RFC §13.1's "no privileged internal path" holds for an
   in-process consumer: the facade is a client of the engine, not a door
   into it.
3. The handle is read-only. `Open` builds its ledger handle with a nil
   writer queue (`direction.NewStore(root, nil)`), and `internal/direction`
   gains a nil-queue guard so every mutator returns an error rather than
   dereferencing nil. No writer entry point is exported: `pkg/serenity`
   never imports `internal/writer`, asserted by an AST test in the same
   family as the file-first gate (T0.2).
4. Writes stay single-process. Exactly one brain-writer -- the Serenity
   daemon or CLI, holding the one `writer.Queue` of ADR 004 -- writes to a
   given brain. A consumer that wants a write submits it over the wire
   (DISPOSITION `propose`, MEMORY_VERBS `remember`) to that process. Sire
   runs one brain-writer process per tenant; its API pods are readers only.
5. The facade is a consumer surface bound by the protocol_version policy
   (RFC §8): field names and semantics frozen once shipped, additive-optional
   changes only, a breaking change is a new package path. The package doc
   links this ADR (T4.18); the DIRECTION v1 document (T4.16) records it
   under a "consumer surfaces" note (T4.19).
6. Reader/writer consistency is the existing SQLite index plus git-canonical
   files: a reader sees the index as of its last open. The facade makes no
   freshness promise beyond what `serenity check` makes today; if Sire
   needs a bounded-staleness guarantee, that is a public protocol change
   through the RFC process (RFC §3), not a facade option.

## Consequences
- Sire's per-step plan checks and recalls cost one in-process call, not one
  HTTP round trip, and need no daemon reachable from the API tier.
- Serenity keeps one engine and one write path. Nothing in ADR 004's
  single-writer invariant, the file-first gate, or the precept-immutability
  test (T3.12) changes; the AST test extends their coverage to `pkg/`.
- The CLI-vs-facade drift test is a second copy of T4.9's CLI-vs-protocol
  discipline. Three surfaces (CLI, wire, facade) now pin each other.
- `pkg/serenity` is a public Go API of a private repo. Until T5.20 opens the
  repo, Sire consumes it by module replace or vendoring; that mechanism is
  Sire's concern, not this ADR's.
- Serenity's HTTP milestones stay off Sire's path: T4.18's deps (T3.7,
  T1.12) are shipped, so it is cross-epic-startable alongside T4.3 and named
  as such in plan.md section 5 (ADR 011 rule 5). `Brief` cannot ship before
  T4.6, so it is split into T4.19, which inherits the M-order gate
  transitively through T4.6 and needs no blocked-by of its own.
- A nil-queue `direction.Store` is a new state the package must handle
  explicitly. The guard is part of T4.18's acceptance, not left to a panic.

Rejected:
- HTTP-only consumption (Sire calls the daemon's DIRECTION/MEMORY_VERBS
  endpoints for every read). Correct by the RFC, but it puts a network hop
  and a reachable daemon on the hot path of every agent step, and it makes
  the daemon's availability Sire's availability. Reads are pure functions of
  files plus a derived index; there is no reason to route them through a
  process boundary.
- Full embed with the writer exported (each Sire pod holds its own
  `writer.Queue`). Breaks ADR 004 outright: per-path ordering is a
  single-process property, and N pods each with a queue over one brain
  race on the same fence pages and shards with no ordering between them.
- A multi-principal daemon (one serenityd serving many tenants' brains over
  the wire). RFC §6 lists multi-user support as a v1 non-goal, and
  it reintroduces the network hop the embed was meant to remove. The
  one-writer-per-tenant process Sire runs is a deployment shape, not a
  Serenity feature.
- Exporting the internal packages directly (`internal/direction/check`,
  `internal/search`, ...) by moving them under `pkg/`. Exposes constructors
  that take a writer queue and mutators the consumer must promise not to
  call; the facade makes the read-only promise structural instead.
