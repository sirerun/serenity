# Independent review: combined operator recovery assembly

Reviewed source: `8fdd32ffdcee5fa069625d33207f0b36a914b9a5` (`hosted/operator-recovery-assembly-20261001`), read-only.

Static review found no integration blocker in the combined Service/Admin transport test. `Service.Handler` does not expose `/operations/resolve`; the route is supplied by `Service.AdminHandler`, and the test mounts that handler only through `admintransport.Listener.HTTPServer`. The transport wrapper checks the peer-derived UID before dispatch. The test's admission fixture then separately requires that authenticated UID and the exact operation/review reference, so forged headers on the public handler and direct wrapper handler do not authorize resolution.

The positive path builds a canonical `fact:` reference using the real writer and pool, puts the real ledger row into `pending_review`, and resolves it through an actual Unix HTTP request. It checks the original quota-period counters before and after same-case replay, and confirms the final committed evidence. The fixture's in-memory approval is explicitly test-only; it does not establish a production human-approval source or activate a listener.

The final `8fdd32f` change canonicalizes the task-owned temporary directory with `filepath.EvalSymlinks` before passing its socket path to the transport. Cleanup closes the server/listener before removing that directory. This addresses the Darwin `/var` alias issue without bypassing the transport's exact-path/ancestor checks. The test still performs real socket binding and HTTP dispatch; it does not replace either with a mock.

No Go command was run for this review because the coordinator's full race gate was active. The prior focused integration check was on `8d40c362dbfb89e39679b0674c38951fbee0d479`, before the final path canonicalization; therefore the exact `8fdd32f` combined fixture is statically clear but not independently runtime-qualified here. The combined full gate was recorded as pending in the source tree. Unix peer behavior was exercised on Darwin; Linux runtime and production approval-adapter behavior remain outside this review.
