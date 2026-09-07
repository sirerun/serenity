# Local report

`serenity report` shows a current snapshot of retained claims and observations.
Run it weekly to review the same metrics. It does not reconstruct the previous
week or promise a weekly trend where history is unavailable.

```sh
serenity -C /path/to/brain report
serenity -C /path/to/brain report --export > report.json
```

`--export` is a boolean flag. It prints one standalone JSON object to stdout;
it takes no filename. Shell redirection saves that output. Choose a destination
outside your canonical brain files. Human output and JSON export propagate
output errors.

The command requires a valid `serenity.yml` and an existing SQLite index. Run
`serenity sync` explicitly if the index is absent. Reporting never initializes
or rebuilds a brain, migrates the index, records size samples, invokes a model,
or sends anything over the network. Its database handle is read-only; SQLite
may use its usual WAL coordination files when an index is already in WAL mode.
Sharing an export remains a separate manual action.

## Metric definitions

| Field | Population, units and time window |
|---|---|
| `claims_by_state` | Current retained canonical history, once per entity/family/claim ID. Fence claims count only for fence families; shards own shard families, including rollover segments. Superseding links make retained predecessors superseded even when their immutable line still says active. Retractions take precedence. Derived fence heads and index-only claims are excluded. Compaction can remove history; this is not all claims ever extracted. |
| `corrections_per_100_extractions` | Unavailable. Neither extraction-event totals nor linked correction outcomes are persisted. `extractions`, `corrections`, and the rate are null. A rejected reconciliation is not automatically an extraction correction. |
| `reconcile_backlog` | Current pending/deferred reconciliation disposition items. Excludes parked/disposed items and does not measure upstream extraction work. |
| `disposition_queue.depth` | Current pending plus deferred items across all kinds. Parked items are excluded. |
| `disposition_queue.p50_age_seconds` | Median time since creation for that same population. Null when empty. Alert above three days; depth alert above 50. |
| `disposition_queue.time_to_dispose_seconds` | Median creation-to-disposition duration across all retained currently disposed items, not only this week. Null with no disposed items. |
| `disposition_queue.abandonment` | Currently parked / (currently parked + disposed). Null for an empty denominator. |
| `spend.per_day_usd` | Recorded USD spend grouped by UTC day, through `generated_at`. Future-dated rows are excluded. |
| `spend.month_to_date_usd` | Recorded spend within the current UTC calendar month through `generated_at`. |
| `spend.projected_month_usd` | Month-to-date / elapsed UTC calendar days including today × days in the month. This is an extrapolation, not a bill. The displayed ceiling is the existing default $50. |
| `ingest_lag_by_connector` | Seconds since the latest completed successful job. This measures time since a successful poll, not source-event ingestion delay. Null if no success has been observed. |
| `connector_health` | Latest observed job state ordered by completion time, or start time for an in-flight job. MTTR is the mean time from the first observed failed/interrupted completion to the next successful completion. Retries belong to one episode; unresolved episodes are excluded. `completed_recoveries` supplies the denominator and `recovery_method` labels this proxy. No observed recovery means null. |
| `repo_growth.current_bytes` | Sum of regular-file lengths under `brain/` and `.dira/`. Excludes symlinks, `.git`, derived `.serenity` data and other files. This is canonical content size, not physical Git storage usage. |
| `repo_growth.bytes_per_month` | Retained historical size delta normalized to 30 days, only when the observed interval spans at least 30 days. The actual delta, start and end accompany it. Future samples are ignored. This is not a prediction or calendar-month total. No production size recorder exists yet; repeated reports do not create history. |
| `rebuild_timing` | Latest recorded rebuild duration in milliseconds and timestamp. No distribution or weekly trend is recorded. `never: true` indicates no measurement. |
| `trust_ladder_promotions_demotions` | Unavailable: ladder state is in memory and transitions/audited outcomes are not durably recorded. A future sampled false-acceptance rate must divide incorrect outcomes by completed audits of sampled automatic acceptances, not by all dispositions or unreviewed samples. |
| `search_p95_ms` | Unavailable: no search latency samples are persisted. |

`unavailable_reasons` accompanies missing instrumentation in exported JSON.
Null queue ages/durations mean no measured population; zero queue depth means
an observed empty queue. Spend reflects the retained ledger, not a completeness
guarantee about external billing.

## Export contents

Reports contain aggregates and observation timestamps, not claim text, entity
names, sources, disposition notes/actors, job errors/cursors, tokens or paths.
Connector identifiers can contain email addresses and local paths, so both
human output and export replace them with `connector-1`, `connector-2`, and so
on. These aliases agree within one report but are not persistent identities
across reports. Aggregate activity and spend may still be personal information.

`serenity report` is not a scheduled cron job. Invoke it manually or schedule
the commands above using the timer patterns in [scheduling](scheduling.md).
