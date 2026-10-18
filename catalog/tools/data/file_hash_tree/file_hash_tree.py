#!/usr/bin/env python3
"""SHA-256 (or SHA-512, SHA-1, MD5, BLAKE2b) manifest of a set of files, given inline as
{path: content} or read from a relative directory: sorted entries, sha256sum-compatible lines
and one digest of the whole tree; or verify the files against such a manifest, reporting
matching, changed, missing and extra files.

Input (JSON on stdin):
  operation   "manifest" (default) | "verify"
  files       {path: text content} (UTF-8)
  files_hex   {path: content as hex} for binary content
  directory   a relative directory to walk instead (symlinks are not followed)
  include     glob patterns a path must match (directory mode), default all
  exclude     glob patterns to skip (directory mode)
  algorithm   "sha256" (default) | "sha512" | "sha1" | "md5" | "blake2b"
  manifest    verify: the manifest text (sha256sum or BSD "SHA256 (path) = hash" lines)
"""
import fnmatch
import hashlib
import json
import os
import pathlib
import re
import sys

ALGORITHMS = {"sha256": 64, "sha512": 128, "sha1": 40, "md5": 32, "blake2b": 128}
BSD_NAMES = {"sha256": "SHA256", "sha512": "SHA512", "sha1": "SHA1", "md5": "MD5", "blake2b": "BLAKE2b"}
MAX_FILES = 20000
MAX_TOTAL = 500_000_000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def safe_relative(path, what):
    if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path):
        fail(f"{what} must be a relative path")
    if ".." in re.split(r"[\\/]", path):
        fail(f"{what} must not contain '..'")
    return path


def escape_line(digest, path):
    """GNU coreutils: a name with a backslash or newline is written escaped, the line prefixed with a backslash."""
    if "\\" in path or "\n" in path or "\r" in path:
        escaped = path.replace("\\", "\\\\").replace("\n", "\\n").replace("\r", "\\r")
        return f"\\{digest}  {escaped}"
    return f"{digest}  {path}"


def unescape(text):
    out, i = [], 0
    while i < len(text):
        if text[i] == "\\" and i + 1 < len(text):
            out.append({"\\": "\\", "n": "\n", "r": "\r"}.get(text[i + 1], text[i + 1]))
            i += 2
        else:
            out.append(text[i])
            i += 1
    return "".join(out)


def parse_manifest(text, algorithm):
    entries, problems = {}, []
    length = ALGORITHMS[algorithm]
    for number, line in enumerate(text.splitlines(), 1):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        escaped = line.startswith("\\")
        body = line[1:] if escaped else line
        bsd = re.match(r"^([A-Za-z0-9-]+) \((.*)\) = ([0-9a-fA-F]+)$", body)
        gnu = re.match(r"^([0-9a-fA-F]+) ([ *])(.*)$", body)
        if bsd:
            path, digest = bsd.group(2), bsd.group(3)
        elif gnu:
            digest, path = gnu.group(1), gnu.group(3)
        else:
            problems.append(f"line {number}: not a checksum line")
            continue
        if escaped:
            path = unescape(path)
        if len(digest) != length:
            problems.append(f"line {number}: a {algorithm} digest has {length} hex digits, this one {len(digest)}")
            continue
        if path in entries:
            problems.append(f"line {number}: {path!r} is listed twice")
        entries[path] = digest.lower()
    return entries, problems


def collect(request):
    contents = {}
    given = [k for k in ("files", "files_hex", "directory") if k in request]
    if not given:
        fail("give files ({path: content}), files_hex or directory")
    if "directory" in request and len(given) > 1:
        fail("directory cannot be combined with files or files_hex")
    for key in ("files", "files_hex"):
        if key in request:
            table = request[key]
            if not isinstance(table, dict) or not all(isinstance(v, str) for v in table.values()):
                fail(f"{key} must be an object {{path: string}}")
            for path, value in table.items():
                if not path:
                    fail(f"{key} has an empty path")
                if path in contents:
                    fail(f"{path!r} is given twice")
                if key == "files":
                    contents[path] = value.encode("utf-8")
                else:
                    try:
                        contents[path] = bytes.fromhex(re.sub(r"\s", "", value))
                    except ValueError:
                        fail(f"files_hex[{path!r}] is not valid hex")
    if "directory" in request:
        root = pathlib.Path(safe_relative(request["directory"], "directory"))
        if not root.is_dir():
            fail(f"no such directory: {request['directory']}")
        base = pathlib.Path.cwd().resolve()
        if base not in root.resolve().parents and root.resolve() != base:
            fail("directory must stay inside the working directory")
        include = request.get("include", ["*"])
        exclude = request.get("exclude", [])
        for name, value in (("include", include), ("exclude", exclude)):
            if not isinstance(value, list) or not all(isinstance(p, str) for p in value):
                fail(f"{name} must be a list of glob patterns")
        total = 0
        for folder, dirs, names in os.walk(root, followlinks=False):
            dirs.sort()
            for name in sorted(names):
                full = pathlib.Path(folder) / name
                if full.is_symlink() or not full.is_file():
                    continue
                relative = full.relative_to(root).as_posix()
                if not any(fnmatch.fnmatchcase(relative, p) or fnmatch.fnmatchcase(name, p) for p in include):
                    continue
                if any(fnmatch.fnmatchcase(relative, p) or fnmatch.fnmatchcase(name, p) for p in exclude):
                    continue
                total += full.stat().st_size
                if len(contents) >= MAX_FILES or total > MAX_TOTAL:
                    fail("too many files or bytes (at most 20000 files and 500 MB)")
                contents[relative] = full.read_bytes()
    return contents


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"operation", "files", "files_hex", "directory", "include", "exclude", "algorithm", "manifest"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    operation = request.get("operation", "manifest")
    if operation not in ("manifest", "verify"):
        fail("operation must be manifest or verify")
    algorithm = request.get("algorithm", "sha256")
    if not isinstance(algorithm, str) or algorithm not in ALGORITHMS:
        fail("algorithm must be sha256, sha512, sha1, md5 or blake2b")
    contents = collect(request)
    digests = {path: hashlib.new(algorithm, data).hexdigest() for path, data in contents.items()}
    order = sorted(digests, key=lambda p: p.encode("utf-8"))
    lines = [escape_line(digests[p], p) for p in order]
    manifest = "".join(line + "\n" for line in lines)
    result = {"operation": operation, "algorithm": algorithm, "count": len(order), "total_bytes": sum(len(contents[p]) for p in order),
              "entries": [{"path": p, "hash": digests[p], "bytes": len(contents[p])} for p in order],
              "manifest": manifest, "tree_digest": hashlib.new(algorithm, manifest.encode("utf-8")).hexdigest(),
              "bsd_manifest": "".join(f"{BSD_NAMES[algorithm]} ({p}) = {digests[p]}\n" for p in order)}
    if operation == "verify":
        text = request.get("manifest")
        if not isinstance(text, str):
            fail("verify needs manifest: the checksum lines")
        expected, problems = parse_manifest(text, algorithm)
        ok = [p for p in sorted(expected, key=lambda s: s.encode("utf-8")) if p in digests and digests[p] == expected[p]]
        changed = [{"path": p, "expected": expected[p], "actual": digests[p]} for p in sorted(expected, key=lambda s: s.encode("utf-8")) if p in digests and digests[p] != expected[p]]
        missing = [p for p in sorted(expected, key=lambda s: s.encode("utf-8")) if p not in digests]
        extra = [p for p in order if p not in expected]
        result["verify"] = {"valid": not changed and not missing and not extra and not problems, "ok": ok, "changed": changed,
                            "missing": missing, "extra": extra, "manifest_problems": problems}
        del result["bsd_manifest"]
    print(json.dumps(result))


if __name__ == "__main__":
    main()
