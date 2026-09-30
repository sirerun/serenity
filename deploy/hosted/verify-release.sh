#!/usr/bin/env bash
# Verify the signed release archive before deploy.sh extracts or installs it.
set -euo pipefail
archive=${1:?usage: verify-release.sh ARCHIVE SIGNATURE-BUNDLE TAG}
bundle=${2:?usage: verify-release.sh ARCHIVE SIGNATURE-BUNDLE TAG}
tag=${3:?usage: verify-release.sh ARCHIVE SIGNATURE-BUNDLE TAG}
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
command -v cosign >/dev/null || { echo 'Cosign is required to verify release signatures' >&2; exit 1; }
identity="https://github.com/sirerun/serenity/.github/workflows/release.yml@refs/tags/${tag}"
cosign verify-blob \
    --bundle "$bundle" \
    --certificate-identity "$identity" \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    "$archive"
