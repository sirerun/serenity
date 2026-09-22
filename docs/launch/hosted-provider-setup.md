# Hosted provider setup packet

Status: **ready for gate review; no live provider validation or deployment performed**.

This packet names the exact configuration and external evidence needed for the paid hosted profile. Secret values stay in the configured secret manager or private runtime environment; this document contains references only.

## Embeddings

- Secret reference: `EMBEDDINGS_API_KEY`
- Endpoint: `https://openrouter.ai/api/v1`
- Model: `perplexity/pplx-embed-v1-0.6b`
- Routing: Perplexity only; fallback disabled; request `ZDR=true` and `DataCollection=deny`
- Qualification input: synthetic corpus only, frozen T23.43 corpus and `exact_ratio_21` lexical-negative criterion
- Total qualification allowance: **$1 maximum**, with no automatic reset, top-up, paid fallback, or background readiness traffic
- Evidence still needed: current serving-provider privacy/retention terms, key-limit metadata, and the real Serenity seed/live run. The existing model-only synthetic probe is not hosted-service qualification.

## Mail

- Secret reference: `RESEND_API_KEY`
- Controlled test recipient: `david+alerts@sire.run`
- Sender/domain: the reviewed `serenity.sire.run` sender identity, after DNS and Resend verification
- Evidence still needed: successful controlled magic-link delivery and redacted delivery receipt. A valid key alone does not prove sender-domain permission.

## Stripe test mode

- Secret references: `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`
- Account: existing Sire Run, Inc. Stripe account, test mode
- Required objects: Serenity-owned Builder and Scale products/prices, Checkout return URLs, customer portal configuration, signed webhook endpoint
- Required evidence: account identity metadata, signature verification against raw bodies, replay/out-of-order lifecycle tests, test-clock renewal/failure/cancellation/upgrade/downgrade, and account-bound portal sessions
- Live mode is a later gate. No customer payment or live key is authorized by this packet.

## AWS and DNS

- Region: `us-west-2`
- Current reviewed topology: one ARM64 `t4g.small`, encrypted 30 GiB data volume, encrypted/versioned S3 backups, KMS key, Secrets Manager references, SSM administration
- Monthly planning ceiling: **$450**, planning-only; it authorizes no resource creation or recurring spend
- Current modeled full-limit known subtotal: `$305.47`; peak sensitivity: `$353.34`; unknown categories remain excluded
- DNS authority is currently Google Cloud DNS; the connected Cloudflare account is not evidence that Cloudflare hosts this zone. The foundation owner must review the exact A/TLS/mail records before apply.
- Operator gate: a named responder and delivered alert destination are required before enabling public traffic.

## Preflight and rollback

Run the metadata-only check before any external call:

```sh
python3 scripts/hosted/provider_preflight.py \
  --manifest docs/launch/hosted-completion/qualification.example.json \
  --output docs/launch/evidence/T23.61/preflight.json \
  --metadata-only
```

It reports only secret reference names and `network_calls: 0`; it never prints values or contacts a provider. Missing references produce `BLOCKED`, while present references produce `READY_FOR_REVIEW`, never `PASS`.

Before private qualification, record the exact change set, max duration/cost, cleanup command and rollback revision. Delete the rehearsal stack and temporary DNS/Stripe objects if the qualification gate fails. Rotate any credential whose scope or redaction evidence is incomplete. Do not enable billing or public DNS from this packet alone.
