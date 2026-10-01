#!/usr/bin/env bash
# Run via SSM on the reviewed hosted instance. Never prints secret values.
set -euo pipefail
version=${1:?usage: deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET cutover|final}
checksum=${2:?usage: deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET cutover|final}
backup_bucket=${3:?usage: deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET cutover|final}
mode=${4:?usage: deploy.sh VERSION ARCHIVE_SHA256 BACKUP_BUCKET cutover|final}
[[ "$mode" == cutover || "$mode" == final ]] || { echo 'Mode must be cutover or final' >&2; exit 1; }
export SERENITY_DOMAIN_CUTOVER=0
[[ "$mode" != cutover ]] || export SERENITY_DOMAIN_CUTOVER=1
[[ "$backup_bucket" =~ ^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$ ]] || { echo "Invalid backup bucket" >&2; exit 1; }
[[ "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([.-][a-zA-Z0-9.-]+)?$ ]] || { echo 'Invalid version' >&2; exit 1; }
[[ "$checksum" =~ ^[a-f0-9]{64}$ ]] || { echo 'Invalid SHA256' >&2; exit 1; }
[[ $(id -u) == 0 ]] || { echo 'Run through SSM as root' >&2; exit 1; }
script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
source "$script_dir/readiness.sh"
"$script_dir/bootstrap.sh"
command -v caddy >/dev/null
command -v aws >/dev/null
command -v git >/dev/null
command -v curl >/dev/null
command -v cosign >/dev/null || { echo 'Cosign is required to verify release signatures' >&2; exit 1; }
mountpoint -q /var/lib/serenity || { echo 'Persistent data volume is not mounted' >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
number=${version#v}
release_tag="v${number}"
release_url="https://github.com/sirerun/serenity/releases/download/${release_tag}"
curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 "$release_url/serenity_${number}_linux_arm64.tar.gz" --output "$work/release.tar.gz"
curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 "$release_url/serenity_${number}_linux_arm64.tar.gz.sigstore.json" --output "$work/release.tar.gz.sigstore.json"
"$script_dir/verify-release.sh" "$work/release.tar.gz" "$work/release.tar.gz.sigstore.json" "$release_tag"
printf '%s  %s\n' "$checksum" "$work/release.tar.gz" | sha256sum --check --status
tar -xzf "$work/release.tar.gz" -C "$work" serenity
id serenity >/dev/null 2>&1 || useradd --system --home-dir /var/lib/serenity --shell /sbin/nologin serenity
install -d -m 0700 -o serenity -g serenity /etc/serenity /etc/serenity/secrets
for name in RESEND_API_KEY EMBEDDINGS_API_KEY; do
    aws --region us-west-2 secretsmanager get-secret-value --secret-id "serenity/hosted/$name" --query SecretString --output text > "$work/$name"
    if [[ ! -s "$work/$name" ]] || grep -qx UNCONFIGURED "$work/$name"; then
        echo "Required secret $name is unconfigured" >&2
        exit 1
    fi
    install -m 0600 -o serenity -g serenity "$work/$name" "/etc/serenity/secrets/$name"
done
# Billing is enabled separately after test-mode qualification; its secrets are
# installed through the same path only when the reviewed config enables it.
if [[ ! -e /etc/serenity/hosted.json ]]; then
    install -m 0600 -o serenity -g serenity "$script_dir/config.example.json" /etc/serenity/hosted.json
fi

caddy validate --config "$script_dir/Caddyfile" --adapter caddyfile
# Keep one pre-cutover rollback snapshot; final activation must not replace it.
rollback=/root/serenity-domain-rollback
if [[ ! -d "$rollback" && -f /etc/caddy/Caddyfile && -L /usr/local/bin/serenity ]]; then
    snapshot=$(mktemp -d /root/serenity-domain-rollback.XXXXXX)
    cp -p /etc/serenity/hosted.json "$snapshot/hosted.json"
    cp -p /etc/caddy/Caddyfile "$snapshot/Caddyfile"
    readlink /usr/local/bin/serenity > "$snapshot/binary-path"
    mv "$snapshot" "$rollback"
fi
install -d -m 0755 /usr/local/lib/serenity
# Retain the exact currently selected binary so a failed readiness check can
# restore it even though the active path is a symlink to a versioned binary.
if [[ -x /usr/local/bin/serenity ]]; then
    previous_target=$(readlink -f -- /usr/local/bin/serenity)
    [[ -x "$previous_target" ]] && ln -sfn -- "$previous_target" /usr/local/bin/serenity.prev
fi
install -m 0755 "$work/serenity" "/usr/local/lib/serenity/serenity-${number}"
chown serenity:serenity /var/lib/serenity
install -m 0644 "$script_dir/serenity-hosted.service" /etc/systemd/system/serenity-hosted.service
# Migrate the old default origin and sender; preserve custom operator settings.
python3 - <<'PYCONFIG'
import json, os
path = "/etc/serenity/hosted.json"
with open(path) as f:
    config = json.load(f)
changed = False
if config.get("sender") == "login@serenity.sire.run":
    config["sender"] = "login@mail.sire.run"
    changed = True
if os.environ.get("SERENITY_DOMAIN_CUTOVER") != "1" and config.get("public_origin") == "https://app.serenity.sire.run":
    config["public_origin"] = "https://serenity.sire.run"
    changed = True
if changed:
    temporary = path + ".new"
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as f:
        json.dump(config, f, indent=2)
        f.write("\n")
    metadata = os.stat(path)
    os.chown(temporary, metadata.st_uid, metadata.st_gid)
    os.replace(temporary, path)
PYCONFIG
# Keep the previous Caddy unit and config once for rollback of the privilege
# drop (ADR 020); the snapshot is only taken when the unit actually changes.
if [[ -f /etc/systemd/system/caddy.service ]] && ! cmp -s "$script_dir/caddy.service" /etc/systemd/system/caddy.service; then
    install -d -m 0700 /root/serenity-caddy-rollback
    cp -p /etc/systemd/system/caddy.service /root/serenity-caddy-rollback/caddy.service
    [[ ! -f /etc/caddy/Caddyfile ]] || cp -p /etc/caddy/Caddyfile /root/serenity-caddy-rollback/Caddyfile
fi
install -m 0644 "$script_dir/Caddyfile" /etc/caddy/Caddyfile
install -d -m 0755 /opt/serenity-hosted
install -m 0755 "$script_dir/backup.sh" /opt/serenity-hosted/backup.sh
printf 'SERENITY_BACKUP_BUCKET=%s\n' "$backup_bucket" > "$work/backup.env"
install -m 0600 -o serenity -g serenity "$work/backup.env" /etc/serenity/backup.env
install -m 0644 "$script_dir/serenity-backup.service" /etc/systemd/system/serenity-backup.service
install -m 0644 "$script_dir/serenity-backup-failed.service" /etc/systemd/system/serenity-backup-failed.service
install -m 0644 "$script_dir/serenity-backup.timer" /etc/systemd/system/serenity-backup.timer
install -m 0644 "$script_dir/caddy.service" /etc/systemd/system/caddy.service
systemctl daemon-reload
systemctl enable --now caddy
# Select the new binary only after Caddy and all deployment files are ready;
# failures above this line leave the previously selected binary untouched.
ln -sfn "/usr/local/lib/serenity/serenity-${number}" /usr/local/bin/serenity
if ! systemctl enable --now serenity-hosted || ! systemctl restart serenity-hosted; then
    serenity_restore_previous_binary /usr/local/bin/serenity /usr/local/bin/serenity.prev serenity-hosted || true
    exit 1
fi
# Reload through the 0600 unix admin socket. A process started under the old
# root unit has no socket, so the first deploy after the privilege drop
# restarts Caddy instead; every later deploy is a zero-downtime reload.
if [[ -S /run/caddy/admin.sock ]]; then
    if ! caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile --address unix//run/caddy/admin.sock; then
        serenity_restore_previous_binary /usr/local/bin/serenity /usr/local/bin/serenity.prev serenity-hosted || true
        echo 'Caddy reload failed; inspect /root/serenity-caddy-rollback if proxy recovery is needed' >&2
        exit 1
    fi
else
    if ! systemctl restart caddy; then
        serenity_restore_previous_binary /usr/local/bin/serenity /usr/local/bin/serenity.prev serenity-hosted || true
        echo 'Caddy restart failed; inspect /root/serenity-caddy-rollback if proxy recovery is needed' >&2
        exit 1
    fi
fi
if ! serenity_rollback_on_readiness_failure /usr/local/bin/serenity /usr/local/bin/serenity.prev serenity-hosted http://127.0.0.1:8090/readyz; then
    exit 1
fi
systemctl start serenity-backup.service
systemctl enable --now serenity-backup.timer
echo "Hosted Serenity ${number} is locally ready. Public smoke and release acceptance are separate."
