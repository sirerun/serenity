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
release_policy=${SERENITY_RELEASE_POLICY:-sigstore}
case "$release_policy" in
    sigstore) command -v cosign >/dev/null || { echo 'Cosign is required to verify release signatures' >&2; exit 1; } ;;
    owner-kms-manifest-v1)
        # This verifier is bootstrapped from separately reviewed source, never
        # extracted from the candidate it is about to authenticate.
        release_verifier=/usr/local/libexec/serenity-release
        trust_parent=$(dirname -- "$release_verifier")
        while :; do
            [[ -d "$trust_parent" && ! -L "$trust_parent" && $(stat -c %u "$trust_parent") == 0 ]] || { echo 'Unsafe verifier ancestor' >&2; exit 1; }
            (( (8#$(stat -c %a "$trust_parent") & 8#022) == 0 )) || { echo 'Writable verifier ancestor' >&2; exit 1; }
            [[ "$trust_parent" != / ]] || break
            trust_parent=$(dirname -- "$trust_parent")
        done
        [[ -x "$release_verifier" ]] || { echo 'Pinned release verifier is missing' >&2; exit 1; }
        for trust_file in "$release_verifier" /etc/serenity-release/policy.json /etc/serenity-release/approval.json; do
            [[ -f "$trust_file" && ! -L "$trust_file" && $(stat -c %u "$trust_file") == 0 ]] || { echo 'Release trust file is not root-owned and regular' >&2; exit 1; }
            (( (8#$(stat -c %a "$trust_file") & 8#022) == 0 )) || { echo 'Release trust file is writable by non-root users' >&2; exit 1; }
        done
        [[ -d /var/lib/serenity-release && ! -L /var/lib/serenity-release && $(stat -c %u /var/lib/serenity-release) == 0 ]] || { echo 'Release state directory requires explicit initialization' >&2; exit 1; }
        (( (8#$(stat -c %a /var/lib/serenity-release) & 8#077) == 0 )) || { echo 'Release state directory must be private' >&2; exit 1; }
        # Open without truncation; the verifier additionally validates the fd
        # inode, owner, permissions, and lock before trusting it.
        exec 9>>/var/lib/serenity-release/state.json.lock
        flock -n 9 || { echo 'A release deployment is already active' >&2; exit 1; }
        ;;
    *) echo 'Unknown explicit release policy' >&2; exit 1 ;;
esac
mountpoint -q /var/lib/serenity || { echo 'Persistent data volume is not mounted' >&2; exit 1; }
work=$(mktemp -d)
pending_install=0
cleanup_deploy() {
    status=$?
    if (( status != 0 && pending_install == 1 )); then
        systemctl stop serenity-hosted serenity-broker || true
        echo 'Uncommitted installation retained for exact owner recovery' >&2
    fi
    rm -rf -- "$work"
    exit "$status"
}
trap cleanup_deploy EXIT
number=${version#v}
release_tag="v${number}"
release_url="https://github.com/sirerun/serenity/releases/download/${release_tag}"
curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 "$release_url/serenity_${number}_linux_arm64.tar.gz" --output "$work/release.tar.gz"
if [[ "$release_policy" == sigstore ]]; then
    curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 "$release_url/serenity_${number}_linux_arm64.tar.gz.sigstore.json" --output "$work/release.tar.gz.sigstore.json"
    "$script_dir/verify-release.sh" "$work/release.tar.gz" "$work/release.tar.gz.sigstore.json" "$release_tag"
else
    curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 "$release_url/serenity_${number}_linux_arm64.tar.gz.manifest.json" --output "$work/manifest.json"
    curl --fail --location --proto '=https' --tlsv1.2 --max-time 120 "$release_url/serenity_${number}_linux_arm64.tar.gz.kms.sig" --output "$work/signature.der"
    release_args=(--archive "$work/release.tar.gz" --manifest "$work/manifest.json"
        --signature "$work/signature.der" --policy /etc/serenity-release/policy.json
        --approval /etc/serenity-release/approval.json --state /var/lib/serenity-release/state.json
        --tag "$release_tag" --lock-fd 9)
    "$release_verifier" verify "${release_args[@]}"
fi
activation_failed() {
    systemctl stop serenity-hosted serenity-broker || true
    echo 'Activation failed; recover an approved compatible binary, units and broker configuration before restart' >&2
}
printf '%s  %s\n' "$checksum" "$work/release.tar.gz" | sha256sum --check --status
tar -xzf "$work/release.tar.gz" -C "$work" serenity
[[ -f "$work/serenity" && ! -L "$work/serenity" ]] || { echo 'Archive executable is not a regular file' >&2; exit 1; }
binary_target="/usr/local/lib/serenity/serenity-${number}"
if [[ "$release_policy" == owner-kms-manifest-v1 ]]; then
    binary_target="$binary_target-$checksum"
    [[ ! -e "$binary_target" && ! -L "$binary_target" ]] || { echo 'Immutable candidate path already exists; reconcile before retry' >&2; exit 1; }
fi
id serenity >/dev/null 2>&1 || useradd --system --home-dir /var/lib/serenity --shell /sbin/nologin serenity
# The broker's root-owned enrollment is prepared in the reviewed activation
# packet. Deployment cannot substitute caller identity or silently adopt data.
python3 - "$work/serenity" > "$work/data-dir" <<'PYBROKER'
import hashlib,json,os,pathlib,sys
p=pathlib.Path("/etc/serenity-authority/broker.json")
for parent in [p.parent,*p.parent.parents]:
    st=parent.lstat()
    if parent.is_symlink() or st.st_uid!=0 or st.st_mode&0o022:
        raise SystemExit("Unsafe broker configuration parent")
st=p.lstat()
if p.is_symlink() or not p.is_file() or st.st_uid!=0 or st.st_mode&0o077:
    raise SystemExit("Broker configuration must be private and root-owned")
config=json.loads(p.read_bytes())
with open(sys.argv[1],"rb") as source:
    digest=hashlib.file_digest(source,"sha256").hexdigest()
data_dir=config.get("data_dir")
if data_dir not in ("/var/lib/serenity", "/var/lib/serenity/hosted"):
    raise SystemExit("Unsupported private data directory")
with open("/etc/serenity/hosted.json", "rb") as hosted_source:
    hosted_config=json.load(hosted_source)
if hosted_config.get("data_dir")!=data_dir:
    raise SystemExit("Broker and hosted service data directories differ")
if config.get("executable_sha256")!=digest or config.get("socket")!="/run/serenity-broker/runtime.sock":
    raise SystemExit("Broker enrollment does not match candidate executable and host paths")
print(data_dir)
PYBROKER
data_dir=$(cat "$work/data-dir")
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
if [[ "$release_policy" == owner-kms-manifest-v1 ]]; then
    "$release_verifier" prepare "${release_args[@]}"
fi
pending_install=1
install -m 0755 "$work/serenity" "$binary_target"
configure_data_parent() {
    case "$1" in
        /var/lib/serenity/hosted)
            # The migrated data remains service-owned; its parent is custody
            # protected so the broker can trust the selected directory path.
            chown root:serenity /var/lib/serenity
            chmod 0750 /var/lib/serenity
            ;;
        /var/lib/serenity)
            chown serenity:serenity /var/lib/serenity
            ;;
        *) echo 'Unsupported private data directory' >&2; return 1 ;;
    esac
}
configure_data_parent "$data_dir"
install -m 0644 "$script_dir/serenity-hosted.service" /etc/systemd/system/serenity-hosted.service
install -m 0644 "$script_dir/serenity-broker.service" /etc/systemd/system/serenity-broker.service
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
install -d -m 0700 -o root -g root /etc/serenity-backup /var/lib/serenity-backup
install -m 0600 -o root -g root "$work/backup.env" /etc/serenity-backup/backup.env
install -m 0644 "$script_dir/serenity-backup.service" /etc/systemd/system/serenity-backup.service
install -m 0644 "$script_dir/serenity-backup-failed.service" /etc/systemd/system/serenity-backup-failed.service
install -m 0644 "$script_dir/serenity-backup.timer" /etc/systemd/system/serenity-backup.timer
install -m 0644 "$script_dir/caddy.service" /etc/systemd/system/caddy.service
systemctl daemon-reload
systemctl enable --now caddy
# Select the new binary only after Caddy and all deployment files are ready;
# failures above this line leave the previously selected binary untouched.
systemctl stop serenity-hosted
ln -sfn "$binary_target" /usr/local/bin/serenity
if ! systemctl enable --now serenity-broker || ! systemctl restart serenity-broker; then
    activation_failed
    exit 1
fi
if ! systemctl enable --now serenity-hosted || ! systemctl restart serenity-hosted; then
    activation_failed
    exit 1
fi
# Reload through the 0600 unix admin socket. A process started under the old
# root unit has no socket, so the first deploy after the privilege drop
# restarts Caddy instead; every later deploy is a zero-downtime reload.
if [[ -S /run/caddy/admin.sock ]]; then
    if ! caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile --address unix//run/caddy/admin.sock; then
        activation_failed
        echo 'Caddy reload failed; inspect /root/serenity-caddy-rollback if proxy recovery is needed' >&2
        exit 1
    fi
else
    if ! systemctl restart caddy; then
        activation_failed
        echo 'Caddy restart failed; inspect /root/serenity-caddy-rollback if proxy recovery is needed' >&2
        exit 1
    fi
fi
if ! serenity_wait_ready http://127.0.0.1:8090/readyz; then
    activation_failed
    exit 1
fi
if [[ "$release_policy" == owner-kms-manifest-v1 ]]; then
    "$release_verifier" commit "${release_args[@]}"
fi
pending_install=0
systemctl start serenity-backup.service
systemctl enable --now serenity-backup.timer
echo "Hosted Serenity ${number} is locally ready. Public smoke and release acceptance are separate."
