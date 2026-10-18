#!/usr/bin/env python3
"""Score students against a weighted analytic rubric, exactly, with grade bands and feedback
built from the level descriptors.

Input (JSON on stdin):
  rubric    {"criteria": [{"name": "Content", "weight": 2,
                           "levels": [{"label": "Excellent", "points": 4, "descriptor": "..."}, ...]}, ...]}
  students  [{"student": "Ana", "scores": {"Content": "Good", "Language": 2}, "comment": "..."}]
            (a score is a level label or a number of points from 0 to the criterion's maximum)
  scores    instead of students: one student's scores
  bands     optional [{"min_percent": 90, "grade": "A"}, ...]
  scale     optional number: also give the result on 0..scale (e.g. 10)
  decimals  optional, default 2

Criterion share = weight x points / max points of the criterion; percentage = sum of shares /
sum of weights x 100.
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, localcontext
from fractions import Fraction
DECIMAL = re.compile(r"\s*[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?\s*")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def number(value, field, allow_zero=True):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    if isinstance(value, str) and not DECIMAL.fullmatch(value):
        fail(f"{field} must be a number or a decimal string")
    try:
        result = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number or a decimal string")
    if abs(result) > 10 ** 12:
        fail(f"{field} is too large")
    if result < 0 or (not allow_zero and result == 0):
        fail(f"{field} must be {'zero or more' if allow_zero else 'positive'}")
    return result


def text(value, places):
    with localcontext() as ctx:
        ctx.prec = 60
        d = Decimal(value.numerator) / Decimal(value.denominator)
        return str(d.quantize(Decimal(1).scaleb(-places), rounding=ROUND_HALF_UP))


def frac(value):
    """Exact text: a terminating decimal when there is one (2.5), else the fraction (19/6)."""
    d = value.denominator
    for p in (2, 5):
        while d % p == 0:
            d //= p
    if d != 1:
        return str(value)
    with localcontext() as ctx:
        ctx.prec = 60
        return format((Decimal(value.numerator) / Decimal(value.denominator)).normalize(), "f")


def read_rubric(rubric):
    if not isinstance(rubric, dict) or set(rubric) - {"criteria", "title"} or not isinstance(rubric.get("criteria"), list):
        fail("rubric must be an object with a criteria list (and an optional title)")
    criteria = []
    names = set()
    for i, c in enumerate(rubric["criteria"]):
        if not isinstance(c, dict) or set(c) - {"name", "weight", "levels"}:
            fail(f"rubric.criteria[{i}] must be an object with name, weight and levels")
        name = c.get("name")
        if not isinstance(name, str) or not name.strip():
            fail(f"rubric.criteria[{i}].name must be a non-empty string")
        if name in names:
            fail(f"criterion {name!r} appears twice")
        names.add(name)
        weight = number(c.get("weight", 1), f"criteria {name!r} weight", allow_zero=False)
        levels = c.get("levels")
        if not isinstance(levels, list) or not levels:
            fail(f"criterion {name!r} needs a non-empty levels list")
        rows, labels = [], set()
        for j, lv in enumerate(levels):
            if not isinstance(lv, dict) or set(lv) - {"label", "points", "descriptor"} or "points" not in lv:
                fail(f"criterion {name!r} levels[{j}] must be an object with label, points and descriptor")
            label = lv.get("label", str(lv["points"]))
            if not isinstance(label, str) or label in labels:
                fail(f"criterion {name!r} has a missing or repeated level label")
            labels.add(label)
            descriptor = lv.get("descriptor", "")
            if not isinstance(descriptor, str):
                fail(f"criterion {name!r} level {label!r} descriptor must be a string")
            rows.append({"label": label, "points": number(lv["points"], f"criterion {name!r} level {label!r} points"),
                         "descriptor": descriptor})
        top = max(r["points"] for r in rows)
        if top == 0:
            fail(f"criterion {name!r} has no level with points above zero")
        criteria.append({"name": name, "weight": weight, "levels": rows, "max": top})
    if not criteria:
        fail("rubric.criteria must not be empty")
    return criteria


def read_bands(bands):
    if bands is None:
        return None
    if not isinstance(bands, list) or not bands:
        fail("bands must be a non-empty list of {min_percent, grade}")
    out = []
    for i, b in enumerate(bands):
        if not isinstance(b, dict) or set(b) != {"min_percent", "grade"} or not isinstance(b["grade"], str):
            fail(f"bands[{i}] must be {{min_percent, grade}}")
        out.append((number(b["min_percent"], f"bands[{i}].min_percent"), b["grade"]))
    return sorted(out, key=lambda b: b[0], reverse=True)


def grade(student, criteria, bands, scale, places):
    scores = student["scores"]
    if not isinstance(scores, dict):
        fail(f"scores of {student['student']!r} must be an object of criterion -> level label or points")
    unknown = sorted(set(scores) - {c["name"] for c in criteria})
    if unknown:
        fail(f"unknown criterion in scores of {student['student']!r}: {', '.join(unknown)}")
    rows, shares, total_w, raw, raw_max = [], Fraction(0), Fraction(0), Fraction(0), Fraction(0)
    feedback = []
    for c in criteria:
        if c["name"] not in scores:
            fail(f"{student['student']!r} has no score for {c['name']!r}")
        given = scores[c["name"]]
        level = None
        if isinstance(given, str) and any(lv["label"] == given for lv in c["levels"]):
            level = next(lv for lv in c["levels"] if lv["label"] == given)
            points = level["points"]
        else:
            points = number(given, f"score {c['name']!r} of {student['student']!r} (a level label or points)")
            if points > c["max"]:
                fail(f"score {c['name']!r} of {student['student']!r} exceeds the maximum {frac(c['max'])}")
            below = [lv for lv in c["levels"] if lv["points"] <= points]
            level = max(below, key=lambda lv: lv["points"]) if below else min(c["levels"], key=lambda lv: lv["points"])
        share = c["weight"] * points / c["max"]
        shares += share
        total_w += c["weight"]
        raw += points
        raw_max += c["max"]
        exact_level = level["points"] == points
        rows.append({"criterion": c["name"], "weight": frac(c["weight"]), "level": level["label"],
                     "points": frac(points), "max_points": frac(c["max"]), "share": str(share),
                     "percent": text(points / c["max"] * 100, places), "between_levels": not exact_level})
        line = f"{c['name']}: {level['label']} ({frac(points)}/{frac(c['max'])})"
        if level["descriptor"]:
            line += f" - {level['descriptor']}"
        if not exact_level:
            line += " [points between levels; descriptor of the level below]"
        feedback.append(line)
    percent = shares / total_w * 100
    out = {"student": student["student"], "criteria": rows, "raw_points": frac(raw), "raw_max": frac(raw_max),
           "weighted_score": str(shares), "weighted_max": frac(total_w), "percentage": text(percent, places),
           "percentage_exact": str(percent)}
    if scale is not None:
        out["score_on_scale"] = text(percent * scale / 100, places)
    if bands is not None:
        out["grade"] = next((g for low, g in bands if percent >= low), None)
    out["feedback"] = feedback
    if student.get("comment"):
        out["comment"] = student["comment"]
    out["_percent"] = percent
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"rubric", "students", "scores", "bands", "scale", "decimals"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    criteria = read_rubric(request.get("rubric"))
    bands = read_bands(request.get("bands"))
    scale = number(request["scale"], "scale", allow_zero=False) if request.get("scale") is not None else None
    places = request.get("decimals", 2)
    if isinstance(places, bool) or not isinstance(places, int) or not 0 <= places <= 10:
        fail("decimals must be an integer from 0 to 10")
    if "students" in request and "scores" in request:
        fail("give students or scores, not both")
    if "students" in request:
        students = request["students"]
        if not isinstance(students, list) or not students or len(students) > 2000:
            fail("students must be a list of 1 to 2000 {student, scores}")
        for i, s in enumerate(students):
            if not isinstance(s, dict) or set(s) - {"student", "scores", "comment"} or "scores" not in s:
                fail(f"students[{i}] must be an object with student, scores and an optional comment")
            s.setdefault("student", str(i + 1))
            if s.get("comment") is not None and not isinstance(s["comment"], str):
                fail(f"students[{i}].comment must be a string")
    elif "scores" in request:
        students = [{"student": "1", "scores": request["scores"]}]
    else:
        fail("give students (a list) or scores (one student)")
    results = [grade(s, criteria, bands, scale, places) for s in students]
    percents = [r.pop("_percent") for r in results]
    out = {"criteria": [{"name": c["name"], "weight": frac(c["weight"]), "max_points": frac(c["max"])} for c in criteria],
           "results": results}
    if len(results) > 1:
        summary = {"students": len(results), "mean_percentage": text(sum(percents) / len(percents), places),
                   "min_percentage": text(min(percents), places), "max_percentage": text(max(percents), places)}
        if bands is not None:
            summary["grades"] = {g: sum(r.get("grade") == g for r in results) for _, g in sorted(bands, key=lambda b: -b[0])}
        out["summary"] = summary
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
