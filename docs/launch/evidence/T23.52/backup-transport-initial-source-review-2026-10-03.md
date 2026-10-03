# Independent backup transport source review

**Verdict: CLEAR for the bounded source change at `6bab4ceae45b8ef092f421808335eec9aaee97cd` against `e6dc3f66ee25fe9b9e30b4b3acae6472b812d075`, under frozen contract v1.2.** No source changes were made. This is source review evidence only; it does not accept T23.52 or qualify provider, deployment, recovery, exclusive-writer, lifecycle, staging-capacity, throughput, or activation behavior.

## Review basis

Read repository `AGENTS.md`, the source handoff receipt, frozen assignment and R5 proposal, v1.1 lifetime/amended scratch terms, v1.2 typed-write ruling, and current Serenity coordination board/feed. The R5 proposal hash is `424438e782f41c3b2831c873a8adf155434fe02f15696c4dc297cae28e5e4991`; the frozen assignment adopts it and its later v1.1/v1.2 rulings. The board and feed contained no applicable hold. Ajent MCP is unavailable. Feed was read from the explicitly assigned path.

Reviewed all six source-owned Python changes, plus the receipt for scope only. The implementation preserves complete versions-plus-delete-markers inventory and bounded pagination; latest-object metadata and exact explicit-version reads; bounded HEAD/range bootstrap; pinned CLI path/version/checksum, private explicit credentials and allowlisted child environment; disabled metadata lookup/pager/autoprompt and explicit no-pagination; typed definite/ambiguous writes; definite COMPLETE collision refusal; ordinary two-pass, ambiguous three-pass and download one-pass full artifact proofs; final complete identity-set comparison; conditional single/multipart writes; 500,000,000,000-byte object/transfer ceiling, 1,863-part maximum and 500,268,435,456-byte extra part-spool allowance; disjoint scratch peaks and incremental caps; operation/child/cleanup deadlines, invocation/stdout/stderr/transfer byte ceilings; and invalidation to read-only reconciliation. Owned spool cleanup is checked before advancing and cleanup errors block success. The launcher applies `RLIMIT_FSIZE` in the child PID.

The changes do not wire callers or alter installed scripts, units, retention, credentials, Go/module source, or provider configuration. The remaining live/deployment gates are explicit in the contract and receipt and are not represented here as passed.

## Verification

- Full hosted suite: **111 passed**, with `ResourceWarning` promoted to an error. Captured output: `hosted-unittest.txt`.
- Ruff check: all six allowed Python files passed. Ruff format check: all four newly owned transport/child Python files passed. `git diff --check` passed. Outputs are captured in this directory.
- Genuine mutation RED: removing the `--version-id` argument causes `test_inventory_and_range_get_are_explicitly_version_pinned` to fail because the request lacks its required pin. Source bytes were restored exactly; the original source SHA-256 is `8606ae4fdfface1da33d7def5147593507fb9531297909c543ab950c76985170`. Output: `mutation-version-pin-red.txt`.
- Genuine mutation RED: broadening the publisher's typed `AmbiguousStorageFailure` catch to `Exception` causes `test_untyped_final_failure_is_never_reconciled` to fail. Source bytes were restored exactly; the original publisher SHA-256 is `14dd3984a31827d386085e08418cbb61a5dc275211f5450e57a66f62520dccaa`. Output: `mutation-typed-failure-red.txt`.
- The hosted suite uses a private system-temp directory because fake executable ancestry checks reject the external-volume test path. This did not alter repository source or test fixtures in the author checkout.

No AWS/S3 requests, provider credentials, live tests, or Go tests were used. Synthetic tests calculate ceilings; no physical 500 GB transfer or capacity claim was made.
