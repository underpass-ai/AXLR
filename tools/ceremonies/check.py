#!/usr/bin/env python3
"""Validate default definitions using a real MADE engine in a disposable store.

Requires only Python's standard library and a compatible made-mcp binary.
All completed work in behavioral checks is explicitly synthetic fixture data.
No user store, live task, agent receiver or external publication is touched.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import select
import shutil
import subprocess
import tempfile

REPO = Path(__file__).resolve().parents[2]
RESOURCES = REPO / "tui/adapters/axlrplugin/builtin/made/skills/axlr-ceremonies/references"


class Engine:
    def __init__(self, binary, directory):
        store = directory / "validation.sqlite3"
        self.host = "axlr-catalogue-test"
        env = dict(os.environ, MADE_MCP_BACKEND="embedded", MADE_MCP_STORE_PATH=str(store),
                   MADE_AUTH_POLICY_ID="axlr-catalogue-test", MADE_AUTH_TRUSTED_HOST_ID=self.host,
                   MADE_CEREMONY_STORE_ID="axlr-catalogue-test",
                   MADE_CEREMONY_SEARCH_CURSOR_HMAC_KEY=secrets.token_hex(32))
        subprocess.run([binary, "bootstrap-authorization", str(store), "--policy-id", self.host,
                        "--trusted-host-id", self.host], check=True, env=env, capture_output=True)
        self.stderr = (directory / "stderr").open("w+")
        self.process = subprocess.Popen([binary], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=self.stderr, env=env, text=True)
        self.counter = 0
        self.request("initialize", dict(protocolVersion="2024-11-05", capabilities={},
                     clientInfo=dict(name="axlr-catalogue-check", version="1.0")))
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

    def call(self, name, arguments, expect_error=False):
        response = self.request("tools/call", dict(name=name, arguments=arguments))
        result = response.get("result", {})
        failed = bool(response.get("error") or result.get("isError"))
        if expect_error:
            assert failed, f"{name} unexpectedly accepted {arguments}"
            return response
        if failed:
            raise RuntimeError(f"{name}: {response}")
        return result.get("structuredContent") or json.loads(result["content"][0]["text"])

    def close(self):
        self.process.terminate()
        self.process.wait(timeout=5)
        self.process.stdin.close()
        self.process.stdout.close()
        self.stderr.close()


def fixture_context(entry):
    return {key: "synthetic fixture only: " + key for key in entry["required_inputs"]}


def start(engine, entries, name, suffix):
    entry = next(e for e in entries if e["name"] == name)
    instance_id = name + "-" + suffix
    engine.call("made_start_published_ceremony", dict(ceremony=name, version=entry["version"],
                ceremony_id=instance_id, actor_id=engine.host, actor_kind="service", context=fixture_context(entry)))
    return instance_id


def complete(engine, instance, step, output, attempt=1):
    claim = engine.call("made_claim_ceremony_step", dict(ceremony_id=instance, step_id=step,
                        actor_kind="service", lease_owner_id=engine.host,
                        idempotency_key=f"{instance}:{step}:{attempt}"))
    # Keep the accepted fence; do not run the engine's no-op step handler.
    fence = claim["claim_fence"]
    engine.call("made_complete_ceremony_step", dict(ceremony_id=instance, step_id=step,
                actor_kind="service", claim_fence=fence, status="completed",
                output=dict(fixture=True, evidence=["synthetic state-machine test"], **output)))
    return fence


def transition(engine, instance, trigger, blocked=False):
    return engine.call("made_apply_ceremony_transition", dict(ceremony_id=instance,
                       trigger=trigger, actor_kind="service"), expect_error=blocked)


def inspect(engine, instance):
    return engine.call("made_get_ceremony_instance", dict(ceremony_id=instance))


def behavior_checks(engine, entries):
    instance = start(engine, entries, "axlr_change", "failed-check")
    complete(engine, instance, "scope_change", dict(ready=True))
    transition(engine, instance, "scope_change_completed")
    complete(engine, instance, "change_and_check", dict(verified=False))
    transition(engine, instance, "change_and_check_completed", blocked=True)

    instance = start(engine, entries, "axlr_change", "happy-path")
    for step, flag in [("scope_change", "ready"), ("change_and_check", "verified"), ("hand_back", "delivered")]:
        complete(engine, instance, step, {flag: True})
        transition(engine, instance, step + "_completed")
    assert inspect(engine, instance)["current_state"] == "COMPLETED"

    instance = start(engine, entries, "axlr_handoff", "no-receiver")
    complete(engine, instance, "prepare_handoff", dict(ready=True, packet_digest="fixture-digest"))
    transition(engine, instance, "prepare_handoff_completed")
    complete(engine, instance, "accept_handoff", dict(accepted=False))
    transition(engine, instance, "accept_handoff_completed", blocked=True)
    engine.call("made_claim_ceremony_step", dict(ceremony_id=instance, step_id="seal_handoff", actor_kind="service"), expect_error=True)

    instance = start(engine, entries, "axlr_handoff", "acknowledged-fixture")
    for step, flag in [("prepare_handoff", "ready"), ("accept_handoff", "accepted"), ("seal_handoff", "transferred")]:
        complete(engine, instance, step, {flag: True, "packet_digest": "fixture-digest"})
        transition(engine, instance, step + "_completed")
    assert inspect(engine, instance)["current_state"] == "COMPLETED"

    for name, first, state_trigger, loop_steps in [
        ("axlr_delivery", "frame_delivery", "delivery_loop_completed", [("build_candidate", {}), ("verify_candidate", {"verified": True}), ("review_candidate", {"accepted": False})]),
        ("axlr_debug", "reproduce_failure", "repair_loop_completed", [("repair_cause", {}), ("prove_repair", {"repaired": False})]),
    ]:
        instance = start(engine, entries, name, "exhaustion")
        complete(engine, instance, first, {"ready" if name == "axlr_delivery" else "reproduced": True})
        transition(engine, instance, first + "_completed")
        if name == "axlr_debug":
            complete(engine, instance, "isolate_cause", {"diagnosed": True})
            transition(engine, instance, "isolate_cause_completed")
        for attempt in range(1, 4):
            for step, output in loop_steps:
                complete(engine, instance, step, output, attempt)
            transition(engine, instance, state_trigger, blocked=True)
        engine.call("made_claim_ceremony_step", dict(ceremony_id=instance, step_id=loop_steps[0][0], actor_kind="service"), expect_error=True)
        assert inspect(engine, instance)["current_state"] != "COMPLETED"

    for name, steps in [
        ("axlr_delivery", [("frame_delivery", "ready", "frame_delivery_completed"),
         ("build_candidate", None, None), ("verify_candidate", "verified", None),
         ("review_candidate", "accepted", "delivery_loop_completed"),
         ("integrate_delivery", "integrated", "integrate_delivery_completed")]),
        ("axlr_debug", [("reproduce_failure", "reproduced", "reproduce_failure_completed"),
         ("isolate_cause", "diagnosed", "isolate_cause_completed"),
         ("repair_cause", None, None), ("prove_repair", "repaired", "repair_loop_completed"),
         ("integrate_repair", "integrated", "integrate_repair_completed")]),
        ("axlr_review", [("establish_review", "ready", "establish_review_completed"),
         ("inspect_artifact", "evaluated", "inspect_artifact_completed"),
         ("report_review", "reported", "report_review_completed")]),
        ("axlr_research", [("frame_question", "ready", "frame_question_completed"),
         ("gather_evidence", "sourced", "gather_evidence_completed"),
         ("check_synthesis", "checked", "check_synthesis_completed"),
         ("deliver_research", "delivered", "deliver_research_completed")]),
    ]:
        instance = start(engine, entries, name, "happy-path")
        for step, flag, trigger in steps:
            complete(engine, instance, step, {flag: True} if flag else {})
            if trigger:
                transition(engine, instance, trigger)
        assert inspect(engine, instance)["current_state"] == "COMPLETED"

    instance = start(engine, entries, "axlr_delivery", "false-verification")
    complete(engine, instance, "frame_delivery", dict(ready=True))
    transition(engine, instance, "frame_delivery_completed")
    for step, output in [("build_candidate", {}), ("verify_candidate", {"verified": False}), ("review_candidate", {"accepted": True})]:
        complete(engine, instance, step, output)
    transition(engine, instance, "delivery_loop_completed", blocked=True)

    instance = start(engine, entries, "axlr_publish", "human-boundary")
    complete(engine, instance, "prepare_publication", dict(ready=True, artifact_digest="fixture-digest"))
    transition(engine, instance, "approve_publication", blocked=True)
    engine.call("made_approve_ceremony_guard", dict(ceremony_id=instance, guard_name="human_approved_publication",
                role_id="INTEGRATOR", role_kind="agent"), expect_error=True)
    engine.call("made_claim_ceremony_step", dict(ceremony_id=instance, step_id="publish_artifact", actor_kind="service"), expect_error=True)

    instance = start(engine, entries, "axlr_change", "string-success")
    complete(engine, instance, "scope_change", dict(ready="true"))
    transition(engine, instance, "scope_change_completed", blocked=True)
    print("Behavior checks passed: success, failed verification, bounded loops, receiver acceptance and human publication boundary")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--made-bin", default="made-mcp")
    parser.add_argument("--write-catalog", action="store_true", help="Refresh reviewed validation identities")
    args = parser.parse_args()
    binary = shutil.which(args.made_bin)
    if not binary:
        parser.error("compatible made-mcp binary is required")
    catalog_path = RESOURCES / "catalog.json"
    catalog = json.loads(catalog_path.read_text())
    with tempfile.TemporaryDirectory(prefix="axlr-made-check-") as directory:
        engine = Engine(binary, Path(directory))
        try:
            discovery = engine.call("made_discover_capabilities", {})
            print("Engine:", discovery["server"])
            engine.call("made_get_help", {"audience": "agent"})
            engine.call("made_issue_authorization_grant", dict(grant_id="catalogue-validation",
                        grantee_id=engine.host, scope={"kind": "global"}, valid_from="2000-01-01T00:00:00Z",
                        delegation_depth=0, actions=["validate_ceremony_draft", "publish_ceremony_definition",
                        "get_ceremony_definition", "start_published_ceremony", "get_ceremony_instance",
                        "claim_ceremony_step", "complete_ceremony_step", "apply_ceremony_transition"]))
            for entry in catalog["ceremonies"]:
                data = (RESOURCES / entry["file"]).read_bytes()
                sha = hashlib.sha256(data).hexdigest()
                report = engine.call("made_validate_ceremony_draft", {"definition_yaml": data.decode()})
                assert report["publishable"] and report["error_count"] == 0, report
                warnings = report["warning_count"]
                assert warnings == (1 if entry["name"] in ("axlr_delivery", "axlr_debug") else 0), report
                engine.call("made_publish_ceremony_definition", {"definition_yaml": data.decode()})
                definition = engine.call("made_get_ceremony_definition", dict(ceremony=entry["name"], version=entry["version"]))
                digest = definition["digest"]
                if args.write_catalog:
                    entry.update(file_sha256=sha, definition_digest=digest, validation_warnings=warnings)
                else:
                    assert entry["file_sha256"] == sha, f"stale file hash: {entry['name']}"
                    assert entry["definition_digest"] == digest, f"stale definition digest: {entry['name']}"
                print(entry["name"], "publishable", digest)
            behavior_checks(engine, catalog["ceremonies"])
        finally:
            engine.close()
    if args.write_catalog:
        catalog_path.write_text(json.dumps(catalog, indent=2) + "\n")


if __name__ == "__main__":
    main()
