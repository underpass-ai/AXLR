#!/usr/bin/env python3
"""Pre-flight copy of the registry's policy scan, so a tool that the registry would block
fails here first. The rules are the ones of axlr-registry's crates/domain/src/scan_rule.rs
(line by line, the program's family plus the generic ones); the registry's scan is the one
that counts.

Usage: python3 tools/catalog/policy_scan.py [catalog/tools]
Exit 1 when a file has a blocking finding; warnings are printed and do not fail.
"""
import json
import pathlib
import re
import sys

GENERIC = [
    ("obfuscation", r"[A-Za-z0-9+/=]{200,}", "block", False),
    ("obfuscation", r"(\\x[0-9a-fA-F]{2}){24,}", "block", False),
    ("escape", r"(^|[^A-Za-z0-9_])(/home/|/etc/|/root/|/var/|\.\./)", "warn", False),
]
PYTHON = [
    ("network", r"^\s*(import|from)\s+.*\b(socket|urllib|http\.client|requests|ftplib|smtplib|ssl|websockets?)\b", "block", True),
    ("process", r"\b(subprocess|multiprocessing|ctypes)\b|\bos\.(system|exec[lvpe]*|popen|spawn[lvpe]*|fork)\s*\(", "block", False),
    ("dynamic-code", r"(^|[^A-Za-z0-9_.])(eval|exec|compile|__import__)\s*\(|\bimportlib\b", "block", False),
    ("obfuscation", r"\bbase64\.(b64|b32|b16|a85|b85)decode\b|\bcodecs\.decode\b|\bmarshal\.loads\b|\bpickle\.loads?\b", "block", False),
    ("escape", r"\bos\.environ\b|\bexpanduser\s*\(|\bPath\.home\s*\(", "warn", False),
]
BASH = [
    ("network", r"(^|[^A-Za-z0-9_-])(curl|wget|nc|ncat|netcat|ssh|scp|sftp|telnet)\s|/dev/(tcp|udp)/", "block", True),
    ("dynamic-code", r"(^|[^A-Za-z0-9_])eval\s|source\s+<\(|\.\s+<\(|\$\(\s*(curl|wget)", "block", False),
    ("obfuscation", r"base64\s+(-d|--decode)|\bopenssl\s+enc\b|\bxxd\s+-r\b", "block", False),
    ("escape", r"\$HOME\b|(^|[\s=])~[/\s]|\brm\s+-[a-zA-Z]*[rR][a-zA-Z]*f|\bsudo\b", "warn", False),
]
NODE = [
    ("network", r"""require\(\s*["'](net|http|https|dns|tls|dgram|http2|ws)["']\s*\)|from\s+["'](net|http|https|dns|tls|dgram|http2)["']|\bfetch\s*\(|\bXMLHttpRequest\b|\bWebSocket\b""", "block", True),
    ("process", r"""require\(\s*["'](child_process|worker_threads|cluster)["']\s*\)|from\s+["'](child_process|worker_threads|cluster)["']""", "block", False),
    ("dynamic-code", r"""(^|[^A-Za-z0-9_.])eval\s*\(|new\s+Function\s*\(|\bvm\.(runIn|compile|Script)|require\(\s*["']vm["']""", "block", False),
    ("obfuscation", r"""Buffer\.from\s*\([^)]*["']base64["']|\batob\s*\(""", "block", False),
    ("escape", r"\bprocess\.env\b|\bos\.homedir\s*\(", "warn", False),
]


def rules_for(program):
    family = program.lower()
    if family.startswith("python"):
        rules = PYTHON
    elif family in ("bash", "sh", "zsh"):
        rules = BASH
    elif family.startswith("node"):
        rules = NODE
    else:
        rules = []
    return [(name, re.compile(pattern), severity, network) for name, pattern, severity, network in rules + GENERIC]


def scan(tool_dir):
    manifest = json.loads((tool_dir / "manifest.json").read_text())
    network = manifest.get("catalog", {}).get("network", False)
    rules = rules_for(manifest["program"])
    findings = []
    for name in manifest["files"]:
        for number, line in enumerate((tool_dir / name).read_text().splitlines(), 1):
            for rule, pattern, severity, needs_network in rules:
                if pattern.search(line):
                    blocks = severity == "block" and not (needs_network and network)
                    findings.append(("block" if blocks else "warn", f"{name}:{number}", rule, line.strip()[:120]))
    return findings


def main():
    root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "catalog/tools")
    blocked = 0
    for manifest in sorted(root.glob("*/*/manifest.json")):
        for severity, where, rule, line in scan(manifest.parent):
            blocked += severity == "block"
            print(f"{severity:5} {manifest.parent.relative_to(root)}/{where} {rule}: {line}")
    print(f"{blocked} blocking findings")
    sys.exit(1 if blocked else 0)


if __name__ == "__main__":
    main()
