# Canonical dashboard and Claude connection

Owner request: refine the hosted experience, align page widths, split the
account dashboard into focused pages, retire the app subdomain, and support
Claude web connectors like Blink.

## Decisions

- Keep the existing brand and public-site frame. All hosted pages use the same
  page-width and gutter tokens as public pages; narrower forms sit inside it.
- Separate Overview, Connections, Memories, Usage and plan, and Settings.
  Put manual tokens and destructive actions behind explicit disclosures.
- Use the existing OAuth discovery, dynamic registration, PKCE consent and
  refresh/revocation implementation. Add regression coverage for Claude's
  HTTPS callback instead of introducing a second authorization system.
- Serve the application only at the canonical domain. On 26 September the
  authoritative Cloudflare CNAME was changed to a proxied A record for the
  same origin; canonical HTTPS returned 200 before the legacy A was deleted.
  Authoritative DNS subsequently returned NXDOMAIN for the retired name.

## Validation and remaining qualification

- Public-site browser suite: 36 passed across four viewport sizes.
- Dashboard package compiles; the full hosted service test package passes.
- OAuth consent/MCP/revocation integration test passes for both loopback and
  Claude HTTPS callbacks.
- Full hosted browser checks and release remain pending the shared build lease.
- Actual Claude web connection remains unqualified: the signed-in Claude
  account displays a disabled Add custom connector control. Blink already
  appears in its connector list. No new Claude grant or tool roundtrip occurred.
- Paid activation and unrelated backup/deletion work remain outside this change.
