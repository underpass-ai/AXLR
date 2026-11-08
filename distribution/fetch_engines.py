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
from urllib.request import HTTPRedirectHandler, Request, build_opener

ATTEMPTS = 6
FIRST_WAIT_SECONDS = 5
SECRET_PATH = Path("/run/secrets/github_token")


def token() -> str:
    value = os.environ.get("GITHUB_TOKEN", "").strip()
    if not value and SECRET_PATH.is_file():
        value = SECRET_PATH.read_text().strip()
    return value


def log(message: str) -> None:
    """Every request and every answer, on stderr, so a failed build says
    exactly what was asked and what GitHub said back (never the token)."""
    print(f"fetch_engines: {message}", file=sys.stderr, flush=True)


class LoggedRedirects(HTTPRedirectHandler):
    """Logs each redirect GitHub answers with: the code and the host it
    sends the download to (release-assets or objects.githubusercontent.com)."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        log(f"  {code} redirect -> {newurl.split('/')[2]}")
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def describe(error: HTTPError) -> str:
    ids = {key: error.headers.get(key) for key in ("x-github-request-id", "x-request-id", "retry-after", "server") if error.headers.get(key)}
    body = ""
    try:
        body = error.read(300).decode("utf-8", "replace").strip()
    except OSError:
        pass
    return f"HTTP {error.code} {error.reason}; headers {ids}; body {body!r}"


def download(url: str, auth: str) -> bytes:
    headers = {"User-Agent": "axlr-fetch-engines"}
    if auth:
        headers["Authorization"] = f"Bearer {auth}"
    opener = build_opener(LoggedRedirects)
    wait = FIRST_WAIT_SECONDS
    for attempt in range(1, ATTEMPTS + 1):
        started = time.monotonic()
        log(f"GET {url} (auth: {'token' if auth else 'none'}; attempt {attempt} of {ATTEMPTS})")
        try:
            with opener.open(Request(url, headers=headers), timeout=60) as response:
                data = response.read()
                log(f"  {response.status} from {response.geturl().split('/')[2]}: {len(data)} bytes in {time.monotonic() - started:.1f}s")
                return data
        except HTTPError as error:
            reason = describe(error)
            if error.code < 500 or attempt == ATTEMPTS:
                log(f"  {reason}; giving up")
                raise
        except (URLError, TimeoutError, OSError) as error:
            reason = f"{type(error).__name__}: {error}"
            if attempt == ATTEMPTS:
                log(f"  {reason}; giving up")
                raise
        log(f"  {reason} after {time.monotonic() - started:.1f}s; next attempt in {wait}s")
        time.sleep(wait)
        wait = min(wait * 2, 60)
    raise RuntimeError("unreachable")


def fetch(lock_path: Path, arch: str, out: Path) -> None:
    lock = json.loads(lock_path.read_text())
    if lock.get("version") != 1 or arch not in {"amd64", "arm64"}:
        raise ValueError("unsupported engine lock or architecture")
    out.mkdir(parents=True, exist_ok=True)
    auth = token()
    log(f"lock {lock_path}, arch {arch}, token {'present' if auth else 'absent'}")
    for name in ("kmp", "made"):
        spec = lock["engines"][name][f"linux/{arch}"]
        if not spec["url"].startswith(f"https://github.com/underpass-ai/{name}/releases/download/"):
            raise ValueError(f"unexpected {name} source")
        data = download(spec["url"], auth)
        digest = hashlib.sha256(data).hexdigest()
        if digest != spec["sha256"]:
            log(f"  {name} sha256 {digest} does not match the lock's {spec['sha256']}")
            raise ValueError(f"{name} checksum mismatch")
        log(f"  {name} sha256 ok")
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
