# E24 docs-chat live hardening — 2026-10-01

Source: T24.27 handler and generated template already merged in main7b0baad.
The old live Lambda configuration contained plaintext `RATE_SALT` and no
secret-reference variable. Its last reported update was 2026-09-08.
No salt or model credential value was printed or written to evidence.

Reviewed change set `site-1790849036` updated only the existing Function and
Role (neither replaced) and added `RateSaltSecret`. Model secret and rate table
were preserved. CREATE_COMPLETE preceded execution; UPDATE_COMPLETE followed.
The coordinator held and released `R-docs-chat-deploy`.

Read-only post-deploy configuration check:

- Lambda Active / update Successful, modified 2026-10-01T10:07:41Z.
- Plaintext `RATE_SALT` absent; `RATE_SALT_SECRET_ARN` present.

`python3 deploy/chat/verify_live.py --ask` passed four live checks: health200
with nonempty corpus, allowed-origin preflight, rejected origin403, and one
public cited response (`mode=answer`). No sensitive question or response was
recorded. Local handler/template suites passed 26 cases, including client
assistant-turn exclusion and per-IP daily caps. Those rejection paths were
not deliberately exercised against production to exhaust its budget.

Incremental resource: one Secrets Manager secret. AWS published pricing is
$0.40 per secret-month plus $0.05 per 10,000 API calls, before credits/tax;
see https://aws.amazon.com/secrets-manager/pricing/ (checked 2026-10-01).
No additional function, instance, database, paid GitHub service or concurrency
reservation was created. Salt rotation resets the per-visitor counter once;
the unsalted service-wide counter is retained.
