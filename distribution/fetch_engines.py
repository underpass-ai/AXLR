#!/usr/bin/env python3
"""Fetch only the locked MCP adapters and verify bytes before image assembly."""
import argparse
import hashlib
import json
import os
from pathlib import Path
from urllib.request import urlopen


def fetch(lock_path: Path, arch: str, out: Path) -> None:
    lock = json.loads(lock_path.read_text())
    if lock.get("version") != 1 or arch not in {"amd64", "arm64"}:
        raise ValueError("unsupported engine lock or architecture")
    out.mkdir(parents=True, exist_ok=True)
    for name in ("kmp", "made"):
        spec = lock["engines"][name][f"linux/{arch}"]
        if not spec["url"].startswith(f"https://github.com/underpass-ai/{name}/releases/download/"):
            raise ValueError(f"unexpected {name} source")
        with urlopen(spec["url"], timeout=60) as response:
            data = response.read()
        if hashlib.sha256(data).hexdigest() != spec["sha256"]:
            raise ValueError(f"{name} checksum mismatch")
        path = out / f"{name}-mcp"
        path.write_bytes(data)
        os.chmod(path, 0o755)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--lock", type=Path, required=True)
    parser.add_argument("--arch", required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    fetch(args.lock, args.arch, args.out)
