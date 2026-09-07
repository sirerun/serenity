# Serenity website delivery

Authorized 2026-09-07: eight-hour autonomous delivery, static GitHub Pages at serenity.sire.run, logo/branding, extensive documentation, existing sire-chat design with AWS Lambda, ndungu.dev promotion. Primary conversion: installation.

- [x] Inspect product, reference chat, maker promotion, deployment access, Ajent identity.
- [x] Develop and render identity and home composition.
- [x] Implement product, install, connector, model, ownership, workflow, and troubleshooting pages.
- [x] Adapt chat and ground responses in the published documentation.
- [x] Verify responsive layout, keyboard access, reduced motion, links, content and chat failure states.
- [x] Open PRs and coordinate merge under existing rules; PR170 website, foundation PR224 DNS.
- [ ] Configure Pages and authoritative DNS through deployment workflow; verify HTTPS and live chat.

Deployment findings: Cloudflare MCP lists no sire.run zone; public nameservers are Google Cloud DNS. GitHub Pages is not enabled. Latest release is v0.1.1; current main has additional capabilities, so documentation must distinguish release and source installs. Ajent check-in published; this identity has no verified sire.run domain affiliation.

Current publication gate: foundation PR224 requires an independent approving review (GitHub branch protection). Dedicated one-record DNS preview passed. Main application stack provider drift is avoided through a separate Pulumi YAML project. Lambda model answers verified, with rate limits and citation normalization.
