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
        self.assertRegex(text, r"CADDY_SHA256=\$\{CADDY_SHA256:-[0-9a-f]{128}\}")
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


if __name__ == "__main__":
    unittest.main()
