# Approved shard compaction

Compaction moves superseded and retracted shard rows into each family's archive,
keeps the resolved live heads and removes emptied numbered rollover segments.
It requires a human-approved disposition item:

```sh
serenity compact --propose
serenity inbox
serenity compact --item <accepted-item-id>
```

The accepted item authorizes one pass. The first application reads clean canonical
shards and prepares exact file transitions in a durable runtime receipt before
changing files. It writes archives and live shards before deleting old segments,
then commits only those exact paths. Unrelated human staging stays untouched.
An uncommitted shard, a symlink or an unsupported edited scope stops the pass.

If publication or Git commit fails, resolve the reported issue and rerun the same
`compact --item` command. `inbox --unapplied` also displays that command until the
pass is committed and marked complete. Retry uses the saved plan, preserving
intervening human edits and avoiding duplicate archive rows or commits. A finished
receipt is historical: repeating an old approval does not compact newly added
claims. Propose and accept a new item for a later pass.

Receipts live under `.serenity/reconcile/compact-*.json`, alongside the shared
canonical-publication lock. Preserve an incomplete receipt when recovering a
failed pass. If a completed receipt is missing, the command refuses to reuse its
old approval. No model calls are needed, and the operation does not rewrite entity
pages or change the resolved current facts. Ordinary canonical publication still
cannot delete files; compaction deletion is limited to numbered shard segments.
