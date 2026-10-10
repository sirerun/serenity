#!/usr/bin/env bash
# Root owns AWS credentials; snapshot reads/serialization run as the service UID.
set -euo pipefail
: "${SERENITY_BACKUP_BUCKET:?backup bucket is required}"
SERENITY_DATA_DIR="${SERENITY_DATA_DIR:-/var/lib/serenity}"
case "$SERENITY_DATA_DIR" in /var/lib/serenity|/var/lib/serenity/hosted) ;; *) echo "Unsupported private data directory" >&2; exit 1;; esac
[[ $(id -u) == 0 ]] || { echo 'Backup uploader must run as trusted root' >&2; exit 1; }
[[ "$SERENITY_BACKUP_BUCKET" =~ ^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$ ]] || exit 1
# Do not allow a caller environment to redirect credential or endpoint discovery.
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_PROFILE AWS_DEFAULT_PROFILE
unset AWS_SHARED_CREDENTIALS_FILE AWS_CONFIG_FILE AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3
unset AWS_CONTAINER_CREDENTIALS_FULL_URI AWS_CONTAINER_CREDENTIALS_RELATIVE_URI AWS_CONTAINER_AUTHORIZATION_TOKEN AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE
unset AWS_WEB_IDENTITY_TOKEN_FILE AWS_ROLE_ARN AWS_ROLE_SESSION_NAME AWS_EC2_METADATA_SERVICE_ENDPOINT AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy AWS_CA_BUNDLE
export AWS_SHARED_CREDENTIALS_FILE=/dev/null AWS_CONFIG_FILE=/dev/null AWS_EC2_METADATA_DISABLED=false
export AWS_REGION=us-west-2 AWS_DEFAULT_REGION=us-west-2
export AWS_IGNORE_CONFIGURED_ENDPOINT_URLS=true
unprivileged() {
    # A separate cgroup also denies metadata/network access to these same-UID
    # helper processes; the root uploader is the only credentialed process.
    systemd-run --quiet --wait --pipe --collect --uid=serenity --gid=serenity \
        --property=IPAddressDeny=any --property=RestrictAddressFamilies=AF_UNIX \
        --property=NoNewPrivileges=yes --property=RuntimeMaxSec=120 --property=TimeoutStopSec=10 "$@"
}
stamp=$(date -u +%Y%m%dT%H%M%SZ)
work=$(unprivileged mktemp -d /var/lib/serenity/snapshot.XXXXXXXX)
[[ "$work" == /var/lib/serenity/snapshot.* ]] || exit 1
install -d -m 0700 -o root -g root /var/lib/serenity-backup
upload=$(mktemp -d /var/lib/serenity-backup/upload.XXXXXXXX)
cleanup() {
    unprivileged rm -rf -- "$work"
    rm -rf -- "$upload"
}
trap cleanup EXIT
unprivileged /usr/local/bin/serenity hosted backup --data-dir "$SERENITY_DATA_DIR" --snapshot "$work/snapshot"
# A root uploader must never walk application-writable source paths. Tar runs
# unprivileged, does not dereference symlinks, and writes only through this pipe.
# The 2GiB transport ceiling and 120s deadline fail the job before any upload.
unprivileged tar -C "$work/snapshot" -cf - . | head -c 2147483649 > "$upload/snapshot.tar"
bytes=$(stat -c %s "$upload/snapshot.tar")
(( bytes > 0 && bytes <= 2147483648 )) || { echo 'Backup transport exceeds 2GiB bound' >&2; exit 1; }
digest=$(sha256sum "$upload/snapshot.tar")
digest=${digest%% *}
prefix="s3://$SERENITY_BACKUP_BUCKET/snapshots/$stamp-${digest:0:16}"
aws --region us-west-2 s3 cp "$upload/snapshot.tar" "$prefix/snapshot.tar" --sse AES256 --only-show-errors
printf '{"archive":"snapshot.tar","bytes":%s,"sha256":"%s","version":1}\n' "$bytes" "$digest" > "$upload/COMPLETE.json"
aws --region us-west-2 s3 cp "$upload/COMPLETE.json" "$prefix/COMPLETE.json" --sse AES256 --only-show-errors
echo "Backup complete: $prefix"
