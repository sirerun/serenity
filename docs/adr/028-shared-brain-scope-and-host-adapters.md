# ADR 028: Reuse brain boundaries and share the host integration client

Date: 2026-10-16

Status: Proposed architecture implementing owner-confirmed scope preferences.
Host documentation was inspected; plugin behavior is not runtime-qualified.

## Context

The owner requires Serenity plugins for Cursor, Codex, Claude Code and Google
Antigravity, in addition to Zati. All agents should share personal knowledge
while keeping restricted projects separate. The owner explicitly wants to
avoid a huge rewrite and approved reuse of existing brain boundaries.

Hosted Serenity authorizes an account's brain through scoped credentials. The
hosted `project_id` denotes that brain. Entity/project labels inside it are not
ACLs, and legacy `world` visibility means authorized clients of that brain.
Existing `connect claude` is a project MCP/plan-check setup, not automatic
conversation capture.

## Decision

1. Use one existing personal brain across enrolled agents/devices. Stable
   project/workspace labels organize retrieval within it. Restricted material
   uses separate existing brains and grants. No new within-brain project ACL
   engine or copied private brain per agent is required by this design.
2. Resolve each request to its current authenticated brain. Missing/revoked
   restricted grants never fall back to personal memory. Multiple authorized
   brains may contribute to client-side context assembly through separate
   authenticated requests; no universal cross-brain credential is introduced.
3. Cross-brain transfer requires explicit authority, preserves source links and
   records destination-copy retention/deletion responsibilities. Current
   unsupported promotion remains unsupported until its public transfer
   contract is implemented and qualified. Scope labels cannot authorize it.
4. Implement one reusable Serenity client for policy, identities, queues,
   protocol recovery and diagnostics, with thin host-specific plugin packages.
   Hooks perform supported lifecycle work; MCP exposes tools; rules/skills guide
   the model. Instructions and tool registration alone are not capture proof.
5. Enrollment enables automatic covered-history import, ongoing capture and
   policy-covered durable fact saves. Keep credentials and private sessions
   excluded; expose pause, status and corrective controls. Preserve unrelated
   host configuration and native hook trust requirements.
6. Each qualified host/version/surface has a capability profile with observed
   capture, attachment, injection and compaction coverage. Missing hooks produce
   truthful gaps or portable continuation packets. Never advertise native
   restore or uninterrupted context solely because MCP connects.
7. Cross-agent handoff transfers recorded evidence and continuation context.
   The receiver revalidates files, tools, permissions and current external state.
   Prior approvals and pending tool calls are not transferable execution grants.

## Consequences and alternatives

Sharing is straightforward for trusted personal agents; they can potentially
read all agent-visible material in that brain. Project filters improve relevance
but cannot promise confidentiality. Restricted projects incur separate brain
selection/grants and explicit sharing; that is preferable to an ACL rewrite.

Per-host code remains necessary because lifecycle schemas differ. The shared
client limits duplication; support must be tested separately for CLI, desktop,
IDE and cloud surfaces. One plugin working does not qualify the others.

Rejected: per-agent permanent memory stores, installing MCP alone and calling
it automatic memory, or an agent-selected project string as authorization.

## References

[RFC 0003](../rfc/0003-agent-memory-plugins.md),
[verified binding](../protocol/HOSTED_BINDING_v1.md),
[governed visibility mapping](../protocol/GOVERNED_MEMORY_v1.md),
[ADR 014](014-hosted-tenancy-topology.md),
[ADR 023](023-partner-api-for-first-party-apps.md).
This decision does not alter existing issuance or revocation contracts or claim
that arbitrary multi-brain provisioning is already available to every account.
