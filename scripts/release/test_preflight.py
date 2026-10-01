import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("preflight", Path(__file__).with_name("preflight.py"))
preflight = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preflight)


class PreflightTests(unittest.TestCase):
    def setUp(self):
        self.lock = {"version": 1, "engines": {name: {"version": "v0.1.0", **{target: {"url": f"https://github.com/underpass-ai/{name}/releases/download/v0.1.0/{name}-{target.split('/')[1]}", "sha256": "a" * 64} for target in ("linux/amd64", "linux/arm64")}} for name in ("kmp", "made")}}

    def test_release_version_requires_chart_identity(self):
        self.assertEqual(preflight.version_for("refs/tags/v1.2.3-rc.1", "a" * 40, ("1.2.3-rc.1", "1.2.3-rc.1"), self.lock), "1.2.3-rc.1")
        for ref in ("refs/tags/1.2.3", "refs/tags/v01.2.3", "refs/tags/v1.2", "refs/tags/v1.2.3+metadata", "refs/tags/v1.2.3-rc.01"):
            with self.subTest(ref=ref), self.assertRaises(ValueError):
                preflight.version_for(ref, "a" * 40, ("1.2.3", "1.2.3"), self.lock)
        with self.assertRaises(ValueError):
            preflight.version_for("refs/tags/v1.2.3", "a" * 40, ("1.2.2", "1.2.3"), self.lock)

    def test_development_version_and_inventory(self):
        version = preflight.version_for("refs/heads/main", "abcdef0123456789", ("0.1.0", "0.1.0"), self.lock)
        self.assertEqual(version, "0.0.0-dev+abcdef012345")
        assets = preflight.inventory("1.2.3")
        self.assertEqual(len(assets), 15)
        self.assertEqual(len(assets), len(set(assets)))
        self.assertIn("axlr-v1.2.3-windows-arm64.zip", assets)
        self.assertIn("axlr-1.2.3.tgz.sha256", assets)

    def test_missing_engine_lock_fails(self):
        with self.assertRaises(ValueError):
            preflight.version_for("refs/heads/main", "abcdef0", ("0.1.0", "0.1.0"), {"engines": {"kmp": {}}})


if __name__ == "__main__":
    unittest.main()
