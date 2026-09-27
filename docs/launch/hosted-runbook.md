# Hosted Serenity operations candidate

This runbook accompanies draft PR #234. No production deployment has been
performed. Bootstrap, monitoring and restore reactivation remain release gates.

## Configuration and secrets

`/etc/serenity/hosted.json` configures loopback bind, absolute data/secrets paths,
HTTPS public origin, pinned embedding model/version, optional provider base URL,
sender, runtime/in-flight/account caps, billing enablement and Builder/Scale price
IDs. See `deploy/hosted/config.example.json`. The service rejects unsafe bind or
origin configuration. Production never enables `SERENITY_HOSTED_DEV`.

Required private files in `/etc/serenity/secrets`: `RESEND_API_KEY`,
`EMBEDDINGS_API_KEY`; when billing is enabled also `STRIPE_SECRET_KEY` and
`STRIPE_WEBHOOK_SECRET`. Files must be mode 0600 and owned by the service user;
the directory must be mode 0700. `UNCONFIGURED` placeholders are rejected.
Server sessions use opaque hashed tokens in SQLite, so there is no session-signing
key to rotate. Never put secret values in command arguments, receipts or logs.

## Deploy and readiness

First provision the reviewed CloudFormation stack, install Caddy/AWS CLI/Git and
mount the encrypted persistent volume at `/var/lib/serenity`. Automated fresh-host
bootstrap is still incomplete. Create the app DNS record through the foundation
workflow; verify Resend sender DNS and provider credentials independently.

On the instance, run the reviewed `deploy/hosted/deploy.sh VERSION ARCHIVE_SHA256
BACKUP_BUCKET` through SSM as root. It verifies the immutable archive checksum,
installs secrets and the service, validates Caddy, checks `/readyz`, runs an initial
off-host backup and enables the hourly backup timer. Billing stays disabled until
test-mode qualification and reviewed price IDs/credentials are installed.

`/healthz` confirms process liveness. `/readyz` checks control storage, writable
data and an actual bounded embedding request, cached for one minute. Caddy uses
readiness for upstream health. Do not substitute a liveness 200 for readiness.

### Caddy privilege drop (ADR 020)

Caddy runs as the `caddy` system user with only `CAP_NET_BIND_SERVICE`, keeps
its state under `/var/lib/caddy`, and exposes its admin API on the 0600 unix
socket `/run/caddy/admin.sock` instead of TCP `localhost:2019`. Behind
Cloudflare the Caddyfile trusts only Cloudflare's published ranges and takes the
client address from `CF-Connecting-IP`, so every per-IP limit keys on the real
client. `deploy/hosted/tests/test_units.py` pins the range list, its source URLs
and the fetch date; refresh the Caddyfile and the test together when Cloudflare
changes the list.

The first `deploy.sh` run after this change migrates the edge in one pass:
`bootstrap.sh` creates the user and directories and copies the root-era
certificate store `/root/.local/share/caddy` into `/var/lib/caddy/caddy` once
(never overwriting an existing copy), `deploy.sh` snapshots the previous unit and
Caddyfile to `/root/serenity-caddy-rollback/`, and because the running root
process has no unix socket the script restarts Caddy instead of reloading it
(a few seconds of edge downtime, once). Every later deploy reloads through the
socket with no downtime. Verify on the host, read-only, after that deploy:

```sh
ss -ltnp | grep 2019                         # expect no output
ps -o user= -C caddy                         # expect caddy
stat -c '%U:%G %a' /run/caddy/admin.sock     # expect caddy:caddy 600
sudo -u serenity curl -s --max-time 5 localhost:2019/config/ ; echo "exit=$?"   # expect a refused connection, non-zero exit
sudo -u serenity curl -s --max-time 5 --unix-socket /run/caddy/admin.sock http://localhost/config/ ; echo "exit=$?"   # expect permission denied, non-zero exit
systemctl reload caddy && systemctl is-active caddy   # expect active
journalctl -u caddy --since '-30 min' | grep -Ei 'obtain|certificate obtained|issu' # expect no new issuance
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8090/readyz   # expect 200
```

## Backup and restore

`serenity hosted backup --data-dir /var/lib/serenity --snapshot EMPTY_DEST`
coordinates with the running service over its mode-0600 Unix socket, stops new
memory mutations for the snapshot, flushes writers, copies SQLite and bundles
canonical Git histories. The timer uploads a completion marker last; never restore
an incomplete prefix. The S3 bucket is encrypted and versioned. Its lifecycle
configuration alone has not yet proven the planned 30-day deletion guarantee.

Download one completed snapshot into an isolated host, then run:
`serenity hosted restore --snapshot SNAPSHOT_DIR --data-dir EMPTY_DATA_DIR`.
Restore refuses a nonempty destination. All restored sessions and client tokens
are invalidated; accounts and subscriptions stay `restore_pending`. A reconciled
reactivation command is still required before production recovery is qualified.
Never manually flip those accounts active without reconciling post-snapshot
revocations, deletions and Stripe state.

## Rotation and rollback

Users rotate or revoke client credentials in the dashboard; every MCP request
re-verifies credentials, including existing sessions. Replace provider secret
files through Secrets Manager and restart the service after validating readiness.

Before binary rollback, create and retain a coordinated snapshot and stop the
service. Preserve the failed data directory. Point `/usr/local/bin/serenity` at
the previous checksum-verified version under `/usr/local/lib/serenity`, then start
and check readiness plus authenticated recall. Database downgrade compatibility
must be proven for that pair of versions; if it is not, use the isolated restore
procedure and reconciliation rather than opening newer data with older code.
No version pair has yet completed the required production rollback rehearsal.

To roll back the Caddy privilege drop alone, reinstall the snapshot taken by the
first migrated deploy: copy `/root/serenity-caddy-rollback/caddy.service` to
`/etc/systemd/system/caddy.service` and its `Caddyfile` to `/etc/caddy/Caddyfile`,
then `systemctl daemon-reload && systemctl restart caddy`. The root-era
certificate store under `/root/.local/share/caddy` is untouched by the migration
(it is copied, not moved), so the rolled-back unit serves the same certificate.

## First response

- Capacity/429: honor Retry-After, inspect runtime load and per-account traffic;
  do not increase caps before measuring memory and provider cost.
- Provider unavailable: verify readiness and provider status/credentials; retain
  canonical storage and do not change the pinned embedding version as a shortcut.
- Backup failure: inspect the systemd backup job and S3 completion marker, repair
  access or disk pressure, rerun the job and verify the uploaded snapshot.
- Disk above 80%, backup age above three hours, readiness failure and elevated
  application errors require alarms. Only EC2 health/CPU alarms exist currently;
  application/disk/backup telemetry and alarms are still an implementation gate.
