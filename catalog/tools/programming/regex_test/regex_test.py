#!/usr/bin/env python3
"""Run a Python regular expression over a text: matches with groups, named groups and spans,
a substitution preview and a split, under a hard time limit.

Input (JSON on stdin):
  pattern       the regular expression (Python `re` syntax)
  text          the subject text
  flags         list of IGNORECASE/i, MULTILINE/m, DOTALL/s, VERBOSE/x, ASCII/a (optional)
  anchor        "search" (default: every match anywhere), "match" (at the start) or "fullmatch"
  max_matches   at most this many matches are listed (default 1000, at most 10000)
  replacement   a re.sub template (\\1, \\g<name>) for a substitution preview (optional)
  count         maximum substitutions (default 0 = all)
  split         true: also split the text on the pattern
  maxsplit      maximum splits (default 0 = all)
  timeout       seconds, 0.1 to 10 (default 2)
"""
import json
import re
import signal
import sys

FLAGS = {"IGNORECASE": re.IGNORECASE, "I": re.IGNORECASE, "MULTILINE": re.MULTILINE, "M": re.MULTILINE,
         "DOTALL": re.DOTALL, "S": re.DOTALL, "VERBOSE": re.VERBOSE, "X": re.VERBOSE,
         "ASCII": re.ASCII, "A": re.ASCII}
CANON = {re.IGNORECASE: "IGNORECASE", re.MULTILINE: "MULTILINE", re.DOTALL: "DOTALL", re.VERBOSE: "VERBOSE",
         re.ASCII: "ASCII"}
MAX_PATTERN = 10000
MAX_TEXT = 200_000
KNOWN = {"pattern", "text", "flags", "anchor", "max_matches", "replacement", "count", "split", "maxsplit", "timeout"}


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


class Timeout(Exception):
    pass


def on_alarm(signum, frame):
    raise Timeout()


def integer(request, key, default, low, high):
    value = request.get(key, default)
    if not isinstance(value, int) or isinstance(value, bool) or not low <= value <= high:
        fail(f"{key} must be an integer from {low} to {high}")
    return value


def describe(m, names):
    item = {"match": m.group(0), "span": list(m.span()), "groups": list(m.groups()),
            "group_spans": [list(m.span(i)) if m.group(i) is not None else None for i in range(1, (m.re.groups or 0) + 1)]}
    if names:
        item["named"] = m.groupdict()
    return item


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
    pattern, text = request.get("pattern"), request.get("text")
    if not isinstance(pattern, str):
        fail("pattern must be a string")
    if not isinstance(text, str):
        fail("text must be a string")
    if len(pattern) > MAX_PATTERN:
        fail(f"pattern is longer than {MAX_PATTERN} characters")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    flags = request.get("flags", [])
    if not isinstance(flags, list) or not all(isinstance(f, str) for f in flags):
        fail("flags must be a list of flag names")
    value = 0
    for f in flags:
        key = f.upper().replace("RE.", "")
        if key not in FLAGS:
            fail(f"unknown flag {f!r}; use IGNORECASE, MULTILINE, DOTALL, VERBOSE or ASCII (or i, m, s, x, a)")
        value |= FLAGS[key]
    anchor = request.get("anchor", "search")
    if anchor not in ("search", "match", "fullmatch"):
        fail("anchor must be search, match or fullmatch")
    max_matches = integer(request, "max_matches", 1000, 1, 10000)
    count = integer(request, "count", 0, 0, 10_000_000)
    maxsplit = integer(request, "maxsplit", 0, 0, 10_000_000)
    replacement = request.get("replacement")
    if replacement is not None and not isinstance(replacement, str):
        fail("replacement must be a string")
    do_split = request.get("split", False)
    if not isinstance(do_split, bool):
        fail("split must be true or false")
    timeout = request.get("timeout", 2)
    if isinstance(timeout, bool) or not isinstance(timeout, (int, float)) or not 0.1 <= timeout <= 10:
        fail("timeout must be a number of seconds from 0.1 to 10")

    signal.signal(signal.SIGALRM, on_alarm)
    signal.setitimer(signal.ITIMER_REAL, float(timeout))
    stage = "compiling the pattern"
    try:
        try:
            rx = re.compile(pattern, value)
        except re.error as error:
            where = f" at position {error.pos}" if error.pos is not None else ""
            fail(f"invalid pattern{where}: {error.msg}")
        names = sorted(rx.groupindex, key=rx.groupindex.get)
        stage = "matching"
        matches, truncated = [], False
        if anchor == "search":
            for m in rx.finditer(text):
                if len(matches) == max_matches:
                    truncated = True
                    break
                matches.append(describe(m, names))
        else:
            m = rx.match(text) if anchor == "match" else rx.fullmatch(text)
            if m:
                matches.append(describe(m, names))
        result = {"pattern": pattern, "flags": sorted({CANON[v] for k, v in FLAGS.items() if value & v}),
                  "anchor": anchor, "groups": rx.groups, "group_names": names,
                  "match_count": len(matches), "matched": bool(matches), "matches": matches}
        if truncated:
            result["truncated"] = f"only the first {max_matches} matches are listed"
        if replacement is not None:
            stage = "substituting"
            try:
                new, n = rx.subn(replacement, text, count=count)
            except (re.error, IndexError) as error:
                fail(f"invalid replacement: {error}")
            result["substitution"] = {"replacement": replacement, "result": new, "replacements": n}
        if do_split:
            stage = "splitting"
            result["split"] = rx.split(text, maxsplit=maxsplit)
    except Timeout:
        fail(f"timed out after {timeout} s while {stage}: the pattern probably backtracks catastrophically "
             f"(nested quantifiers such as (a+)+ or (a|a)*); make the inner repetition unambiguous or possessive")
    except (OverflowError, RecursionError, MemoryError) as error:
        fail(f"the pattern is too large to run: {type(error).__name__}")
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
