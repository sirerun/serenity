# Renewed independent review: OAuth method-scope proposal

Verdict: **CLEAR for the method-scope proposal only** at author commit `6bfe7fcaa93ef4907b27693cd2c58d743bc84376` (tree `4e670b26fb11f879b3bd3de0c8e92751543c1d76`). The exact detached review clone is clean. This does not qualify a source implementation, alter the pinned dependency, settle the shared 5,000/minute policy, or establish live/hosted acceptance.

The correction resolves the earlier `594cc86586afb3152dd9a61d23a50086a3bd52d7` HOLD. The proposal now distinguishes the T24.3 contractual metering set from methods accepted by the pinned OAuth handler: `POST /oauth/authorize` is counted because T24.3 explicitly includes it, while the pinned handler still returns its existing 405. The regression contract now requires preserving that 405/body while counting the request. POST registration and GET authorization remain the other charged method/path pairs; methods outside the set skip only the shared counter and continue through existing route handling and any outer/per-route limits.

I verified the proposal against the exact source pin `c2d5a5439675fb7a964aadedb1a0a461f0fce3865` and its `go.mod` dependency `github.com/ajent-social/go v0.0.0-20260924042100-b90bbb417d9d`:

- `mcpoauth.AuthorizeHandler` accepts GET only; POST and HEAD get 405. A valid GET creates and persists a consent record before calling Serenity's consent callback, where session validation occurs. The shared guard must therefore charge GET before the callback/session outcome.
- `mcpoauth.RegisterHandler` accepts POST only; GET and HEAD return 405 before registration writes. Those methods are outside the shared meter set, while the existing per-prefix/register-specific limiters remain unchanged.
- Serenity currently uses methodless `mux.Handle` routes, then wraps authorize directly with the shared limiter and register with route-specific registration limiting outside the shared limiter. The proposed exact method checks and response-preservation rules fit this nesting.
- T24.3 acceptance criterion 2 explicitly includes POST authorization even though the dependency does not accept it. This review confirms that reconciliation and its limits. Merged PR #285's “any method” wording remains historical context; it does not override T24.3.

The earlier HOLD was specifically caused by calling POST authorization a “valid method pair” without documenting that the dependency rejects it. The corrected text removes that ambiguity and specifies the required behavior for POST authorization, valid GET authorization, and HEAD. The 5,000/minute capacity question, live 17-prefix acceptance, gateway admission, dependency changes, provider/deployment, and hosted acceptance remain open as the proposal states. No builds or source changes were made.

Original external report SHA-256: `19596ce3974e1c040347ca30f7cd24dd438cfd60a983087db3e9f49348350b4f`; local path locators redacted.
