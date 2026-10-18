#!/usr/bin/env python3
"""Deduplicate and sort lines: duplicates found exactly, ignoring case, after trimming, after
Unicode normalisation (NFC or NFKC) or ignoring accents, keeping the first occurrence and
counting the others; sorting by code point or naturally (file2 before file10), optionally
case- and accent-insensitive without any locale, and reversed.

Input (JSON on stdin):
  text              the lines (or lines: an array of strings, or path: a relative file)
  dedupe            true (default) | false
  ignore_case       compare with Unicode case folding, default false
  trim              strip leading and trailing whitespace (also in the output), default false
  normalize         "none" (default) | "NFC" | "NFKC"
  ignore_accents    compare without diacritics (café = cafe), default false
  sort              "none" (default: input order) | "lexical" | "natural"
  reverse           reverse the sort, default false
  drop_empty        remove empty lines, default false
  only              "all" (default) | "duplicates" (lines seen more than once) | "unique" (seen once)
"""
import json
import pathlib
import re
import sys
import unicodedata

MAX_BYTES = 50_000_000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_lines(request):
    given = [k for k in ("text", "lines", "path") if k in request]
    if len(given) != 1:
        fail("give exactly one of text, lines or path")
    if "lines" in request:
        lines = request["lines"]
        if not isinstance(lines, list) or not all(isinstance(line, str) for line in lines):
            fail("lines must be an array of strings")
        return lines, None
    if "text" in request:
        text = request["text"]
        if not isinstance(text, str):
            fail("text must be a string")
    else:
        path = request["path"]
        if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path) or ".." in re.split(r"[\\/]", path):
            fail("path must be a relative path inside the working directory, without '..'")
        target = pathlib.Path(path)
        if not target.is_file():
            fail(f"no such file: {path}")
        if target.stat().st_size > MAX_BYTES:
            fail("the file is larger than 50 MB")
        text = target.read_bytes().decode("utf-8-sig", errors="replace")
    if len(text) > MAX_BYTES:
        fail("the text is larger than 50 MB")
    ending = "\r\n" if "\r\n" in text else "\n"
    lines = text.splitlines()
    return lines, ending


def strip_accents(text):
    return "".join(c for c in unicodedata.normalize("NFD", text) if unicodedata.category(c) != "Mn")


def natural_key(text):
    """Digits compare as numbers: file2 < file10; numbers sort before text at the same place."""
    parts = re.split(r"(\d+)", text)
    key = []
    for index, part in enumerate(parts):
        if index % 2:
            key.append((0, int(part), ""))
        elif part:
            key.append((1, 0, part))
    return key


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    known = {"text", "lines", "path", "dedupe", "ignore_case", "trim", "normalize", "ignore_accents", "sort", "reverse", "drop_empty", "only"}
    unknown = sorted(set(request) - known)
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    flags = {}
    for name, default in (("dedupe", True), ("ignore_case", False), ("trim", False), ("ignore_accents", False), ("reverse", False), ("drop_empty", False)):
        value = request.get(name, default)
        if not isinstance(value, bool):
            fail(f"{name} must be true or false")
        flags[name] = value
    normalize = request.get("normalize", "none")
    if normalize not in ("none", "NFC", "NFKC"):
        fail("normalize must be none, NFC or NFKC")
    sort = request.get("sort", "none")
    if sort not in ("none", "lexical", "natural"):
        fail("sort must be none, lexical or natural")
    only = request.get("only", "all")
    if only not in ("all", "duplicates", "unique"):
        fail("only must be all, duplicates or unique")
    lines, ending = read_lines(request)

    def compare_form(line):
        text = line.strip() if flags["trim"] else line
        if normalize != "none":
            text = unicodedata.normalize(normalize, text)
        if flags["ignore_accents"]:
            text = strip_accents(text)
        if flags["ignore_case"]:
            text = text.casefold()
        return text

    kept, counts, order = {}, {}, []
    empty_dropped = 0
    for position, line in enumerate(lines):
        shown = line.strip() if flags["trim"] else line
        if normalize != "none":
            shown = unicodedata.normalize(normalize, shown)
        if flags["drop_empty"] and not shown.strip():
            empty_dropped += 1
            continue
        key = compare_form(line) if flags["dedupe"] else (position,)
        if key not in kept:
            kept[key] = shown
            counts[key] = 0
            order.append(key)
        counts[key] += 1
    items = [(kept[k], counts[k], compare_form(kept[k])) for k in order]
    if only == "duplicates":
        items = [item for item in items if item[1] > 1]
    elif only == "unique":
        items = [item for item in items if item[1] == 1]
    if sort == "lexical":
        items.sort(key=lambda item: (item[2], item[0]), reverse=flags["reverse"])
    elif sort == "natural":
        items.sort(key=lambda item: (natural_key(item[2]), item[0]), reverse=flags["reverse"])
    elif flags["reverse"]:
        items.reverse()
    out_lines = [item[0] for item in items]
    duplicates = [{"line": line, "count": count} for line, count, _ in sorted(items, key=lambda i: (-i[1], i[0])) if count > 1]
    print(json.dumps({
        "lines": out_lines,
        "text": (ending or "\n").join(out_lines) + ((ending or "\n") if out_lines else ""),
        "counts": [{"line": line, "count": count} for line, count, _ in items],
        "duplicates": duplicates,
        "input_lines": len(lines),
        "output_lines": len(out_lines),
        "removed_duplicates": sum(count - 1 for _, count, _ in items) if flags["dedupe"] and only == "all" else None,
        "empty_dropped": empty_dropped,
        "sort": sort,
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
