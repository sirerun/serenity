from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]
GENERATOR = REPO / "scripts/site/build-content.py"


class BrandGeneratorTest(unittest.TestCase):
    def test_default_brand_refresh_preserves_hosted_copy_and_routes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scripts = root / "scripts/site"
            scripts.mkdir(parents=True)
            shutil.copyfile(GENERATOR, scripts / "build-content.py")
            site = root / "site"
            site.mkdir()
            content = [
                {"url": "https://serenity.sire.run/pricing/", "title": "Hosted plans", "description": "Accepted plan copy.", "html": "<p>Free preview remains limited.</p>"},
                {"url": "https://serenity.sire.run/docs/connections/", "title": "Connect your agent", "description": "Accepted connection copy.", "html": "<p>OAuth and bearer-token details.</p>"},
            ]
            (site / "content.json").write_text(json.dumps(content), encoding="utf-8")
            llms = (
                "# Serenity\n\nHosted project memory with a free preview.\n\n"
                "- [Hosted plans](https://serenity.sire.run/pricing/)\n"
                "- [Connect your agent](https://serenity.sire.run/docs/connections/)\n"
                "- [Start free](https://serenity.sire.run/login)\n"
            )
            sitemap = (
                '<urlset><url><loc>https://serenity.sire.run/pricing/</loc></url>'
                '<url><loc>https://serenity.sire.run/docs/connections/</loc></url></urlset>'
            )
            (site / "llms.txt").write_text(llms, encoding="utf-8")
            (site / "sitemap.xml").write_text(sitemap, encoding="utf-8")

            result = subprocess.run(
                [sys.executable, str(scripts / "build-content.py")],
                check=True,
                capture_output=True,
                text=True,
            )

            self.assertIn("preserved other site content", result.stdout)
            self.assertEqual((site / "llms.txt").read_text(encoding="utf-8"), llms)
            self.assertEqual((site / "sitemap.xml").read_text(encoding="utf-8"), sitemap)
            rows = json.loads((site / "content.json").read_text(encoding="utf-8"))
            by_url = {row["url"]: row for row in rows}
            self.assertEqual(by_url["https://serenity.sire.run/pricing/"]["description"], "Accepted plan copy.")
            self.assertEqual(by_url["https://serenity.sire.run/docs/connections/"]["description"], "Accepted connection copy.")
            brand = by_url["https://serenity.sire.run/brand/"]
            self.assertEqual(brand["title"], "Book Binder Rose")
            self.assertIn("rose Book Binder", brand["description"])
            self.assertIn("Original PNG mark", brand["html"])
            self.assertIn("not vector tracings", brand["html"])


if __name__ == "__main__":
    unittest.main()
