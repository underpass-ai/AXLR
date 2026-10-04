#!/usr/bin/env python3
"""Validate the console-driven ceremony definitions against a real MADE engine.

Every YAML under tui/adapters/ceremonyhost/definitions/ is validated and
published into a disposable store; its semantic digest must equal the pin in
tui/adapters/ceremonyhost/definitions.go, and every pin must have a YAML. Only
Python's standard library and a compatible made-mcp binary are needed. No user
store, live ceremony or external publication is touched.
"""

import argparse
import json
import os
import re
import secrets
import select
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
DEFINITIONS = REPO / "tui/adapters/ceremonyhost/definitions"
PINS = REPO / "tui/adapters/ceremonyhost/definitions.go"
PIN = re.compile(r'\{Name: "([a-z_]+)", Version: "([0-9.]+)", Digest: "([0-9a-f]{64})"\}')


class Engine:
    def __init__(self, binary, directory):
        store = directory / "validation.sqlite3"
        self.host = "axlr-pin-check"
        env = dict(os.environ, MADE_MCP_BACKEND="embedded", MADE_MCP_STORE_PATH=str(store),
                   MADE_AUTH_POLICY_ID=self.host, MADE_AUTH_TRUSTED_HOST_ID=self.host,
                   MADE_CEREMONY_STORE_ID=self.host,
                   MADE_CEREMONY_SEARCH_CURSOR_HMAC_KEY=secrets.token_hex(32))
        subprocess.run([binary, "bootstrap-authorization", str(store), "--policy-id", self.host,
                        "--trusted-host-id", self.host], check=True, env=env, capture_output=True)
        self.stderr = (directory / "stderr").open("w+")
        self.process = subprocess.Popen([binary], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=self.stderr, env=env, text=True)
        self.counter = 0
        self.request("initialize", dict(protocolVersion="2024-11-05", capabilities={},
                     clientInfo=dict(name="axlr-pin-check", version="1.0")))
        self.process.stdin.write(json.dumps(dict(jsonrpc="2.0", method="notifications/initialized")) + "\n")
        self.process.stdin.flush()

    def request(self, method, params):
        self.counter += 1
        self.process.stdin.write(json.dumps(dict(jsonrpc="2.0", id=self.counter, method=method, params=params)) + "\n")
        self.process.stdin.flush()
        while True:
            if not select.select([self.process.stdout], [], [], 25)[0]:
                raise TimeoutError("MADE response timed out")
            line = self.process.stdout.readline()
            if not line:
                self.stderr.seek(0)
                raise RuntimeError("MADE exited: " + self.stderr.read())
            result = json.loads(line)
            if result.get("id") == self.counter:
                return result

    def call(self, name, arguments):
        response = self.request("tools/call", dict(name=name, arguments=arguments))
        result = response.get("result", {})
        if response.get("error") or result.get("isError"):
            raise RuntimeError(f"{name}: {response}")
        return result.get("structuredContent") or json.loads(result["content"][0]["text"])

    def close(self):
        self.process.terminate()
        self.process.wait(timeout=5)
        self.process.stdin.close()
        self.process.stdout.close()
        self.stderr.close()


def pins():
    found = {(name, version): digest for name, version, digest in PIN.findall(PINS.read_text())}
    if not found:
        raise SystemExit(f"no pins found in {PINS}")
    return found


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--made-bin", required=True, help="absolute path to a compatible made-mcp binary")
    args = parser.parse_args()
    pinned = pins()
    files = sorted(DEFINITIONS.glob("*.yaml"))
    if not files:
        raise SystemExit(f"no definitions under {DEFINITIONS}")
    failures = []
    with tempfile.TemporaryDirectory(prefix="axlr-pin-check-") as directory:
        engine = Engine(args.made_bin, Path(directory))
        try:
            engine.call("made_issue_authorization_grant", dict(
                grant_id="pin-check", grantee_id=engine.host, scope={"kind": "global"},
                valid_from="2000-01-01T00:00:00Z", delegation_depth=0,
                actions=["validate_ceremony_draft", "publish_ceremony_definition", "get_ceremony_definition"]))
            seen = set()
            for path in files:
                text = path.read_text()
                name = re.search(r"^name: (\S+)$", text, re.M).group(1)
                version = re.search(r"^version: '?([0-9.]+)'?$", text, re.M).group(1)
                seen.add((name, version))
                report = engine.call("made_validate_ceremony_draft", {"definition_yaml": text})
                if not report.get("publishable") or report.get("error_count"):
                    failures.append(f"{path.name}: not publishable: {report}")
                    continue
                engine.call("made_publish_ceremony_definition", {"definition_yaml": text})
                digest = engine.call("made_get_ceremony_definition", dict(ceremony=name, version=version))["digest"]
                expected = pinned.get((name, version))
                status = "ok" if digest == expected else f"MISMATCH (pinned {expected})"
                print(f"{name} {version} {digest} {status}")
                if digest != expected:
                    failures.append(f"{path.name}: digest {digest} differs from the pin {expected}")
            for key in pinned:
                if key not in seen:
                    failures.append(f"pin {key[0]} {key[1]} has no YAML under {DEFINITIONS}")
        finally:
            engine.close()
    for failure in failures:
        print("FAIL:", failure, file=sys.stderr)
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
