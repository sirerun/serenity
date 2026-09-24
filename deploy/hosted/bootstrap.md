# Fresh-host bootstrap

`bootstrap.sh` prepares a blank Amazon Linux 2023 ARM64 instance for the reviewed hosted deployment. It is invoked by `deploy.sh` before any release or secret work, and it is safe to rerun after a failed deployment.

The script waits for the attached data volume, accepts the CloudFormation `/dev/sdf` name or the Nitro `/dev/nvme1n1` mapping, and formats only a truly blank device. Existing filesystems and partition data are refused. It records a UUID based `/etc/fstab` entry, mounts `/var/lib/serenity`, installs the AWS CLI, Git, curl and other host tools, installs a checksum-pinned Caddy ARM64 binary, creates the locked-down `serenity` user/directories, and enables the SSM agent. It does not create application secrets, download a Serenity release, start the service, or make provider calls.

The Caddy version and SHA256 are pinned in the script. Updating either requires a reviewed change and a new checksum. `SERENITY_DATA_DEVICE` may override the default only for a reviewed host mapping; it must still pass the empty-device and ext4 checks.

Validation is local and static until an authorized qualification instance exists. Run:

```sh
bash -n deploy/hosted/bootstrap.sh deploy/hosted/deploy.sh
python3 deploy/hosted/test_bootstrap.py
```
