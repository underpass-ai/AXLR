#!/usr/bin/env python3
"""Fetch only the locked MCP adapters and verify bytes before image assembly.

GitHub's anonymous release downloads time out or answer 502/503/504 at
times (8 November 2026: an incident on timeouts for unauthenticated
users failed every image-smoke run for hours, on main as on pull
requests). Each download is retried with a growing wait, and a token in
GITHUB_TOKEN, or in the BuildKit secret github_token, takes the
authenticated path, which answered throughout.
"""
import argparse
import hashlib
import json
import os
import sys
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

ATTEMPTS = 6
FIRST_WAIT_SECONDS = 5
SECRET_PATH = Path("/run/secrets/github_token")


def token() -> str:
    value = os.environ.get("GITHUB_TOKEN", "").strip()
    if not value and SECRET_PATH.is_file():
        value = SECRET_PATH.read_text().strip()
    return value


def download(url: str, auth: str) -> bytes:
    headers = {"User-Agent": "axlr-fetch-engines"}
    if auth:
        headers["Authorization"] = f"Bearer {auth}"
    wait = FIRST_WAIT_SECONDS
    for attempt in range(1, ATTEMPTS + 1):
        try:
            with urlopen(Request(url, headers=headers), timeout=60) as response:
                return response.read()
        except HTTPError as error:
            if error.code < 500 or attempt == ATTEMPTS:
                raise
            reason = f"HTTP {error.code}"
        except (URLError, TimeoutError, OSError) as error:
            if attempt == ATTEMPTS:
                raise
            reason = str(error)
        print(f"{url}: {reason}; attempt {attempt} of {ATTEMPTS}, next in {wait}s", file=sys.stderr)
        time.sleep(wait)
        wait = min(wait * 2, 60)
    raise RuntimeError("unreachable")


def fetch(lock_path: Path, arch: str, out: Path) -> None:
    lock = json.loads(lock_path.read_text())
    if lock.get("version") != 1 or arch not in {"amd64", "arm64"}:
        raise ValueError("unsupported engine lock or architecture")
    out.mkdir(parents=True, exist_ok=True)
    auth = token()
    for name in ("kmp", "made"):
        spec = lock["engines"][name][f"linux/{arch}"]
        if not spec["url"].startswith(f"https://github.com/underpass-ai/{name}/releases/download/"):
            raise ValueError(f"unexpected {name} source")
        data = download(spec["url"], auth)
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
