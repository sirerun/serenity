# Signed operator-case admission correction receipt

Correction source commit: `4cec36e6e517be0a7449f3cea8e19ba6f374d965`, atop implementation `e55563bf1f62443bf276cb933dfd763d5d935d2d` and receipt `c6551bf40e4ca801d05af479d1e4ec97cfdf2262`.

The coordinator review identified three admission-edge cases. `operatorreview.New` now takes `context.Context` first, refuses nil or pre-canceled contexts before dependency checks or filesystem access, and passes the caller context to `privatefs.ValidateDirectory`. `Authorize` validates the exact `ed25519-sha256:` plus 64 lowercase-hex key ID before calling `TrustedKeySource.Lookup`. Age and lifetime checks now compare against `now.Add(-MaxCaseAge)` and `approvedAt.Add(MaxCaseLifetime)` rather than subtracting timestamps into a saturating `time.Duration`; zero clock and case times are refused explicitly.

`privatefs.ReadFile` now propagates close failure as a fixed sanitized error and discards the bytes in that case. Its Darwin mount control now requires an explicitly configured `SERENITY_PRIVATEFS_UNOWNED_TEST_PATH`, checks `Statfs` for `MNT_IGNORE_OWNERSHIP`, and skips with an explicit reason if the path is unset or not on an ownership-disabled mount. This prevents a missing path or generic statfs error from satisfying the test.

Focused final Darwin race runs passed separately on the ownership-enabled APFS fixture:

- `go test -race ... ./internal/hosted/operatorreview -count=1` — pass (1.424s).
- `go test -race ... ./internal/hosted/privatefs -count=1` — pass (1.178s), including the actual ownership-disabled BuildOffload mount control; it did not skip.
- Focused `go vet` and `golangci-lint` passed for `operatorreview`, `privatefs`, and `admintransport`. The first focused linter run found two QF1001 expressions in hex validation; the predicate was consolidated and all three final lint runs reported zero issues.

Runtime mutation controls failed as intended; the mutations were restored and the final green runs used the restored source:

- `context-new-background-red.log`: reverting `New` to background filesystem validation returned `no such file or directory` instead of `context.Canceled` for a canceled context and nonexistent path.
- `keyid-prelookup-red.log`: replacing the exact key-ID check with generic opaque-text validation let a malformed key ID reach the trust source (`calls=1`; expected zero).
- `duration-saturation-red.log`: restoring timestamp subtraction admitted the centuries-old signed case when both limits were `math.MaxInt64`.
- `unowned-mount-red.log`: bypassing Darwin's mount-flag check accepted the path whose `Statfs` confirmed `MNT_IGNORE_OWNERSHIP`.

These logs and their SHA-256 digests are in the external validation directory `serenity-signed-case-validation-20261001/`. An earlier duration-control attempt appeared green because its fixture used Go's zero `time.Time` as `NotBefore`, triggering the independent zero-validity rejection. The fixture was corrected to a nonzero year-0001 timestamp before collecting the runtime red; the nonqualifying attempt is preserved in `nonqualifying-duration-fixture-note.txt`.

The corrected `admission.go` hash is `63d089636d0fa2133fef656bc368a3804171120c5f6cc2d58f10297199b54015`; the corrected `privatefs.go` hash is `c62c390344f39c1a2f99e0f85869654fa8d2f6c966983dba4ad89889d0ae4bc1`. No signer, production key source, factory, service wiring, provider, CLI, real case, or activation was added. Linux ARM64 compile and combined repository gates remain coordinator work; this receipt makes no production admission or human-authentication claim.

The fresh canonical correction claims were R-hosted-operatorreview (`b55902f31ad41f123801a9e4ab816ca6631b2b85`) and R-hosted-privatefs (`aea3b51bbd6111524dc768c6e8c42b832617982b`). They are released after this source and receipt are banked.
