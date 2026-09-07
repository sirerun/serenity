# Website validation

Checked 7 September 2026 against the site branch.

- Static site: 16 HTML pages including 404, with 315 internal asset/navigation/anchor references verified. No framework or deployment build step.
- Backend: 12 unittest cases executed and passed. Coverage includes request/origin limits, fail-closed rate enforcement, retrieval, unavailable-model fallback, invented citations, normalization of valid plain-URL citations, and exact packaged corpus equivalence.
- Browsers: 30 captures over home/product/install/docs/chat at 390×844, 1024×768 and 1440×900, in light and dark, showed no horizontal overflow or missing images. Additional fresh captures verified subsequent fixes.
- Reduced motion: videos and decorative animations are disabled while content remains visible. The reference home stage deliberately keeps its light art direction in either OS scheme; supporting pages have paired themes.
- Fresh-eyes review found and drove fixes to the middle-card source-pill collision, mobile launcher covering card buttons, site-wide launcher covering reading text, and inherited prose-link color overriding CTA contrast.
- A retained browser stylesheet initially produced stale captures. Static CSS/JS URLs now carry content hashes; fresh computed styles verify the launcher is in document flow and prose CTA text uses the correct foreground.
- Real AWS Lambda POST: source-install question returned a model answer with the correct @main command, Go prerequisite, release distinction and links into the guides. Browser POST from the allowed development origin also returned cited guidance on using local search without a model. No console errors observed in that flow.
- CloudFormation deployment: Python 3.13 Lambda, dedicated model secret and atomic DynamoDB rate limiting. Existing account concurrency quota required omitting a reservation. No existing secret value was changed.
- DNS: Cloudflare MCP lists no sire.run zone; authoritative nameservers are Google. Dedicated one-record Pulumi preview passes in foundation PR224. GitHub branch protection requires one independent approving review before it can merge.

Do not interpret local browser or Lambda verification as a receipt for public-domain publication. Live Pages, DNS, TLS and installed-domain chat are checked separately after the domain is activated.
