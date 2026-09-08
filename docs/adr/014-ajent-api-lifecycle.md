# ADR 014: Ajent API lifecycle and distribution policy

## Status
Accepted

## Date
2026-09-08

## Context

The Ajent readiness audit found partial API versioning and no published deprecation convention. The launch also needs to serve both agents and people across more than one installation channel.

## Decision

Ajent publishes agent and human workflows through multiple appropriate package channels: Homebrew, PyPI, and npm. Public self-serve installation and signup are part of the launch path. Versioned and authenticated REST routes use standard rate-limit headers. API versions remain compatible for 12 months after release; removals receive at least 90 days' notice, with `Deprecation` on affected responses during the notice period and `Sunset` carrying an RFC 3339 removal timestamp.

## Consequences

Agents get predictable installation, throttling, and migration signals while people retain a first-class login path. Ajent must maintain package smoke tests and lifecycle documentation across channels. Search indexing and press evidence still depend on external indexing delay or credentials.
