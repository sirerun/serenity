import importlib.util
import io
import os
from pathlib import Path
import tarfile
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[3]
spec=importlib.util.spec_from_file_location("unpack_snapshot",ROOT/"deploy/hosted/unpack_snapshot.py")
module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class UnpackTests(unittest.TestCase):
    def make_archive(self,path,extra=None):
        with tarfile.open(path,"w") as tf:
            for name in ("manifest.json","authority-cut.json","control.db","A"*26+".bundle"):
                item=tarfile.TarInfo(name);item.size=2
                tf.addfile(item,io.BytesIO(b"{}"))
            if extra is not None:tf.addfile(extra)
    def test_regular_transport_and_no_replace(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);os.chmod(root,0o700)
            archive=root/"snapshot.tar";self.make_archive(archive)
            destination=root/"snapshot"
            module.extract(archive,destination)
            self.assertEqual((destination/"manifest.json").read_bytes(),b"{}")
            self.assertEqual((destination/("A"*26+".bundle")).read_bytes(),b"{}")
            with self.assertRaises(OSError):module.extract(archive,destination)
            self.assertEqual(sorted(p.name for p in root.iterdir()),["snapshot","snapshot.tar"])
    def test_links_traversal_duplicates_and_missing_files_fail_without_publication(self):
        for name,kind in [("brains/"+"A"*26+".bundle",tarfile.REGTYPE),("outside",tarfile.SYMTYPE),("../escape",tarfile.REGTYPE),("manifest.json",tarfile.REGTYPE),("pipe",tarfile.FIFOTYPE)]:
            with self.subTest(name=name),tempfile.TemporaryDirectory() as directory:
                root=Path(directory);os.chmod(root,0o700)
                archive=root/"snapshot.tar";extra=tarfile.TarInfo(name);extra.type=kind;extra.linkname="/etc/passwd"
                self.make_archive(archive,extra)
                with self.assertRaises(ValueError):module.extract(archive,root/"out")
                self.assertFalse((root/"out").exists())
                self.assertEqual(list(root.iterdir()),[archive])
    def test_root_unpack_refused(self):
        # The security precondition can be checked without running tests as root.
        original=module.os.geteuid
        module.os.geteuid=lambda:0
        try:
            with self.assertRaisesRegex(ValueError,"unprivileged"):module.extract("missing","missing")
        finally:module.os.geteuid=original

if __name__=="__main__":unittest.main()
