#!/usr/bin/env python3
"""Regenerate or verify the license texts shipped with every AXLR binary."""
import argparse
import os
import subprocess
import tempfile
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]
DEST = REPO / "distribution" / "third_party"
TARGETS = tuple((system, arch) for system in ("linux", "darwin", "windows") for arch in ("amd64", "arm64"))


def collect(tool: str) -> dict[str, bytes]:
    result = {}
    with tempfile.TemporaryDirectory(prefix="axlr-licenses-") as temp:
        for system, arch in TARGETS:
            env = dict(os.environ, GOOS=system, GOARCH=arch, GOWORK="off", CGO_ENABLED="0")
            for module, packages in ((REPO, ("./cmd/axlr",)), (REPO / "tui", ("./cmd/axlr-tui", "./cmd/axlr-serve"))):
                destination = Path(temp) / f"{module.name}-{system}-{arch}"
                subprocess.run((tool, "save", "--ignore", "github.com/underpass-ai/AXLR", "--save_path", str(destination), *packages), cwd=module, env=env, check=True)
                for path in destination.rglob("*"):
                    if not path.is_file():
                        continue
                    name = path.relative_to(destination).as_posix()
                    content = path.read_bytes()
                    if name in result and result[name] != content:
                        raise ValueError(f"conflicting license texts for {name}")
                    result[name] = content
    return result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tool", default="go-licenses")
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    wanted = collect(args.tool)
    if not wanted:
        raise SystemExit("no dependency licenses found")
    if args.write:
        if DEST.exists():
            for path in sorted(DEST.rglob("*"), reverse=True):
                if path.is_file():
                    path.unlink()
                elif path.is_dir():
                    path.rmdir()
        for name, content in wanted.items():
            path = DEST / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content)
    actual = {p.relative_to(DEST).as_posix(): p.read_bytes() for p in DEST.rglob("*") if p.is_file()} if DEST.exists() else {}
    if actual != wanted:
        raise SystemExit(f"license bundle differs: missing={sorted(wanted.keys() - actual.keys())}, extra={sorted(actual.keys() - wanted.keys())}, changed={sorted(name for name in wanted.keys() & actual.keys() if wanted[name] != actual[name])}")
    print(f"verified {len(wanted)} dependency license files")


if __name__ == "__main__":
    main()
