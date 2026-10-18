#!/usr/bin/env python3
"""Scan text for key-like strings with rules modelled on gitleaks' default configuration
(AWS access key ids, GitHub, GitLab, Slack, Stripe, Google, npm tokens, private-key PEM
headers, JWTs and generic high-entropy assignments), reporting each finding with its rule,
place and Shannon entropy and the secret REDACTED to its first 4 characters.

Input (JSON on stdin):
  text                 text to scan (inline)
  files                {name: content} more texts to scan, inline
  paths                relative paths of files in the working directory to scan
  entropy_threshold    minimum Shannon entropy (bits per character) for the generic rule, default 3.5
  rules                restrict to these rule ids (default: all)
  include_examples     keep matches that look like documented placeholders (EXAMPLE, xxxx...), default true
"""
import bisect
import json
import math
import pathlib
import re
import sys

MAX_BYTES = 5_000_000

# id, description, pattern, group holding the secret, minimum entropy (None = no check)
RULES = [
    ("aws-access-token", "AWS access key id", r"\b((?:A3T[A-Z0-9]|AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16})\b", 1, 3.0),
    ("aws-secret-access-key", "AWS secret access key next to an aws/secret keyword",
     r"(?i)aws.{0,20}?(?:secret|key|sec).{0,20}?['\"]?\s*(?:=|:|=>)\s*['\"]?([A-Za-z0-9/+=]{40})(?![A-Za-z0-9/+=])", 1, 3.0),
    ("github-pat", "GitHub personal access token (classic)", r"\b(ghp_[0-9a-zA-Z]{36})\b", 1, 3.0),
    ("github-fine-grained-pat", "GitHub fine-grained personal access token", r"\b(github_pat_\w{82})\b", 1, 3.0),
    ("github-oauth", "GitHub OAuth access token", r"\b(gho_[0-9a-zA-Z]{36})\b", 1, 3.0),
    ("github-app-token", "GitHub app token", r"\b((?:ghu|ghs)_[0-9a-zA-Z]{36})\b", 1, 3.0),
    ("github-refresh-token", "GitHub refresh token", r"\b(ghr_[0-9a-zA-Z]{36})\b", 1, 3.0),
    ("gitlab-pat", "GitLab personal access token", r"\b(glpat-[\w-]{20})(?![\w-])", 1, 3.0),
    ("slack-bot-token", "Slack bot token", r"\b(xoxb-[0-9]{10,13}-[0-9]{10,13}[a-zA-Z0-9-]*)", 1, None),
    ("slack-user-token", "Slack user token", r"\b(xox[pe](?:-[0-9]{10,13}){3}-[a-zA-Z0-9-]{28,34})", 1, None),
    ("slack-app-token", "Slack app-level token", r"(?i)\b(xapp-\d-[A-Z0-9]+-\d+-[a-z0-9]+)", 1, None),
    ("slack-legacy-token", "Slack legacy or other token", r"\b(xox[aors]-(?:\d+-)+[a-z0-9]+)", 1, None),
    ("slack-webhook-url", "Slack incoming webhook URL", r"(https?://hooks\.slack\.com/(?:services|workflows|triggers)/[A-Za-z0-9+/]{43,56})", 1, None),
    ("stripe-access-token", "Stripe secret or restricted key", r"\b((?:sk|rk)_(?:test|live|prod)_[a-zA-Z0-9]{10,99})\b", 1, 2.0),
    ("gcp-api-key", "Google API key", r"\b(AIza[\w-]{35})(?![\w-])", 1, 3.0),
    ("npm-access-token", "npm access token", r"\b(npm_[a-z0-9]{36})\b", 1, 2.0),
    ("private-key", "Private key (PEM header)", r"(-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----)", 1, None),
    ("jwt", "JSON Web Token", r"\b(ey[a-zA-Z0-9]{17,}\.ey[a-zA-Z0-9/\\_-]{17,}\.(?:[a-zA-Z0-9/\\_-]{10,}={0,2})?)", 1, 3.0),
    ("generic-api-key", "Generic secret assigned to a key, token, secret or password name",
     r"(?i)\b[\w.-]{0,40}?(?:access|auth|api|credential|creds|key|passw(?:or)?d|secret|token)[\w.-]{0,20}[\s'\"]{0,3}"
     r"(?:=|:{1,3}=?|\|\|:|<=|=>|\?=)(?:[\s'\"]|\\[\"'])*?([\w.=+/~@-]{10,150})(?:[\s'\";,`]|\\[\"']|$)", 1, "threshold"),
]
PLACEHOLDER = re.compile(r"(?i)example|xxxx|placeholder|changeme|your[_-]?(?:api|secret|token|key)|dummy|fake|sample|<[^>]+>|\$\{|\{\{")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def shannon(text):
    """Bits per character of the string's own character distribution."""
    if not text:
        return 0.0
    counts = {}
    for char in text:
        counts[char] = counts.get(char, 0) + 1
    return -sum(c / len(text) * math.log2(c / len(text)) for c in counts.values())


def redact(hit):
    return hit[:4] + "*" * min(max(len(hit) - 4, 0), 12) + f" ({len(hit)} chars)"


def read_relative(path):
    if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path):
        fail("paths must be relative paths inside the working directory")
    parts = re.split(r"[\\/]", path)
    if ".." in parts:
        fail("paths must not contain '..'")
    base = pathlib.Path.cwd().resolve()
    target = (base / path).resolve()
    if base not in target.parents and target != base:
        fail("paths must stay inside the working directory")
    if not target.is_file():
        fail(f"no such file: {path}")
    if target.stat().st_size > MAX_BYTES:
        fail(f"{path} is larger than 5 MB")
    return target.read_bytes().decode("utf-8", errors="replace")


def scan(name, text, rules, threshold, include_examples, findings, skipped):
    line_starts = [0] + [m.end() for m in re.finditer(r"\n", text)]
    seen = set()
    for rule_id, description, pattern, group, minimum in rules:
        for match in re.finditer(pattern, text):
            hit = match.group(group)
            start = match.start(group)
            entropy = shannon(hit)
            limit = threshold if minimum == "threshold" else minimum
            if limit is not None and entropy < limit:
                continue
            if rule_id == "generic-api-key":
                # Skip what another rule already reported, and values that are plainly not secrets.
                if any(s <= start < e for s, e in seen) or re.fullmatch(r"[a-z]+(?:[._-][a-z]+)*", hit) or re.fullmatch(r"[\d.]+", hit):
                    continue
                if not re.search(r"\d", hit) and not re.search(r"[A-Z]", hit):
                    continue
            key = (start, start + len(hit))
            if any(s <= start < e for s, e in seen):
                continue
            seen.add(key)
            placeholder = bool(PLACEHOLDER.search(hit))
            if placeholder and not include_examples:
                skipped["placeholders"] += 1
                continue
            line = bisect.bisect_right(line_starts, start) - 1
            findings.append({
                "rule": rule_id, "description": description, "source": name,
                "line": line + 1, "column": start - line_starts[line] + 1,
                "secret_redacted": redact(hit), "length": len(hit), "entropy": round(entropy, 3),
                "looks_like_placeholder": placeholder,
                "fingerprint": f"{name}:{rule_id}:{line + 1}",
            })


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"text", "files", "paths", "entropy_threshold", "rules", "include_examples"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    sources = []
    if "text" in request:
        if not isinstance(request["text"], str):
            fail("text must be a string")
        sources.append(("text", request["text"]))
    files = request.get("files", {})
    if not isinstance(files, dict) or not all(isinstance(v, str) for v in files.values()):
        fail("files must be an object {name: content}")
    sources += sorted(files.items())
    paths = request.get("paths", [])
    if not isinstance(paths, list):
        fail("paths must be a list of relative paths")
    for path in paths:
        sources.append((path, read_relative(path)))
    if not sources:
        fail("give text, files or paths to scan")
    if sum(len(content) for _, content in sources) > MAX_BYTES:
        fail("the input is larger than 5 MB")
    threshold = request.get("entropy_threshold", 3.5)
    if isinstance(threshold, bool) or not isinstance(threshold, (int, float)) or not 0 <= threshold <= 8:
        fail("entropy_threshold must be a number between 0 and 8")
    include_examples = request.get("include_examples", True)
    if not isinstance(include_examples, bool):
        fail("include_examples must be true or false")
    ids = [rule[0] for rule in RULES]
    wanted = request.get("rules", ids)
    if not isinstance(wanted, list) or not wanted or any(r not in ids for r in wanted):
        fail("rules must be a non-empty list of: " + ", ".join(ids))
    rules = [rule for rule in RULES if rule[0] in wanted]

    findings, skipped = [], {"placeholders": 0}
    for name, content in sources:
        scan(name, content, rules, threshold, include_examples, findings, skipped)
    findings.sort(key=lambda f: (f["source"], f["line"], f["column"], f["rule"]))
    by_rule = {}
    for finding in findings:
        by_rule[finding["rule"]] = by_rule.get(finding["rule"], 0) + 1
    print(json.dumps({
        "findings": findings,
        "count": len(findings),
        "by_rule": dict(sorted(by_rule.items())),
        "sources_scanned": [name for name, _ in sources],
        "skipped": skipped,
        "entropy_threshold": threshold,
        "rules": [{"id": r[0], "description": r[1]} for r in rules],
        "note": "Secrets are redacted to their first 4 characters. Rotate any real secret found: removing it from the file does not remove it from history.",
    }))


if __name__ == "__main__":
    main()
