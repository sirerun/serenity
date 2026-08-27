# ADR 010: `check` exit codes and the unverified floor; event delivery per transport; token rotation; docs toolchain; name-decision timing

## Status
Accepted

## Date
2026-08-27

## Context
Decomposing M3-M5 surfaced six small decisions the RFC leaves open or states
only partly: the exit code for `unverified` and for `no_applicable_constraints`;
the classifier confidence floor; how `subscribe` works over stdio, where SSE
does not exist; bearer-token lifetime; the docs toolchain; and when the name
decision (OQ1) is executed relative to protocol schema ids.

## Decision
- `serenity check` exit codes follow RFC section 8.3 exactly: 0 for `pass`
  and for `no_applicable_constraints` (the verdict string distinguishes them
  in stdout and `--json`), 2 for any `violated`, 1 for errors including
  `unverified`. No exit 3.
- Free-text classification below confidence 0.80, or with no model
  configured, yields `unverified`. The floor is a named constant with a test.
- The event log is one implementation with persisted monotonic cursors.
  Over HTTP, `subscribe` is SSE with long-poll fallback and `Last-Event-ID`
  resume; over stdio it is MCP notifications carrying the same event ids.
  One fixture set exercises both framings.
- The daemon bearer token is long-lived, per install, held in the keychain;
  `serenity connect --rotate-token` replaces it. No expiry in v1; the threat
  model records this as an accepted single-principal, loopback trade-off.
- Protocol versioning is the MEMORY_VERBS policy already adopted by the RFC:
  an integer `protocol_version` on every response, additive-optional changes
  only, a breaking change is a new document. No negotiation logic in v1.
- Docs are built with mkdocs-material; the threat model and protocol specs
  are included at build time from their canonical files, never copied.
- The name decision is taken at M4 exit, after schema `$id`s stabilize and
  before conformance transcripts are frozen for release; the rename is one
  mechanical sweep with a one-release `serenity` alias shim.

## Consequences
- Scripts can branch on exit codes without parsing, and `--json` carries
  the finer verdict; the RFC's contract holds unchanged.
- One event log means cursor semantics cannot drift between transports.
- Rotation without expiry keeps the CLI usable offline for months; a leaked
  token is revoked by one command.
- Docs stay CI-buildable without a JS toolchain.
