#!/usr/bin/env python3
"""Package a Helm chart with stable tar and gzip metadata."""
import argparse
import gzip
import hashlib
import io
import subprocess
import tarfile
import tempfile
from pathlib import Path


def package(chart: Path, version: str, output: Path) -> Path:
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="axlr-chart-") as temp:
        subprocess.run(("helm", "package", str(chart), "--destination", temp), check=True, capture_output=True, text=True)
        original = Path(temp) / f"axlr-{version}.tgz"
        if not original.is_file():
            raise ValueError("chart version does not match release version")
        with tarfile.open(original, "r:gz") as source:
            files = {entry.name: source.extractfile(entry).read() for entry in source if entry.isfile()}
        if not files or any(not name.startswith("axlr/") or ".." in Path(name).parts for name in files):
            raise ValueError("invalid chart paths")
        archive = output / original.name
        with archive.open("wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", mtime=0, filename="") as compressed, tarfile.open(fileobj=compressed, mode="w") as target:
            for name, data in sorted(files.items()):
                info = tarfile.TarInfo(name)
                info.size = len(data)
                info.mode = 0o644
                info.mtime = 0
                target.addfile(info, io.BytesIO(data))
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    (output / (archive.name + ".sha256")).write_text(f"{digest}  {archive.name}\n")
    return archive


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument("--chart", type=Path, default=Path(__file__).resolve().parents[2] / "charts/axlr")
    parser.add_argument("--output", type=Path, default=Path("dist"))
    args = parser.parse_args()
    print(package(args.chart, args.version, args.output))
