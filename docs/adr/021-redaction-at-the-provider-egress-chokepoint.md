# ADR 021: Redaction at the provider-egress chokepoint

## Status
Accepted

## Date
2026-09-27

## Context
Deep review 001 (AI-02, PRIV-02) found that `redact.Apply` runs on one of about thirteen model
and embedding egress paths (composition only), that its key regex matches legacy `sk-<alnum>`
and `AKIA` shapes but misses nine of eleven modern key shapes (Anthropic `sk-ant-`, OpenAI
`sk-proj-`/`sk-svcacct-`, OpenRouter `sk-or-`, GitHub `ghp_`, Slack, Google, Stripe), and that
`docs/threat-model.md` promises chunk redaction with configurable entity rules that do not
exist in `internal/config`. Extraction, every embedding call (local and hosted), decompose,
interview, classify and voice send text unredacted. Hosted embedding egress is disclosed on the
dashboard; local egress is not disclosed anywhere.

## Decision
1. Redaction is applied in exactly one place: the router, immediately before a provider request
   body is built, for every `Complete` and `Embed` call on every provider. Callers cannot bypass
   it; the composer's own call site is removed in favor of the chokepoint.
2. The pattern set is widened to the modern key shapes listed above plus `sk_live_`, `rk_live_`,
   `xox[bpa]-`, `AIza` and `AKIA`, and is table-driven with a test fixture of real-shaped
   synthetic keys that must all be redacted.
3. The configuration promised by the threat model exists in minimal form:
   `redact.patterns: [<named regex>...]` extends the built-in set; there is no way to disable
   the built-ins. The threat model is corrected to describe exactly this.
4. Local egress is disclosed: the README's provider section and `serenity config` output state
   which text leaves the machine and to which provider; the dashboard disclosure for hosted
   embedding egress is unchanged.

## Consequences
- Positive: one audit point for what leaves the machine; new providers inherit redaction.
- Positive: the threat model and the code say the same thing.
- Negative: redaction in the router runs on every call, including tests; the cost is a regex
  pass over the prompt and is measured in the router benchmark.
- Negative: a redacted secret inside a fact is stored intact and only masked on egress; that is
  the intended semantics (the person's own memory keeps the value), and the disclosure says so.
- Related: E24 task T24.12 in `docs/plans/deep-review-001-remediation.md`.
