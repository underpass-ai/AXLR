#!/usr/bin/env python3
"""Check an outline's beats against a story structure: three-act (Syd Field's paradigm),
Save the Cat (Blake Snyder's beat sheet, 15 beats on a 110-page script) or the hero's journey
(Christopher Vogler's 12 stages in three acts). Reports beats that are missing, early, late or
out of order, with the expected range.

Input (JSON on stdin):
  framework    "three_act" | "save_the_cat" | "heros_journey"
  scenes       [{"beat": "catalyst", "position": 11}, {"beat": "Midpoint", "page": 60}, ...]
               position is a percentage of the story (0-100); page needs total_pages
  total_pages  pages (or minutes) of the whole, to read "page" and to scale the targets
  tolerance    percentage points allowed around a point beat or a range (default 5)
  list_beats   true to list the framework's beats and stop
"""
import json
import re
import sys
from fractions import Fraction as F

# (id, name, start page, end page, aliases) on the framework's reference length
FRAMEWORKS = {
    "save_the_cat": {
        "pages": 110,
        "source": "Blake Snyder, Save the Cat! (2005), ch. 4, the Blake Snyder Beat Sheet (BS2), page numbers for a 110-page script",
        "beats": [
            ("opening_image", "Opening Image", 1, 1, []),
            ("theme_stated", "Theme Stated", 5, 5, ["theme"]),
            ("set_up", "Set-Up", 1, 10, ["setup"]),
            ("catalyst", "Catalyst", 12, 12, ["inciting_incident"]),
            ("debate", "Debate", 12, 25, []),
            ("break_into_two", "Break into Two", 25, 25, ["break_into_2", "act_two"]),
            ("b_story", "B Story", 30, 30, []),
            ("fun_and_games", "Fun and Games", 30, 55, []),
            ("midpoint", "Midpoint", 55, 55, []),
            ("bad_guys_close_in", "Bad Guys Close In", 55, 75, []),
            ("all_is_lost", "All Is Lost", 75, 75, []),
            ("dark_night_of_the_soul", "Dark Night of the Soul", 75, 85, ["dark_night"]),
            ("break_into_three", "Break into Three", 85, 85, ["break_into_3", "act_three"]),
            ("finale", "Finale", 85, 110, []),
            ("final_image", "Final Image", 110, 110, []),
        ],
    },
    "three_act": {
        "pages": 120,
        "source": "Syd Field, Screenplay (1979), the paradigm for a 120-page script; midpoint from Field, The Screenwriter's Workbook (1984)",
        "beats": [
            ("setup", "Act I: Setup", 1, 30, ["act_one", "act_i", "set_up"]),
            ("plot_point_1", "Plot Point I", 25, 27, ["plot_point_i", "first_plot_point", "pp1"]),
            ("confrontation", "Act II: Confrontation", 30, 90, ["act_two", "act_ii"]),
            ("midpoint", "Midpoint", 60, 60, []),
            ("plot_point_2", "Plot Point II", 85, 90, ["plot_point_ii", "second_plot_point", "pp2"]),
            ("resolution", "Act III: Resolution", 90, 120, ["act_three", "act_iii"]),
        ],
    },
    "heros_journey": {
        "pages": 100,
        "source": "Christopher Vogler, The Writer's Journey (3rd ed. 2007), the 12 stages and their place in a three-act structure "
                  "(Act One: stages 1-5, Act Two: 6-9 with the Ordeal as the central crisis, Act Three: 10-12). Vogler gives "
                  "no page numbers: the ranges are this tool's reading of that mapping on acts of 25 / 50 / 25 %",
        "beats": [
            ("ordinary_world", "Ordinary World", 0, 25, []),
            ("call_to_adventure", "Call to Adventure", 0, 25, ["call"]),
            ("refusal_of_the_call", "Refusal of the Call", 0, 25, ["refusal"]),
            ("meeting_with_the_mentor", "Meeting with the Mentor", 0, 25, ["meeting_the_mentor", "mentor"]),
            ("crossing_the_first_threshold", "Crossing the First Threshold", 20, 30, ["crossing_the_threshold", "threshold"]),
            ("tests_allies_enemies", "Tests, Allies, Enemies", 25, 50, ["tests_allies_and_enemies", "tests"]),
            ("approach_to_the_inmost_cave", "Approach to the Inmost Cave", 25, 50, ["approach"]),
            ("ordeal", "Ordeal", 40, 60, ["the_ordeal"]),
            ("reward", "Reward (Seizing the Sword)", 50, 75, ["seizing_the_sword"]),
            ("the_road_back", "The Road Back", 70, 80, ["road_back"]),
            ("resurrection", "Resurrection", 75, 100, ["the_resurrection"]),
            ("return_with_the_elixir", "Return with the Elixir", 90, 100, ["return"]),
        ],
    },
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def key(label):
    return re.sub(r"[^a-z0-9]+", "_", label.lower().replace("'", "")).strip("_")


def number(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number")
    try:
        return F(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number")


def pct(value):
    return f"{float(value):.2f}"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    name = request.get("framework")
    if not isinstance(name, str) or name not in FRAMEWORKS:
        fail("framework must be three_act, save_the_cat or heros_journey")
    fw = FRAMEWORKS[name]
    ref = fw["pages"]
    total = request.get("total_pages")
    if total is not None:
        total = number(total, "total_pages")
        if total <= 0:
            fail("total_pages must be positive")
    tolerance = number(request.get("tolerance", 5), "tolerance")
    if not 0 <= tolerance <= 50:
        fail("tolerance must be from 0 to 50 percentage points")
    beats = []
    for bid, label, start, end, aliases in fw["beats"]:
        lo, hi = F(start, ref) * 100, F(end, ref) * 100
        beats.append({"id": bid, "name": label, "lo": lo, "hi": hi, "aliases": aliases, "pages": (start, end)})
    lookup = {}
    for b in beats:
        for alias in [b["id"], key(b["name"])] + b["aliases"]:
            lookup.setdefault(alias, b["id"])
    if request.get("list_beats") is True:
        print(json.dumps({"framework": name, "source": fw["source"], "reference_pages": ref,
                          "beats": [{"id": b["id"], "name": b["name"], "pages": list(b["pages"]),
                                     "percent": [pct(b["lo"]), pct(b["hi"])]} for b in beats]}, ensure_ascii=False))
        return
    scenes = request.get("scenes")
    if not isinstance(scenes, list) or not scenes:
        fail("scenes must be a non-empty list of {beat, position} or {beat, page}")
    if len(scenes) > 1000:
        fail("more than 1000 scenes")
    placed, unknown = {}, []
    for i, scene in enumerate(scenes):
        if not isinstance(scene, dict) or not isinstance(scene.get("beat"), str):
            fail(f"scenes[{i}] must be an object with a beat label")
        if "position" in scene:
            position = number(scene["position"], f"scenes[{i}].position")
        elif "page" in scene:
            if total is None:
                fail(f"scenes[{i}] gives a page: total_pages is needed")
            position = number(scene["page"], f"scenes[{i}].page") / total * 100
        else:
            fail(f"scenes[{i}] needs position (percent) or page")
        if not 0 <= position <= 100:
            fail(f"scenes[{i}] is outside 0-100 % of the story")
        bid = lookup.get(key(scene["beat"]))
        if bid is None:
            unknown.append(scene["beat"])
        elif bid not in placed:
            placed[bid] = (position, i)
    report, counts = [], {"ok": 0, "early": 0, "late": 0, "missing": 0}
    for b in beats:
        entry = {"id": b["id"], "name": b["name"], "expected_percent": [pct(b["lo"]), pct(b["hi"])],
                 "allowed_percent": [pct(max(b["lo"] - tolerance, 0)), pct(min(b["hi"] + tolerance, 100))],
                 "reference_pages": list(b["pages"])}
        if b["lo"] == b["hi"]:
            entry["target_percent"] = pct(b["lo"])
        if total is not None:
            entry["expected_pages"] = [pct(b["lo"] * total / 100), pct(b["hi"] * total / 100)]
        if b["id"] not in placed:
            entry["status"] = "missing"
        else:
            position = placed[b["id"]][0]
            entry["position_percent"] = pct(position)
            if position < b["lo"] - tolerance:
                entry["status"], entry["off_by_percent"] = "early", pct(b["lo"] - tolerance - position)
            elif position > b["hi"] + tolerance:
                entry["status"], entry["off_by_percent"] = "late", pct(position - b["hi"] - tolerance)
            else:
                entry["status"] = "ok"
        counts[entry["status"]] += 1
        report.append(entry)
    expected = {b["id"]: b for b in beats}
    by_scene = sorted(placed, key=lambda bid: (placed[bid][0], placed[bid][1]))
    out_of_order = []
    for i, a in enumerate(by_scene):  # a comes before b in the outline, but b's range ends before a's begins
        for b in by_scene[i + 1:]:
            if placed[b][0] > placed[a][0] and expected[b]["hi"] < expected[a]["lo"]:
                out_of_order.append({"first": a, "then": b, "expected": f"{b} before {a}"})
    print(json.dumps({"framework": name, "source": fw["source"], "reference_pages": ref, "tolerance": pct(tolerance),
                      "summary": counts, "beats": report,
                      "missing": [e["id"] for e in report if e["status"] == "missing"],
                      "misplaced": [e["id"] for e in report if e["status"] in ("early", "late")],
                      "out_of_order": out_of_order, "unknown_labels": unknown}, ensure_ascii=False))


if __name__ == "__main__":
    main()
