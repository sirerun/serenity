#!/usr/bin/env bash
# Run on the hosted instance after the ADR 023 tagged release is deployed.
# The partner secret stays in Secrets Manager and a private service-owned file.
set -euo pipefail
[[ $(id -u) == 0 ]] || { echo 'Run through SSM as root' >&2; exit 1; }
socket=/var/lib/serenity/.hosted-admin.sock
[[ -S "$socket" ]] || { echo 'Hosted admin socket is unavailable' >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
aws --region us-west-2 secretsmanager get-secret-value \
    --secret-id serenity/hosted/BLINK_PARTNER_SECRET \
    --query SecretString --output text > "$work/secret"
[[ -s "$work/secret" ]] || { echo 'Blink partner secret is empty' >&2; exit 1; }
if grep -qx UNCONFIGURED "$work/secret"; then
    echo 'Blink partner secret is unconfigured' >&2
    exit 1
fi
install -d -m 0700 -o serenity -g serenity /etc/serenity/secrets
install -m 0600 -o serenity -g serenity "$work/secret" /etc/serenity/secrets/BLINK_PARTNER_SECRET
curl --fail --silent --show-error --unix-socket "$socket" \
    -H 'Content-Type: application/json' \
    --data '{"id":"blink","display_name":"Blink","secret_file":"/etc/serenity/secrets/BLINK_PARTNER_SECRET","redirect_prefix":"blink://serenity-linked","status":"active"}' \
    http://localhost/partners
echo 'Blink partner seeded through the private admin socket.'
