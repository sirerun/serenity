#!/usr/bin/env bash
# Restore the pre-cutover application configuration; revert DNS through foundation.
set -euo pipefail
[[ $(id -u) == 0 ]] || { echo 'Run through SSM as root' >&2; exit 1; }
backup=/root/serenity-domain-rollback
for name in hosted.json Caddyfile binary-path; do
    [[ -f "$backup/$name" ]] || { echo 'No complete domain rollback snapshot' >&2; exit 1; }
done
binary=$(cat "$backup/binary-path")
[[ -x "$binary" ]] || { echo 'Previous binary is unavailable' >&2; exit 1; }
caddy validate --config "$backup/Caddyfile" --adapter caddyfile
cp -p "$backup/hosted.json" /etc/serenity/hosted.json
cp -p "$backup/Caddyfile" /etc/caddy/Caddyfile
ln -sfn "$binary" /usr/local/bin/serenity
systemctl restart serenity-hosted
systemctl reload caddy
curl --fail --retry 5 --retry-connrefused --max-time 10 http://127.0.0.1:8090/readyz
echo 'Previous app configuration restored. Revert the website DNS through foundation IaC.'
