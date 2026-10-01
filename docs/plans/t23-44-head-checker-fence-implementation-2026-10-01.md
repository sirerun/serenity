# T23.44 hosted HEAD checker and fence candidate

Source candidate: `263d5bb0ba463a70365004dc0f66e350141e8847` in the isolated hosted head-fence worktree, based on main `9e6db7e`. This is a partial, reviewable candidate only. It does not activate a checker, startup/ticker reconciliation, or operator workflow, and it does not establish T23.44 or hosted acceptance.

The candidate adds a queue commit fence with context-aware flush locking, trusted hosted remember publication before provider/index work, and a canonical Git HEAD reader that validates repository metadata and reads the committed source payload by object ID. It also threads request context through hosted forget’s index purge and history rewrite. The history rewrite now runs Git commands in owned process groups on Darwin/Linux; cancellation kills the group and waits for it before queue/fence release. Other platforms fail closed for this path. The forget legacy wrapper retains its existing API and background-context behavior.

The candidate deliberately reports `Unknown` when an entered operation has no matching committed HEAD evidence. HEAD absence alone cannot establish `Absent`: worktree/index/touched-path proof and a complete routing audit are not implemented here. Cancellation records and forget/history purge remain ambiguous when the fact is absent. No retained applied marker was added; that retention decision remains open. All hosted mutation/flush routes must be audited before checker activation, and providers must stay outside the exclusive checker fence.

Focused verification on this exact source state:

- `go test -race ./internal/writer -run 'TestHistoryProcessGroupCancellationStopsDescendants|TestFlushContext|TestSubmitAndFlush|TestForget|TestHistoryRewrite' -count=1` — passed.
- `go test -race ./internal/hosted/pool -run 'TestCanonical' -count=1` — passed.
- `go test -race ./internal/server/memory -run 'TestHostedForgetPublishesErasureBeforeHandlerReturns' -count=1` — passed.

Tests ran one package at a time with caches and temporary files on the external build volume. The process cancellation regression verified the delayed child writer could not create a late file after cancellation; a follow-up process scan found no test child remaining. Independent review is pending. No multi-package build, live provider call, startup/ticker activation, merge, or push was performed.
