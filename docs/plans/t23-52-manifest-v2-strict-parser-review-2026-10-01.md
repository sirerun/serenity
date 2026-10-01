# T23.52 strict-manifest parser review — hold receipt

Reviewed exact source `0c8e7816d3f420d9c820edee47b6c0542c134510` in isolated full clone `/Volumes/BuildOffload/worktrees/serenity-manifest-v2-review-0c8e78-20261001` (external SSD, `--no-hardlinks`; repacked reachable objects and removed this clone's inherited alternates file). The source was clean before the temporary regression probe.

## Result

The parser correction closes the prior literal unknown-field, exact duplicate-key, trailing-document, oversized-manifest, and excessive-nesting gaps. The package's new tests exercise duplicate keys at root and nested fields, unknown fields at root and source levels, trailing documents, size and nesting limits, while the implementation bounds the input and recursively scans JSON objects before decoding with `DisallowUnknownFields`.

However, the duplicate-key scanner compares raw JSON key strings while Go's `encoding/json` matches struct field names case-insensitively. Thus `"build_sha"` and `"Build_SHA"` are distinct to the scanner but both bind to the same struct field. A temporary runtime test changed a valid manifest to contain both `"build_sha": "test-build-sha"` and `"Build_SHA": "alternate-build"`; `parseManifest` accepted it. This remains a manifest ambiguity and is a **hold** until parser validation rejects case-variant aliases (or uses exact-key decoding) throughout nested manifest structs.

## Validation

- `go test -race -p 1 ./internal/hosted/backup` — PASS, 18.614s, external caches; immediately beforehand load averages were 4.38/5.06/5.42.
- Temporary case-variant duplicate-key regression probe — RED as intended: `parseManifest accepted case-variant duplicate struct field`. Probe file was removed; it is not a committed test.
- No lasting source/test mutations or cloud calls were made. Prior receipts retain the checksum/no-overwrite RED controls and the earlier unknown-field acceptance evidence; this receipt supplements them without rewriting history.

## Scope limits

This is a bounded library review and does not grant final T23.52 acceptance, production adapter/uploader/scheduler qualification, IAM/S3 validation, or restore acceptance. Review the exact follow-up parser source before removing the strict-schema hold.
