# Independent source review: recovery envelope codec at 96bd06e

Verdict: **CLEAR for the frozen inert envelope codec scope only.** This review does not qualify any recovery factory, admission, journal/source proof, backup pin authority, store, provider, READY, restore, startup, CLI, or production behavior.

Reviewed exact commit `96bd06e5033929a32a9586e7ecf43b1cf28f846f` in a fresh detached clone at `[external evidence path omitted] The clone is clean at completion (`git status --porcelain` empty; `git diff --exit-code` passes). Restored Go-source fingerprint from the stage runner: `7a42b001433710b84e9503187bc9e75f2d400874ce6307c3b79fc4f017153812`.

The exact head changes the public decoder's preflight-error branch to return `ErrRecoveryEnvelopeContext` errors without recategorizing them as noncanonical. Its regression test calls `DecodeRecoveryEnvelopeV1` with cancellation during scanning and asserts both `errors.Is(err, ErrRecoveryEnvelopeContext)` and `errors.Is(err, context.Canceled)`, with zero value/hash outputs. The preflight remains before typed wire decoding and retains the closed path schema, required fields, duplicate/unknown-key rejection, union-arm checks, per-field string limits, array caps, integer ranges, nesting and total-byte limits, and context checks through traversal. This closes the two earlier findings at eb10eda (preflight bounds before typed decode) and 42ab661 (public cancellation classification).

I independently ran two short mutation controls, each through the shared lease runner; every stage records `source_unchanged_during_stage: true` and `release_exit: 0`, with `RELEASED: R-build-lease` in its release log:

- Context-loss mutant: removed `errors.Is(err, ErrRecoveryEnvelopeContext)` from the decoder wrapper condition. `TestDecodeRecoveryEnvelopePreservesMidScanCancellation` failed because the returned error no longer matched either expected sentinel. Restored the committed line; the same focused test passed.
- Array-bound mutant: weakened `count >= limit` to `count > limit` in `preflightValue`. `TestRecoveryEnvelopePreflightBoundsBeforeTypedDecode` failed specifically on `over-limit eligible array` because preflight accepted the out-of-bound input. Restored the committed condition; the focused test passed.

Raw runner metadata, test stdout/stderr, and lease release records are retained under `context-loss-mutant/`, `context-loss-restored/`, `array-cap-mutant/`, and `array-cap-restored/`. This review's four focused commands used the repository's prescribed worker stage runner; the coordinator's separate full-module qualification is not claimed as independent evidence.

The remaining source boundary is explicit: this package validates and hashes inert data only. A valid envelope digest or `Verified*` data value cannot prove the truth of approval references, current writer/store identity, snapshot provenance, journal history, or provider facts, and cannot authorize effects.
