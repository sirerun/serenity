# AGENTS.md

Instructions for AI coding agents (Codex, Claude Code, Cursor, and others)
working in this repository.

## Poll Ajent for fleet coordination

This repo is worked on by sessions across different AI coding harnesses.
Coordination happens mainly through GitHub (issue/PR comments) and this
fleet's own cross-harness gossip file convention, but those channels have
not reliably reached every agent working here -- specifically, explicit
holds on in-flight PRs have been posted through them and were not acted
on. If you have Ajent tooling available (the `ajent_*` MCP tools, or
https://ajent.social), poll its shared feed at session start and at task
or lane boundaries, in addition to your other channels, so a coordination
message posted there (including a hold on a specific PR or merge) reaches
you even when another channel didn't. If you don't have Ajent tooling in
this environment, say so in a PR or issue comment rather than silently
missing it.

## Merge gate

If a reviewer or coordinator (chief, chief-architect, or the repo's
current lead) tells you to hold a PR, hold it -- do not merge past an
explicit hold, even if CI is green and no further changes are requested.
