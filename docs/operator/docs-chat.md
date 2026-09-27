# Docs-chat Lambda: secrets, rate limits and history

The public documentation assistant (`deploy/chat`, stack
`serenity-adoption-chat`) is an unauthenticated Lambda function URL. This note
covers the three controls an operator needs to know about when deploying or
reviewing it. The full deployment and alarm runbook is
[`deploy/chat/README.md`](https://github.com/sirerun/serenity/blob/main/deploy/chat/README.md).

## Rate-limit salt lives in Secrets Manager

Visitor IPs are hashed with a salt before they are used as rate-limit keys.
The salt is a CloudFormation-generated Secrets Manager secret
(`serenity/adoption/rate-salt`, logical ID `RateSaltSecret`, 64 alphanumeric
characters). Nobody chooses, types or copies it, and it never appears in a
template parameter, in `stack.json` or in the function environment.

- The function environment carries only `RATE_SALT_SECRET_ARN`. The role may
  call `secretsmanager:GetSecretValue` on that one secret (and on the model
  secret when one is configured).
- The handler reads the salt once per execution environment and caches it.
- A plaintext `RATE_SALT` environment variable is refused: while it is present,
  or while the secret is missing or shorter than 32 characters, every chat
  request gets a 503 before any model call. `/healthz` is unaffected.

The first UPDATE after this change creates the secret and drops the old
`RateSalt` parameter. Because the salt changes once, per-visitor counters start
fresh at that moment; the service-wide daily counter is not salted and carries
on. `deploy.py` no longer passes any salt.

Rolling back to a template from before this change would need a `RateSalt`
parameter value again and would put the salt back in the environment; roll
forward with a fix instead.

## Rate limits

Each chat request atomically increments three DynamoDB counters in one
transaction, and is rejected with 429 if any is at its cap:

| Counter | Cap |
| --- | --- |
| hashed visitor IP, per hour | 20 |
| hashed visitor IP, per UTC day | 50 |
| whole service, per UTC day | 200 |

The per-visitor daily cap keeps a single IP from exhausting the service-wide
budget (previously possible in about ten hours at the hourly rate). Counters
expire after about 25 hours.

## Client history is untrusted

The browser sends up to eight earlier turns as `history`. Only the last four
turns are considered, and of those only `role: user` entries reach the model.
Assistant and system turns from the client are dropped, so a caller cannot
forge what the assistant previously said.

## Post-deploy check (read-only)

After the change set completes, confirm the environment holds the secret
reference and no salt. This prints variable names only:

```sh
aws lambda get-function-configuration --function-name serenity-adoption-chat \
  --region us-west-2 --query 'keys(Environment.Variables)'
```

Expect `RATE_SALT_SECRET_ARN` in the list and no `RATE_SALT`. Then run
`python3 deploy/chat/verify_live.py --ask` to confirm a real cited answer still
goes through the rate limiter.
