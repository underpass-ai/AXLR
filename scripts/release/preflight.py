#!/usr/bin/env python3
"""Validate release identity and write the expected asset inventory before builds."""
import argparse
import json
import re
import subprocess
from pathlib import Path

TAG = re.compile(r"^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$")
TARGETS = (("linux", "amd64", "tar.gz"), ("linux", "arm64", "tar.gz"),
           ("darwin", "amd64", "tar.gz"), ("darwin", "arm64", "tar.gz"),
           ("windows", "amd64", "zip"), ("windows", "arm64", "zip"))


def run(*args: str) -> str:
    return subprocess.check_output(args, text=True).strip()


def chart_version(path: Path) -> tuple[str, str]:
    fields = {}
    for line in path.read_text().splitlines():
        if ":" in line:
            key, value = line.split(":", 1)
            fields[key] = value.strip().strip('"')
    return fields["version"], fields["appVersion"]


def version_for(ref: str, sha: str, chart: tuple[str, str], lock: dict) -> str:
    if lock.get("version") != 1 or not isinstance(lock.get("engines"), dict):
        raise ValueError("engine lock is incomplete")
    for name in ("kmp", "made"):
        engine = lock["engines"].get(name, {})
        engine_version = engine.get("version", "")
        if not TAG.fullmatch(engine_version):
            raise ValueError(f"{name} version is missing or invalid")
        for system in ("linux/amd64", "linux/arm64"):
            asset = engine.get(system, {})
            if not asset.get("url", "").startswith(f"https://github.com/underpass-ai/{name}/releases/download/{engine_version}/") or not re.fullmatch(r"[0-9a-f]{64}", asset.get("sha256", "")):
                raise ValueError(f"{name} {system} lock is incomplete")
    if ref.startswith("refs/tags/"):
        tag = ref.removeprefix("refs/tags/")
        if not TAG.fullmatch(tag):
            raise ValueError("release tag must be semver")
        prerelease = tag.split("-", 1)[1] if "-" in tag else ""
        if any(part.isdigit() and len(part) > 1 and part.startswith("0") for part in prerelease.split(".")):
            raise ValueError("numeric prerelease identifiers cannot have leading zeros")
        version = tag[1:]
        if chart != (version, version):
            raise ValueError("chart version and appVersion must match tag")
        return version
    if not re.fullmatch(r"[0-9a-f]{7,40}", sha):
        raise ValueError("invalid commit SHA")
    return f"0.0.0-dev+{sha[:12]}"


def inventory(version: str) -> list[str]:
    assets = []
    for system, arch, extension in TARGETS:
        name = f"axlr-v{version}-{system}-{arch}.{extension}"
        assets.extend((name, name + ".sha256"))
    chart = f"axlr-{version}.tgz"
    assets.extend((chart, chart + ".sha256", "image-digest.json"))
    if len(assets) != len(set(assets)):
        raise ValueError("duplicate asset")
    return assets


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ref", required=True)
    parser.add_argument("--sha", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--require-clean", action="store_true")
    args = parser.parse_args()
    if args.require_clean and run("git", "-C", str(args.repo), "status", "--porcelain"):
        raise SystemExit("checkout is dirty")
    if args.ref.startswith("refs/tags/"):
        tag = args.ref.removeprefix("refs/tags/")
        if run("git", "-C", str(args.repo), "rev-list", "-n", "1", tag) != args.sha:
            raise SystemExit("tag does not match checkout")
        subprocess.run(("git", "-C", str(args.repo), "merge-base", "--is-ancestor", args.sha, "origin/main"), check=True)
    lock = json.loads((args.repo / "distribution/engines.lock.json").read_text())
    version = version_for(args.ref, args.sha, chart_version(args.repo / "charts/axlr/Chart.yaml"), lock)
    data = {"version": version, "commit": args.sha, "assets": inventory(version)}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")
    print(version)


if __name__ == "__main__":
    main()
