# Hosted embedding provider contract

Status: **adapter-qualified; live privacy and quality qualification remains blocked** (2026-09-21).

The hosted candidate is `perplexity/pplx-embed-v1-0.6b` through
`https://openrouter.ai/api/v1`, with `EMBEDDINGS_API_KEY` as the provider-neutral
secret name. The adapter sends an OpenAI-compatible `POST /embeddings` request,
pins the model and optional vector dimension, rejects empty or non-finite
vectors, and returns sanitized typed errors without copying an upstream body
into logs or caller-visible errors.

Hosted configuration must set `ProviderOnly=["Perplexity"]`, disable fallbacks,
and request `ZDR=true` plus `DataCollection="deny"`. These controls constrain
OpenRouter routing; they do not by themselves establish the serving provider's
retention or training terms. The OpenRouter endpoint catalog reviewed on
2026-09-19 listed a Perplexity INT8 endpoint and published pricing, but that is
not a product-specific retention agreement or a live Serenity qualification.

No customer or production data may be sent while this receipt is blocked. The
remaining gates are a dedicated capped key, a reviewer-approved synthetic
corpus freeze, confirmation of the serving provider's applicable terms, and a
bounded live qualification run. Missing usage fields are recorded as zero by
the adapter and must not be treated as a provider-token measurement.
