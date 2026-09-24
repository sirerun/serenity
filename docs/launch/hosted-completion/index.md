# Task contract index

All34 tasks are planned work; no acceptance is implied. Read [the execution plan](../hosted-plan.md) before dispatch.

| ID | Task | Lane | Dependencies | Live gates |
|---|---|---|---|---|
| [T23.41](../../tasks/hosted-completion/T23.41.md) | Freeze integration contracts and reconcile the merged baseline | integrator | none | none |
| [T23.42](../../tasks/hosted-completion/T23.42.md) | Qualify the OpenRouter-compatible adapter and fail closed on invalid vectors | embeddings | T23.41 | none |
| [T23.43](../../tasks/hosted-completion/T23.43.md) | Build a reproducible semantic retrieval qualification corpus | embeddings | none | none |
| [T23.44](../../tasks/hosted-completion/T23.44.md) | Make quota accounting recoverable and storage admission bounded | runtime | T23.41 | none |
| [T23.45](../../tasks/hosted-completion/T23.45.md) | Remove avoidable tenant blocking and qualify fair admission | runtime | T23.44 | none |
| [T23.46](../../tasks/hosted-completion/T23.46.md) | Complete identity edge cases and optional pilot admission | experience | T23.41 | none |
| [T23.47](../../tasks/hosted-completion/T23.47.md) | Complete billing reconciliation and deletion-safe provider closure | billing | T23.41 | none |
| [T23.48](../../tasks/hosted-completion/T23.48.md) | Make deletion crash-safe with an independent durable tombstone journal | durability | T23.41, T23.47 | none |
| [T23.49](../../tasks/hosted-completion/T23.49.md) | Version and verify backup manifests before restoring any bytes | durability | T23.41 | none |
| [T23.50](../../tasks/hosted-completion/T23.50.md) | Implement explicit restore reconciliation and safe reactivation | durability | T23.47, T23.48, T23.49 | none |
| [T23.51](../../tasks/hosted-completion/T23.51.md) | Bootstrap a blank ARM host idempotently and validate topology | operations | T23.41 | none |
| [T23.52](../../tasks/hosted-completion/T23.52.md) | Implement verified uploads and bounded all-version retention | operations | T23.48, T23.49 | none |
| [T23.53](../../tasks/hosted-completion/T23.53.md) | Add bounded telemetry and prove production log redaction | operations | T23.41, T23.42 | none |
| [T23.54](../../tasks/hosted-completion/T23.54.md) | Integrate least-privilege infrastructure and delivered alarms | integrator | T23.51, T23.52, T23.53, T23.60 | none |
| [T23.55](../../tasks/hosted-completion/T23.55.md) | Finish onboarding, recovery UX and multi-account browser coverage | experience | T23.44, T23.45, T23.46, T23.47, T23.48 | none |
| [T23.56](../../tasks/hosted-completion/T23.56.md) | Prepare honest pricing, hosted instructions and reversible site activation | experience | T23.55 | none |
| [T23.57](../../tasks/hosted-completion/T23.57.md) | Wire all features and gate transactional deployment and releases | integrator | T23.42, T23.44, T23.45, T23.46, T23.47, T23.48, T23.49, T23.50, T23.53, T23.54, T23.55 | none |
| [T23.58](../../tasks/hosted-completion/T23.58.md) | Prove crash recovery with real subprocess fault injection | qualification | T23.44, T23.47, T23.48, T23.49, T23.50, T23.57 | none |
| [T23.59](../../tasks/hosted-completion/T23.59.md) | Execute the complete adversarial boundary matrix | qualification | T23.45, T23.46, T23.47, T23.48, T23.50, T23.55, T23.57 | none |
| [T23.60](../../tasks/hosted-completion/T23.60.md) | Build the cost model and bounded capacity workload | operations | none | none |
| [T23.61](../../tasks/hosted-completion/T23.61.md) | Prepare the exact credential, budget and operator prerequisite packet | integrator | T23.42, T23.43, T23.60 | none |
| [T23.62](../../tasks/hosted-completion/T23.62.md) | Prepare foundation DNS changes and verify real email | operations | T23.51, T23.61, T23.64 | MAIL, DNS, SPEND |
| [T23.63](../../tasks/hosted-completion/T23.63.md) | Create isolated Stripe test products, portal and webhook configuration | billing | T23.47, T23.61 | STRIPE_TEST |
| [T23.64](../../tasks/hosted-completion/T23.64.md) | Provision and privately deploy one cost-capped qualification environment | operations | T23.54, T23.57, T23.58, T23.59, T23.61; paid: T23.63 | SPEND, EMBEDDINGS, MAIL, DNS, OPS |
| [T23.65](../../tasks/hosted-completion/T23.65.md) | Prove real embeddings and the actual Rakazo customer journey | qualification | T23.42, T23.43, T23.55, T23.62, T23.64 | EMBEDDINGS, MAIL |
| [T23.66](../../tasks/hosted-completion/T23.66.md) | Rehearse off-host restore, deletion retention, migration and rollback | qualification | T23.48, T23.49, T23.50, T23.52, T23.57, T23.64; paid: T23.67 | SPEND, EMBEDDINGS |
| [T23.67](../../tasks/hosted-completion/T23.67.md) | Execute the full Stripe lifecycle with test clocks | billing | T23.47, T23.63, T23.64, T23.62 | STRIPE_TEST |
| [T23.68](../../tasks/hosted-completion/T23.68.md) | Measure capacity and recompute the launch bill | operations | T23.45, T23.60, T23.64, T23.65 | SPEND, EMBEDDINGS |
| [T23.69](../../tasks/hosted-completion/T23.69.md) | Prepare and record first-time human walkthroughs | experience | T23.55, T23.56, T23.65 | HUMANS |
| [T23.70](../../tasks/hosted-completion/T23.70.md) | Assemble the reviewed release packet and exact launch actions | integrator | T23.56, T23.57, T23.58, T23.59, T23.65, T23.66, T23.68, T23.69; paid: T23.67 | none |
| [T23.71](../../tasks/hosted-completion/T23.71.md) | Activate an optional invitation-only Free pilot | release | T23.70 | PILOT_GO |
| [T23.72](../../tasks/hosted-completion/T23.72.md) | Activate live Stripe with a bounded operator payment | release | T23.70 | LIVE_BILLING |
| [T23.73](../../tasks/hosted-completion/T23.73.md) | Perform the public launch go/no-go and final customer smoke | release | T23.70, T23.72 | PUBLIC_GO |
| [T23.74](../../tasks/hosted-completion/T23.74.md) | Observe the launch and close the epic with owned follow-ups | release | none; pilot: T23.71; paid: T23.73 | none |
