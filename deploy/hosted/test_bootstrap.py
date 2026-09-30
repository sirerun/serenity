import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).parent


class BootstrapContractTests(unittest.TestCase):
    def test_bootstrap_is_strict_and_arch_pinned(self):
        text = (ROOT / "bootstrap.sh").read_text()
        self.assertIn("set -euo pipefail", text)
        self.assertIn('[[ $(uname -m) == aarch64 ]]', text)
        self.assertRegex(text, r"CADDY_VERSION=\$\{CADDY_VERSION:-2\.8\.4\}")
        self.assertRegex(text, r"CADDY_SHA256=\$\{CADDY_SHA256:-[0-9a-f]{64}\}")
        self.assertIn('sha256sum --check --status', text)

    def test_blank_volume_is_never_formatted_without_empty_probe(self):
        text = (ROOT / "bootstrap.sh").read_text()
        guard = re.search(
            r'if ! blkid "\$DATA_DEVICE".*?mkfs\.ext4 -L "\$DATA_LABEL" "\$DATA_DEVICE"',
            text,
            re.S,
        )
        self.assertIsNotNone(guard)
        self.assertIn('wipefs --noheadings "$DATA_DEVICE"', guard.group(0))
        self.assertIn('Refusing to format non-empty data device', guard.group(0))

    def test_deploy_bootstraps_before_mount_and_prerequisite_checks(self):
        text = (ROOT / "deploy.sh").read_text()
        self.assertLess(text.index('"$script_dir/bootstrap.sh"'), text.index('mountpoint -q /var/lib/serenity'))
        self.assertLess(text.index('"$script_dir/bootstrap.sh"'), text.index('command -v caddy'))

    def test_deploy_installs_the_same_caddyfile_that_it_validates(self):
        text = (ROOT / "deploy.sh").read_text()
        self.assertIn('caddy validate --config "$script_dir/Caddyfile" --adapter caddyfile', text)
        self.assertIn('install -m 0644 "$script_dir/Caddyfile" /etc/caddy/Caddyfile', text)
        self.assertNotIn('$caddy_config', text)

    def test_blink_partner_secret_is_generated_and_scoped_to_host_read(self):
        stack = json.loads((ROOT / "stack.json").read_text())
        secret = stack["Resources"]["BlinkPartnerSecret"]["Properties"]
        self.assertEqual(secret["Name"], "serenity/hosted/BLINK_PARTNER_SECRET")
        self.assertNotIn("SecretString", secret)
        self.assertGreaterEqual(secret["GenerateSecretString"]["PasswordLength"], 32)
        statements = stack["Resources"]["Role"]["Properties"]["Policies"][0]["PolicyDocument"]["Statement"]
        reads = [s for s in statements if "secretsmanager:GetSecretValue" in s["Action"]]
        self.assertEqual(len(reads), 1)
        self.assertIn({"Ref": "BlinkPartnerSecret"}, reads[0]["Resource"])
        self.assertNotIn("*", reads[0]["Resource"])

    def test_blink_seed_keeps_secret_out_of_the_admin_request(self):
        text = (ROOT / "seed-blink-partner.sh").read_text()
        self.assertIn('install -m 0600 -o serenity -g serenity', text)
        self.assertIn('--unix-socket "$socket"', text)
        self.assertIn('"secret_file":"/etc/serenity/secrets/BLINK_PARTNER_SECRET"', text)
        self.assertIn('"redirect_prefix":"blink://serenity-linked"', text)
        self.assertNotIn('"secret":', text)


if __name__ == "__main__":
    unittest.main()
