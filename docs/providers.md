# Model providers

Serenity pins three models in `serenity.yml` under `models`: `extraction`,
`embedding`, and `composer`. Each pin is a `<model>@<version>` string (for
example `anthropic/claude-sonnet-4.5@v1`); `none@v0` means "not configured,
skip this task class." This document covers which provider each pin
resolves to, the credentials each provider needs, and the one command that
changes a pin without hand-editing the file.

## OpenRouter is the default provider

`config.Default()` — the seed written the first time `serenity init` creates
a `serenity.yml` — sets `models.provider: openrouter` ([ADR
013](adr/013-openrouter-default-provider.md)). This does not pin any actual
model: `models.extraction`, `models.embedding`, and `models.composer` all
start at `none@v0`, and every task class that depends on one stays
explicitly skipped (never a silent no-op) until you pin a real model.
`models.provider: openrouter` only decides *which adapter* a pin resolves
through once you do pin one, for the `extraction` and `composer` purposes.

Set the `OPENROUTER_API_KEY` environment variable to your OpenRouter API
key before running anything that calls extraction or the composer
(`serenity sync`, `serenity ask`, etc.). It is read from the process
environment, the same way `ANTHROPIC_API_KEY` and `OPENAI_API_KEY` are —
none of these credentials live in `serenity.yml` or the OS keychain.

With `models.provider: openrouter`, a pinned model is sent to OpenRouter's
OpenAI-compatible chat-completions endpoint
(`https://openrouter.ai/api/v1/chat/completions`), so the model id should be
one of OpenRouter's own vendor-prefixed ids (for example
`anthropic/claude-sonnet-4.5`, `google/gemini-3-pro`) rather than a bare
provider-native model name.

## Overriding to Anthropic or OpenAI

To bypass OpenRouter and call a provider directly, edit `models.provider` in
`serenity.yml` to `anthropic` or `openai`:

```yaml
models:
  provider: anthropic
  extraction: claude-sonnet-4-5@v1
  composer: claude-sonnet-4-5@v1
  embedding: none@v0
```

Both `anthropic` and `openai` behave identically to leaving `models.provider`
empty: the pinned model name is inferred from a `claude` substring match — a
model name containing `claude` resolves via `ANTHROPIC_API_KEY` against
`api.anthropic.com`, and every other model name resolves via
`OPENAI_API_KEY`/`OPENAI_BASE_URL` (an OpenAI-compatible endpoint, including
a local server). Neither value forces the adapter; only `openrouter` does,
because OpenRouter's own vendor-prefixed ids (like `anthropic/claude-...`)
would otherwise trip that same substring check and misroute to the native
Anthropic adapter instead of OpenRouter.

`models.provider` is a field you edit directly in `serenity.yml` today —
there is no `serenity config` subcommand that changes the provider itself,
only the model pins (see below). If you want a CLI convenience for
`models.provider`, ask for one; T1.25 only covers the model pins.

## Embeddings: always OpenAI-shaped, never OpenRouter

OpenRouter has no embeddings endpoint at any price. Regardless of
`models.provider`, `models.embedding` always resolves through the same
OpenAI-shaped `/embeddings` adapter, reading `OPENAI_API_KEY` and
optionally `OPENAI_EMBEDDINGS_BASE_URL` (for a local or self-hosted
embeddings server) — `internal/providers.BuildEmbeddingRouter` never reads
`models.provider` at all. A brain running extraction and the composer
through OpenRouter still needs a real OpenAI-shaped (or Ollama-class local)
embeddings endpoint configured separately; embedding is not silently
degraded or routed through OpenRouter, it is simply a different
credential.

## Changing a pinned model: `serenity config set-model`

```
serenity config set-model <extraction|embedding|composer> <model>
```

Rewrites exactly one of the three pins in `serenity.yml` — the other two,
and the rest of the file, are left untouched. `<model>` is the full
`<model>@<version>` pin string. `<purpose>` must be exactly one of
`extraction`, `embedding`, or `composer`; anything else is an error and the
file is not written.

```
$ serenity config set-model extraction anthropic/claude-sonnet-4.5@v1
models.extraction = anthropic/claude-sonnet-4.5@v1
```

This changes only the model pin, never `models.provider` — to change which
provider a pin resolves through, edit `models.provider` in `serenity.yml`
directly (see above).
