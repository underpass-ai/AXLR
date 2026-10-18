#!/usr/bin/env python3
"""Write a .env.example from a .env text: every value blanked or replaced by a type hint
(<url>, <number>, <secret>...), comments, blank lines, `export` and quoting kept, duplicate
keys and secret-looking entries flagged. No value of the input is ever printed.

Input (JSON on stdin):
  text   the .env content
  hints  true (default): replace each value by a type hint; false: leave it empty
"""
import json
import math
import re
import sys

MAX_TEXT = 500_000
ASSIGN = re.compile(r"^(?P<indent>\s*)(?P<export>export\s+)?(?P<key>[A-Za-z_][A-Za-z0-9_.\-]*)(?P<eq>\s*=\s*)(?P<rest>.*)$")
COMMENTED = re.compile(r"^(?P<hash>\s*#+\s*)(?P<export>export\s+)?(?P<key>[A-Za-z_][A-Za-z0-9_]*)(?P<eq>\s*=\s*)(?P<rest>.*)$")
SECRET_KEY = re.compile(r"(SECRET|PASSW(OR)?D|\bPWD\b|_PWD$|^PWD|TOKEN|API_?KEY|ACCESS_?KEY|PRIVATE|CREDENTIAL|"
                        r"AUTH|SIGNING|ENCRYPTION|SALT|PASSPHRASE|_KEY$|^KEY$|DSN$|WEBHOOK)", re.I)
TOKEN_PATTERNS = [
    ("an AWS access key id", re.compile(r"\b(AKIA|ASIA)[0-9A-Z]{16}\b")),
    ("a GitHub token", re.compile(r"\b(gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})")),
    ("a Slack token", re.compile(r"\bxox[abprs]-[A-Za-z0-9-]{10,}")),
    ("an OpenAI/Anthropic-style API key", re.compile(r"\bsk-(ant-)?[A-Za-z0-9_\-]{20,}")),
    ("a Stripe live key", re.compile(r"\b[sr]k_live_[A-Za-z0-9]{16,}")),
    ("a Google API key", re.compile(r"\bAIza[0-9A-Za-z_\-]{35}\b")),
    ("a JSON Web Token", re.compile(r"\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}")),
    ("a PEM private key", re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")),
]
URL_CRED = re.compile(r"^[A-Za-z][A-Za-z0-9+.\-]*://[^/\s:@]+:[^/\s@]+@")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def entropy(value):
    counts = {}
    for ch in value:
        counts[ch] = counts.get(ch, 0) + 1
    return -sum(c / len(value) * math.log2(c / len(value)) for c in counts.values())


def split_value(rest):
    """Return (quote, value, tail, closed) for the text after '='. tail is an inline comment."""
    if rest[:1] in ("'", '"', "`"):
        q = rest[0]
        i = 1
        while i < len(rest):
            if rest[i] == "\\" and q == '"':
                i += 2
                continue
            if rest[i] == q:
                return q, rest[1:i], rest[i + 1:], True
            i += 1
        return q, rest[1:], "", False
    m = re.search(r"\s+#", rest)
    if m:
        return "", rest[:m.start()], rest[m.start():], True
    return "", rest.rstrip(), "", True


def kind(key, value):
    """A hint and the flags for one value; the value itself is never returned."""
    flags = []
    v = value.strip()
    for label, rx in TOKEN_PATTERNS:
        if rx.search(v):
            flags.append(f"value looks like {label}")
    if URL_CRED.match(v):
        flags.append("URL with an embedded user and password")
    secret_name = bool(SECRET_KEY.search(key))
    if not v:
        return "", flags
    if not flags and ("${" in v or re.search(r"(^|[^\\])\$[A-Za-z_]", v)):
        return "<interpolated>", flags
    if secret_name:
        flags.append("secret-looking key with a value (rotate it if this file was ever shared or committed)")
    if flags or secret_name:
        return "<secret>", flags
    if re.fullmatch(r"[A-Za-z][A-Za-z0-9+.\-]*://\S+", v):
        return "<url>", flags
    if re.fullmatch(r"[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?", v):
        return "<number>", flags
    if v.lower() in ("true", "false", "yes", "no", "on", "off"):
        return "<boolean>", flags
    if re.fullmatch(r"[^@\s]+@[^@\s]+\.[A-Za-z]{2,}", v):
        return "<email>", flags
    if re.fullmatch(r"(\d{1,3}\.){3}\d{1,3}(:\d+)?|[A-Za-z0-9\-]+(\.[A-Za-z0-9\-]+)+:\d+|localhost(:\d+)?", v):
        return "<host>", flags
    if re.fullmatch(r"(\.{0,2}/|~/)\S*", v):
        return "<path>", flags
    if len(v) >= 24 and " " not in v and entropy(v) >= 4.0:
        flags.append("long random-looking value")
        return "<secret>", flags
    return "<string>", flags


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - {"text", "hints"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str):
        fail("text must be a string with the .env content")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    hints = request.get("hints", True)
    if not isinstance(hints, bool):
        fail("hints must be true or false")

    lines = text.splitlines()
    out, variables, notes = [], [], []
    seen = {}
    i = 0
    while i < len(lines):
        number, line = i + 1, lines[i]
        i += 1
        stripped = line.strip()
        if not stripped:
            out.append(line)
            continue
        m = ASSIGN.match(line)
        commented = False
        if not m and stripped.startswith("#"):
            m = COMMENTED.match(line)
            if not m:
                out.append(line)
                continue
            commented = True
        if not m:
            out.append(f"# (line {number} removed: not a KEY=value assignment)")
            notes.append({"line": number, "issue": "not a KEY=value assignment; removed so no value can leak"})
            continue
        key = m.group("key")
        quote, value, tail, closed = split_value(m.group("rest"))
        flags = []
        if quote and not closed:
            # a multi-line quoted value: consume until the closing quote
            parts = [value]
            while i < len(lines):
                nxt = lines[i]
                i += 1
                end = nxt.find(quote)
                if end >= 0:
                    parts.append(nxt[:end])
                    tail = nxt[end + 1:]
                    closed = True
                    break
                parts.append(nxt)
            value = "\n".join(parts)
            flags.append("multi-line value" if closed else "unterminated quote: the rest of the file was read as its value")
        if tail and not re.match(r"\s*(#.*)?$", tail):
            flags.append("text after the closing quote was dropped")
            tail = ""
        hint, value_flags = kind(key, value)
        flags += value_flags
        placeholder = hint if hints else ""
        lead = m.group("hash") if commented else m.group("indent")
        export = m.group("export") or ""
        out.append(f"{lead}{export}{key}{m.group('eq').rstrip() if not placeholder and not quote else m.group('eq')}"
                   f"{quote}{placeholder}{quote}{tail}")
        if commented:
            flags.append("commented-out assignment: its value was removed too")
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
            flags.append("key is not a portable shell variable name")
        entry = {"key": key, "line": number, "hint": hint or "<empty>", "export": bool(export),
                 "quote": {"'": "single", '"': "double", "`": "backtick"}.get(quote, "none"), "commented": commented}
        if flags:
            entry["flags"] = flags
        variables.append(entry)
        if not commented:
            seen.setdefault(key, []).append(number)
    duplicates = [{"key": k, "lines": v, "note": "most loaders keep the last one"} for k, v in seen.items() if len(v) > 1]
    suspicious = [{"key": e["key"], "line": e["line"], "flags": [f for f in e["flags"] if "secret" in f or "looks like" in f
                                                                   or "password" in f or "random" in f]}
                  for e in variables if e.get("flags")]
    suspicious = [s for s in suspicious if s["flags"]]
    example = "\n".join(out) + ("\n" if text.endswith(("\n", "\r")) or not out else "")
    result = {"example": example, "variables": variables, "count": len([v for v in variables if not v["commented"]]),
              "duplicates": duplicates, "suspicious": suspicious, "mode": "hints" if hints else "blank"}
    if notes:
        result["removed_lines"] = notes
    print(json.dumps(result))


if __name__ == "__main__":
    main()
