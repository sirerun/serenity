# Integration request: the cost-measurement input

Status: **open. T23.60 changed no task registry entry and no other task's files.**

## Finding

`docs/tasks/hosted-completion/T23.68.md` runs:

```sh
python3 scripts/hosted/cost_model.py --manifest "$QUALIFICATION_MANIFEST" --measurements "$EVIDENCE_DIR/load.json" --output "$EVIDENCE_DIR/cost.json"
```

`load.json` is the load client's result. It has no `snapshot_size_bytes`, request count or CPU
surplus, and `scripts/hosted/load.py` does not produce them. The earlier `cost_model.py` read
those names from any JSON object, so a load result, an empty object or a malformed file could
mark cost lines `measured`. It also applied one measurement to all four scenarios.

`cost_model.py` now accepts only the `serenity-hosted-cost-measurements` version 1 schema
(see [hosted-economics.md](../../hosted-economics.md#measurement-input)) and returns `BLOCKED`,
with no output file, for anything else. With T23.68's command as written it returns `BLOCKED`
for `load.json`.

## Requests

1. **Registry owner (coordinator).** Choose one:
   - **A.** Drop `--measurements` from T23.68's `cost_model.py` command until a producer exists.
     The model then prices on assumptions and reports `CALCULATED` and `NOT_QUALIFIED`.
   - **B.** Assign an owner to produce a `serenity-hosted-cost-measurements` document from real
     evidence, then pass that file. Each record needs a scenario, one quantity in its exact
     unit, an observation time, and a source kind, reference and SHA-256 of the evidence file.
2. **Producer owner (T23.68 or T23.64, if B).** The three quantities the model can take are
   `snapshot_size_bytes` (a listing of one backup prefix), `s3_put_requests_per_month` (S3
   request metrics) and `ec2_surplus_credit_vcpu_hours` (billed surplus credits). A quantity
   that no evidence supports stays out; the model prices it on an assumption.

Recommendation: A now, B when a live qualification has produced evidence. T23.60 does not
claim the load client produces any of these fields.

## Not requested

No change to runtime code, thresholds, retention, plan limits, credit mode or infrastructure.
