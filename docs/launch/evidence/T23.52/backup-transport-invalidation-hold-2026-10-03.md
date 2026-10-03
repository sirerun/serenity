# Supplementary independent review: failure invalidation

**Verdict: HOLD for bounded source merge pending transaction invalidation correction and review.** This finding supplements the prior CLEAR. Source commit remains `6bab4ceae45b8ef092f421808335eec9aaee97cd`; no source files were changed in this review clone or author checkout.

## Finding

The public `S3Storage` API rejects malformed successful remote results but does not invalidate the storage transaction for failures raised after `_request` returns. `_request` marks `_failed` only for child/request exceptions; validation errors in `list_prefix` and `read_to` occur outside that wrapper. I exercised three public sequences using only the existing local fake AWS child:

1. `list_prefix` receives successful JSON with string `IsTruncated`. It raises `TransportError("versions response has invalid IsTruncated")`; a subsequent public `put_if_absent` in the same context then succeeds and the fake child records `put-object`.
2. `read_to` receives successful HEAD JSON with the wrong `VersionId`. It raises `TransportError("version-bound HEAD failed length or identity checks")`; a subsequent public `put_if_absent` succeeds.
3. `read_to` receives successful range output with the wrong `VersionId` in the response metadata. It raises `TransportError("range response metadata failed identity checks")`; a subsequent public `put_if_absent` succeeds.

The destination remains publicly writable after a rejected inventory or failed version-bound integrity proof. That contradicts the adapter's fail-closed invalidation/read-only-reconciliation contract and leaves public callers able to proceed after the transaction has observed inconsistent remote state. The issue is independent of whether current `backup_publish` call paths happen to abort at that exception.

Expected semantic: once a public operation observes malformed/contradictory remote control data, a version/length/range mismatch, local destination failure, or owned scratch cleanup failure, mark the context failed before propagating. Subsequent public writes in that context must fail with the transaction-invalidated error, while explicitly authorized COMPLETE ambiguous-outcome reconciliation remains read-only. Add regression coverage that asserts each malformed public call raises, then asserts a same-context `put_if_absent` refuses before any `put-object` fake-child call. Ensure existing narrow ObjectExists/ambiguous COMPLETE semantics remain intact.

## Reproduction and integrity

Reproduction used the existing `TransportTests` fixture and its trusted fake CLI only. The fixture executable was edited in a private system-temp directory to return malformed successful `list-object-versions`, HEAD, and range metadata. For each case the public write was attempted in the same `S3Storage` context. Output:

```text
inventory: rejected=True; subsequent_public_put_succeeded=True; error=versions response has invalid IsTruncated
head: rejected=True; subsequent_public_put_succeeded=True; error=version-bound HEAD failed length or identity checks
range: rejected=True; subsequent_public_put_succeeded=True; error=range response metadata failed identity checks
```

No source/provider binary, cloud endpoint, credentials, provider request, or live test was involved. The independent review clone remains clean at the pinned commit. This finding does not itself change full-suite status (111 hosted tests pass with the fixture configured); those tests currently omit the public same-context follow-up-write assertion.
