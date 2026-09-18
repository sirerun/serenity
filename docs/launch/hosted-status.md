# Hosted Serenity status

Updated2026-09-18 for the requested parallel Sonnet completion plan. Source-of-truth execution queue: [hosted-plan.md](hosted-plan.md), [34 task contracts](hosted-completion/index.md), [registry](hosted-completion/tasks.json). Coordinator alone updates this aggregate file; workers write task-specific evidence.

## Merged versus live

- Hosted implementation PR [#234](https://github.com/sirerun/serenity/pull/234) was independently reviewed and rebase-merged at `b9863824faaea77b0dbe059af0e0b25efdce5750`.
- Its final branch head `2d7902d` passed15 checks; Pages deployment was skipped for a PR. This is CI evidence, not a production deployment.
- Local full race evidence previously counted1,902 passing cases and6 explicit skips, plus two real-CLI browser viewport cases. Test providers were explicit fixtures.
- No hosted VM/public service or live billing was created by this lane. Real Resend delivery, Stripe lifecycle, semantic retrieval, capacity/cost, recovery and public onboarding are unqualified.
- Planning audits identified additional correctness gaps: accounting/storage growth, provider billing closure during deletion, independent deletion journal/restore reactivation, backup integrity/retention and cross-tenant contention. See task44–50; merged code is not a claim these are solved.

## Next dispatch

First batch: **T23.41** integration/schema contracts, **T23.43** synthetic semantic harness, **T23.60** cost/load model. After41 review/acceptance, six disjoint tasks42/44/46/47/49/51 can run on already-authorized capacity. Mini builds remain serialized by the shared lease.

All completion tasks are planned, not accepted. No implementation worker was launched by this planning deliverable. Read-only audit agents helped check the plan; their recommendations are incorporated with explicit dependencies.

## External prerequisites

- A dedicated capped OpenRouter credential can serve as `EMBEDDINGS_API_KEY`; a separate OpenAI account is not required. Candidate `perplexity/pplx-embed-v1-0.6b` still needs serving-provider privacy and actual Serenity retrieval qualification.
- The last available Resend credential returned401 on a read-only lookup; valid sending access/domain verification remains needed. A sending-only key may not permit domain administration; confirm required scopes rather than infer provider failure from a forbidden admin endpoint.
- Dedicated Stripe test credentials/webhook secret remain needed for paid qualification. Live key/payment/go-no-go come after the paid release packet.
- Task61 prepares exact secret references, provider/workload caps and costed resource changes before escalating missing authority. The proposed$1 qualification cap and historical$60/month ceiling are not new spending approvals.
- Invitation-only Free pilot is a proposal, not an approved scope replacement. Default plan profile preserves original paid launch; pilot activation is a separate explicit gate.

The [historical plan](archive/hosted-plan-2026-09-11.md) preserves September11 discoveries. Its PR states, “no implementation exists,” OpenRouter capability claim and task statuses are superseded. Do not dispatch from it.
