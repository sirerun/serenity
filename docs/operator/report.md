# Weekly report card

Run `serenity report` against a brain to see the metrics RFC 0001 section 16
names: claims by state, corrections per 100 extractions, the trust ladder,
spend, ingest lag, reconcile backlog, disposition queue health, repo growth,
rebuild timing, connector health/MTTR, and search p95.

```sh
serenity -C /path/to/brain report
```

Prints one line per metric to stdout, "n/a" wherever a metric has no
underlying population yet (a fresh brain, nothing ever disposed, no rebuild
run) rather than a misleading zero.

## Manual export

```sh
serenity -C /path/to/brain report --export report.json
```

`serenity report --export <path>` writes the full report as indented JSON to
`<path>` instead of printing the summary. Nothing is transmitted anywhere:
the command reads only the brain repo's own local index and canonical
files, and performs no network calls. Sharing the export — with a
collaborator, in an issue, wherever — is a manual step you take with the
file afterward; the command itself never does it for you.

Every run also measures the canonical brain repo's own on-disk size
(`brain/` and `.dira/`, excluding `.git` and the derived `.serenity` index)
and records one sample. Repo growth needs at least two samples spanning
real time to report a rate — the first run on a brain reports
`bytes_per_month: null` and `sample_count: 1`; later runs report a rate
once history exists.

## Fields with no data source yet

Two RFC section 16 metrics are present in the export, honestly empty,
because nothing in Serenity persists their underlying data yet:

- `trust_ladder_promotions_demotions`: the earned-automation ladder
  (`internal/ladder`) keeps trust state in memory only and is not wired
  into the reconcile engine in production, so no cell has ever promoted or
  demoted for real. `cells` is an empty list and
  `sampled_false_acceptance_rate` is `null` until that wiring exists.
- `search_p95_ms`: hybrid search has no latency-sampling pipeline. `null`
  until one exists.

## Scheduling

`serenity report` is not itself a `serenity cron` job — see the
[scheduling guide](scheduling.md) for the jobs that are. Run it by hand, or
add your own weekly timer calling `serenity report --export`, the same
launchd/systemd pattern that guide documents for `cron` jobs.
