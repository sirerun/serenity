# ADR 013: OpenRouter is the default model provider; provider selection is explicit, not inferred

## Status
Accepted

## Date
2026-09-04

## Context
`internal/providers` (the one place a `serenity.yml` model pin becomes a live
`router.Provider`, T1.15/T4.18) infers which adapter to build purely from a
substring match on the pinned model name: if the model name contains
"claude" it builds `router.AnthropicProvider` (hitting `api.anthropic.com`
directly with `ANTHROPIC_API_KEY`); otherwise it builds
`router.OpenAICompatibleProvider` (`OPENAI_API_KEY`/`OPENAI_BASE_URL`). This
repeats identically in `BuildExtractionRouter`, `BuildEmbeddingRouter`, and
`BuildComposerRouter`.

David asked (2026-09-04) that Serenity default to the OpenRouter API with a
user-configurable model. OpenRouter's completions endpoint
(`https://openrouter.ai/api/v1/chat/completions`) speaks the exact
OpenAI-compatible chat-completions shape `OpenAICompatibleProvider` already
implements (ADR 003: no new dependency at the provider edge, this adapter
covers it as-is) -- but OpenRouter's model ids are vendor-prefixed (e.g.
`anthropic/claude-sonnet-4.5`, `google/gemini-3-pro`). Pinning such a model
today would trip the existing "claude" substring check and misroute the call
to the native `AnthropicProvider` against `api.anthropic.com` instead of
OpenRouter -- silently sending the request to the wrong endpoint (and, if
`ANTHROPIC_API_KEY` happens to be set for an unrelated reason, e.g. the
composer or an eval harness, billing the wrong account instead of failing
loudly). This is a real, pre-existing routing bug that OpenRouter's model-id
convention surfaces; it is not specific to picking OpenRouter as the default.

OpenRouter does not offer an embeddings endpoint. `BuildEmbeddingRouter`'s
existing `OPENAI_API_KEY`/`OPENAI_EMBEDDINGS_BASE_URL` path (a real
`/embeddings` adapter, T1.15) already lets embeddings point at a different
host than chat/extraction/composer, so this is a pre-solved case, not a new
one -- it simply must NOT be routed through `models.provider`.

## Decision
1. `serenity.yml`'s `Models` struct gains an explicit `Provider` field
   (`models.provider: openrouter | anthropic | openai`, empty/omitted means
   "infer from the model name," today's exact behavior). `internal/providers`'
   three `Build*Router` functions check this field first; the existing
   substring inference remains the fallback only when the field is empty, so
   every existing `serenity.yml` without the field is completely unaffected
   (additive, not breaking).
2. `models.provider: openrouter` builds
   `router.OpenAICompatibleProvider{BaseURL: "https://openrouter.ai/api/v1",
   APIKey: <OPENROUTER_API_KEY>, Model: <pin>, Version: <pin>}` for
   extraction and composer -- zero new adapter type, reusing the existing
   OpenAI-compatible adapter exactly as ADR 003 intends for this edge.
3. `config.Default()` (the install-time seed for a brand new brain) sets
   `models.provider: openrouter`. This changes only which provider a
   pinned model resolves to; it does not un-skip the "no model pinned"
   default (`models.extraction`/`embedding`/`composer` stay `none@v0`, and
   `BuildExtractionRouter`/etc. still return `ok=false` with a named reason
   until the user pins a real model, per the existing explicit-skip
   contract).
4. The user picks the model by editing `models.extraction`/`composer` (and,
   independently, `models.embedding`) in `serenity.yml` to any
   OpenRouter-listed model id, or via a new `serenity config set-model
   <extraction|embedding|composer> <model>` CLI convenience (T1.25) that
   rewrites exactly the named pin.
5. `BuildEmbeddingRouter` is unaffected by `models.provider`: embeddings
   keep requiring `OPENAI_API_KEY` or `OPENAI_EMBEDDINGS_BASE_URL`
   regardless of the chat/extraction provider choice. A brain using
   `models.provider: openrouter` for extraction/composer still needs a real
   OpenAI-shaped (or Ollama-class local) embeddings endpoint configured
   separately -- disclosed in docs/providers.md (T1.25), not silently
   degraded.
6. Credential: new `OPENROUTER_API_KEY` environment variable, read the same
   way `ANTHROPIC_API_KEY`/`OPENAI_API_KEY` are today (process environment,
   not the OS keychain `internal/connector/imap` uses -- consistent with
   every other router credential, none of which use the keychain yet).

## Consequences
- Fixes a real routing bug independent of this decision's main point: any
  vendor-prefixed model name containing "claude" (which is exactly
  OpenRouter's naming convention for Anthropic models) was silently
  misrouted to native Anthropic before this change. `models.provider:
  anthropic` (or the field left empty against a non-OpenRouter pin) is the
  explicit escape hatch that preserves today's exact behavior, verified by
  a regression test pinning the pre-existing substring cases.
- No new third-party dependency (ADR 003 upheld) -- OpenRouter reuses
  `OpenAICompatibleProvider` unchanged in shape, only in `BaseURL`.
- New installs (`config.Default()`) default to OpenRouter; existing brains
  are untouched until their owner edits `serenity.yml` or upgrades and
  re-runs init.
- A new credential surface (`OPENROUTER_API_KEY`) needs documenting
  (docs/providers.md, T1.25) alongside the embeddings caveat so a user who
  points extraction/composer at OpenRouter is not surprised when embeddings
  still needs a separate OpenAI-shaped credential.

Rejected:
- A dedicated `OpenRouterProvider` type duplicating
  `OpenAICompatibleProvider`'s request/response shape for no behavioral
  difference: OpenRouter's chat-completions endpoint IS the OpenAI shape.
  If OpenRouter-specific attribution headers (`HTTP-Referer`, `X-Title`)
  are wanted later, they are an additive field on the existing adapter, not
  a new type.
- Leaving provider selection purely inferred from the model name: this is
  the actual bug OpenRouter's model-id convention surfaces. Explicit
  selection is the fix, not a preference.
- Making OpenRouter provide embeddings by translating through its chat
  endpoint: OpenRouter has no embeddings endpoint at any price; there is
  nothing to route to.
