# Hosted Serenity implementation handoff

PR [#234](https://github.com/sirerun/serenity/pull/234) is merged at `b9863824faaea77b0dbe059af0e0b25efdce5750`. The candidate is implemented and locally tested; it is not deployed or release-qualified.

Execution now follows the [prescriptive completion plan](hosted-plan.md), [machine-readable registry](hosted-completion/tasks.json), [individual contracts](hosted-completion/index.md) and [Sonnet worker prompt](hosted-completion/worker-prompt.md). These replace the previous coarse “remaining work” table. All40 original requirements have an explicit [crosswalk](hosted-completion/crosswalk.md).

Start41/43/60 in parallel; after the schema/interface freeze,42/44/46/47/49/51 form the first six-way implementation batch. The coordinator owns shared schema, service/CLI assembly, CI, stack.json and aggregate docs. Every worker owns a scoped branch, contract and evidence directory. Do not edit another lane’s files or infer completion from a closed PR alone.

Critical unfinished work: crash-safe accounting/storage admission; deletion that closes pending provider billing; independent deletion journal; checksummed backup and safe restore reactivation; real all-version retention; fair cold-runtime admission; fresh-host deployment/rollback; observable failures; real provider/client/security/cost qualification. These are named tasks, not optional post-launch cleanup.

Provider choice is configurable. Perplexity0.6b on OpenRouter is a candidate, not a live-tested pin. Resend and Stripe test credentials remain external prerequisites. Task61 prepares exact private secret references and bounded cost proposals. No provider key values belong in this handoff, PRs or ajent.social.

The source contains `serenity hosted serve`, `backup`, `restore`, and `plans`. Existing restore invalidates old sessions/credentials and freezes accounts; do not manually reactivate them. Task50 supplies the safe reconciliation commands. The existing deploy script assumes installed dependencies/mounted data; task51 closes bootstrap. Follow the current runbook only within its documented limitations until task70 publishes the rehearsed version.
