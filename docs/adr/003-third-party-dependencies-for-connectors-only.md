# ADR 003: Third-party Go dependencies are allowed at the connector and provider edge only

## Status
Accepted

## Date
2026-08-27

## Context
The engine is a single static binary (RFC §2, §13.1). The existing tree already
depends on cobra (CLI), go-keyring (OS keychain), yaml.v3 (config), and
modernc.org/sqlite (pure-Go index). M1 needs three capabilities the standard
library does not provide: filesystem change notification, an IMAP client, and
HTTP access to model providers. Pulling in provider SDKs would bloat the binary
and hide the exact model identifiers the pinned-model-set rule (§7.5) depends
on.

## Decision
- **Core engine packages (`store`, `index`, `reconcile`, `ladder`,
  `disposition`, `direction`, `protocol`, `writer`, `extract`) use the standard
  library only.** New dependencies there require an ADR.
- **Connectors and provider adapters may take one focused dependency each:**
  `github.com/fsnotify/fsnotify` for the file watcher (with a `--poll`
  fallback for network mounts), `github.com/emersion/go-imap/v2` for IMAP.
  Both are permissively licensed and pure Go (`CGO_ENABLED=0` stays true).
- **Model providers are reached over `net/http` directly**, not SDKs: one
  Anthropic Messages adapter and one OpenAI-compatible adapter (which also
  covers Ollama-class local servers). The adapter records `provider/model@version`
  verbatim in every observation's provenance.
- Existing dependencies (cobra, go-keyring, yaml.v3, modernc sqlite) stay.

## Consequences
- The binary stays static and small; `go.mod` growth is reviewable per ADR.
- Provider quirks and connector quirks are isolated behind `Connector` and
  `Router` interfaces, both public API per RFC §10.1 and §9.
- A dependency CVE affects one edge package, never the writers or the index.
