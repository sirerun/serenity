# MCP OAuth interoperability audit

Source baseline: 28010b94. Review scope: hosted OAuth adapter, CIMD resolver, pinned mcpoauth implementation and authenticated MCP gateway. This is a source review with targeted HTTP regressions, not a blanket certification or successful Claude/Gumloop connection claim.

## Standards and findings

References: [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization), [current MCP authorization](https://modelcontextprotocol.io/specification/latest/basic/authorization), [CIMD draft02](https://www.ietf.org/archive/id/draft-ietf-oauth-client-id-metadata-document-02.html), [RFC7591 section2.1](https://www.rfc-editor.org/rfc/rfc7591.html#section-2.1), [RFC9110 section11.1](https://www.rfc-editor.org/rfc/rfc9110.html#section-11.1).

| Area | Finding and disposition |
| --- | --- |
| Client metadata | Claude's public document at https://claude.ai/oauth/mcp-oauth-client-metadata declares authorization_code, refresh_token and urn:ietf:params:oauth:grant-type:jwt-bearer with response code and public authentication none. Previous parser rejected the whole document for an additional capability. Accept the code-flow intersection while retaining server endpoint grant restrictions. This is a concrete interoperability defect; additional client capabilities do not require server implementation. |
| Invalid metadata | Rejected metadata formerly became HTTP500 through the SDK lookup error path. Classify it as an unrecognized client: HTTP400 without redirect to an unvalidated destination. Transport/storage failures remain service failures. |
| Bearer scheme | Exact-case gateway parsing rejected valid lowercase bearer. RFC9110 requires case-insensitive scheme matching. Accept casing and one-or-more space separators; token verification remains unchanged. |
| Discovery | Protected-resource metadata, AS metadata, registration, S256, code and none authentication advertisements present. Raw challenge quotes are correct; an initial audit escaping concern was a tool-rendering misread and was retracted. |
| Authorization security | Exact registered redirects validated before redirects; S256 PKCE, single-use consent/codes, configured scopes and token resource binding retained. Actual unsupported grants still fail. |
| DCR and CIMD | Both retained. Public none clients supported; private_key_jwt is not advertised or implemented. Unsupported auth methods remain rejected. |
| Resource indicators | Supplied incorrect resource rejected and token audience bound. MCP requires clients to send resource; server acceptance of omission is not itself a demonstrated server violation. |
| Browser policy | Consent page permits only its registered callback origin. No browser protections disabled or alternate callback navigation used. |

## Evidence and acceptance boundaries

Before the metadata fix, the additional-Claude-grant and additional-response tests failed. Empty explicit capabilities also exposed insufficient validation; now rejected. Before gateway fix, real HTTP OAuth-to-MCP tests failed401 with lowercase bearer for loopback, Claude and Gumloop callback fixtures. After fixes, verify candidate via affected race tests, full build/vet and lint; final PR receipt records exact revision and results.

The real Gumloop trace showed fresh consent POST303 followed by a request for its registered callback and Chrome ERR_ABORTED. Reusing the consumed consent produces missing_or_used; it is not evidence of an originally expired request. Sanitized server records also show a recent read-only grant, but do not prove the corresponding Gumloop UI connection or all retries succeeded. Claude's published metadata reproduces a definite source rejection; a real Claude connection after release remains required. Neither browser extension blame nor a common cause across clients is established.

Production release remains coordinated through the existing deployment owner. This source batch does not alter runtime trust, data, CSP policy or installed browser settings. Ajent MCP unavailable; repository coordination file is used.
