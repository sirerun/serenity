"""Kernel-limit and same-PID tests for the private AWS CLI launcher."""

from __future__ import annotations

import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

LAUNCHER = Path(__file__).parents[1] / "backup_s3_child.py"


class ChildLauncherTests(unittest.TestCase):
    def test_limit_is_enforced_while_child_writes(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "oversized"
            process = subprocess.run(
                [
                    sys.executable,
                    "-I",
                    str(LAUNCHER),
                    "--file-size-limit",
                    "1024",
                    "--",
                    sys.executable,
                    "-c",
                    "from pathlib import Path; Path(__import__('sys').argv[1]).write_bytes(b'x' * 4096)",
                    str(output),
                ],
                capture_output=True,
                check=False,
            )
            self.assertNotEqual(process.returncode, 0)
            self.assertEqual(output.stat().st_size, 1024)

    def test_exec_preserves_registered_child_pid_and_environment(self):
        with tempfile.TemporaryDirectory() as temporary:
            pid_path = Path(temporary) / "pid"
            child = subprocess.Popen(
                [
                    sys.executable,
                    "-I",
                    str(LAUNCHER),
                    "--file-size-limit",
                    "1024",
                    "--",
                    sys.executable,
                    "-c",
                    "import os, pathlib, sys; pathlib.Path(sys.argv[1]).write_text(str(os.getpid()))",
                    str(pid_path),
                ],
                env={"PATH": os.environ.get("PATH", "")},
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            stdout, stderr = child.communicate(timeout=10)
            self.assertEqual(child.returncode, 0, (stdout, stderr))
            self.assertEqual(int(pid_path.read_text()), child.pid)

    def test_invalid_request_does_not_exec_target(self):
        with tempfile.TemporaryDirectory() as temporary:
            marker = Path(temporary) / "ran"
            process = subprocess.run(
                [
                    sys.executable,
                    "-I",
                    str(LAUNCHER),
                    "--file-size-limit",
                    "not-an-integer",
                    "--",
                    sys.executable,
                    "-c",
                    "from pathlib import Path; Path(__import__('sys').argv[1]).touch()",
                    str(marker),
                ],
                capture_output=True,
                check=False,
            )
            self.assertEqual(process.returncode, 125)
            self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
