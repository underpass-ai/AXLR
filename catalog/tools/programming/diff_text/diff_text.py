#!/usr/bin/env python3
"""Compare two texts: unified diff, side-by-side rows, word-level diff with inline markers and
similarity ratios (Python difflib).

Input (JSON on stdin):
  a, b            the two texts (old and new)
  from_name       label of a in the unified header (default "a")
  to_name         label of b (default "b")
  context         context lines around each hunk (default 3)
  side_by_side    true: add side-by-side rows and a rendered table
  width           column width of the rendered side-by-side table (default 40)
  words           true: add a word-level diff with [-removed-]{+added+} markers
"""
import difflib
import json
import re
import sys
from fractions import Fraction

MAX_TEXT = 200_000
CHAR_RATIO_LIMIT = 20_000
NO_EOL = "\\ No newline at end of file\n"
KNOWN = {"a", "b", "from_name", "to_name", "context", "side_by_side", "width", "words"}


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def ratio(matcher):
    total = len(matcher.a) + len(matcher.b)
    matched = sum(block.size for block in matcher.get_matching_blocks())
    exact = Fraction(2 * matched, total) if total else Fraction(1)
    return {"exact": f"{exact.numerator}/{exact.denominator}", "value": round(float(exact), 6),
            "matched": matched, "total": total}


def unified(a_lines, b_lines, from_name, to_name, context):
    out = []
    for line in difflib.unified_diff(a_lines, b_lines, fromfile=from_name, tofile=to_name, n=context):
        if line.startswith(("---", "+++", "@@")):
            out.append(line if line.endswith("\n") else line + "\n")
        elif line.endswith("\n"):
            out.append(line)
        else:
            out.append(line + "\n" + NO_EOL)
    return "".join(out)


def side_rows(a_lines, b_lines):
    rows = []
    sm = difflib.SequenceMatcher(None, a_lines, b_lines, autojunk=False)
    strip = lambda s: s.rstrip("\n")
    for tag, i1, i2, j1, j2 in sm.get_opcodes():
        if tag == "equal":
            for k in range(i2 - i1):
                rows.append({"left_line": i1 + k + 1, "left": strip(a_lines[i1 + k]), "op": " ",
                             "right_line": j1 + k + 1, "right": strip(b_lines[j1 + k])})
        else:
            n = max(i2 - i1, j2 - j1)
            for k in range(n):
                li, rj = i1 + k, j1 + k
                left = li < i2
                right = rj < j2
                op = "|" if left and right else ("<" if left else ">")
                rows.append({"left_line": li + 1 if left else None, "left": strip(a_lines[li]) if left else None,
                             "op": op,
                             "right_line": rj + 1 if right else None, "right": strip(b_lines[rj]) if right else None})
    return rows


def render(rows, width):
    def cell(text):
        text = "" if text is None else text.expandtabs(4)
        return (text[:width - 1] + "…") if len(text) > width else text.ljust(width)
    lines = []
    for r in rows:
        ln = f"{r['left_line']:>4}" if r["left_line"] else "    "
        rn = f"{r['right_line']:>4}" if r["right_line"] else "    "
        lines.append(f"{ln} {cell(r['left'])} {r['op']} {rn} {cell(r['right'])}".rstrip())
    return "\n".join(lines) + ("\n" if lines else "")


TOKEN = re.compile(r"\s+|\w+|[^\w\s]", re.UNICODE)


def word_diff(a, b):
    ta, tb = TOKEN.findall(a), TOKEN.findall(b)
    sm = difflib.SequenceMatcher(None, ta, tb, autojunk=False)
    inline, changes = [], []
    for tag, i1, i2, j1, j2 in sm.get_opcodes():
        old, new = "".join(ta[i1:i2]), "".join(tb[j1:j2])
        if tag == "equal":
            inline.append(old)
            continue
        if old:
            inline.append(f"[-{old}-]")
        if new:
            inline.append(f"{{+{new}+}}")
        changes.append({"op": {"replace": "replace", "delete": "delete", "insert": "insert"}[tag],
                        "old": old, "new": new})
    return {"inline": "".join(inline), "changes": changes, "ratio": ratio(sm)}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - KNOWN)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    a, b = request.get("a"), request.get("b")
    if not isinstance(a, str) or not isinstance(b, str):
        fail("a and b must be strings (the two texts)")
    if len(a) > MAX_TEXT or len(b) > MAX_TEXT:
        fail(f"each text must be at most {MAX_TEXT} characters")
    from_name, to_name = request.get("from_name", "a"), request.get("to_name", "b")
    if not isinstance(from_name, str) or not isinstance(to_name, str) or "\n" in from_name + to_name:
        fail("from_name and to_name must be one-line strings")
    context = request.get("context", 3)
    if not isinstance(context, int) or isinstance(context, bool) or not 0 <= context <= 1000:
        fail("context must be an integer from 0 to 1000")
    width = request.get("width", 40)
    if not isinstance(width, int) or isinstance(width, bool) or not 8 <= width <= 200:
        fail("width must be an integer from 8 to 200")
    for key in ("side_by_side", "words"):
        if not isinstance(request.get(key, False), bool):
            fail(f"{key} must be true or false")

    a_lines, b_lines = a.splitlines(keepends=True), b.splitlines(keepends=True)
    lines_sm = difflib.SequenceMatcher(None, a_lines, b_lines, autojunk=False)
    added = removed = 0
    for tag, i1, i2, j1, j2 in lines_sm.get_opcodes():
        if tag != "equal":
            removed += i2 - i1
            added += j2 - j1
    diff = unified(a_lines, b_lines, from_name, to_name, context)
    result = {"identical": a == b, "unified": diff, "hunks": sum(1 for l in diff.splitlines() if l.startswith("@@")),
              "lines": {"a": len(a_lines), "b": len(b_lines), "added": added, "removed": removed},
              "line_ratio": ratio(lines_sm)}
    if len(a) <= CHAR_RATIO_LIMIT and len(b) <= CHAR_RATIO_LIMIT:
        result["char_ratio"] = ratio(difflib.SequenceMatcher(None, a, b, autojunk=False))
    else:
        result["char_ratio"] = None
        result["note"] = f"char_ratio is only computed for texts up to {CHAR_RATIO_LIMIT} characters"
    if request.get("side_by_side"):
        rows = side_rows(a_lines, b_lines)
        result["side_by_side"] = {"rows": rows, "rendered": render(rows, width)}
    if request.get("words"):
        result["words"] = word_diff(a, b)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
