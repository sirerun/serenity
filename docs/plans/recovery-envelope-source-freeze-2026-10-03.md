# Recovery envelope source freeze

Contract ID: `recovery-envelope-v1`. Author proposal `8e2d5dbd1c4ae90c64697edd8853c730258ac0c6`, document SHA-256 `424a1dfa2159f0e591d830dd21949eedc903edf1858526c61c4add24d677ed75`, has independent CLEAR for this bounded pure component. Earlier HOLD findings remain historical evidence and are superseded only for this exact revision.

The root owns narrow source claim `R-recovery-envelope-codec` at `ae211bff8b61bcb55ef7d6a11732c26e6bdd21e4`; logical T23.50 remains open. Source ownership is only new `internal/hosted/recovery/envelope*.go` files and `docs/plans/recovery-envelope-implementation-receipt-2026-10-03.md`. Do not edit existing recovery, shared contracts, module files, CLI, service or other packages.

Implement the exact pure value types, ordered canonical wire, ASCII-plus-NUL domain separators, summary/full-projection inventory helpers, ELIGIBLE/FROZEN_ONLY union, strict decoder, bounds, context checks and deep copies. Preserve the existing RCP hash domain and 1 MiB limit. Journal H/C/G are distinct facts; no ancestry is inferred. Public values carry no authority. Missing owner proofs remain fail-closed integration dependencies.

This freeze does not clear the full recovery store, READY factory, journal ancestry authority, snapshot lifecycle owner, approvals, provider, startup, admission, physical quota or hosted acceptance. Source completion requires current-byte verification, independent exact-head review, normal merge and landed tree proof.
