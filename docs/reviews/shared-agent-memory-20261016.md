# Shared agent memory design record

Date: 2026-10-16

Status: Design work only. No runtime changes, plugin installation, provider
qualification or deployment. Baseline: Serenity `67c67e13`.

## Owner decisions

The owner requested Quorum brainstorming, clarification until intent was well
understood, then RFC revisions and ADRs. The following choices were explicit:

1. Agent-neutral Serenity for all coding, marketing and other agents; Zati first.
2. Plugins for Cursor, Codex, Claude Code and Google Antigravity.
3. Automatic conversation/tool capture and useful fact saving under a standing
   policy, not per-message approval.
4. Hosted shared brain across computers, with a bounded local outage queue.
5. Recover recorded conversations after host-history loss and continue work in
   another agent, rechecking current files and permissions.
6. Automatically import existing histories during setup, within enrollment.
7. Full visible text and exposed attachments; accurately report unsupported gaps.
8. Retain until explicit deletion, subject to visible storage limits.
9. Shared personal brain plus project/workspace organization; separate existing
   brains/grants for restricted data. Avoid a huge rewrite or new within-brain
   permission model.
10. Default exclusions for credentials, secret files and private sessions;
    redaction before upload and a visible pause control.

The editor assessed the requested confidence threshold as met after these
answers. This is confidence in understood intent, not a statistical claim or
permission to mark the architecture implemented.

## Quorum exploration

Used the owner's configured Quorum client and existing credentials without
printing or copying credential values. Panel seats were the configured Claude,
GPT and Gemini models. The first pass used a source-grounded design brief; the
refinement pass included relevant source excerpts and all confirmed choices.

| Run | Outcome | API-reported cost |
| --- | --- | --- |
| `2026-10-16T02-55-13` | Three first-pass reviews plus GPT synthesis | USD 0.36748350 |
| `2026-10-16T03-00-21` | GPT refinement plus synthesis; Claude and Gemini abstained because output hit the token cap | USD 0.68798375 |
| `2026-10-16T03-07-41` | Final candidate: Gemini completed; Claude abstained at the increased output cap | USD 0.61684550 |

Total API-reported cost across the three runs: **USD 1.67231275** (10 calls).

The second pass is one completed opinion, not three-way consensus. Agreement
does not establish correctness, runtime behavior or release evidence.

Findings incorporated:

- Separate archival evidence, continuation checkpoints, durable statements and
  accepted direction; do not infer authorization from historical instructions.
- Preserve original admitted bytes; attachment references alone are not custody.
- Reuse existing storage and brain grants; separate restricted brains instead
  of adding project ACLs. Expose lifecycle capabilities honestly per host.
- Make publication/retry, offline reconciliation and purge explicit contracts.
  Current governed retraction is not erasure; artifact references are not export.
- Keep direct retrieval; make hierarchy earn its maintenance cost through evals.
- Require Zati context integration and all four host plugins as actual outcomes.

Not adopted as shortcuts: using existing remember/recall as if they already
archive/export arbitrary transcripts; hiding full-history requirements behind
summaries; treating token-capped panel outputs as completed independent reviews.

## Document changes and compatibility

- RFC 0001 v2.3 links the extensions and explains the historical v1 baseline.
- RFC 0002 defines long-horizon episodic memory and end-to-end acceptance.
- RFC 0003 defines host plugins, lifecycle coverage and cross-agent continuation.
- ADR 027 records episodic custody and the incremental storage/retrieval design.
- ADR 028 records existing brain boundaries and shared host-adapter architecture.
- ADR 029 records derived erasure and replay-fence requirements.

Existing MEMORY_VERBS/governed protocol schemas, code, dependencies, release
gates and unrelated work are unchanged. Numerical quotas and latency/capacity
ceilings require explicit configuration and evidence before qualification;
this work does not authorize unbounded model calls, new hosting or deployment.

## Validation

Final candidate review completed with one usable independent review (Gemini).
Claude's incomplete output is not counted as acceptance. Findings were triaged:

- Strengthened reconnect ordering with fence synchronization before queue drain,
  plus server generation checks to close a concurrent-delete race.
- Specified fenced, batched physical Git cleanup and stable logical archive IDs;
  immediate exclusion remains distinct from physical purge completion.
- Made unknown-workspace behavior and organizational-label versus hosted-brain
  identifiers explicit; repository files cannot establish trusted mappings.
- Kept the already specified separate episode API and current lexical/hybrid
  distinction. Endpoint spellings and bounded wire schemas are frozen at the
  implementation gate, not invented as already available routes.
- Did not adopt an arbitrary 1 MB attachment cutoff or a mandatory new blob
  backend without measurements. Full exposed attachment custody remains a
  required outcome within declared qualified limits, with visible gaps outside
  them; a future storage change needs its own decision.
- Did not defer deletion correctness until after all plugins ship, standardize
  nonexistent cross-host hook names, or change the owner's retention decision
  to automatic FIFO eviction. These suggestions conflicted with confirmed
  requirements or documented host differences.

Local checks covered document links, code-fence balance, whitespace and private
path/key-shaped text, plus the staged Git diff. A first naive link scan matched
an existing inline grammar expression; excluding inline code resolved the
validator false positive without changing that grammar.
No implementation tests are appropriate to claim behavior from this docs-only
change. Host official documentation is cited in RFC 0003; actual host/version
tests remain required.
