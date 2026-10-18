#!/usr/bin/env python3
"""Run every catalog tool's tests.json with the axlr_run_tool contract: arguments as one
JSON object on stdin (object keys sorted, as the registry's runner sends them), result as exit code, stdout and stderr; only PATH and HOME in the
environment; the tool's directory as working directory.

Usage: python3 tools/catalog/run_tests.py [catalog/tools]
"""
import json
import os
import pathlib
import subprocess
import sys


def contains(actual, expected):
    """expected is a subset of actual, recursively (dicts by key, lists by position). Scalars
    compare as the registry's runner (serde_json) does: an integer never equals a float, so
    1 and 1.0 differ, and a boolean is not a number."""
    if isinstance(expected, dict):
        return isinstance(actual, dict) and all(k in actual and contains(actual[k], v) for k, v in expected.items())
    if isinstance(expected, list):
        return isinstance(actual, list) and len(actual) == len(expected) and all(contains(a, e) for a, e in zip(actual, expected))
    return type(actual) is type(expected) and actual == expected


def strict_json(text):
    """Parse as the registry does: no unpaired surrogate escapes (serde_json refuses them)."""
    value = json.loads(text)
    json.dumps(value, ensure_ascii=False).encode("utf-8")
    return value


def run(tool_dir):
    manifest = json.loads((tool_dir / "manifest.json").read_text())
    try:
        cases = strict_json((tool_dir / "tests.json").read_text())
    except (ValueError, UnicodeEncodeError) as error:
        return 1, [("tests.json", [f"the registry cannot parse it: {error}"])]
    env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": os.environ.get("HOME", "/")}
    failures = []
    for case in cases:
        proc = subprocess.run([manifest["program"], *manifest["args"]], input=json.dumps(case["input"], sort_keys=True),
                              capture_output=True, text=True, cwd=tool_dir, env=env, timeout=30)
        expect = case["expect"]
        problems = []
        if proc.returncode != expect.get("exit", 0):
            problems.append(f"exit {proc.returncode} != {expect.get('exit', 0)}; stderr: {proc.stderr.strip()[:200]}")
        if "stdout_json" in expect:
            try:
                out = strict_json(proc.stdout)
            except (ValueError, UnicodeEncodeError):
                problems.append(f"stdout is not JSON: {proc.stdout[:200]!r}")
            else:
                if not contains(out, expect["stdout_json"]):
                    problems.append(f"stdout {json.dumps(out)[:300]} lacks {json.dumps(expect['stdout_json'])}")
        if "stdout_contains" in expect and expect["stdout_contains"] not in proc.stdout:
            problems.append(f"stdout {proc.stdout[:200]!r} lacks {expect['stdout_contains']!r}")
        if problems:
            failures.append((case["name"], problems))
    return len(cases), failures


def main():
    root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "catalog/tools")
    total = failed = 0
    for manifest in sorted(root.glob("*/*/manifest.json")):
        count, failures = run(manifest.parent)
        total += count
        failed += len(failures)
        status = "ok" if not failures else "FAIL"
        print(f"{status:4} {manifest.parent.relative_to(root)} ({count} cases)")
        for name, problems in failures:
            for problem in problems:
                print(f"       {name}: {problem}")
    print(f"{total - failed}/{total} cases passed")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
