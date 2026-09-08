# Synthetic import benchmark messages

From the repository root:

```sh
go run ./evals/gen/messages -n 10000 -seed 1
```

This writes `evals/generated/messages/manifest.json` and ten mboxrd files of
1,000 messages each. The manifest records the format version, seed, count,
partition size, and SHA-256 of every file. Dates start at 2026-01-01 09:00 UTC
and advance one minute per message. Each message has a unique, seed-qualified
Message-ID. All people, projects and correspondence are invented; every email
address uses a reserved `.invalid` domain. There are no model or network calls.

Options: `-out <directory>` and `-messages-per-file <count>`. Identical repeat
runs are no-ops; a different seed, partition size, count, or unrelated existing
file requires a different destination. Completed corpora are published as a
directory, so a cancelled generation never presents a partial manifest.

The same options produce identical bytes independent of the output directory.
Envelope lines follow mbox conventions; body lines beginning with `From ` or
`>From ` use mboxrd escaping. Counts and checksums are verified by tests that
parse all 10,000 RFC 5322 messages. This generates input only; timing the real
ingest pipeline and enforcing regression budgets is the separate T5.8 task.
