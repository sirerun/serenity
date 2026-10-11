import os
import json
import sys
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "backup.sh"

class BackupStageTests(unittest.TestCase):
    def test_installed_backup_environment_selects_legacy_or_staged_data(self):
        deploy = (SCRIPT.parent / "deploy.sh").read_text()
        emitter = next(line for line in deploy.splitlines() if line.startswith("printf 'SERENITY_BACKUP_BUCKET="))
        installer = next(line for line in deploy.splitlines() if line.startswith('install -m 0600 -o root -g root "$work/backup.env"'))
        self.assertEqual(installer,'install -m 0600 -o root -g root "$work/backup.env" /etc/serenity-backup/backup.env')
        for selected in ("/var/lib/serenity", "/var/lib/serenity/hosted"):
            with self.subTest(selected=selected), tempfile.TemporaryDirectory() as d:
                run = subprocess.run(["bash", "-c", emitter],
                    env={**os.environ,"work":d,"backup_bucket":"synthetic-backup","data_dir":selected},
                    capture_output=True,text=True)
                self.assertEqual(run.returncode,0,run.stderr)
                emitted=Path(d)/"backup.env"
                consumer=subprocess.run(["bash","-c",'set -a; . "$1"; printf "%s\n%s\n" "$SERENITY_BACKUP_BUCKET" "$SERENITY_DATA_DIR"',"fixture",str(emitted)],
                    capture_output=True,text=True)
                self.assertEqual(consumer.returncode,0,consumer.stderr)
                self.assertEqual(consumer.stdout,"synthetic-backup\n"+selected+"\n")

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

    def test_config_install_retains_root_parent_and_service_secret_child(self):
        deploy = (SCRIPT.parent / "deploy.sh").read_text()
        commands = [line for line in deploy.splitlines()
            if line.startswith("install -d") and "/etc/serenity" in line and
            "/etc/serenity-" not in line]
        run = subprocess.run(["bash", "-c",
            'install() { printf "%s\\n" "$*"; };\n' + "\n".join(commands)],
            capture_output=True, text=True)
        self.assertEqual(run.returncode, 0, run.stderr)
        self.assertEqual(run.stdout,
            "-d -m 0750 -o root -g serenity /etc/serenity\n"
            "-d -m 0700 -o serenity -g serenity /etc/serenity/secrets\n")
        self.assertIn('install -m 0640 -o root -g serenity "$script_dir/config.example.json" /etc/serenity/hosted.json', deploy)
        self.assertIn("os.fchmod(f.fileno(), 0o640)", deploy)

    def test_config_updater_exclusive_temp_and_preserved_read_access(self):
        deploy = (SCRIPT.parent / "deploy.sh").read_text()
        updater = deploy.split("<<'PYCONFIG'\n", 1)[1].split("\nPYCONFIG", 1)[0]
        with tempfile.TemporaryDirectory() as d:
            config = Path(d) / "hosted.json"
            config.write_text(json.dumps({"sender": "login@serenity.sire.run"}))
            config.chmod(0o640)
            # Exercise the exact updater as the fixture's current owner; root
            # custody checks remain separately asserted by deployment fixtures.
            fixture = updater.replace('path = "/etc/serenity/hosted.json"', "path = " + repr(str(config)))
            fixture = fixture.replace("st.st_uid!=0", "st.st_uid not in (0,os.geteuid())").replace("metadata.st_uid!=0", "metadata.st_uid!=os.geteuid()")
            victim = Path(d) / "victim"
            victim.write_text("preserved")
            temporary = Path(str(config) + ".new")
            temporary.symlink_to(victim)
            refused = subprocess.run([sys.executable, "-c", fixture], capture_output=True)
            self.assertNotEqual(refused.returncode, 0)
            self.assertEqual(victim.read_text(), "preserved")
            temporary.unlink()
            accepted = subprocess.run([sys.executable, "-c", fixture], capture_output=True)
            self.assertEqual(accepted.returncode, 0, accepted.stderr)
            self.assertEqual(config.stat().st_mode & 0o777, 0o640)
            self.assertEqual(json.loads(config.read_text())["sender"], "login@mail.sire.run")
            self.assertFalse(temporary.exists())

    def test_bootstrap_preserves_existing_migration_parent(self):
        bootstrap = (SCRIPT.parent / "bootstrap.sh").read_text()
        function = bootstrap.split("configure_bootstrap_data_parent() {", 1)[1].split("\nconfigure_bootstrap_data_parent", 1)[0]
        with tempfile.TemporaryDirectory() as d:
            parent = Path(d) / "data"
            parent.mkdir()
            fixture = "configure_bootstrap_data_parent() {" + function.replace("/var/lib/serenity", str(parent))
            for staged in (False, True):
                if staged: (parent / "hosted").mkdir()
                run = subprocess.run(["bash", "-c",
                    'install() { printf "%s\\n" "$*"; };\n' + fixture + "\nconfigure_bootstrap_data_parent"],
                    capture_output=True, text=True)
                self.assertEqual(run.returncode, 0, run.stderr)
                custody = "-m 0750 -o root -g serenity" if staged else "-m 0700 -o serenity -g serenity"
                self.assertEqual(run.stdout.splitlines()[0], "-d " + custody + " " + str(parent))
        self.assertIn("install -d -m 0750 -o root -g serenity /etc/serenity", bootstrap)
        self.assertIn('chown root:root "$bootstrap_state"', bootstrap)
