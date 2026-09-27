"""Static contract tests for the hosted edge units (T24.1, ADR 020).

These tests parse deploy/hosted/caddy.service, deploy/hosted/Caddyfile,
deploy/hosted/bootstrap.sh and deploy/hosted/deploy.sh and assert the
posture that closes SEC-H01 (Caddy as root with an open TCP admin API) and
SEC-M03 (limiters keyed on Cloudflare edge addresses). They run without a
host: `python3 -m unittest deploy/hosted/tests/test_units.py`.
"""

import ipaddress
import re
import unittest
from pathlib import Path

HOSTED = Path(__file__).resolve().parents[1]
CADDY_SERVICE = HOSTED / "caddy.service"
CADDYFILE = HOSTED / "Caddyfile"
BOOTSTRAP = HOSTED / "bootstrap.sh"
DEPLOY = HOSTED / "deploy.sh"

ADMIN_SOCKET = "unix//run/caddy/admin.sock"
CLIENT_IP_HEADER = "CF-Connecting-IP"

# Cloudflare's published proxy ranges. Source: https://www.cloudflare.com/ips-v4
# and https://www.cloudflare.com/ips-v6, fetched 2026-09-27. The Caddyfile must
# carry exactly this set; refresh both when Cloudflare changes the list.
CLOUDFLARE_IPV4 = {
    "173.245.48.0/20",
    "103.21.244.0/22",
    "103.22.200.0/22",
    "103.31.4.0/22",
    "141.101.64.0/18",
    "108.162.192.0/18",
    "190.93.240.0/20",
    "188.114.96.0/20",
    "197.234.240.0/22",
    "198.41.128.0/17",
    "162.158.0.0/15",
    "104.16.0.0/13",
    "104.24.0.0/14",
    "172.64.0.0/13",
    "131.0.72.0/22",
}
CLOUDFLARE_IPV6 = {
    "2400:cb00::/32",
    "2606:4700::/32",
    "2803:f800::/32",
    "2405:b500::/32",
    "2405:8100::/32",
    "2a06:98c0::/29",
    "2c0f:f248::/32",
}


def parse_unit(path):
    """Parse a systemd unit into {section: {key: [values]}}."""
    sections = {}
    current = None
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith(("#", ";")):
            continue
        if line.startswith("[") and line.endswith("]"):
            current = line[1:-1]
            sections.setdefault(current, {})
            continue
        if current is None or "=" not in line:
            raise AssertionError(f"{path.name}: unexpected line outside a section: {raw!r}")
        key, _, value = line.partition("=")
        sections[current].setdefault(key.strip(), []).append(value.strip())
    return sections


def strip_caddy_comments(text):
    """Drop `#` comments (Caddyfile comments run to end of line)."""
    return "\n".join(re.sub(r"\s#.*$|^\s*#.*$", "", line) for line in text.splitlines())


def caddyfile_directives(text):
    """Return the non-empty, comment-free lines of a Caddyfile, stripped."""
    return [line.strip() for line in strip_caddy_comments(text).splitlines() if line.strip()]


class CaddyServiceTests(unittest.TestCase):
    def setUp(self):
        self.unit = parse_unit(CADDY_SERVICE)
        self.service = self.unit.get("Service", {})

    def assertServiceValue(self, key, expected):
        self.assertEqual(self.service.get(key), [expected], f"caddy.service [Service] {key}")

    def test_runs_as_the_caddy_user_with_only_the_bind_capability(self):
        self.assertServiceValue("User", "caddy")
        self.assertServiceValue("Group", "caddy")
        self.assertServiceValue("AmbientCapabilities", "CAP_NET_BIND_SERVICE")
        self.assertServiceValue("CapabilityBoundingSet", "CAP_NET_BIND_SERVICE")
        self.assertServiceValue("NoNewPrivileges", "yes")

    def test_filesystem_is_sandboxed(self):
        self.assertServiceValue("ProtectSystem", "strict")
        self.assertServiceValue("ProtectHome", "yes")
        self.assertServiceValue("PrivateTmp", "yes")
        self.assertServiceValue("RuntimeDirectory", "caddy")
        self.assertServiceValue("StateDirectory", "caddy")

    def test_state_lives_under_var_lib_caddy(self):
        environment = " ".join(self.service.get("Environment", []))
        self.assertIn("XDG_DATA_HOME=/var/lib/caddy", environment.split())
        self.assertIn("XDG_CONFIG_HOME=/var/lib/caddy/config", environment.split())

    def test_reload_uses_the_unix_admin_socket(self):
        reload_commands = self.service.get("ExecReload", [])
        self.assertEqual(len(reload_commands), 1, reload_commands)
        self.assertIn(f"--address {ADMIN_SOCKET}", reload_commands[0])
        self.assertIn("--config /etc/caddy/Caddyfile", reload_commands[0])


class CaddyfileTests(unittest.TestCase):
    def setUp(self):
        self.text = CADDYFILE.read_text()
        self.lines = caddyfile_directives(self.text)

    def global_block(self):
        """Return the directives inside the leading global options block."""
        self.assertTrue(self.lines, "Caddyfile is empty")
        self.assertEqual(self.lines[0], "{", "Caddyfile must begin with a global options block")
        depth = 0
        body = []
        for line in self.lines:
            if line.endswith("{"):
                depth += 1
                if depth > 1:
                    body.append(line)
                continue
            if line == "}":
                depth -= 1
                if depth == 0:
                    return body
                body.append(line)
                continue
            body.append(line)
        self.fail("global options block is not closed")

    def test_admin_api_is_a_0600_unix_socket(self):
        self.assertIn(f"admin {ADMIN_SOCKET}|0600", self.global_block())

    def test_trusted_proxies_are_exactly_the_published_cloudflare_ranges(self):
        static = [line for line in self.global_block() if line.startswith("trusted_proxies static ")]
        self.assertEqual(len(static), 1, f"expected one trusted_proxies static line, got {static}")
        ranges = static[0].split()[2:]
        self.assertEqual(len(ranges), len(set(ranges)), "duplicate trusted_proxies ranges")
        for cidr in ranges:
            ipaddress.ip_network(cidr, strict=True)
        self.assertEqual(set(ranges), CLOUDFLARE_IPV4 | CLOUDFLARE_IPV6)

    def test_cloudflare_source_and_fetch_date_are_recorded(self):
        self.assertIn("https://www.cloudflare.com/ips-v4", self.text)
        self.assertIn("https://www.cloudflare.com/ips-v6", self.text)
        self.assertRegex(self.text, r"#.*\b(fetched|Fetched)\b.*\b20\d\d-\d\d-\d\d\b")

    def test_client_ip_comes_from_cf_connecting_ip(self):
        self.assertIn(f"client_ip_headers {CLIENT_IP_HEADER}", self.global_block())

    def test_upstream_sees_the_resolved_client_ip(self):
        self.assertIn("header_up X-Serenity-Client-IP {client_ip}", self.lines)
        self.assertNotIn("{remote_host}", strip_caddy_comments(self.text))


class BootstrapTests(unittest.TestCase):
    def setUp(self):
        self.text = BOOTSTRAP.read_text()

    def test_creates_the_caddy_system_user_idempotently(self):
        self.assertRegex(
            self.text,
            r"id caddy >/dev/null 2>&1 \|\| useradd --system[^\n]* caddy\n",
        )

    def test_creates_state_and_runtime_directories_for_caddy(self):
        self.assertRegex(self.text, r"install -d -m 07[05]0 -o caddy -g caddy [^\n]*/var/lib/caddy\b")
        self.assertRegex(self.text, r"install -d -m 07[05]0 -o caddy -g caddy [^\n]*/run/caddy\b")

    def test_migrates_root_certificate_storage_before_the_unit_starts(self):
        migration = re.search(
            r"if \[\[ -d /root/\.local/share/caddy && ! -e /var/lib/caddy/caddy \]\]; then\n"
            r"(?P<body>.*?)\nfi\n",
            self.text,
            re.S,
        )
        self.assertIsNotNone(migration, "bootstrap.sh must copy /root/.local/share/caddy once")
        self.assertIn("cp -a /root/.local/share/caddy /var/lib/caddy/", migration.group("body"))
        self.assertIn("chown -R caddy:caddy /var/lib/caddy", self.text)
        self.assertNotIn("systemctl enable --now caddy", self.text)


class DeployTests(unittest.TestCase):
    def setUp(self):
        self.text = DEPLOY.read_text()

    def test_reloads_caddy_through_the_unix_admin_socket(self):
        self.assertIn(
            f"caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile --address {ADMIN_SOCKET}",
            self.text,
        )
        self.assertNotIn("systemctl reload caddy", self.text)

    def test_first_start_under_the_new_unit_restarts_instead_of_reloading(self):
        self.assertRegex(
            self.text,
            r"if \[\[ -S /run/caddy/admin\.sock \]\]; then\n\s+caddy reload [^\n]*\nelse\n\s+systemctl restart caddy\nfi",
        )


if __name__ == "__main__":
    unittest.main()
