#!/usr/bin/env bash
# Bootstrap a blank Amazon Linux 2023 ARM64 host for the reviewed deploy path.
# This script is safe to rerun. It never writes application secrets or starts the service.
set -euo pipefail

CADDY_VERSION=${CADDY_VERSION:-2.8.4}
CADDY_SHA256=${CADDY_SHA256:-93a3eb31883d678c6590c6d823eb6bb9f7a3af66dcd9d53df3eb2c7528b2af05}
DATA_DEVICE=${SERENITY_DATA_DEVICE:-/dev/sdf}
DATA_LABEL=serenity-data

[[ $(id -u) == 0 ]] || { echo 'Run as root' >&2; exit 1; }
[[ $(uname -m) == aarch64 ]] || { echo "Unsupported architecture: $(uname -m); expected aarch64" >&2; exit 1; }
command -v dnf >/dev/null || { echo 'Amazon Linux dnf is required' >&2; exit 1; }

# AL2023 may expose the CloudFormation /dev/sdf attachment as an NVMe path.
for _ in $(seq 1 60); do
    [[ -b "$DATA_DEVICE" ]] && break
    if [[ "$DATA_DEVICE" == /dev/sdf && -b /dev/nvme1n1 ]]; then DATA_DEVICE=/dev/nvme1n1; break; fi
    sleep 2
done
[[ -b "$DATA_DEVICE" ]] || { echo "Data volume did not appear: $DATA_DEVICE" >&2; exit 1; }

# Never format a volume that already contains a filesystem or partition table.
if ! blkid "$DATA_DEVICE" >/dev/null 2>&1; then
    [[ -z "$(wipefs --noheadings "$DATA_DEVICE" 2>/dev/null)" ]] || {
        echo "Refusing to format non-empty data device: $DATA_DEVICE" >&2
        exit 1
    }
    mkfs.ext4 -L "$DATA_LABEL" "$DATA_DEVICE"
fi
mkdir -p /var/lib/serenity
fs_type=$(blkid -s TYPE -o value "$DATA_DEVICE")
[[ "$fs_type" == ext4 ]] || { echo "Data volume must use ext4: $fs_type" >&2; exit 1; }
uuid=$(blkid -s UUID -o value "$DATA_DEVICE")
[[ -n "$uuid" ]] || { echo "Data volume has no UUID: $DATA_DEVICE" >&2; exit 1; }
if ! grep -qE "^[[:space:]]*UUID=${uuid}[[:space:]]+/var/lib/serenity[[:space:]]" /etc/fstab; then
    printf 'UUID=%s /var/lib/serenity ext4 defaults,nofail,x-systemd.device-timeout=120 0 2\n' "$uuid" >> /etc/fstab
fi
mountpoint -q /var/lib/serenity || mount /var/lib/serenity
mountpoint -q /var/lib/serenity || { echo 'Persistent data volume is not mounted' >&2; exit 1; }

# Install only host prerequisites; no application release or provider call occurs here.
dnf install -y awscli-2 git gzip jq tar util-linux shadow-utils
command -v curl >/dev/null || { echo 'curl is required (curl-minimal is acceptable)' >&2; exit 1; }

# Install a pinned Caddy release for the HTTPS reverse proxy.
if ! command -v caddy >/dev/null || [[ "$(caddy version 2>/dev/null | awk 'NR==1 {print $1}' | sed 's/^v//')" != "$CADDY_VERSION" ]]; then
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    archive="$tmp/caddy_${CADDY_VERSION}_linux_arm64.tar.gz"
    curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 \
      "https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}/caddy_${CADDY_VERSION}_linux_arm64.tar.gz" \
      --output "$archive"
    printf '%s  %s\n' "$CADDY_SHA256" "$archive" | sha256sum --check --status
    tar -xzf "$archive" -C "$tmp" caddy
    install -m 0755 "$tmp/caddy" /usr/local/bin/caddy
fi

id serenity >/dev/null 2>&1 || useradd --system --home-dir /var/lib/serenity --shell /sbin/nologin serenity
install -d -m 0700 -o serenity -g serenity /etc/serenity /etc/serenity/secrets
install -d -m 0755 /etc/caddy
install -d -m 0700 -o serenity -g serenity /var/lib/serenity
systemctl enable --now amazon-ssm-agent.service
printf 'architecture=%s\ndata_device=%s\ndata_uuid=%s\ncaddy_version=%s\n' \
  "$(uname -m)" "$DATA_DEVICE" "$uuid" "$CADDY_VERSION" > /etc/serenity/bootstrap.state
chmod 0600 /etc/serenity/bootstrap.state
chown serenity:serenity /etc/serenity/bootstrap.state
printf '%s\n' 'Serenity host bootstrap complete; application deployment remains a separate reviewed step.'
