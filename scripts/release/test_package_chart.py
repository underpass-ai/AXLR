import importlib.util
import shutil
import tarfile
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("package_chart", Path(__file__).with_name("package_chart.py"))
package_chart = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package_chart)


@unittest.skipUnless(shutil.which("helm"), "helm is not installed")
class PackageChartTests(unittest.TestCase):
    def test_chart_takes_the_release_version(self):
        chart = Path(__file__).resolve().parents[2] / "charts/axlr"
        with tempfile.TemporaryDirectory() as output:
            archive = package_chart.package(chart, "9.9.9-rc.1", Path(output))
            self.assertEqual(archive.name, "axlr-9.9.9-rc.1.tgz")
            with tarfile.open(archive, "r:gz") as source:
                lines = source.extractfile("axlr/Chart.yaml").read().decode().splitlines()
        self.assertIn("version: 9.9.9-rc.1", lines)
        self.assertIn("appVersion: 9.9.9-rc.1", lines)


if __name__ == "__main__":
    unittest.main()
