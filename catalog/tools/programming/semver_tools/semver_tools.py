#!/usr/bin/env python3
"""Semantic Versioning 2.0.0: parse, compare, sort, bump, and match npm (node-semver) ranges.

Input (JSON on stdin):
  operation  "parse" | "compare" | "sort" | "bump" | "match"
  version    a version string (parse, compare, bump, match)
  other      the second version (compare)
  versions   a list of versions (sort, match)
  release    major | minor | patch | premajor | preminor | prepatch | prerelease | release (bump)
  preid      prerelease identifier for the pre* bumps, e.g. "beta" (optional)
  range      an npm range such as "^1.2.3 || >=2.0.0-rc.1 <2.1" (match)
  include_prerelease  true: prereleases match like normal versions (match; default false)
  descending true: sort newest first (sort)
"""
import functools
import json
import re
import sys

NUM = r"0|[1-9]\d*"
IDENT = r"(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)"
SEMVER = re.compile(
    rf"^(?P<major>{NUM})\.(?P<minor>{NUM})\.(?P<patch>{NUM})"
    rf"(?:-(?P<pre>{IDENT}(?:\.{IDENT})*))?"
    r"(?:\+(?P<build>[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$")
XR = r"x|X|\*|0|[1-9]\d*"
PARTIAL = re.compile(
    rf"^v?(?P<M>{XR})(?:\.(?P<m>{XR})(?:\.(?P<p>{XR})"
    rf"(?:-(?P<pre>{IDENT}(?:\.{IDENT})*))?"
    r"(?:\+(?P<build>[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?)?)?$")
MAX_ITEMS = 10000
RELEASES = ["major", "minor", "patch", "premajor", "preminor", "prepatch", "prerelease", "release"]


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


class Version:
    def __init__(self, major, minor, patch, pre=(), build=()):
        self.major, self.minor, self.patch = major, minor, patch
        self.pre = list(pre)
        self.build = list(build)

    def __str__(self):
        text = f"{self.major}.{self.minor}.{self.patch}"
        if self.pre:
            text += "-" + ".".join(str(p) for p in self.pre)
        return text

    def full(self):
        return str(self) + ("+" + ".".join(self.build) if self.build else "")

    def tuple(self):
        return (self.major, self.minor, self.patch)


def ident(text):
    return int(text) if text.isdigit() else text


def parse_version(text, field):
    if not isinstance(text, str) or not text.strip():
        fail(f"{field} must be a version string")
    raw = text.strip()
    stripped = raw[1:] if raw[:1] in ("v", "V", "=") else raw
    m = SEMVER.match(stripped)
    if not m:
        fail(f"{field} {raw!r} is not a valid SemVer 2.0.0 version (MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD], no leading zeros)")
    pre = [ident(p) for p in m.group("pre").split(".")] if m.group("pre") else []
    build = m.group("build").split(".") if m.group("build") else []
    v = Version(int(m.group("major")), int(m.group("minor")), int(m.group("patch")), pre, build)
    v.prefixed = stripped != raw
    return v


def compare_ids(a, b):
    """SemVer 2.0.0 section 11.4: numeric < alphanumeric; numeric by value; text in ASCII order."""
    if isinstance(a, int) and isinstance(b, int):
        return (a > b) - (a < b)
    if isinstance(a, int):
        return -1
    if isinstance(b, int):
        return 1
    return (a > b) - (a < b)


def compare(a, b):
    if a.tuple() != b.tuple():
        return -1 if a.tuple() < b.tuple() else 1
    if not a.pre and not b.pre:
        return 0
    if not a.pre:
        return 1
    if not b.pre:
        return -1
    for x, y in zip(a.pre, b.pre):
        c = compare_ids(x, y)
        if c:
            return c
    return (len(a.pre) > len(b.pre)) - (len(a.pre) < len(b.pre))


def describe(v):
    return {"version": v.full(), "major": v.major, "minor": v.minor, "patch": v.patch,
            "prerelease": v.pre, "build": v.build, "is_prerelease": bool(v.pre),
            "stable_api": v.major > 0}


def bump(v, release, preid):
    if preid is not None:
        if not isinstance(preid, str) or not re.fullmatch(rf"{IDENT}(?:\.{IDENT})*", preid):
            fail("preid must be a valid prerelease identifier such as beta or rc")
    w = Version(v.major, v.minor, v.patch, v.pre)

    def pre():
        if not w.pre:
            w.pre = [0]
        else:
            for i in range(len(w.pre) - 1, -1, -1):
                if isinstance(w.pre[i], int):
                    w.pre[i] += 1
                    break
            else:
                w.pre.append(0)
        if preid:
            parts = [ident(p) for p in preid.split(".")]
            if w.pre[:len(parts)] == parts:
                if not (len(w.pre) > len(parts) and isinstance(w.pre[len(parts)], int)):
                    w.pre = parts + [0]
            else:
                w.pre = parts + [0]

    if release == "major":
        if w.minor or w.patch or not w.pre:
            w.major += 1
        w.minor = w.patch = 0
        w.pre = []
    elif release == "minor":
        if w.patch or not w.pre:
            w.minor += 1
        w.patch = 0
        w.pre = []
    elif release == "patch":
        if not w.pre:
            w.patch += 1
        w.pre = []
    elif release == "premajor":
        w.major, w.minor, w.patch, w.pre = w.major + 1, 0, 0, []
        pre()
    elif release == "preminor":
        w.minor, w.patch, w.pre = w.minor + 1, 0, []
        pre()
    elif release == "prepatch":
        w.patch, w.pre = w.patch + 1, []
        pre()
    elif release == "prerelease":
        if not w.pre:
            w.patch += 1
        pre()
    elif release == "release":
        if not v.pre:
            fail(f"{v} is not a prerelease")
        w.pre = []
    return w


# ---- ranges (node-semver desugaring) -------------------------------------------------

def isx(part):
    return part is None or part in ("x", "X", "*")


def parse_partial(text, where):
    m = PARTIAL.match(text)
    if not m:
        fail(f"range: {where} {text!r} is not a version or partial version")
    return m.group("M"), m.group("m"), m.group("p"), m.group("pre")


def cmp(op, M, m, p, pre=None):
    text = f"{M}.{m}.{p}" + (f"-{pre}" if pre else "")
    return (op, parse_version(text, "range comparator"))


def desugar_hyphen(left, right, incpr):
    fM, fm, fp, fpr = parse_partial(left, "hyphen start")
    tM, tm, tp, tpr = parse_partial(right, "hyphen end")
    out = []
    z = "0" if incpr else None
    if isx(fM):
        pass
    elif isx(fm):
        out.append(cmp(">=", fM, 0, 0, z))
    elif isx(fp):
        out.append(cmp(">=", fM, fm, 0, z))
    elif fpr:
        out.append(cmp(">=", fM, fm, fp, fpr))
    else:
        out.append(cmp(">=", fM, fm, fp, z))
    if isx(tM):
        pass
    elif isx(tm):
        out.append(cmp("<", int(tM) + 1, 0, 0, "0"))
    elif isx(tp):
        out.append(cmp("<", tM, int(tm) + 1, 0, "0"))
    elif tpr:
        out.append(cmp("<=", tM, tm, tp, tpr))
    elif incpr:
        out.append(cmp("<", tM, tm, int(tp) + 1, "0"))
    else:
        out.append(cmp("<=", tM, tm, tp))
    return out


def desugar_simple(token, incpr):
    z = "0" if incpr else None
    if token[0] == "~":
        rest = token[1:].lstrip(">").lstrip("=").strip() if token.startswith("~>") else token[1:].strip()
        M, m, p, pr = parse_partial(rest, "tilde")
        if isx(M):
            return []
        if isx(m):
            return [cmp(">=", M, 0, 0, z), cmp("<", int(M) + 1, 0, 0, "0")]
        if isx(p):
            return [cmp(">=", M, m, 0, z), cmp("<", M, int(m) + 1, 0, "0")]
        return [cmp(">=", M, m, p, pr), cmp("<", M, int(m) + 1, 0, "0")]
    if token[0] == "^":
        M, m, p, pr = parse_partial(token[1:].strip(), "caret")
        if isx(M):
            return []
        if isx(m):
            return [cmp(">=", M, 0, 0, z), cmp("<", int(M) + 1, 0, 0, "0")]
        if isx(p):
            if M == "0":
                return [cmp(">=", M, m, 0, z), cmp("<", M, int(m) + 1, 0, "0")]
            return [cmp(">=", M, m, 0, z), cmp("<", int(M) + 1, 0, 0, "0")]
        if M == "0":
            if m == "0":
                return [cmp(">=", M, m, p, pr), cmp("<", M, m, int(p) + 1, "0")]
            return [cmp(">=", M, m, p, pr), cmp("<", M, int(m) + 1, 0, "0")]
        return [cmp(">=", M, m, p, pr), cmp("<", int(M) + 1, 0, 0, "0")]
    op_match = re.match(r"^(<=|>=|<|>|=)?\s*(.*)$", token)
    op, rest = op_match.group(1) or "", op_match.group(2)
    M, m, p, pr = parse_partial(rest, "comparator")
    xM, xm = isx(M), isx(M) or isx(m)
    anyx = xm or isx(p)
    if op == "=" and anyx:
        op = ""
    if xM:
        if op in (">", "<"):
            return [("never", None)]
        return []
    if op and anyx:
        m = 0 if xm else m
        p = 0
        pre = z
        if op == ">":
            op = ">="
            if xm:
                M, m = int(M) + 1, 0
            else:
                m = int(m) + 1
        elif op == "<=":
            op = "<"
            if xm:
                M = int(M) + 1
            else:
                m = int(m) + 1
        if op == "<":
            pre = "0"
        return [cmp(op, M, m, p, pre)]
    if xm:
        return [cmp(">=", M, 0, 0, z), cmp("<", int(M) + 1, 0, 0, "0")]
    if isx(p):
        return [cmp(">=", M, m, 0, z), cmp("<", M, int(m) + 1, 0, "0")]
    return [cmp(op or "=", M, m, p, pr)]


def parse_range(text, incpr):
    if not isinstance(text, str):
        fail("range must be a string")
    if len(text) > 4096:
        fail("range is longer than 4096 characters")
    sets = []
    for part in text.split("||"):
        part = part.strip()
        hy = re.match(r"^(\S+)\s+-\s+(\S+)$", part)
        if hy:
            comps = desugar_hyphen(hy.group(1), hy.group(2), incpr)
        else:
            # glue operators to their version: ">= 1.2.3" -> ">=1.2.3", "~ 1.2" -> "~1.2"
            glued = re.sub(r"(<=|>=|<|>|=|~>|~|\^)\s+", r"\1", part)
            comps = []
            for token in glued.split():
                comps.extend(desugar_simple(token, incpr))
        if any(c[0] == "never" for c in comps):
            comps = [("never", None)]
        sets.append(comps)
    return sets


def format_set(comps):
    if not comps:
        return "*"
    if comps[0][0] == "never":
        return "<0.0.0-0"
    return " ".join(op + str(v) for op, v in comps)


def test_comp(op, ref, v):
    c = compare(v, ref)
    return {"=": c == 0, "<": c < 0, "<=": c <= 0, ">": c > 0, ">=": c >= 0}[op]


def test_set(comps, v, incpr):
    if comps and comps[0][0] == "never":
        return False
    if not all(test_comp(op, ref, v) for op, ref in comps):
        return False
    if v.pre and not incpr:
        return any(ref.pre and ref.tuple() == v.tuple() for _, ref in comps)
    return True


def satisfies(sets, v, incpr):
    return any(test_set(s, v, incpr) for s in sets)


def versions_list(request):
    versions = request.get("versions")
    if not isinstance(versions, list) or not versions:
        fail("versions must be a non-empty list of version strings")
    if len(versions) > MAX_ITEMS:
        fail(f"versions has more than {MAX_ITEMS} items")
    return [parse_version(v, f"versions[{i}]") for i, v in enumerate(versions)]


def sort_versions(vs, descending=False):
    return sorted(vs, key=functools.cmp_to_key(compare), reverse=descending)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    known = {"operation", "version", "other", "versions", "release", "preid", "range", "include_prerelease", "descending"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    operation = request.get("operation")
    if operation == "parse":
        v = parse_version(request.get("version"), "version")
        result = {"operation": "parse", "valid": True, **describe(v)}
        if v.prefixed:
            result["note"] = "a leading v or = was removed; it is not part of SemVer 2.0.0"
    elif operation == "compare":
        a = parse_version(request.get("version"), "version")
        b = parse_version(request.get("other"), "other")
        c = compare(a, b)
        result = {"operation": "compare", "version": a.full(), "other": b.full(), "result": c,
                  "relation": {-1: "less", 0: "equal", 1: "greater"}[c],
                  "build_ignored": bool(a.build or b.build)}
    elif operation == "sort":
        descending = request.get("descending", False)
        if not isinstance(descending, bool):
            fail("descending must be true or false")
        vs = sort_versions(versions_list(request), descending)
        result = {"operation": "sort", "order": "descending" if descending else "ascending",
                  "sorted": [v.full() for v in vs]}
    elif operation == "bump":
        v = parse_version(request.get("version"), "version")
        release = request.get("release")
        if release not in RELEASES:
            fail(f"release must be one of {', '.join(RELEASES)}")
        w = bump(v, release, request.get("preid"))
        result = {"operation": "bump", "version": v.full(), "release": release, "result": str(w)}
        if v.build:
            result["note"] = "build metadata is dropped by a bump"
    elif operation == "match":
        incpr = request.get("include_prerelease", False)
        if not isinstance(incpr, bool):
            fail("include_prerelease must be true or false")
        rng = request.get("range")
        sets = parse_range(rng, incpr)
        if "versions" in request:
            vs = versions_list(request)
        elif "version" in request:
            vs = [parse_version(request.get("version"), "version")]
        else:
            fail("match needs version or versions")
        matched = [v for v in vs if satisfies(sets, v, incpr)]
        ordered = sort_versions(matched)
        result = {"operation": "match", "range": rng, "desugared": " || ".join(format_set(s) for s in sets),
                  "include_prerelease": incpr,
                  "results": [{"version": v.full(), "satisfies": satisfies(sets, v, incpr)} for v in vs],
                  "max_satisfying": ordered[-1].full() if ordered else None,
                  "min_satisfying": ordered[0].full() if ordered else None}
        if len(vs) == 1:
            result["satisfies"] = bool(matched)
    else:
        fail("operation must be parse, compare, sort, bump or match")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
