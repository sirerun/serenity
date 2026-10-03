# Independent recovery-envelope decoder re-review

Verdict: HOLD at exact commit `42ab661a0341ccfe4ee1b13924ba9f973ecfa38a` for public decode cancellation propagation.

Reviewed the exact candidate in a fresh detached SSD clone. This is a static exact-head review; no source changes or Go/build commands were made.

## Closed previous bounds finding

The new `preflightEnvelopeJSON` runs before typed `json.Decoder.Decode(&wire)`. It has fixed path schemas and required-key sets; rejects unknown/duplicate keys; enforces tagged-union arm presence and refuses `null`; bounds per-path strings, arrays, object fields, integer syntax/ranges/int conversion, nesting and overall input; checks context during token/array/object traversal; and checks for trailing input. This prevents an over-limit account/disposition array or unknown-member map from being allocated as an unbounded typed slice/map. The frozen wire format and existing absent-optional-arm encoding remain unchanged. New preflight tests cover array/string/key/integer/type/union bounds and direct context cancellation.

## Blocking finding: cancellation is misclassified by the public decoder

`preflightEnvelopeJSON` returns the joined `ErrRecoveryEnvelopeContext` and underlying `ctx.Err()` when canceled during its scan. `DecodeRecoveryEnvelopeV1` then wraps every preflight error other than `ErrRecoveryEnvelopeTooLarge` using `%w` for `ErrRecoveryEnvelopeNonCanonical` and `%v` for the original error. A cancellation after Decode's initial context check is therefore returned as noncanonical input; `errors.Is` cannot find either `ErrRecoveryEnvelopeContext` or `context.Canceled`. The new cancellation regression calls `preflightEnvelopeJSON` directly, so it does not exercise the public API. This violates the frozen API contract that mid-operation cancellation returns the context sentinel and underlying cancellation and the zero value.

Fix the preflight error dispatch to preserve context errors and their `errors.Is` identities, without labeling cancellation as malformed input. Add a public `DecodeRecoveryEnvelopeV1` regression using the existing cancellation-after-checks context; assert `errors.Is(err, ErrRecoveryEnvelopeContext)`, `errors.Is(err, context.Canceled)`, and zero envelope/hash results. Then rerun the direct preflight check and focused decoder controls.

## Remaining scope

No other static blocker found in the revised preflight path or untouched envelope logic. This component remains an inert canonical codec; it does not establish source inspection, evidence authenticity, journal ancestry, pin, approval, provider, READY, restore, startup or admission authority. The author receipt's Go checks and behavioral mutant results are not independent checks by this review.
