#!/usr/bin/env python3
"""Build catalog/index.json from the tool manifests and ceremony definitions under catalog/,
with the SHA-256 of every file (decision D2 of the tool registry design). Signing is a
separate step (minisign over the written file).

Usage: python3 tools/catalog/build_index.py [catalog] [--check]
  --check  exit 1 when the written index differs from the computed one
"""
import datetime as dt
import hashlib
import json
import pathlib
import re
import sys

NAME = re.compile(r"^[a-z][a-z0-9_]{1,47}$")


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tool_entry(root, manifest_path):
    manifest = json.loads(manifest_path.read_text())
    meta = manifest["catalog"]
    name = manifest["name"]
    if not NAME.match(name) or manifest_path.parent.name != name:
        raise SystemExit(f"{manifest_path}: name {name!r} is invalid or differs from its directory")
    if manifest["args"] and not any(arg in manifest["files"] for arg in manifest["args"]):
        raise SystemExit(f"{manifest_path}: args must name one of the tool's files")
    if len(manifest["files"]) not in range(1, 9):
        raise SystemExit(f"{manifest_path}: a tool has one to eight files")
    files = {f: sha256(manifest_path.parent / f) for f in manifest["files"]}
    return {
        "name": name,
        "version": meta["version"],
        "category": meta["category"],
        "tags": meta.get("tags", []),
        "summary": manifest["description"].split(". ")[0].rstrip(".") + ".",
        "program": manifest["program"],
        "requires": meta.get("requires", {"packages": []}),
        "network": meta.get("network", False),
        "license": meta["license"],
        "manifest": str(manifest_path.relative_to(root)),
        "manifest_sha256": sha256(manifest_path),
        "files": files,
    }


def ceremony_entry(root, definition):
    text = definition.read_text()
    head = {}
    for key in ("name", "version", "description"):
        m = re.search(rf"^{key}: (.*)$", text, re.M)
        head[key] = m.group(1).strip().strip("'\"") if m else ""
    guards = sorted(set(re.findall(r"check: '?([a-z_]+):", text)))
    return {
        "name": head["name"],
        "version": head["version"],
        "domain": definition.parent.name,
        "summary": head["description"].split(". ")[0].rstrip(".") + ".",
        "definition": str(definition.relative_to(root)),
        "sha256": sha256(definition),
        "guards": guards,
        "approvals": "none",
    }


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    root = pathlib.Path(args[0] if args else "catalog")
    index = {
        "schema_version": 1,
        "catalog": "axlr",
        "generated_at": dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "tools": [tool_entry(root, m) for m in sorted(root.glob("tools/*/*/manifest.json"))],
        "ceremonies": [ceremony_entry(root, d) for d in sorted(root.glob("ceremonies/*/*.yaml"))],
    }
    out = root / "index.json"
    text = json.dumps(index, indent=2, ensure_ascii=False) + "\n"
    if "--check" in sys.argv:
        old = json.loads(out.read_text()) if out.exists() else {}
        old.pop("generated_at", None)
        new = dict(index)
        new.pop("generated_at")
        if old != new:
            print("index.json is stale; run tools/catalog/build_index.py")
            sys.exit(1)
        print("index.json is current")
        return
    out.write_text(text)
    print(f"{out}: {len(index['tools'])} tools, {len(index['ceremonies'])} ceremonies")


if __name__ == "__main__":
    main()
