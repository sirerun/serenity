# Deep review 001: remediation status

Status of remediation work against `docs/deep-reviews/001-full-codebase.md`,
tracked by the plan `docs/plans/deep-review-001-remediation.md`.

## T24.29: swallowed-error (`_ =`) audit

Finding: code quality row of the maturity table ("79 swallowed `_ =`
sites"), FUN-05 local half. The hosted half of FUN-05 (billing Scan errors)
is T23.47, verified by T24.34.

### Count reconciliation

The review's 79 is the repository-wide count of non-test lines matching
`^\s*_ = `. The contract's verification command scopes to
`internal cmd pkg` and finds 66; the other 13 live under `evals/` (10) and
`testdata/conformance/` (3). All 79 are listed below. Line numbers are as of
commit 2bf0d98 (the base of this audit); the two fixed sites no longer match
the pattern.

```sh
grep -rn --include='*.go' -E '^\s*_ = ' . | grep -v _test.go          # 79 at 2bf0d98
grep -rn --include='*.go' -E '^\s*_ = ' internal cmd pkg | grep -v _test.go  # 66 at 2bf0d98
```

`_, _ =` multi-value discards (for example `fmt.Fprintf` to CLI output) do
not match the pattern and are not part of the review's count.

### Summary

| Disposition | Sites |
|---|---|
| fixed (material, with a test) | 2 |
| hosted (report only; owned by a T23 claim) | 18, of which 3 material (FUN-05, T23.47) |
| deferred | 1 |
| benign-documented | 58 |

Categories: **material** means an error that changes an outcome a caller or
user relies on is lost; **minor** means an error is lost but only degrades
diagnostics or a best-effort side effect; **harmless** means the discarded
error is on a cleanup path after the primary error has already been decided
or returned, or has no observer.

### Sites

Fixed sites link to the PR that fixed them: [#295](https://github.com/sirerun/serenity/pull/295) (T24.29,
branch `t24-29-swallowed-errors`).

| # | Site (at 2bf0d98) | Expression | Category | Disposition | Reason |
|---|---|---|---|---|---|
| 1 | `evals/hosted-load/fixtureprep/verify.go:268` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 2 | `evals/hosted-load/fixtureprep/verify.go:273` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 3 | `evals/hosted-load/fixtureprep/verify.go:336` | `_ = brows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 4 | `evals/hosted-load/fixtureprep/verify.go:355` | `_ = brows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 5 | `evals/hosted-load/fixtureprep/verify.go:398` | `_ = srows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 6 | `evals/hosted-load/fixtureprep/verify.go:403` | `_ = srows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 7 | `evals/hosted-load/fixtureprep/verify.go:445` | `_ = crows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 8 | `evals/hosted-load/fixtureprep/verify.go:450` | `_ = crows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 9 | `evals/hosted-load/fixtureprep/verify.go:679` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 10 | `evals/hosted-load/fixtureprep/verify.go:718` | `_ = vrows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 11 | `internal/cli/check.go:279` | `_ = enc.Encode(check.ToWire(result, matched, confidence, haveConfidence))` | material | fixed | `serenity check --json` exited 0 with no output when stdout could not be written; now returns the write error. Test: `TestCheckJSONWriteFailureIsAnError`. Fixed in [#295](https://github.com/sirerun/serenity/pull/295). |
| 12 | `internal/cli/hosted.go:63` | `_ = adminListener.Close()` | harmless | benign-documented | Closing the admin listener on a Chmod failure; the Chmod error is returned. |
| 13 | `internal/cli/serve.go:318` | `_ = unix.Close(fd)` | harmless | benign-documented | Closing a duplicated fd on an error path; the SetNonblock error is returned. |
| 14 | `internal/connector/file/file.go:89` | `_ = w.Close()` | harmless | benign-documented | Closing the watcher after the initial tree watch failed; that error is returned. |
| 15 | `internal/connector/file/file.go:373` | `_ = c.addTreeWatches(ev.Name) // best-effort: watch the new subtree too` | material | fixed | A new subdirectory that could not be watched (e.g. inotify watch limit) was silently never ingested; the error is now recorded in `watchErr` and returned by the next Poll (a vanished subtree is ignored). Test: `TestWatchNewSubdirFailureSurfacesFromPoll`. Fixed in [#295](https://github.com/sirerun/serenity/pull/295). |
| 16 | `internal/connector/imap/connector.go:91` | `_ = client.Close()` | harmless | benign-documented | IMAP connection teardown after login failure or in a deferred cleanup; the session result is already decided. |
| 17 | `internal/connector/imap/connector.go:131` | `_ = client.Logout().Wait()` | harmless | benign-documented | IMAP connection teardown after login failure or in a deferred cleanup; the session result is already decided. |
| 18 | `internal/connector/imap/connector.go:132` | `_ = client.Close()` | harmless | benign-documented | IMAP connection teardown after login failure or in a deferred cleanup; the session result is already decided. |
| 19 | `internal/direction/publication.go:315` | `_ = entries.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 20 | `internal/direction/publication_lock_unix.go:22` | `_ = file.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 21 | `internal/eval/runner/checkpoint.go:158` | `_ = f.Close() // best-effort; the write error above is what's reported` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 22 | `internal/eval/runner/runner.go:453` | `_ = ckpt.Close() // best-effort cleanup on an early-return path; nothing left to report...` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 23 | `internal/hosted/backup/backup.go:67` | `_ = rows.Close()` | harmless | hosted (T23.49/T23.52) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 24 | `internal/hosted/backup/backup.go:71` | `_ = rows.Close()` | harmless | hosted (T23.49/T23.52) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 25 | `internal/hosted/backup/backup.go:77` | `_ = rows.Close()` | harmless | hosted (T23.49/T23.52) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 26 | `internal/hosted/billing/billing.go:316` | `_ = rows.Close()` | harmless | hosted (T23.47) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 27 | `internal/hosted/billing/billing.go:330` | `_ = tx.QueryRowContext(ctx, `SELECT status,COALESCE(grace_until,''),COALESCE(grace_invo...` | material | hosted (T23.47) | FUN-05: failed Scan feeds blank prior state into grace/window accounting. Owned by T23.47; verified by T24.34. |
| 28 | `internal/hosted/billing/billing.go:388` | `_ = s.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(grace_until,'') FROM subscriptio...` | material | hosted (T23.47) | FUN-05: failed Scan feeds blank prior state into grace/window accounting. Owned by T23.47; verified by T24.34. |
| 29 | `internal/hosted/billing/billing.go:1047` | `_ = tx.QueryRowContext(ctx, `SELECT status,COALESCE(grace_until,''),COALESCE(grace_invo...` | material | hosted (T23.47) | FUN-05: failed Scan feeds blank prior state into grace/window accounting. Owned by T23.47; verified by T24.34. |
| 30 | `internal/hosted/billing/billing.go:1214` | `_ = rows.Close()` | harmless | hosted (T23.47) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 31 | `internal/hosted/contracts/contractstest/ledger_suite.go:432` | `_ = leave // the process died before Finalize; the restarted process has a fresh fence` | harmless | hosted (T23.41) | Not an error: discards an unused variable in a contract test suite (documented inline). |
| 32 | `internal/hosted/gateway/gateway.go:202` | `_ = g.record(r.Context(), binding, "connected")` | minor | hosted (T23.44/T23.45) | Best-effort "connected" status record after a successful tools/list; losing it only delays the dashboard status. Reported to the gateway claim. |
| 33 | `internal/hosted/gateway/lifecycle.go:130` | `_ = rows.Close()` | harmless | hosted (T23.48) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 34 | `internal/hosted/gateway/lifecycle.go:172` | `_ = rows.Close()` | harmless | hosted (T23.48) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 35 | `internal/hosted/gateway/lifecycle.go:178` | `_ = rows.Close()` | harmless | hosted (T23.48) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 36 | `internal/hosted/oauth/connections.go:40` | `_ = rows.Close()` | harmless | hosted (no T23 claim names oauth) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 37 | `internal/hosted/oauth/connections.go:60` | `_ = connectionsPage.Execute(w, struct {` | harmless | hosted (no T23 claim names oauth) | Response body write (JSON or HTML template) after headers are sent; the client is the only consumer and the status is already committed. |
| 38 | `internal/hosted/oauth/hosted.go:125` | `_ = consentPage.Execute(w, struct {` | harmless | hosted (no T23 claim names oauth) | Response body write (JSON or HTML template) after headers are sent; the client is the only consumer and the status is already committed. |
| 39 | `internal/hosted/oauth/ratelimit.go:92` | `_ = json.NewEncoder(w).Encode(v)` | harmless | hosted (no T23 claim names oauth) | Response body write (JSON or HTML template) after headers are sent; the client is the only consumer and the status is already committed. |
| 40 | `internal/hosted/provision/provision.go:147` | `_ = rows.Close()` | harmless | hosted (T23.46) | `rows.Close` on an early-return path; the returned error is the one reported. |
| 41 | `internal/import/gbrain/checkpoint.go:53` | `_ = root.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 42 | `internal/import/gbrain/checkpoint.go:58` | `_ = root.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 43 | `internal/import/gbrain/checkpoint.go:62` | `_ = root.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 44 | `internal/import/gbrain/checkpoint.go:66` | `_ = root.Close()` | harmless | benign-documented | Closing the parent `os.Root` after descending into its child; a directory handle close has no data to lose, and the OpenRoot error is checked on the next line. |
| 45 | `internal/import/gbrain/checkpoint.go:86` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 46 | `internal/import/gbrain/checkpoint.go:92` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 47 | `internal/import/gbrain/checkpoint.go:98` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 48 | `internal/import/gbrain/checkpoint.go:102` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 49 | `internal/import/gbrain/checkpoint.go:153` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 50 | `internal/import/gbrain/checkpoint.go:157` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 51 | `internal/index/rebuild.go:340` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 52 | `internal/index/rebuild.go:346` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 53 | `internal/index/report.go:34` | `_ = db.Close()` | harmless | benign-documented | Closing a DB handle after Ping/migrate failed; that failure is returned. |
| 54 | `internal/index/revisit.go:90` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 55 | `internal/index/sqlite.go:134` | `_ = db.Close()` | harmless | benign-documented | Closing a DB handle after Ping/migrate failed; that failure is returned. |
| 56 | `internal/index/sqlite.go:326` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 57 | `internal/index/sqlite.go:334` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 58 | `internal/index/sqlite.go:339` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 59 | `internal/index/vectors.go:110` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 60 | `internal/index/vectors.go:115` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 61 | `internal/index/vectors.go:120` | `_ = rows.Close()` | harmless | benign-documented | Read-only handle closed on an early-return (or read-complete) path; the returned error is the one reported. |
| 62 | `internal/server/direction/direction.go:204` | `_ = json.NewEncoder(w).Encode(v)` | harmless | benign-documented | Response body write after `WriteHeader`; the status is already committed and the client is the only party that could observe the error. |
| 63 | `internal/server/disposition/disposition.go:147` | `_ = json.NewEncoder(w).Encode(v)` | harmless | benign-documented | Response body write after `WriteHeader`; the status is already committed and the client is the only party that could observe the error. |
| 64 | `internal/server/mcp/server.go:262` | `_ = input.Close()` | harmless | benign-documented | Closing stdio on ctx cancel to unblock the reader; the Serve result already carries ctx.Err(). |
| 65 | `internal/server/mcp/server.go:264` | `_ = closer.Close()` | harmless | benign-documented | Closing stdio on ctx cancel to unblock the reader; the Serve result already carries ctx.Err(). |
| 66 | `internal/server/server.go:272` | `_ = s.httpSrv.Shutdown(shutdownCtx)` | minor | deferred | Graceful `Shutdown` error on ctx cancel. The only caller (`internal/cli/serve.go`) already treats `context.DeadlineExceeded`, the only error Shutdown returns here, as a clean stop, so propagating it changes nothing without a caller-contract change outside this minimal fix; left for a follow-up that also force-closes lingering connections. |
| 67 | `internal/store/shard.go:576` | `_ = tf.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 68 | `internal/store/shard.go:577` | `_ = os.Remove(tmp)` | harmless | benign-documented | Removing a temp file/dir on an error or teardown path; the original error (or a completed run) is what matters. |
| 69 | `internal/store/shard.go:597` | `_ = os.Remove(tmp)` | harmless | benign-documented | Removing a temp file/dir on an error or teardown path; the original error (or a completed run) is what matters. |
| 70 | `internal/supersede/publication_lock_unix.go:22` | `_ = file.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 71 | `internal/writer/derived.go:55` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 72 | `internal/writer/derived.go:59` | `_ = f.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 73 | `internal/writer/import.go:66` | `_ = tmp.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 74 | `internal/writer/import.go:70` | `_ = tmp.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 75 | `internal/writer/ownership_unix.go:45` | `_ = file.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 76 | `internal/writer/ownership_unix.go:49` | `_ = file.Close()` | harmless | benign-documented | Closing a file/lock/root handle on an error path; the primary error is returned. Success paths check `Close` explicitly. |
| 77 | `testdata/conformance/direction/gen_transcripts.go:104` | `_ = s.Serve(ctx)` | harmless | benign-documented | Conformance transcript generator (dev tool, `go run`); the harness waits on `done` and fatals on a hang. |
| 78 | `testdata/conformance/direction/gen_transcripts.go:122` | `_ = os.RemoveAll(e.root)` | harmless | benign-documented | Removing a temp file/dir on an error or teardown path; the original error (or a completed run) is what matters. |
| 79 | `testdata/conformance/disposition/gen_transcripts.go:96` | `_ = s.Serve(ctx)` | harmless | benign-documented | Conformance transcript generator (dev tool, `go run`); the harness waits on `done` and fatals on a hang. |

## 2026-10-01 current-tree error-discard reconciliation

At main `7b0baad`, the original table remains historical evidence pinned to
`2bf0d98`. The current scoped count is 70 (`internal cmd pkg`), versus 66 at
that baseline: six added assignments and two removed. Repository-wide count
is 83 versus 79. New sites and dispositions are below; this is not a claim
that hosted tasks or every material site are fixed.

| Added site | Disposition | Reason / follow-up |
|---|---|---|
| `internal/cli/gitx.go:132` | benign cleanup | Temporary close after the primary error is already selected. |
| `internal/cli/gitx.go:136` | benign cleanup | Same cleanup after a failed hook write. |
| `internal/hosted/partner/consent.go:72` | minor response-write diagnostics | Response may already be committed; preserve response policy and improve diagnostics separately. |
| `internal/hosted/partner/partner.go:161` | fixed follow-up in continuation | The transaction error now produces a fixed, sanitized diagnostic while preserving mutation response semantics. Regression verifies the response and absence of sensitive details. |
| `internal/hosted/partner/partner.go:208` | minor response-write diagnostics | Encoder error after response commit; keep response semantics. |
| `internal/writer/history.go:106` | minor cleanup; open | Deferred worktree removal is ignored; main success path checks removal. Failure can leave a temporary worktree requiring operator cleanup. |

Removed sites: the ignored encoder result in `internal/cli/check.go` and
best-effort watch assignment in `internal/connector/file/file.go`.
The three material hosted billing Scan discards remain assigned to T23.47;
this coordinator has not overwritten that held lane.

Current task/readiness audits are recorded in
`docs/plans/e24-local-audit-2026-10-01.md` and
`docs/plans/e24-router-hosted-audit-2026-10-01.md`.
