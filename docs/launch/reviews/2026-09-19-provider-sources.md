# Public provider metadata follow-up — 2026-09-19

No authenticated request or inference was made. This is source preparation for gated task T23.42, not provider qualification or an approved privacy disclosure.

The [OpenRouter ZDR endpoint catalog](https://openrouter.ai/api/v1/endpoints/zdr) lists `perplexity/pplx-embed-v1-0.6b`, serving provider Perplexity, tag `perplexity/int8`, on this read. The [candidate endpoint catalog](https://openrouter.ai/api/v1/models/perplexity/pplx-embed-v1-0.6b/endpoints) lists that single endpoint at $0.004 per million input tokens, 32,000 context, INT8, with implicit caching false. This establishes OpenRouter's published endpoint classification; it improves on the earlier chat-only policy citation but does not prove the eventual account settings or serving route. Catalog values are mutable and are not a model version pin or measured quality/latency.

[Embeddings request documentation](https://openrouter.ai/docs/api/api-reference/embeddings/submit-an-embedding-request) includes a provider-routing object, input_type, dimensions, and float/base64 encoding. The rendered page did not expand the nested routing fields: exact embeddings support and enforcement of provider pinning, no fallback, and ZDR must still be verified against the full schema and an authorized integration run.

[OpenRouter response caching](https://openrouter.ai/docs/guides/features/response-caching) covers embeddings. It is OFF when neither a request header nor a preset enables it; the source does not establish that Serenity enables caching. A false X-OpenRouter-Cache header disables it even when a preset enables it. Account-level ZDR disables response caching, but per-request provider.zdr alone does not. Cache hits report zero billable usage. Qualification should record cache policy and observed cache status so a cached answer is not mistaken for a fresh upstream probe.

[OpenRouter ZDR policy](https://openrouter.ai/docs/guides/features/zdr) describes endpoint-specific classifications and distinguishes them from provider defaults. Routing enforcement and disclosure should reflect the chosen account/key settings. No customer-facing absolute zero-retention claim is approved by this metadata capture.

## Raw public response hashes

- `openrouter-public-zdr-endpoints.json`: `b191376e38d27ae1b893532d131154d584870aad7fb2ad1b7d6130b018cd0219`
- `openrouter-public-candidate-endpoints.json`: `f5c05a1b3e213f8487a6e56ddbc296047f9f0f1a9a27c5e6f0a098b72b24695b`
- `openrouter-public-embedding-models.json`: `a75691739ddf190447ecb0d2006396bdc52451f6584d1d98b6bebab817b40533`
- `openrouter-public-providers.json`: `7504fbd7cfe5af276001cedb487b2b8e25ef1fd22f0f76b1cab4d8210ca2e457`
