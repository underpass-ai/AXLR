#!/usr/bin/env python3
"""Build a .gitignore from named stacks (Python, Node, Go, macOS, JetBrains...): the
github/gitignore templates bundled in templates.json, combined in the order given, with a
header per section and repeated patterns removed when that cannot change the result.

Input (JSON on stdin):
  stacks       list of stack names or aliases, e.g. ["Python", "macOS", "vscode"]
  comments     keep the templates' own comment lines (default true)
  dedupe       drop patterns already present earlier (default true)
  extra        your own patterns, appended in a final section (optional)
  list_stacks  true: only list the available stacks and their aliases
"""
import json
import pathlib
import sys

DATA = pathlib.Path(__file__).resolve().parent / "templates.json"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load():
    try:
        return json.loads(DATA.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        fail(f"cannot read the bundled templates.json: {error}")


def index(data):
    names = {}
    for name, t in data["templates"].items():
        for key in [name] + t["aliases"]:
            names[key.lower()] = name
    return names


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra_keys = sorted(set(request) - {"stacks", "comments", "dedupe", "extra", "list_stacks"})
    if extra_keys:
        fail(f"unknown field(s): {', '.join(extra_keys)}")
    data = load()
    source = {"repository": data["repository"], "commit": data["commit"], "commit_date": data["commit_date"],
              "license": data["license"], "fetched": data["fetched"]}
    if request.get("list_stacks"):
        print(json.dumps({"stacks": [{"name": n, "kind": t["kind"], "path": t["path"], "aliases": t["aliases"]}
                                     for n, t in data["templates"].items()], "source": source}))
        return
    for key in ("comments", "dedupe", "list_stacks"):
        if key in request and not isinstance(request[key], bool):
            fail(f"{key} must be true or false")
    stacks = request.get("stacks")
    if not isinstance(stacks, list) or not stacks or not all(isinstance(s, str) for s in stacks):
        fail("stacks must be a non-empty list of names such as [\"Python\", \"macOS\"] (list_stacks: true lists them)")
    if len(stacks) > 50:
        fail("at most 50 stacks")
    extra = request.get("extra", [])
    if not isinstance(extra, list) or not all(isinstance(s, str) and "\n" not in s for s in extra):
        fail("extra must be a list of one-line patterns")
    names = index(data)
    chosen, unknown = [], []
    for s in stacks:
        name = names.get(s.strip().lower())
        if name is None:
            unknown.append(s)
        elif name not in chosen:
            chosen.append(name)
    if unknown:
        fail(f"unknown stack(s): {', '.join(unknown)}; available: {', '.join(data['templates'])}")
    keep_comments = request.get("comments", True)
    dedupe = request.get("dedupe", True)

    out = [f"# Generated from github/gitignore ({data['commit'][:7]}, {data['commit_date']}, CC0-1.0): "
           + ", ".join(chosen), ""]
    first_at = {}         # pattern -> position of its first emission
    last_negation = -1    # position of the last emitted negation (!pattern)
    position = 0
    sections = []
    sources = [(n, data["templates"][n]["lines"], data["templates"][n]["path"]) for n in chosen]
    if extra:
        sources.append(("Custom", extra, None))
    for name, lines, path in sources:
        header = f"### {name} ###" + (f" (github/gitignore {path})" if path else "")
        body, dropped, kept = [], 0, 0
        for raw in lines:
            line = raw.rstrip("\r")
            stripped = line.strip()
            if not stripped:
                if body and body[-1] != "":
                    body.append("")
                continue
            if stripped.startswith("#"):
                if keep_comments:
                    body.append(line)
                continue
            if dedupe and not stripped.startswith("!") and line in first_at and first_at[line] > last_negation:
                dropped += 1
                continue
            body.append(line)
            kept += 1
            position += 1
            if stripped.startswith("!"):
                last_negation = position
            first_at.setdefault(line, position)
        while body and body[-1] == "":
            body.pop()
        out.append(header)
        out.extend(body)
        out.append("")
        sections.append({"name": name, "path": path, "patterns": kept, "duplicates_dropped": dropped})
    content = "\n".join(out).rstrip("\n") + "\n"
    print(json.dumps({"content": content, "stacks": chosen, "sections": sections,
                      "patterns": sum(s["patterns"] for s in sections), "source": source}))


if __name__ == "__main__":
    main()
