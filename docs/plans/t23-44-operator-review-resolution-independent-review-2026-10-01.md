# T23.44 private operator resolution independent review

Reviewed exact source `590ae145eac5781914bae1dd747e055f90986291` in isolated clone `/Volumes/BuildOffload/worktrees/serenity-operator-review-independent-20261001`, against frozen contract `docs/plans/t23-44-private-operator-resolution-contract-2026-10-01.md`.

**No blocker found in the service-only resolver.** The public `Service.Handler` does not mount `/operations/resolve`; only `AdminHandler` handles it. Missing and typed-nil admission dependencies return 503. The request parser imposes a 4096-byte bound and accepts exactly two string fields, rejecting duplicate, unknown/case-aliased, nested/non-string, whitespace/control, and trailing content. The service checks request cancellation and calls the trusted admission capability before any ledger lookup or brain fence. It requires exact operation/case binding, opaque operator ID, a nonzero approval timestamp, a bounded expected canonical ref, and the fixed `committed` outcome. Admission errors and mismatched cases use fixed responses; no private error text is returned.

After authorization, the service reads the record, and for a pending-review row fences its brain, rereads the record under the fence, verifies identity/account/brain binding and pending phase, then checks canonical state while the fence remains held through the ledger transition. Only `CanonicalLanded` with the exact approved reference can resolve; Unknown, Absent, cold/inactive, mismatched proof, and cancellation leave the row pending. It cannot request `Released`. A committed row with the same reauthorized case is idempotent and does not run another check or transition. The real-proof test exercises the original quota-period counters and exactly-once accounting. No frozen contract or state machine changed.

Focused independent race validation, run with a fresh one-minute load guard at or below 10 and `TMPDIR=/Volumes/SerenityPrivateFixture20261001/tmp`:

`go test -race ./internal/hosted/service -run 'Test(ParseOperatorReviewRequestStrict|AdminOperatorReview.*)$' -count=1` — PASS (`2.907s`).

This is only a service-level development seam. It does not supply a production human authenticator, immutable case store, private admin listener, or production activation. Authorization, durable case integrity, unique case-reference binding, and revocation remain obligations of the trusted admission implementation. No provider, live service, or cloud action was performed.
