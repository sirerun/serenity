# E24 live unit hardening — 2026-10-01

Source: reviewed units from main `7b0baad` / PR310. Existing candidate binary
`0.1.13-hosted-partner-candidate` and infrastructure are unchanged.

The coordinator held `R-serenity-hosted-deploy` during maintenance. Before
installation, exact SHA256 checks confirmed that both live units matched the
reviewed pre-hardening versions. Existing units were retained in a root-only
snapshot. The activation script had an error trap to restore old units and
restart the app if activation/readiness failed. No rollback was needed.

SSM activation `45823366-6a99-4b5f-9324-83a88dfa1a9e` succeeded:

- App is active with `IPAddressDeny=169.254.169.254/32`.
- Backup has `OnFailure=serenity-backup-failed.service`; its IMDS deny is empty.
- `/readyz` passed on the local listener, and Caddy remained active.

SSM verification `e2ac0820-9571-4b54-9496-a8329396c6c3` succeeded:

- `systemd-analyze verify` accepted the three installed units (an unrelated
  legacy acpid path warning was emitted).
- A root IMDSv2 token request returned HTTP200; output/token body was discarded.
- A transient service running as `serenity` with the same IMDS deny property
  timed out with curl exit28. This verifies host support/enforcement for the
  cgroup rule; it is not a network trace of the application itself.
- App readiness remained successful. No BPF/IP firewall warnings appeared in
  the scoped app journal query.
- The transient failed probe was reset; deployment lease was released.

The backup failure handler is installed and selected by the backup unit.
No destructive live backup failure or failed-release rollback was induced.
The existing readiness/rollback tests establish those code paths locally;
this receipt does not claim a live failed-release rehearsal or a new binary
rollout. The source-history erasure changes are not yet in the running binary.
