# Hosted Serenity status

Updated2026-09-18 for the requested parallel Sonnet completion plan. Source-of-truth execution queue: [hosted-plan.md](hosted-plan.md), [34 task contracts](hosted-completion/index.md), [registry](hosted-completion/tasks.json). Coordinator alone updates this aggregate file; workers write task-specific evidence.

## Merged versus live

- Hosted implementation PR [#234](https://github.com/sirerun/serenity/pull/234) was independently reviewed and rebase-merged at `b9863824faaea77b0dbe059af0e0b25efdce5750`.
- Its final branch head `2d7902d` passed15 checks; Pages deployment was skipped for a PR. This is CI evidence, not a production deployment.
- Local full race evidence previously counted1,902 passing cases and6 explicit skips, plus two real-CLI browser viewport cases. Test providers were explicit fixtures.
- No hosted VM/public service or live billing was created by this lane. Real Resend delivery, Stripe lifecycle, semantic retrieval, capacity/cost, recovery and public onboarding are unqualified.
- Planning audits identified additional correctness gaps: accounting/storage growth, provider billing closure during deletion, independent deletion journal/restore reactivation, backup integrity/retention and cross-tenant contention. See task44–50; merged code is not a claim these are solved.

## Next dispatch

First batch: **T23.41** integration/schema contracts, **T23.43** synthetic semantic harness, **T23.60** cost/load model.

**T23.41 accepted** by the coordinator 2026-09-21 at revision `c61ab91bb48099dfbfcfec2ee86ea2cd590439c4` — see `docs/launch/evidence/T23.41/coordinator-acceptance.md`. This unblocks downstream implementation dispatch of the six disjoint tasks42/44/46/47/49/51 on already-authorized capacity. Storage admission stays conditional on task44's OS-enforced staging limit and allocation-based accounting per ADR 017; the acceptance does not qualify live S3 or storage admission. Mini builds remain serialized by the shared lease.

**T23.43's corpus and thresholds are partially frozen** as of 2026-09-21 — see the freeze receipt in `docs/launch/hosted-completion/embedding-eval.md`. Corpus hash, expected case IDs and the numeric thresholds are reviewed and accepted at revision `b1049482a1f1ffbd42f960a2b68341bb51691116`. The lexical-negative criterion choice is still open (named case-subset vs. exact ratio); no `--live` run may start until that choice is recorded. Model/provider configuration cannot be frozen yet: T23.42 (the real provider adapter) is still `planned`, not merged, so the manifest's `provider` fields have no concrete pin to freeze.

All completion tasks other than T23.41 are planned, not accepted. No implementation worker was launched by this planning deliverable. Read-only audit agents helped check the plan; their recommendations are incorporated with explicit dependencies.

## External prerequisites

- A dedicated capped OpenRouter credential can serve as `EMBEDDINGS_API_KEY`; a separate OpenAI account is not required. Candidate `perplexity/pplx-embed-v1-0.6b` still needs serving-provider privacy and actual Serenity retrieval qualification.
- The last available Resend credential returned401 on a read-only lookup; valid sending access/domain verification remains needed. A sending-only key may not permit domain administration; confirm required scopes rather than infer provider failure from a forbidden admin endpoint.
- Dedicated Stripe test credentials/webhook secret remain needed for paid qualification. Live key/payment/go-no-go come after the paid release packet.
- Task61 prepares exact secret references, provider/workload caps and costed resource changes before escalating missing authority. The proposed$1 qualification cap and historical$60/month ceiling are not new spending approvals.
- Invitation-only Free pilot is a proposal, not an approved scope replacement. Default plan profile preserves original paid launch; pilot activation is a separate explicit gate.

The [historical plan](archive/hosted-plan-2026-09-11.md) preserves September11 discoveries. Its PR states, “no implementation exists,” OpenRouter capability claim and task statuses are superseded. Do not dispatch from it.

Historical prerequisite-stack merge and verification receipts are preserved in [the September 12 T23.1 snapshot](archive/t23-1-merge-receipt-2026-09-12.md). That snapshot does not change the current execution queue.
