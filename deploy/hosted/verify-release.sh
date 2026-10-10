#!/usr/bin/env bash
# Verify the signed release archive before deploy.sh extracts or installs it.
set -euo pipefail
archive=${1:?usage: verify-release.sh ARCHIVE SIGNATURE-BUNDLE TAG}
bundle=${2:?usage: verify-release.sh ARCHIVE SIGNATURE-BUNDLE TAG}
tag=${3:?usage: verify-release.sh ARCHIVE SIGNATURE-BUNDLE TAG}
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
policy=${SERENITY_RELEASE_POLICY:-sigstore}
case "$policy" in
    sigstore) ;;
    owner-kms-manifest-v1)
        verifier=${SERENITY_RELEASE_VERIFIER:?Installed release verifier required}
        [[ "$verifier" = /* && -f "$verifier" && -x "$verifier" ]] || { echo "Verifier must be an absolute installed executable" >&2; exit 1; }
        [[ $(id -u) = 0 ]] || { echo "KMS release verification requires root" >&2; exit 1; }
        # The verifier comes from independent trust installation, never the archive.
        path=$verifier
        while :; do
            [[ ! -L "$path" ]] || { echo "Symlink in verifier path" >&2; exit 1; }
            owner=$(stat -c %u -- "$path")
            mode=$(stat -c %a -- "$path")
            [[ "$owner" = 0 ]] && (( (8#$mode & 8#22) == 0 )) || { echo "Unsafe verifier ownership or mode" >&2; exit 1; }
            [[ "$path" = / ]] && break
            path=$(dirname -- "$path")
        done
        args=(verify --archive "$archive" --signature "$bundle" --tag "$tag"
            --manifest "${SERENITY_RELEASE_MANIFEST:?Release manifest required}"
            --policy "${SERENITY_RELEASE_POLICY_FILE:?Independent policy required}"
            --approval "${SERENITY_RELEASE_APPROVAL_FILE:?Exact approval required}"
            --state "${SERENITY_RELEASE_STATE_FILE:?Deployment state required}")
        if [[ -n "${SERENITY_RELEASE_LOCK_FD:-}" ]]; then
            [[ "$SERENITY_RELEASE_LOCK_FD" =~ ^[0-9]+$ ]] || { echo "Invalid lock descriptor" >&2; exit 1; }
            args+=(--lock-fd "$SERENITY_RELEASE_LOCK_FD")
        fi
        exec "$verifier" "${args[@]}"
        ;;
    *) echo "Unsupported release policy" >&2; exit 1 ;;
esac
command -v cosign >/dev/null || { echo 'Cosign is required to verify release signatures' >&2; exit 1; }
identity="https://github.com/sirerun/serenity/.github/workflows/release.yml@refs/tags/${tag}"
cosign verify-blob \
    --bundle "$bundle" \
    --certificate-identity "$identity" \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    "$archive"
