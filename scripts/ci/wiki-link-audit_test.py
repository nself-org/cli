"""Audit the same Unicode page names under different caller locales."""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest


class WikiLocaleTest(unittest.TestCase):
    def test_locales(self):
        script = pathlib.Path(__file__).with_name("wiki-link-audit.sh")
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            target = root / "scripts/ci/wiki-link-audit.sh"
            target.parent.mkdir(parents=True)
            shutil.copyfile(script, target)
            wiki = root / ".github/wiki"
            wiki.mkdir(parents=True)
            pages = ["Alpha", "Zebra", "apple", "éclair"]
            for page in pages:
                (wiki / f"{page}.md").write_text("\n".join(f"[[{p}]]" for p in pages))
            for locale in ["C", "en_US.UTF-8"]:
                for missing in [False, True]:
                    with self.subTest(locale=locale, missing=missing):
                        (wiki / "Home.md").write_text("[[Absent]]" if missing else "")
                        result = subprocess.run(
                            ["bash", str(target)], env={**os.environ, "LC_ALL": locale},
                            capture_output=True, text=True,
                        )
                        self.assertEqual(result.returncode, int(missing), result.stderr)
                        if missing:
                            self.assertIn("[[Absent]]", result.stderr)


if __name__ == "__main__":
    unittest.main()
