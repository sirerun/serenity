# Unix admin transport independent security review

Reviewed exact source commit `e0e705db4bbbca08e52092f7a227fa035ff5d6e7` in an isolated full clone. Scope is only `internal/hosted/admintransport/**`; no production source was changed.

## Result

No blocking transport-authentication or socket-lifecycle defect found in this bounded review. The package validates an absolute clean path, requires an existing non-symlink owner-private directory, refuses any existing socket path, chmods the new socket to `0600`, and removes the path at close only if it still names the inode captured at creation. Wrong or unavailable peer credentials are closed before `Accept` returns. The HTTP wrapper supplies identity only for the package-private accepted-connection wrapper with the verified owner UID; it rejects requests without that context, so request headers and direct/public-handler calls cannot assert identity. Typed-nil `http.Handler` values are rejected by the nilable-kind check.

The credential implementations are platform-specific: Darwin obtains `LOCAL_PEERCRED` xucred; Linux obtains `SO_PEERCRED`; every other target fails closed. This review's runtime was Darwin. Linux ARM64 compile-only evidence exists in the coordinator's bounded control artifacts; it is not Linux runtime proof.

## Focused evidence and limits

My first `go test -race ./internal/hosted/admintransport` attempt used a temporary path long enough to exceed Darwin's Unix-domain socket path limit and failed at bind. Repeating with a short temporary root on the external SSD passed: `ok github.com/sirerun/serenity/internal/hosted/admintransport 1.230s`. The source was unchanged. The coordinator's same-source controls also report current race, vet, lint, and Linux ARM64 compile success; their three mutation controls fail the intended peer-rejection, forged-public-request, and replacement-preservation regressions.

This component authenticates an OS UID, not a human operator or an approval decision. Any process running as the service's effective UID has the same peer identity. The code comment states that boundary. It also does not mount or launch the handler: a caller must serve only through the returned listener and explicitly shut down the HTTP server and listener. This checkout has no production `AdminHandler` mount. Therefore this receipt clears only the transport package; it does not establish an operator authorization policy, production reachability, recovery authorization, or T23.44 acceptance.

Validation was single-package only. No multi-package build or provider operation was run.
