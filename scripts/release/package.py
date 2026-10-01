#!/usr/bin/env python3
"""Deterministic native or cross-target AXLR archive builder."""
import argparse
import gzip
import hashlib
import io
import os
import platform
import shutil
import subprocess
import tarfile
import tempfile
import zipfile
from pathlib import Path


def build(repo: Path, version: str, system: str, arch: str, output: Path, allow_dirty: bool = False) -> Path:
    if (system, arch) not in {(x, y) for x in ("linux", "darwin", "windows") for y in ("amd64", "arm64")}:
        raise ValueError("unsupported target")
    license_path = repo / "LICENSE"
    notice_path = repo / "NOTICE"
    third_party = repo / "distribution" / "third_party"
    if not license_path.is_file():
        raise ValueError("LICENSE is required by the release contract")
    if not notice_path.is_file() or not any(third_party.rglob("LICENSE*")):
        raise ValueError("NOTICE and third-party license bundle are required")
    if not allow_dirty and subprocess.check_output(("git", "status", "--porcelain"), cwd=repo, text=True).strip():
        raise ValueError("checkout is dirty")
    environment = dict(os.environ, GOWORK="off", CGO_ENABLED="0", GOOS=system, GOARCH=arch)
    linker = f"-s -w -X github.com/underpass-ai/AXLR/buildinfo.Version={version}"
    with tempfile.TemporaryDirectory(prefix="axlr-package-") as temp:
        staging = Path(temp)
        suffix = ".exe" if system == "windows" else ""
        for name, module, target in (("axlr", repo, "./cmd/axlr"), ("axlr-tui", repo / "tui", "./cmd/axlr-tui"), ("axlr-serve", repo / "tui", "./cmd/axlr-serve")):
            binary = staging / (name + suffix)
            subprocess.run(("go", "build", "-trimpath", f"-ldflags={linker}", "-o", str(binary), target), cwd=module, env=environment, check=True)
            if binary.stat().st_size == 0:
                raise ValueError(f"empty binary: {name}")
            metadata = subprocess.check_output(("go", "version", "-m", str(binary)), text=True, env=environment)
            for field, value in (("GOOS", system), ("GOARCH", arch), ("CGO_ENABLED", "0")):
                if f"\tbuild\t{field}={value}\n" not in metadata + "\n":
                    raise ValueError(f"{name} {field} mismatch")
            if platform.system().lower() == system and platform.machine().lower() in {arch, "x86_64" if arch == "amd64" else "aarch64"}:
                probe = subprocess.run((str(binary), "--version"), text=True, capture_output=True, check=True)
                value = (probe.stdout or probe.stderr).strip()
                if value != version:
                    raise ValueError(f"{name} version mismatch: {value}")
        (staging / "VERSION").write_text(version + "\n")
        (staging / "LICENSE").write_bytes(license_path.read_bytes())
        (staging / "NOTICE").write_bytes(notice_path.read_bytes())
        shutil.copytree(third_party, staging / "THIRD_PARTY_LICENSES")
        (staging / "README.md").write_text("AXLR release binaries: axlr (worker), axlr-tui (console), axlr-serve (mTLS service).\nSee https://github.com/underpass-ai/AXLR for setup.\n")
        output.mkdir(parents=True, exist_ok=True)
        filename = f"axlr-v{version}-{system}-{arch}." + ("zip" if system == "windows" else "tar.gz")
        archive = output / filename
        paths = sorted((path for path in staging.rglob("*") if path.is_file()), key=lambda item: item.relative_to(staging).as_posix())
        if system == "windows":
            with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
                for path in paths:
                    name = path.relative_to(staging).as_posix()
                    info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
                    info.compress_type = zipfile.ZIP_DEFLATED
                    info.external_attr = (0o100755 if path.suffix == ".exe" else 0o100644) << 16
                    z.writestr(info, path.read_bytes(), compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
        else:
            with archive.open("wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", mtime=0, filename="") as compressed, tarfile.open(fileobj=compressed, mode="w") as tar:
                for path in paths:
                    data = path.read_bytes()
                    name = path.relative_to(staging).as_posix()
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    info.mtime = 0
                    info.uid = info.gid = 0
                    info.mode = 0o755 if name in {"axlr", "axlr-tui", "axlr-serve"} else 0o644
                    tar.addfile(info, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (output / (filename + ".sha256")).write_text(f"{digest}  {filename}\n")
        return archive


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument("goos")
    parser.add_argument("goarch")
    parser.add_argument("--output", type=Path, default=Path("dist"))
    parser.add_argument("--allow-dirty", action="store_true", help="development-only packaging check")
    args = parser.parse_args()
    print(build(Path(__file__).resolve().parents[2], args.version, args.goos, args.goarch, args.output, args.allow_dirty))
