# Integration request: running a real local service over the offline fixture

Status: **open. T23.60 changed no runtime file and no shared seam.** The offline fixture (`evals/hosted-load/fixtureprep`) is built and verified without a service. Running a service over it needs the four items below. Each names the owner who can decide it. None is a T23.60 decision.

## 1. The embedding pin and vector width

A brain records its embedding pin in `serenity.yml` when the hosted pool first opens it, and every later open compares the pin to the service's embedder (`internal/hosted/pool/pool.go`, `embed.ErrPinMismatch`). The fixture's brains carry the pin `fixture-hash-embedder-d64@infrastructure-only-v1`, and every vector row is 64 floats wide.

A service built by `service.New` takes its pin from `embedding_model` and `embedding_version` (`internal/hosted/service/service.go`). Two conditions must both hold for a recall over the fixture to work:

- **Pin.** Set `embedding_model` to `fixture-hash-embedder-d64` and `embedding_version` to `infrastructure-only-v1`. Any other pin makes the first open of every brain fail with `ErrPinMismatch`. Two in-process tests in `evals/hosted-load/fixtureprep/readpath_test.go` check this against the real pool: the pool opens a prepared brain under the fixture pin and the production `recall` tool returns exactly the regenerated facts (total and text); the pool refuses a brain under two other pins with `embed.ErrPinMismatch` and leaves its config unchanged. No listener or provider is involved.
- **Width.** `SearchVectors` compares the query vector with every stored vector and returns an error on a width mismatch (`internal/index/vectors.go`, `vector dimension mismatch`). A provider that returns another width therefore breaks vector recall on every fixture brain, and it also computes new vectors for `remember` under the fixture pin at the wrong width.

The service builds its embedder only from `OpenAIEmbeddingsProvider`. A local, offline run needs an OpenAI-compatible `/embeddings` endpoint on a loopback address that returns the same 64-wide hash vectors. In development mode the service accepts `http://127.0.0.1:<port>` for `embedding_base_url` and still reads a secret named `EMBEDDINGS_API_KEY` from its secrets directory, so that stub needs a synthetic key.

**Decision needed (load-harness owner, T23.68):** build that loopback endpoint, or assemble the service with `service.Assemble` and an in-process embedder in a Go test harness. T23.60 built neither. The fixture tool imports no network package, and a stub server would be a second component to qualify.

**Cost of not deciding.** No service run over the fixture is possible. The fixture stays a preparation and an inventory.

## 2. Paid entitlements come from direct SQL

The gateway resolves a paid plan from an `active` or `trialing` subscription row whose period includes the current time (`internal/hosted/meter/meter.go`, `Entitlement`). Billing writes that row from a Stripe webhook. No ordinary interface grants a paid plan without one, so the preparer inserts the rows by SQL into its own new control database. The price and subscription ids are synthetic (`fixture-price-*`, `fixture-sub-*`), not Stripe ids.

- The period is `now - 1 hour` to `now + 30 days` at preparation time (`-entitlement-days`). After it ends, the account falls back to the Free plan and `verify` fails `control_db.entitlements_match_plan_and_period`. A run must start inside that window, or prepare again.
- The service must not run during preparation. It cannot: the tool creates the database, so no process holds it.

**Decision needed (billing owner):** none for a fixture. State it so nobody reads the rows as a billing-path test.

## 3. The load client reaches 14 of the 29 brains

`scripts/hosted/load.py` reads one credential per account, and each credential is bound to the account's default brain. The fixture issues exactly that: one credential per account on brain index 0. The other 15 brains (9 Scale, 2 per Builder account) hold facts and count toward the account's memory total, but no request in the current client can reach them.

| | Facts on default brains | Facts on other brains | Total |
|---|---:|---:|---:|
| Full fixture | 25,002 | 64,998 | 90,000 |

25,002 is 5,000 (Scale) + 3 × 3,334 (Builder) + 10 × 1,000 (Free). The 90,000 is the account-wide memory total the gateway checks; the client's recall corpus is the 25,002.

**Decision needed (task41 reviewer, load-client owner):** whether the load client gains per-brain credentials or the plan declares that the 15 non-default brains are inventory only. This is question 4 of `decision-request-full-cardinality-eligible-traffic.md` extended. T23.60 did not change the client.

## 4. Seeded ids are not known to the load client

The fixture's facts are content-addressed (`fact_id` is the SHA-256 of the canonical bytes) with sequential legacy ids `1..N` in each brain. The current client forgets only an id it remembered, so every forget offered against a seeded fact is skipped (`SKIPPED_FORGET`). The verifier can list the seeded ids from the directory; nothing hands them to the client.

**Decision needed (load-client owner):** either the fixture supplies a seeded-id list the client may forget, or the skipped forgets stay in the accounting as `skipped_forget_no_fact` (decision request, question 4).

## What none of these change

The 90,000 memories, the 29 brains, the 15%/5% mix, every advertised entitlement and every acceptance criterion are as frozen. `--live` stays unconditionally blocked.
