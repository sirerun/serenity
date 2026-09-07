# Devlog

Newest first. Investigation findings, benchmarks, and ops notes for the greenfield code. Architecture goes to docs/design.md, decisions to docs/adr/.

## 2026 09 06 -- T1.31 full-corpus extraction throughput: root-caused and measured

Task: docs/plans/E1-m1-ingest.md T1.31, filed from docs/evals/m1-report.md's finding that `serenity extract all` over the real 92,586-chunk corpus projects to ~2-3 months at `qwen3.8-27b`'s observed per-chunk rate.

**Bottleneck, precisely measured against the live DGX SGLang endpoint** (`http://192.168.86.250:30000/v1`, same pod as T1.23's run): sent real extraction-shaped prompts (matching `internal/extract.buildPrompt`'s exact shape, one email-style chunk and one clinical-note-style chunk) via raw `chat/completions` calls, both with default settings and with `chat_template_kwargs: {"enable_thinking": false}` added to the request body.

| Chunk | Mode | Wall time | completion_tokens | reasoning_tokens | reasoning share |
|---|---|---|---|---|---|
| email (360 prompt tokens) | default (thinking on) | 57.7s (one earlier attempt hit a 120s client timeout entirely -- host-contention variance, consistent with T1.23's own report) | 1573 | 1471 | 93.5% |
| email | `enable_thinking: false` | 3.9s | 189 | 0 | 0% |
| clinical note (227 prompt tokens) | default (thinking on) | 20.1s | 774 | 494 | 63.8% |
| clinical note | `enable_thinking: false` (x2 trials) | 3.2s, 3.1s | 141, 146 | 0, 0 | 0% |

**Root cause: `qwen3.8-27b`'s default "thinking" reasoning pass, not sequential single-connection extraction.** Both are real (`internal/extract.Extract` does call `ExtractChunk` in a plain `for` loop, one chunk at a time, zero concurrency -- confirmed by reading the code), but the reasoning overhead dominates: it alone explains a 6-15x per-call wall-clock multiplier on these two samples, comfortably enough on its own to turn "a few days" into "~2-3 months" over 92,586 chunks at the default rate (92,586 x ~30-58s serial ≈ 32-62 days, matching the M1 report's estimate). Sequential-connection latency is a real, secondary, additive cost on top of this, not the dominant one.

**Not a content-parsing bug**: the SGLang server returns reasoning text in a separate `message.reasoning_content` field, never inline in `message.content` -- `internal/router.openAIMessage` only maps `content`, so `internal/extract.parseResponse` never sees the reasoning trace either way. This rules out "unparsed `<think>` tags breaking JSON decode" as a contributor to T1.28/T1.29's accuracy gaps; those remain a separate, real question this task does not answer.

**Extraction quality was not degraded on the two disable-thinking samples** -- both produced well-formed, schema-conformant JSON with the same (or a cleaner) set of observations as the thinking-mode run on the same chunk.

**Fix shipped, opt-in**: `internal/router.OpenAICompatibleProvider` gained `ExtraBody map[string]any`, merged verbatim into the outgoing request body when non-nil (nil by default -- a request is byte-identical to before this field existed). `internal/config.Models` gained `disable_thinking` (bool, `omitempty`, default false); `internal/providers.buildChatProvider` wires it to `ExtraBody: {"chat_template_kwargs": {"enable_thinking": false}}` for both the `openrouter`-selected and substring-inferred local-server paths, never for `AnthropicProvider` (no such concept) and never for `BuildEmbeddingRouter` (embeddings don't reason). This is deliberately NOT a default: a real OpenAI/OpenRouter endpoint (served by the same adapter) does not recognize `chat_template_kwargs` and some APIs reject an unrecognized top-level field outright, so a brain pointed at a real cloud provider must never send it un-opted-in.

**What this does not close**: even at the measured ~3-4s/chunk disable-thinking rate, a strictly sequential 92,586-chunk run is still ~90 hours (~3.75 days) of serial single-connection wall-clock -- from "not viable on any timeline" to "a viable background/overnight-scale run", not to "fast". Concurrency in `internal/extract.Extract`'s chunk loop (batching multiple in-flight requests to the same SGLang endpoint, which can typically serve several concurrent generations) is the next lever if a faster full-corpus run is ever needed, and is intentionally out of scope here -- it is a bigger, separate change to `internal/extract`'s core loop, not a config knob, and would need its own task and its own ordering-safety analysis (extraction has no cross-chunk state, so is safe to parallelize, but was not verified here).

## 2026 08 27 -- Planning pass to code complete

- Verified at 13dc0d2: `GOWORK=off go test -race ./...` green for internal/cli, internal/index, internal/store (16 tests). No open PRs. Remote: sirerun/serenity (private).
- gbrain (dndungu/gbrain@d35c9c9e441e, branch master) DOES keep facts and takes as markdown fences: `src/core/facts-fence.ts` (markers `<!--- gbrain:facts:begin -->` / `end`, 10 columns plus row number, kinds event|preference|commitment|belief|fact, strikethrough with context `superseded by #N` or `forgotten: <reason>`) and `takes-fence.ts` (7 columns, `since` ranges `A -> B`). A research agent's full-text scan of the docs concluded no fence syntax existed; the source refuted it. Lesson: verify structural claims about a dependency in its source tree, not its prose docs. The GitHub code-search API returned 0 hits for every query on this repo (including `fence`), so it is not usable as evidence of absence here.
- `src/schema.sql` at master has no `facts` or `takes` CREATE TABLE; those tables arrive through migrations, so schema.sql alone under-describes gbrain's DB.
- gbrain `protocol conformance` is a live write test: it seeds `people/conformance-<marker>` (when put_page exists), cycles remember/forget, and does not clean up. CI must point it at a throwaway brain. Cases ship as data at `test/fixtures/memory-verbs/cases.json`.
- dira (kazi-org/dira@15686940aa08): `additionalProperties: false` on the entry root and every $defs object, so `applies_when` cannot ride in frontmatter without failing dira's validator (ADR 008 puts it in a body block). `dira check` is lexical and offline by construction (`internal/nomodel`); exit 2 = conflict cited, 1 = its own errors. Interview is a fixed four-prompt script, no model. LICENSE/NOTICE name Sire Run, Inc.
- RFC internal inconsistencies found: section 10.1 lists the voice-note connector as P0 while section 17's M1 AC omits it (resolved: M2, ADR 005); section 8.3 fixes `check` exit codes but leaves `no_applicable_constraints` and `unverified` codes implicit (resolved: ADR 010).
- Tooling: the Claude-in-Chrome extension reported "not connected" on this machine; `agent-browser` drove oxalpha.com/chat instead (file upload works, `fill` does not enable Send, real keystrokes do; long replies need a typed `continue`). oxAlpha's decomposition was useful for task granularity and invented several RFC deviations (exit 3, `.dira/rules` sidecars, a different orphan detector); every structural claim was checked against the RFC before adoption.
