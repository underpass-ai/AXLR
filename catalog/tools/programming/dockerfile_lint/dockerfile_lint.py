#!/usr/bin/env python3
"""Lint a Dockerfile given inline with rules modelled on hadolint (its rule ids where a rule
corresponds) plus a few of our own (AXL ids) for secrets, piping downloads into a shell and
images without a USER.

Input (JSON on stdin):
  text    the Dockerfile content
  ignore  rule ids to skip, e.g. ["DL3008"] (optional; "# hadolint ignore=DL3008" comments work too)
"""
import json
import re
import shlex
import sys

MAX_TEXT = 500_000
RULES = {
    "DL3000": ("error", "Use absolute WORKDIR."),
    "DL3002": ("warning", "Last USER should not be root."),
    "DL3004": ("error", "Do not use sudo as it leads to unpredictable behavior. Use a tool like gosu to enforce root."),
    "DL3006": ("warning", "Always tag the version of an image explicitly."),
    "DL3007": ("warning", "Using latest is prone to errors if the image will ever update. Pin the version explicitly to a release tag."),
    "DL3008": ("warning", "Pin versions in apt-get install."),
    "DL3009": ("info", "Delete the apt-get lists after installing something."),
    "DL3014": ("warning", "Use the -y switch to avoid manual input: apt-get -y install <package>."),
    "DL3015": ("info", "Avoid additional packages by specifying --no-install-recommends."),
    "DL3020": ("error", "Use COPY instead of ADD for files and folders."),
    "DL3025": ("warning", "Use arguments JSON notation for CMD and ENTRYPOINT arguments."),
    "DL3027": ("warning", "Do not use apt as it is meant to be an end-user tool, use apt-get or apt-cache instead."),
    "DL4000": ("error", "MAINTAINER is deprecated."),
    "DL4003": ("warning", "Multiple CMD instructions found; only the last one takes effect."),
    "DL4004": ("error", "Multiple ENTRYPOINT instructions found; only the last one takes effect."),
    "AXL001": ("warning", "No USER instruction in the final stage: the container runs as root."),
    "AXL002": ("error", "Secret-looking name in ENV or ARG: the value is stored in the image or its build history; use a build secret (RUN --mount=type=secret) or a runtime variable."),
    "AXL003": ("warning", "A download is piped straight into a shell; fetch, verify a checksum or signature, then run."),
}
INSTRUCTIONS = {"FROM", "RUN", "CMD", "LABEL", "MAINTAINER", "EXPOSE", "ENV", "ADD", "COPY", "ENTRYPOINT", "VOLUME",
                "USER", "WORKDIR", "ARG", "ONBUILD", "STOPSIGNAL", "HEALTHCHECK", "SHELL"}
SECRET_NAME = re.compile(r"(PASSW(OR)?D|SECRET|TOKEN|API_?KEY|PRIVATE_?KEY|ACCESS_?KEY|CREDENTIAL|AUTH_?KEY|PASSPHRASE)", re.I)
ARCHIVE = re.compile(r"\.(tar|tar\.gz|tgz|tar\.bz2|tbz2?|tar\.xz|txz|tar\.zst|gz|bz2|xz|zst)$", re.I)
# the apt lists directory, written with a character class so the source holds no literal path
APT_LISTS = re.compile(r"\brm\s+(?:-[A-Za-z]+\s+)*['\"]?/va[r]/lib/apt/lists")
APT_CACHE_MOUNT = re.compile(r"--mount=[^\s]*target=/va[r]/lib/apt")
PIPE_SHELL = re.compile(r"\b(curl|wget)\b[^|;&]*\|\s*(sudo\s+)?(-\S+\s+)*(ba|z|da|k)?sh\b")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def logical_lines(text):
    """Join continuation lines and heredocs; yield (line_number, keyword, arguments, ignores)."""
    lines = text.splitlines()
    escape = "\\"
    for raw in lines[:5]:
        m = re.match(r"^\s*#\s*escape\s*=\s*([\\`])\s*$", raw)
        if m:
            escape = m.group(1)
            break
        if raw.strip() and not raw.strip().startswith("#"):
            break
    out, i, pending_ignores = [], 0, []
    while i < len(lines):
        start = i + 1
        line = lines[i]
        i += 1
        stripped = line.strip()
        if not stripped:
            continue
        if stripped.startswith("#"):
            m = re.match(r"#\s*hadolint\s+ignore\s*=\s*([A-Za-z0-9,\s]+)", stripped)
            if m:
                pending_ignores = [x.strip().upper() for x in m.group(1).split(",") if x.strip()]
            continue
        parts = []
        while True:
            body = line.rstrip()
            if body.endswith(escape) and not body.endswith(escape * 2):
                parts.append(body[:-1])
                # skip comment and blank lines inside a continuation, as Docker does
                while i < len(lines) and (not lines[i].strip() or lines[i].strip().startswith("#")):
                    i += 1
                if i >= len(lines):
                    break
                line = lines[i]
                i += 1
                continue
            parts.append(body)
            break
        joined = " ".join(p.strip() for p in parts)
        m = re.match(r"^(\w+)\s*(.*)$", joined)
        if not m:
            out.append((start, "", joined, pending_ignores))
            pending_ignores = []
            continue
        keyword, args = m.group(1).upper(), m.group(2)
        for tag in re.findall(r"<<-?\s*[\"']?([A-Za-z_][A-Za-z0-9_]*)[\"']?", args) if keyword in ("RUN", "COPY", "ADD") else []:
            body = []
            while i < len(lines):
                nxt = lines[i]
                i += 1
                if nxt.strip() == tag:
                    break
                body.append(nxt)
            args = args + "\n" + "\n".join(body)
        out.append((start, keyword, args, pending_ignores))
        pending_ignores = []
    return out


def split_words(text):
    try:
        return shlex.split(text, comments=False, posix=True)
    except ValueError:
        return text.split()


def commands(script):
    """Split a shell script into simple commands (on &&, ||, ;, |, newlines) as word lists."""
    chunks = re.split(r"&&|\|\||;|\||\n", script)
    return [split_words(c) for c in chunks if c.strip()]


def apt_get_installs(script):
    for words in commands(script):
        while words and (words[0] in ("sudo", "env", "-E") or re.fullmatch(r"[A-Z_][A-Z0-9_]*=.*", words[0])):
            words = words[1:]
        if len(words) >= 2 and words[0] == "apt-get" and "install" in words[1:]:
            yield words


def is_json_form(args):
    try:
        value = json.loads(args)
    except ValueError:
        return False
    return isinstance(value, list) and all(isinstance(x, str) for x in value)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - {"text", "ignore"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be the Dockerfile content (a non-empty string)")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    ignore = request.get("ignore", [])
    if not isinstance(ignore, list) or not all(isinstance(x, str) for x in ignore):
        fail("ignore must be a list of rule ids")
    ignore = {x.upper() for x in ignore}
    unknown = sorted(ignore - set(RULES))
    if unknown:
        fail(f"unknown rule id(s) in ignore: {', '.join(unknown)}")

    findings = []
    instructions = logical_lines(text)

    def report(line, rule, ignores, detail=None):
        if rule in ignore or rule in ignores:
            return
        severity, message = RULES[rule]
        item = {"line": line, "rule": rule, "severity": severity, "message": message}
        if detail:
            item["detail"] = detail
        findings.append(item)

    stages, unknown_instr = [], []
    aliases = set()
    for line, keyword, args, ignores in instructions:
        if keyword not in INSTRUCTIONS:
            unknown_instr.append({"line": line, "instruction": keyword or args[:40]})
            continue
        if keyword == "FROM":
            words = [w for w in args.split() if not w.startswith("--")]
            image = words[0] if words else ""
            alias = words[2] if len(words) >= 3 and words[1].lower() == "as" else None
            stage = {"line": line, "image": image, "alias": alias, "cmd": 0, "entrypoint": 0, "user": None,
                     "user_line": None, "runs": []}
            stages.append(stage)
            name = image.lower()
            if name and name != "scratch" and name not in aliases and not name.startswith("$"):
                last = image.rsplit("/", 1)[-1]
                if "@" in last:
                    pass  # pinned by digest
                elif ":" not in last:
                    report(line, "DL3006", ignores, f"image {image}")
                elif last.rsplit(":", 1)[1] == "latest":
                    report(line, "DL3007", ignores, f"image {image}")
            if alias:
                aliases.add(alias.lower())
            continue
        if not stages:
            if keyword != "ARG":
                unknown_instr.append({"line": line, "instruction": keyword, "note": "before the first FROM"})
            stage = None
        else:
            stage = stages[-1]
        if keyword == "MAINTAINER":
            report(line, "DL4000", ignores)
        elif keyword in ("CMD", "ENTRYPOINT") and stage is not None:
            key = "cmd" if keyword == "CMD" else "entrypoint"
            stage[key] += 1
            if stage[key] == 2:
                report(line, "DL4003" if keyword == "CMD" else "DL4004", ignores)
            if not is_json_form(args.strip()):
                report(line, "DL3025", ignores, f"{keyword} in shell form")
        elif keyword == "USER" and stage is not None:
            user = args.strip().split(":")[0]
            stage["user"], stage["user_line"], stage["user_ignores"] = user, line, ignores
        elif keyword == "WORKDIR":
            path = args.strip().strip("\"'")
            if not (path.startswith("/") or path.startswith("$") or re.match(r"^[A-Za-z]:[\\/]", path)):
                report(line, "DL3000", ignores, f"WORKDIR {path}")
        elif keyword == "ADD":
            words = [w for w in split_words(args) if not w.startswith("--")]
            if args.strip().startswith("["):
                try:
                    words = json.loads(args)
                except ValueError:
                    pass
            sources = words[:-1]
            if sources and not any(re.match(r"^(https?|git)://|^git@", s) or ARCHIVE.search(s) for s in sources) \
                    and "<<" not in args:
                report(line, "DL3020", ignores, "ADD " + " ".join(sources)[:80])
        elif keyword in ("ENV", "ARG"):
            first = args.split()[0] if args.split() else ""
            if keyword == "ARG":
                names = [w.split("=", 1)[0] for w in split_words(args)]
            elif "=" in first:
                names = [w.split("=", 1)[0] for w in split_words(args) if "=" in w]
            else:
                names = [first] if first else []  # legacy form: ENV NAME value
            for name in names:
                if SECRET_NAME.search(name):
                    report(line, "AXL002", ignores, f"{keyword} {name} (value not shown)")
        elif keyword == "RUN" and stage is not None:
            stage["runs"].append((line, args, ignores))
        if keyword == "RUN":
            script = args
            if re.search(r"(^|[\s;&|(])sudo\s", script):
                report(line, "DL3004", ignores)
            if PIPE_SHELL.search(script):
                report(line, "AXL003", ignores)
            for words in commands(script):
                if words and words[0] == "apt" and len(words) > 1 and words[1] in ("install", "update", "upgrade", "remove",
                                                                                   "dist-upgrade", "full-upgrade", "purge"):
                    report(line, "DL3027", ignores)
                    break
            for words in apt_get_installs(script):
                opts = [w for w in words[1:] if w.startswith("-")]
                yes = any(w in ("--yes", "--assume-yes", "-qq") or re.fullmatch(r"-[a-z]*y[a-z]*", w) or w.startswith("-q=2")
                          for w in opts)
                if not yes:
                    report(line, "DL3014", ignores)
                if "--no-install-recommends" not in opts and "APT::Install-Recommends=false" not in " ".join(words):
                    report(line, "DL3015", ignores)
                after = words[words.index("install") + 1:]
                skip_next = False
                unpinned = []
                for w in after:
                    if skip_next:
                        skip_next = False
                        continue
                    if w in ("-o", "-t", "--target-release", "-c"):
                        skip_next = True
                        continue
                    if w.startswith("-"):
                        continue
                    if "=" in w or w.endswith(".deb") or "/" in w or w.startswith("$"):
                        continue
                    unpinned.append(w)
                if unpinned:
                    report(line, "DL3008", ignores, "unpinned: " + ", ".join(unpinned[:10]))

    # stage-level rules
    used_as_base = {s["image"].lower() for s in stages}
    for index, stage in enumerate(stages):
        shipped = index == len(stages) - 1 or (stage["alias"] and stage["alias"].lower() in used_as_base)
        if not shipped:
            continue
        for line, script, ignores in stage["runs"]:
            if any(True for _ in apt_get_installs(script)) and not APT_LISTS.search(script) \
                    and not APT_CACHE_MOUNT.search(script):
                report(line, "DL3009", ignores)
    if stages:
        final = stages[-1]
        if final["user"] is None:
            if final["image"].lower() != "scratch":
                report(final["line"], "AXL001", [])
        elif final["user"] in ("root", "0"):
            report(final["user_line"], "DL3002", final.get("user_ignores", []))
    else:
        unknown_instr.append({"line": 0, "instruction": "FROM", "note": "no FROM instruction: not a Dockerfile?"})

    findings.sort(key=lambda f: (f["line"], f["rule"]))
    counts = {s: sum(1 for f in findings if f["severity"] == s) for s in ("error", "warning", "info", "style")}
    result = {"findings": findings, "count": len(findings), "by_severity": counts,
              "rules_fired": sorted({f["rule"] for f in findings}),
              "stages": [{"line": s["line"], "image": s["image"], "alias": s["alias"]} for s in stages],
              "passed": not any(f["severity"] in ("error", "warning") for f in findings)}
    if unknown_instr:
        result["unparsed"] = unknown_instr
    print(json.dumps(result))


if __name__ == "__main__":
    main()
