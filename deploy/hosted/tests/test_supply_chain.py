import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[3]


class ReleaseSignatureTests(unittest.TestCase):
    def test_action_pin_validator_checks_named_steps(self):
        validator = ROOT / "scripts/check-action-pins.py"
        with tempfile.TemporaryDirectory() as temp:
            workflow_dir = Path(temp) / ".github/workflows"
            workflow_dir.mkdir(parents=True)
            workflow = workflow_dir / "release.yml"
            workflow.write_text("jobs:\n  sign:\n    steps:\n      - name: Install Cosign\n        uses: sigstore/cosign-installer@v4.1.0\n")
            result = subprocess.run(["python3", str(validator)], cwd=temp, text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("release.yml:5", result.stderr)

            workflow.write_text("jobs:\n  sign:\n    steps:\n      - name: Install Cosign\n        uses: sigstore/cosign-installer@ba7bc0a3fef59531c69a25acd34668d6d3fe6f22 # v4.1.0\n")
            result = subprocess.run(["python3", str(validator)], cwd=temp, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            workflow.rename(workflow_dir / "release.yaml")
            (workflow_dir / "release.yaml").write_text("jobs:\n  sign:\n    steps:\n      - name: Install Cosign\n        uses: sigstore/cosign-installer@v4.1.0\n")
            result = subprocess.run(["python3", str(validator)], cwd=temp, text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0, ".yaml workflow escaped pin validation")

    def test_tampered_archive_is_refused_before_extract(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            bin_dir = root / "bin"
            bin_dir.mkdir()
            archive = root / "release.tar.gz"
            bundle = root / "release.tar.gz.sigstore.json"
            archive.write_bytes(b"original release bytes")
            bundle.write_text(hashlib.sha256(archive.read_bytes()).hexdigest())
            fake_cosign = bin_dir / "cosign"
            fake_cosign.write_text(
                "#!/usr/bin/env python3\n"
                "import hashlib, pathlib, sys\n"
                "args = sys.argv[1:]\n"
                "bundle = pathlib.Path(args[args.index('--bundle') + 1]).read_text()\n"
                "identity = args[args.index('--certificate-identity') + 1]\n"
                "issuer = args[args.index('--certificate-oidc-issuer') + 1]\n"
                "archive = pathlib.Path(args[-1])\n"
                "expected = 'https://github.com/sirerun/serenity/.github/workflows/release.yml@refs/tags/v1.2.3'\n"
                "actual = hashlib.sha256(archive.read_bytes()).hexdigest()\n"
                "sys.exit(0 if identity == expected and issuer == 'https://token.actions.githubusercontent.com' and actual == bundle else 1)\n"
            )
            fake_cosign.chmod(0o755)
            env = os.environ.copy()
            env["PATH"] = str(bin_dir) + os.pathsep + env["PATH"]
            helper = ROOT / "deploy/hosted/verify-release.sh"
            result = subprocess.run(
                [str(helper), str(archive), str(bundle), "v1.2.3"],
                env=env,
                text=True,
                capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

            archive.write_bytes(b"tampered release bytes")
            result = subprocess.run(
                [str(helper), str(archive), str(bundle), "v1.2.3"],
                env=env,
                text=True,
                capture_output=True,
            )
            self.assertNotEqual(result.returncode, 0, "tampered archive passed signature verification")

    def test_deploy_verifies_before_extracting(self):
        deploy = (ROOT / "deploy/hosted/deploy.sh").read_text()
        self.assertLess(deploy.index("verify-release.sh"), deploy.index('tar -xzf "$work/release.tar.gz"'))


if __name__ == "__main__":
    unittest.main()
