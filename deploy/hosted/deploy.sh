#!/usr/bin/env bash
# Run via SSM on the reviewed hosted instance. Never prints secret values.
set -euo pipefail
version=${1:?usage: deploy.sh VERSION ARCHIVE_SHA256}
checksum=${2:?usage: deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET}
backup_bucket=${3:?usage: deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET}
[[ "$backup_bucket" =~ ^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$ ]] || { echo "Invalid backup bucket" >&2; exit 1; }
[[ "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([.-][a-zA-Z0-9.-]+)?$ ]] || { echo 'Invalid version' >&2; exit 1; }
[[ "$checksum" =~ ^[a-f0-9]{64}$ ]] || { echo 'Invalid SHA256' >&2; exit 1; }
[[ $(id -u) == 0 ]] || { echo 'Run through SSM as root' >&2; exit 1; }
command -v caddy >/dev/null
command -v aws >/dev/null
command -v git >/dev/null
mountpoint -q /var/lib/serenity || { echo 'Persistent data volume is not mounted' >&2; exit 1; }
script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
number=${version#v}
curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 "https://github.com/sirerun/serenity/releases/download/v${number}/serenity_${number}_linux_arm64.tar.gz" --output "$work/release.tar.gz"
printf '%s  %s\n' "$checksum" "$work/release.tar.gz" | sha256sum --check --status
tar -xzf "$work/release.tar.gz" -C "$work" serenity
id serenity >/dev/null 2>&1 || useradd --system --home-dir /var/lib/serenity --shell /sbin/nologin serenity
install -d -m 0700 -o serenity -g serenity /etc/serenity /etc/serenity/secrets
for name in RESEND_API_KEY EMBEDDINGS_API_KEY; do
    aws --region us-west-2 secretsmanager get-secret-value --secret-id "serenity/hosted/$name" --query SecretString --output text > "$work/$name"
    [[ -s "$work/$name" ]] && ! grep -qx UNCONFIGURED "$work/$name" || { echo "Required secret $name is unconfigured" >&2; exit 1; }
    install -m 0600 -o serenity -g serenity "$work/$name" "/etc/serenity/secrets/$name"
done
# Billing is enabled separately after test-mode qualification; its secrets are
# installed through the same path only when the reviewed config enables it.
if [[ ! -e /etc/serenity/hosted.json ]]; then
    install -m 0600 -o serenity -g serenity "$script_dir/config.example.json" /etc/serenity/hosted.json
fi
install -d -m 0755 /usr/local/lib/serenity
install -m 0755 "$work/serenity" "/usr/local/lib/serenity/serenity-${number}"
ln -sfn "/usr/local/lib/serenity/serenity-${number}" /usr/local/bin/serenity
chown serenity:serenity /var/lib/serenity
install -m 0644 "$script_dir/serenity-hosted.service" /etc/systemd/system/serenity-hosted.service
install -m 0644 "$script_dir/Caddyfile" /etc/caddy/Caddyfile
caddy validate --config /etc/caddy/Caddyfile
install -d -m 0755 /opt/serenity-hosted
install -m 0755 "$script_dir/backup.sh" /opt/serenity-hosted/backup.sh
printf 'SERENITY_BACKUP_BUCKET=%s\n' "$backup_bucket" > "$work/backup.env"
install -m 0600 -o serenity -g serenity "$work/backup.env" /etc/serenity/backup.env
install -m 0644 "$script_dir/serenity-backup.service" /etc/systemd/system/serenity-backup.service
install -m 0644 "$script_dir/serenity-backup.timer" /etc/systemd/system/serenity-backup.timer
systemctl daemon-reload
systemctl enable --now serenity-hosted
systemctl restart serenity-hosted
systemctl reload caddy
curl --fail --max-time 10 http://127.0.0.1:8090/readyz
systemctl start serenity-backup.service
systemctl enable --now serenity-backup.timer
echo "Hosted Serenity ${number} is locally ready. Public smoke and release acceptance are separate."
