#!/usr/bin/env python3
"""Walk the catalog's ceremony definitions through a real MADE engine in a disposable store:
validate, publish, then drive the happy and blocked paths with the fields the generic
/ceremony loop would hand back. Reuses the Engine of tools/ceremonies/check_pins.py and the
Walk of tools/ceremonies/spike_drafts.py.

Usage: python3 tools/catalog/walk_ceremonies.py --made-bin /absolute/path/to/made-mcp
"""
import argparse
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / "tools/ceremonies"))
from check_pins import Engine  # noqa: E402
from spike_drafts import ACTIONS, Walk, expect, refused  # noqa: E402

CATALOG = REPO / "catalog/ceremonies"


def publish(engine, path):
    text = path.read_text()
    report = engine.call("made_validate_ceremony_draft", {"definition_yaml": text})
    if not report.get("publishable"):
        raise SystemExit(f"FAIL {path.name}: not publishable: {report}")
    engine.call("made_publish_ceremony_definition", {"definition_yaml": text})
    print(f"{path.name}: validated and published ({report.get('warning_count', 0)} warning(s))")


def draft_critique_revise(w):
    inputs = dict(brief="a 300-word note on D'Hondt", workspace="/ws")
    print("  accepted at the first critique")
    i = w.start("draft_critique_revise", "1.0", inputs)
    w.step(i, "draft", "draft:1", dict(path="note.md", criteria=["1. under 300 words"], summary="a note"))
    expect("drafted", w.transition(i, "drafted"), "CRITIQUE")
    w.step(i, "critique", "critique:1", dict(accepted=True, exhausted=False, findings=[]))
    refused("revision_needed after acceptance", lambda: w.transition(i, "revision_needed"))
    expect("accepted", w.transition(i, "accepted"), "DELIVER")
    w.step(i, "deliver", "deliver:1", dict(path="note.md", summary="delivered"))
    expect("delivered", w.transition(i, "delivered"), "COMPLETED")

    print("  rejected, revised and settled, accepted at the second critique")
    i = w.start("draft_critique_revise", "1.0", inputs)
    w.step(i, "draft", "draft:1", dict(path="note.md", criteria=["1. under 300 words"], summary="a note"))
    w.transition(i, "drafted")
    w.step(i, "critique", "critique:1", dict(accepted=False, exhausted=False, findings=[{"criterion": 1, "what": "320 words"}]))
    refused("accepted after rejection", lambda: w.transition(i, "accepted"))
    expect("revision_needed", w.transition(i, "revision_needed"), "REVISE")
    w.step(i, "revise", "revise:1", dict(path="note.md", resolved=["1"], settled=True))
    expect("revised", w.transition(i, "revised"), "CRITIQUE")
    w.step(i, "critique", "critique:2", dict(accepted=True, exhausted=False, findings=[]))
    expect("accepted", w.transition(i, "accepted"), "DELIVER")

    print("  the revision cannot settle twice: escalated")
    i = w.start("draft_critique_revise", "1.0", inputs)
    w.step(i, "draft", "draft:1", dict(path="note.md", criteria=["1. under 300 words"], summary="a note"))
    w.transition(i, "drafted")
    w.step(i, "critique", "critique:1", dict(accepted=False, exhausted=False, findings=[{"criterion": 1}]))
    w.transition(i, "revision_needed")
    w.step(i, "revise", "revise:1", dict(path="note.md", resolved=[], settled=False))
    refused("revised while unsettled", lambda: w.transition(i, "revised"))
    refused("rounds_exhausted after one attempt", lambda: w.transition(i, "rounds_exhausted"))
    w.step(i, "revise", "revise:2", dict(path="note.md", resolved=[], settled=False))
    refused("revised while still unsettled", lambda: w.transition(i, "revised"))
    expect("rounds_exhausted", w.transition(i, "rounds_exhausted"), "ESCALATE")
    w.step(i, "escalate", "escalate:1", dict(open_findings=[{"criterion": 1}], question="relax criterion 1?"))
    expect("escalated", w.transition(i, "escalated"), "BLOCKED")

    print("  three rejections: the critic declares the rounds exhausted")
    i = w.start("draft_critique_revise", "1.0", inputs)
    w.step(i, "draft", "draft:1", dict(path="note.md", criteria=["1. under 300 words"], summary="a note"))
    w.transition(i, "drafted")
    for round in (1, 2):
        w.step(i, "critique", f"critique:{round}", dict(accepted=False, exhausted=False, findings=[{"criterion": 1}]))
        expect(f"revision_needed {round}", w.transition(i, "revision_needed"), "REVISE")
        # Two attempts in each round: the repeat budget is per entry into REVISE, not per instance.
        w.step(i, "revise", f"revise:{round}a", dict(path="note.md", resolved=[], settled=False))
        w.step(i, "revise", f"revise:{round}b", dict(path="note.md", resolved=["1"], settled=True))
        expect(f"revised {round} after two attempts", w.transition(i, "revised"), "CRITIQUE")
    w.step(i, "critique", "critique:3", dict(accepted=False, exhausted=True, findings=[{"criterion": 1}]))
    refused("revision_needed on the third rejection", lambda: w.transition(i, "revision_needed"))
    expect("critique_exhausted", w.transition(i, "critique_exhausted"), "ESCALATE")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--made-bin", required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory() as directory:
        engine = Engine(args.made_bin, Path(directory))
        engine.call("made_issue_authorization_grant", dict(
            grant_id="catalog-walk", grantee_id=engine.host, scope={"kind": "global"},
            valid_from="2000-01-01T00:00:00Z", delegation_depth=0, actions=ACTIONS))
        for path in sorted(CATALOG.glob("*/*.yaml")):
            publish(engine, path)
        w = Walk(engine)
        draft_critique_revise(w)
        engine.close()
    print("ok")


if __name__ == "__main__":
    main()
