import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "backup.sh"

class BackupStageTests(unittest.TestCase):
    def test_stage_created_under_selected_data_directory(self):
        lines = SCRIPT.read_text().splitlines()
        stage = next(line for line in lines if line.startswith("work=$(unprivileged mktemp"))
        guard = next(line for line in lines if line.startswith('[[ "$work" =='))
        with tempfile.TemporaryDirectory() as d:
            parent = Path(d)
            selected = parent / "hosted"
            selected.mkdir(mode=0o700)
            run = subprocess.run(["bash", "-c",
                "unprivileged() { \"$@\"; }\n" + stage + "\n" + guard + '\nprintf "%s" "$work"'],
                env={**os.environ, "SERENITY_DATA_DIR": str(selected)},
                capture_output=True, text=True)
            self.assertEqual(run.returncode, 0, run.stderr)
            work = Path(run.stdout)
            self.assertEqual(work.parent, selected)
            self.assertTrue(work.name.startswith("snapshot."))
            self.assertEqual(work.stat().st_mode & 0o777, 0o700)
            self.assertEqual(list(parent.iterdir()), [selected])

    def test_deploy_preserves_migrated_parent_custody(self):
        deploy = (SCRIPT.parent / "deploy.sh").read_text()
        function = deploy[deploy.index("configure_data_parent() {"):deploy.index('configure_data_parent "$data_dir"')]
        for selected, expected in (
            ("/var/lib/serenity/hosted", "chown root:serenity /var/lib/serenity\nchmod 0750 /var/lib/serenity\n"),
            ("/var/lib/serenity", "chown serenity:serenity /var/lib/serenity\n"),
        ):
            run = subprocess.run(["bash", "-c",
                'chown() { printf "chown %s %s\\n" "$1" "$2"; }; chmod() { printf "chmod %s %s\\n" "$1" "$2"; };\n' +
                function + '\nconfigure_data_parent "$1"', "test", selected],
                capture_output=True, text=True)
            self.assertEqual(run.returncode, 0, run.stderr)
            self.assertEqual(run.stdout, expected)
