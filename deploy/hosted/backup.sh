#!/usr/bin/env bash
set -euo pipefail
: "${SERENITY_BACKUP_BUCKET:?Set SERENITY_BACKUP_BUCKET to the stack-owned bucket}"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
# Shared with the service despite its PrivateTmp namespace and filesystem sandbox.
work=$(mktemp -d /var/lib/serenity/.backup-XXXXXXXX)
trap 'rm -rf -- "$work"' EXIT
/usr/local/bin/serenity hosted backup --data-dir /var/lib/serenity --snapshot "$work/snapshot"
# A success marker is uploaded last; incomplete prefixes are not restorable.
aws --region us-west-2 s3 cp "$work/snapshot/" "s3://$SERENITY_BACKUP_BUCKET/snapshots/$stamp/" --recursive --only-show-errors
printf '%s\n' "$stamp" > "$work/COMPLETE"
aws --region us-west-2 s3 cp "$work/COMPLETE" "s3://$SERENITY_BACKUP_BUCKET/snapshots/$stamp/COMPLETE" --only-show-errors
