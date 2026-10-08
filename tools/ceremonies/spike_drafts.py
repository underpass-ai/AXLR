#!/usr/bin/env python3
"""Walk the plan, task, sync and improve definitions through a real MADE engine.

Every draft under tools/ceremonies/drafts/, and every pinned definition that
has a scenario here, is validated and published into a disposable store,
then its happy and blocked paths are driven with the same calls the console
makes: start, claim, complete, transition and guard approval. The scenarios
document the design in docs/plans; a change that breaks one of them fails
here. Only Python's standard library and a compatible made-mcp binary are
needed.
"""

import argparse
import re
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from check_pins import Engine  # noqa: E402

REPO = Path(__file__).resolve().parents[2]
DRAFTS = REPO / "tools/ceremonies/drafts"
DEFINITIONS = REPO / "tui/adapters/ceremonyhost/definitions"
ACTIONS = ["validate_ceremony_draft", "publish_ceremony_definition", "get_ceremony_definition",
           "start_published_ceremony", "claim_ceremony_step", "complete_ceremony_step",
           "apply_ceremony_transition", "get_ceremony_instance", "approve_ceremony_guard"]


class Walk:
    """The console's view of one MADE store: instances, steps and transitions."""

    def __init__(self, engine):
        self.engine = engine
        self.count = 0

    def start(self, name, version, inputs):
        self.count += 1
        instance = f"spike-{name}-{self.count}"
        self.engine.call("made_start_published_ceremony", dict(
            ceremony=name, version=version, ceremony_id=instance, actor_id="axlr", actor_kind="agent", context=inputs))
        return instance

    def step(self, instance, step, key, output):
        claim = self.engine.call("made_claim_ceremony_step", dict(
            ceremony_id=instance, step_id=step, actor_kind="agent", lease_owner_id="axlr",
            idempotency_key=f"{instance}:{key}", lease_ttl_ms=3600000))
        self.engine.call("made_complete_ceremony_step", dict(
            ceremony_id=instance, step_id=step, actor_kind="agent", claim_fence=claim["claim_fence"],
            status="completed", output=output))

    def transition(self, instance, trigger):
        self.engine.call("made_apply_ceremony_transition", dict(ceremony_id=instance, trigger=trigger, actor_kind="agent"))
        return self.engine.call("made_get_ceremony_instance", dict(ceremony_id=instance))["current_state"]

    def approve(self, instance, guard):
        self.engine.call("made_approve_ceremony_guard", dict(
            ceremony_id=instance, guard_name=guard, role_id="HUMAN_APPROVER", role_kind="human"))


def expect(label, got, want):
    print(f"    {label}: {got}")
    if got != want:
        raise SystemExit(f"FAIL {label}: got {got}, want {want}")


def refused(label, action):
    try:
        action()
    except RuntimeError as err:
        reason = re.search(r"invariant violated: ([^']*)", str(err))
        print(f"    {label}: refused ({reason.group(1) if reason else 'engine error'})")
        return
    raise SystemExit(f"FAIL {label}: the engine accepted it")


def bounces(label, cycle):
    """Count the cycles the engine accepts before its bounce limit refuses one."""
    for round in range(1, 9):
        try:
            cycle(round)
        except RuntimeError as err:
            if "bounce limit" not in str(err):
                raise
            print(f"    {label}: {round - 1} cycle(s) accepted, the next refused by the bounce limit")
            return round - 1
    raise SystemExit(f"FAIL {label}: the engine never refused a cycle")


def plan(w):
    print("axlr_plan 1.0")
    inputs = dict(brief="split the exporter", workspace="/ws")
    print("  verified, approved by the person")
    i = w.start("axlr_plan", "1.0", inputs)
    w.step(i, "decompose", "decompose:1", dict(settled=False, verified=False, defects=["t2 depends on unknown t9"]))
    refused("decomposed while unverified", lambda: w.transition(i, "decomposed"))
    w.step(i, "decompose", "decompose:2", dict(settled=True, verified=True, tasks=[{"id": "t1"}], waves=[["t1"]]))
    expect("decomposed", w.transition(i, "decomposed"), "APPROVAL")
    w.step(i, "present", "present:1", dict(decision="approve"))
    refused("approved without the human guard", lambda: w.transition(i, "approved"))
    w.approve(i, "person_approves")
    refused("approved_automatically after a manual approve", lambda: w.transition(i, "approved_automatically"))
    expect("approved", w.transition(i, "approved"), "READY")
    print("  returned, decomposed again, declined")
    i = w.start("axlr_plan", "1.0", inputs)
    w.step(i, "decompose", "decompose:1", dict(settled=True, verified=True))
    w.transition(i, "decomposed")
    w.step(i, "present", "present:1", dict(decision="return", reason="t1 too big"))
    expect("returned", w.transition(i, "returned"), "DECOMPOSE")
    w.step(i, "decompose", "decompose:r1", dict(settled=True, verified=True))
    expect("decomposed again", w.transition(i, "decomposed"), "APPROVAL")
    w.step(i, "present", "present:r1", dict(decision="decline", reason="not now"))
    expect("declined", w.transition(i, "declined"), "BLOCKED")
    print("  approved automatically")
    i = w.start("axlr_plan", "1.0", inputs)
    w.step(i, "decompose", "decompose:1", dict(settled=True, verified=True))
    w.transition(i, "decomposed")
    w.step(i, "present", "present:1", dict(decision="automatic"))
    expect("approved_automatically", w.transition(i, "approved_automatically"), "READY")
    print("  the engine's return backstop (the console allows two returns)")
    i = w.start("axlr_plan", "1.0", inputs)

    def cycle(round):
        w.step(i, "decompose", f"decompose:b{round}", dict(settled=True, verified=True))
        w.transition(i, "decomposed")
        w.step(i, "present", f"present:b{round}", dict(decision="return", reason="again"))
        w.transition(i, "returned")
    if bounces("returns", cycle) < 2:
        raise SystemExit("FAIL: the engine must allow at least the console's two returns")
    print("  decomposition exhausted")
    i = w.start("axlr_plan", "1.0", inputs)
    for n in range(3):
        w.step(i, "decompose", f"decompose:x{n}", dict(settled=False, verified=False))
    refused("a fourth decompose round", lambda: w.step(i, "decompose", "decompose:x3", dict(settled=True, verified=True)))
    expect("decompose_exhausted", w.transition(i, "decompose_exhausted"), "BLOCKED")


def task(w):
    print("axlr_task 1.0")
    inputs = dict(plan="spike-plan", task="t1", workspace="/ws")
    print("  test-first: start, red, green, handback")
    i = w.start("axlr_task", "1.0", inputs)
    w.step(i, "start", "start", dict(started=True, test_first=True, baseline_exit=0))
    refused("test_present when test_first=true", lambda: w.transition(i, "test_present"))
    expect("test_first", w.transition(i, "test_first"), "RED")
    w.step(i, "red", "red:1", dict(settled=False, red=False))
    refused("test_failing with red=false", lambda: w.transition(i, "test_failing"))
    w.step(i, "red", "red:2", dict(settled=True, red=True, test_files=["a_test.go"]))
    expect("test_failing", w.transition(i, "test_failing"), "GREEN")
    w.step(i, "green", "green:1", dict(green=False))
    w.step(i, "green", "green:2", dict(green=True, summary="done", summary_en="done"))
    expect("check_passing", w.transition(i, "check_passing"), "HANDBACK")
    w.step(i, "handback", "handback", dict(done=True, changed_files=["a.go", "a_test.go"]))
    expect("handed_back", w.transition(i, "handed_back"), "DONE")
    print("  without test-first: start, green")
    i = w.start("axlr_task", "1.0", inputs)
    w.step(i, "start", "start", dict(started=True, test_first=False))
    refused("test_first when test_first=false", lambda: w.transition(i, "test_first"))
    expect("test_present", w.transition(i, "test_present"), "GREEN")
    print("  declared untestable")
    i = w.start("axlr_task", "1.0", inputs)
    w.step(i, "start", "start", dict(started=True, test_first=True))
    w.transition(i, "test_first")
    w.step(i, "red", "red:1", dict(settled=True, red=False, untestable=True, observed="needs a network"))
    refused("test_failing when untestable", lambda: w.transition(i, "test_failing"))
    expect("untestable", w.transition(i, "untestable"), "BLOCKED")
    print("  green exhausted")
    i = w.start("axlr_task", "1.0", inputs)
    w.step(i, "start", "start", dict(started=True, test_first=False))
    w.transition(i, "test_present")
    for n in range(3):
        w.step(i, "green", f"green:{n}", dict(green=False))
    expect("green_exhausted", w.transition(i, "green_exhausted"), "BLOCKED")


def sync(w):
    print("axlr_sync 1.0")
    inputs = dict(plan="spike-plan", wave="1", tasks="t1,t2", workspace="/ws")
    print("  green at once")
    i = w.start("axlr_sync", "1.0", inputs)
    w.step(i, "integrate", "integrate:0", dict(verdict="green", exit_code=0))
    expect("integrated", w.transition(i, "integrated"), "SYNCED")
    print("  red, reconcile, red, reconcile, blocked")
    i = w.start("axlr_sync", "1.0", inputs)
    for n in range(2):
        w.step(i, "integrate", f"integrate:{n}", dict(verdict="red", exit_code=1))
        refused("integrated while red", lambda: w.transition(i, "integrated"))
        expect(f"conflict {n + 1}", w.transition(i, "conflict"), "RECONCILE")
        w.step(i, "reconcile", f"reconcile:{n}", dict(reconciled=True, responses=[{"task": "t1", "changed": True}]))
        expect(f"reconciled {n + 1}", w.transition(i, "reconciled"), "INTEGRATE")
    w.step(i, "integrate", "integrate:2", dict(verdict="blocked", exit_code=1))
    expect("exhausted", w.transition(i, "exhausted"), "BLOCKED")
    print("  red, reconcile, green")
    i = w.start("axlr_sync", "1.0", inputs)
    w.step(i, "integrate", "integrate:0", dict(verdict="red"))
    w.transition(i, "conflict")
    w.step(i, "reconcile", "reconcile:0", dict(reconciled=True))
    w.transition(i, "reconciled")
    w.step(i, "integrate", "integrate:1", dict(verdict="green"))
    expect("integrated after one round", w.transition(i, "integrated"), "SYNCED")
    print("  the engine's round backstop (the console allows two rounds)")
    i = w.start("axlr_sync", "1.0", inputs)

    def cycle(round):
        w.step(i, "integrate", f"integrate:b{round}", dict(verdict="red"))
        w.transition(i, "conflict")
        w.step(i, "reconcile", f"reconcile:b{round}", dict(reconciled=True))
        w.transition(i, "reconciled")
    if bounces("rounds", cycle) < 2:
        raise SystemExit("FAIL: the engine must allow at least the console's two rounds")


def improve(w):
    print("axlr_improve 1.0")
    inputs = dict(improvement_brief="show the app log to the agent", workspace="/ws", repository="underpass-ai/axlr-repair-lab")

    def to_watch(i, key):
        w.step(i, "build", f"build:{key}", dict(verified=True, summary="done", summary_en="done"))
        w.transition(i, "verified")
        w.step(i, "propose", f"propose:{key}", dict(proposed=True, pull_request=7, url="https://example.invalid/pull/7", head_sha="abc"))
        expect(f"proposed {key}", w.transition(i, "proposed"), "WATCH")

    print("  a check that already passes, then one that fails; approved by the person")
    i = w.start("axlr_improve", "1.0", inputs)
    w.step(i, "brief", "brief:1", dict(missing=False, settled=False, criteria="c", scope="s"))
    refused("briefed while the check passes", lambda: w.transition(i, "briefed"))
    w.step(i, "brief", "brief:2", dict(missing=True, settled=True, criteria="c", scope="s"))
    expect("briefed", w.transition(i, "briefed"), "BUILD")
    w.step(i, "build", "build:1", dict(verified=False, summary="wip", summary_en="wip"))
    refused("verified while the check fails", lambda: w.transition(i, "verified"))
    to_watch(i, "2")
    w.step(i, "watch", "watch:1", dict(verdict="green"))
    expect("checks_passed", w.transition(i, "checks_passed"), "DECIDE")
    w.step(i, "decide", "decide:1", dict(decision="approve", pull_request=7))
    refused("merge_approved without the human guard", lambda: w.transition(i, "merge_approved"))
    refused("merge_automatic, which the definition does not have", lambda: w.transition(i, "merge_automatic"))
    w.approve(i, "person_approves")
    expect("merge_approved", w.transition(i, "merge_approved"), "MERGE")
    w.step(i, "merge", "merge:1", dict(merged=True, merge_sha="def"))
    expect("merged", w.transition(i, "merged"), "COMPLETED")
    print("  an automatic decision has no way into MERGE")
    i = w.start("axlr_improve", "1.0", inputs)
    w.step(i, "brief", "brief:1", dict(missing=True, settled=True))
    w.transition(i, "briefed")
    to_watch(i, "1")
    w.step(i, "watch", "watch:1", dict(verdict="green"))
    w.transition(i, "checks_passed")
    w.step(i, "decide", "decide:1", dict(decision="automatic"))
    refused("merge_approved after an automatic decision", lambda: w.transition(i, "merge_approved"))
    refused("merge_declined after an automatic decision", lambda: w.transition(i, "merge_declined"))
    print("  declared not feasible")
    i = w.start("axlr_improve", "1.0", inputs)
    w.step(i, "brief", "brief:1", dict(feasible=False, missing=False, settled=True, observed="already there"))
    refused("briefed when not feasible", lambda: w.transition(i, "briefed"))
    expect("not_feasible", w.transition(i, "not_feasible"), "BLOCKED")
    print("  briefs exhausted")
    i = w.start("axlr_improve", "1.0", inputs)
    for n in range(3):
        w.step(i, "brief", f"brief:{n}", dict(missing=False, settled=False))
    expect("brief_exhausted", w.transition(i, "brief_exhausted"), "BLOCKED")
    print("  builds exhausted")
    i = w.start("axlr_improve", "1.0", inputs)
    w.step(i, "brief", "brief:1", dict(missing=True, settled=True))
    w.transition(i, "briefed")
    for n in range(3):
        w.step(i, "build", f"build:{n}", dict(verified=False))
    expect("build_exhausted", w.transition(i, "build_exhausted"), "BLOCKED")
    print("  red, build, red, build, blocked; then declined")
    i = w.start("axlr_improve", "1.0", inputs)
    w.step(i, "brief", "brief:1", dict(missing=True, settled=True))
    w.transition(i, "briefed")
    to_watch(i, "c0")
    for n in range(2):
        w.step(i, "watch", f"watch:c{n}", dict(verdict="red", failed=["go"]))
        expect(f"checks_failed {n + 1}", w.transition(i, "checks_failed"), "BUILD")
        to_watch(i, f"c{n + 1}")
    w.step(i, "watch", "watch:c2", dict(verdict="blocked", reason="checks failed after 2 rounds"))
    expect("watch_blocked", w.transition(i, "watch_blocked"), "BLOCKED")
    i = w.start("axlr_improve", "1.0", inputs)
    w.step(i, "brief", "brief:1", dict(missing=True, settled=True))
    w.transition(i, "briefed")
    to_watch(i, "1")
    w.step(i, "watch", "watch:1", dict(verdict="green"))
    w.transition(i, "checks_passed")
    w.step(i, "decide", "decide:1", dict(decision="decline", reason="not now"))
    expect("merge_declined", w.transition(i, "merge_declined"), "BLOCKED")


SCENARIOS = {"axlr_plan": plan, "axlr_task": task, "axlr_sync": sync, "axlr_improve": improve}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--made-bin", required=True, help="absolute path to a compatible made-mcp binary")
    args = parser.parse_args()
    files = sorted(DRAFTS.glob("*.yaml"))
    files += sorted(p for name in SCENARIOS for p in DEFINITIONS.glob(f"{name}-*.yaml"))
    if not files:
        raise SystemExit(f"no drafts under {DRAFTS} and no pinned definition with a scenario")
    with tempfile.TemporaryDirectory(prefix="axlr-spike-") as directory:
        engine = Engine(args.made_bin, Path(directory))
        try:
            engine.call("made_issue_authorization_grant", dict(
                grant_id="spike", grantee_id=engine.host, scope={"kind": "global"},
                valid_from="2000-01-01T00:00:00Z", delegation_depth=0, actions=ACTIONS))
            published = []
            for path in files:
                text = path.read_text()
                report = engine.call("made_validate_ceremony_draft", {"definition_yaml": text})
                if not report.get("publishable") or report.get("error_count"):
                    raise SystemExit(f"{path.name}: not publishable: {report}")
                for finding in report.get("findings", []):
                    print(f"{path.name}: validator {finding.get('severity')}: {finding.get('message')}")
                engine.call("made_publish_ceremony_definition", {"definition_yaml": text})
                name = re.search(r"^name: (\S+)$", text, re.M).group(1)
                version = re.search(r"^version: '?([0-9.]+)'?$", text, re.M).group(1)
                digest = engine.call("made_get_ceremony_definition", dict(ceremony=name, version=version))["digest"]
                print(f"published {name} {version} {digest}")
                published.append(name)
            walk = Walk(engine)
            for name in published:
                scenario = SCENARIOS.get(name)
                if scenario is None:
                    raise SystemExit(f"{name} has no scenario in {__file__}; add one with the draft")
                scenario(walk)
            print("all plan, task, sync and improve scenarios passed")
        finally:
            engine.close()


if __name__ == "__main__":
    main()
