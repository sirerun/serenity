# Hosted launch authorizations and external gates

This is a sanitized operator matrix. It records scope and evidence requirements, not secret values or permission to execute gated actions.

| Gate | Owner | Authorized scope / required evidence | Current state |
|---|---|---|---|
| EMBEDDINGS | Founder/provider reviewer | Synthetic-only OpenRouter qualification, `perplexity/pplx-embed-v1-0.6b`, exact `$1` cap, no fallback/top-up/reset; provider terms and capped seed/live receipt | Model-only probe recorded; hosted run pending |
| MAIL | Account owner/foundation | Resend send permission for reviewed `serenity.sire.run` sender; controlled recipient `david+alerts@sire.run`; DNS and redacted delivery receipt | Key/config available; live delivery and DNS evidence pending |
| STRIPE_TEST | Founder/billing owner | Existing Sire Run, Inc. test account; isolated products/prices, portal, signed webhook and test-clock lifecycle | Test-mode credentials available; Serenity objects and lifecycle receipt pending |
| SPEND | Chief/coordinator | Exact us-west-2 stack, max duration/cost and cleanup; `$450` is planning-only | No resources created; qualification stack approval/action pending |
| DNS | Foundation owner | Reviewed Google Cloud DNS records and TLS/mail verification for canonical site/app origin | Authority identified; change not applied |
| OPS | Incident responder | Delivered alert destination and named responder before public traffic | Founder is interim responder; delivery proof pending |
| HUMANS | Founder/coordinator | Five consenting first-time walkthroughs, or an explicit unavailable-participants exception | Not run |
| LIVE_BILLING | Founder | Paid release packet, restricted live key, bounded operator payment and rollback | Not authorized/executed |
| PUBLIC_GO | Founder | Complete acceptance matrix, live smoke and dated go/no-go | Not reached |

Existing approvals cover the synthetic embedding cap and planning ceiling. They do not convert a planning estimate into spend authority or qualify a missing external gate.
