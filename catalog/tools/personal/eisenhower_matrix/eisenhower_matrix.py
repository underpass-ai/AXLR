#!/usr/bin/env python3
"""Eisenhower matrix: sort tasks into the four quadrants of urgency and importance and return an
action plan ordered by due date and effort, as JSON and as Markdown.

Input (JSON on stdin):
  tasks       [{"name", "urgent"?, "important"?, "urgency"?, "importance"?, "due"?, "effort"?, "notes"?}]
              urgent / important  booleans (win over everything else)
              urgency / importance  scores 1-5, compared with the thresholds
              due     "YYYY-MM-DD": urgent when due within `due_within_days` of `today` (or overdue)
              effort  hours (or any unit, used only to order)
  today       "YYYY-MM-DD"  required when a task has a due date (never the system clock)
  thresholds  {"urgency": 4, "importance": 3, "due_within_days": 2}
  include_eliminate  true: list quadrant 4 in the plan too (default false)
"""
import datetime as dt
import json
import sys
from decimal import Decimal, InvalidOperation

QUADRANTS = {
    1: ("do", "Do first", "urgent and important"),
    2: ("schedule", "Schedule", "important, not urgent"),
    3: ("delegate", "Delegate", "urgent, not important"),
    4: ("eliminate", "Eliminate", "neither urgent nor important"),
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def date(value, field):
    try:
        return dt.date.fromisoformat(value)
    except (TypeError, ValueError):
        fail(f"{field} must be a date YYYY-MM-DD")


def score(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not 1 <= value <= 5:
        fail(f"{field} must be a number from 1 to 5")
    return value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"tasks", "today", "thresholds", "include_eliminate"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    tasks = request.get("tasks")
    if not isinstance(tasks, list) or not tasks or len(tasks) > 1000:
        fail("tasks must be a list of 1 to 1000 tasks")
    th = request.get("thresholds", {})
    if not isinstance(th, dict) or set(th) - {"urgency", "importance", "due_within_days"}:
        fail("thresholds may have urgency, importance and due_within_days")
    u_th = score(th.get("urgency", 4), "thresholds.urgency")
    i_th = score(th.get("importance", 3), "thresholds.importance")
    window = th.get("due_within_days", 2)
    if isinstance(window, bool) or not isinstance(window, int) or window < 0:
        fail("thresholds.due_within_days must be a non-negative integer")
    include4 = request.get("include_eliminate", False)
    if not isinstance(include4, bool):
        fail("include_eliminate must be a boolean")
    today = date(request["today"], "today") if "today" in request else None

    rows = []
    for i, task in enumerate(tasks):
        f = f"tasks[{i}]"
        if isinstance(task, str):
            task = {"name": task}
        if not isinstance(task, dict):
            fail(f"{f} must be an object")
        extra = set(task) - {"name", "urgent", "important", "urgency", "importance", "due", "effort", "notes"}
        if extra:
            fail(f"{f}: unknown keys {sorted(extra)}")
        name = task.get("name")
        if not isinstance(name, str) or not name.strip():
            fail(f"{f}.name must be a non-empty string")
        reasons = []
        due = date(task["due"], f + ".due") if task.get("due") is not None else None
        days = None
        if due is not None:
            if today is None:
                fail("today is required when tasks have due dates")
            days = (due - today).days
        if "urgent" in task:
            if not isinstance(task["urgent"], bool):
                fail(f"{f}.urgent must be a boolean")
            urgent = task["urgent"]
            reasons.append("urgent: given")
        elif "urgency" in task:
            urgent = score(task["urgency"], f + ".urgency") >= u_th
            reasons.append(f"urgency {task['urgency']} {'>=' if urgent else '<'} {u_th}")
            if not urgent and days is not None and days <= window:
                urgent = True
                reasons[-1] += f", but due in {days} day(s)" if days >= 0 else f", but overdue by {-days} day(s)"
        elif days is not None:
            urgent = days <= window
            reasons.append(f"due in {days} day(s) {'<=' if urgent else '>'} {window}" if days >= 0 else f"overdue by {-days} day(s)")
        else:
            urgent = False
            reasons.append("no urgency or due date: not urgent")
        if "important" in task:
            if not isinstance(task["important"], bool):
                fail(f"{f}.important must be a boolean")
            important = task["important"]
            reasons.append("important: given")
        elif "importance" in task:
            important = score(task["importance"], f + ".importance") >= i_th
            reasons.append(f"importance {task['importance']} {'>=' if important else '<'} {i_th}")
        else:
            important = False
            reasons.append("no importance given: not important")
        effort = None
        if task.get("effort") is not None:
            try:
                effort = Decimal(str(task["effort"]))
            except InvalidOperation:
                fail(f"{f}.effort must be a number")
            if isinstance(task["effort"], bool) or not effort.is_finite() or effort < 0:
                fail(f"{f}.effort must be a non-negative number")
        quadrant = 1 if urgent and important else 2 if important else 3 if urgent else 4
        row = {"name": name.strip(), "quadrant": quadrant, "action": QUADRANTS[quadrant][0], "urgent": urgent,
               "important": important, "why": "; ".join(reasons)}
        if due is not None:
            row["due"] = due.isoformat()
            row["days_left"] = days
            if days < 0:
                row["overdue"] = True
        if effort is not None:
            row["effort"] = float(effort) if effort != effort.to_integral_value() else int(effort)
        if task.get("notes") is not None:
            if not isinstance(task["notes"], str):
                fail(f"{f}.notes must be a string")
            row["notes"] = task["notes"]
        row["_order"] = (quadrant, due or dt.date.max, effort if effort is not None else Decimal("Infinity"),
                         -(task.get("importance") or 0), i)
        rows.append(row)

    ordered = sorted(rows, key=lambda r: r["_order"])
    plan = []
    for r in ordered:
        if r["quadrant"] == 4 and not include4:
            continue
        step = {"step": len(plan) + 1, "task": r["name"], "action": QUADRANTS[r["quadrant"]][1]}
        if "due" in r:
            step["due"] = r["due"]
        if "effort" in r:
            step["effort"] = r["effort"]
        plan.append(step)
    quadrants = {}
    for q, (key, title, meaning) in QUADRANTS.items():
        quadrants[key] = {"quadrant": q, "title": title, "meaning": meaning,
                          "tasks": [r["name"] for r in ordered if r["quadrant"] == q]}
    for r in rows:
        del r["_order"]

    md = ["# Eisenhower matrix", ""]
    if today:
        md += [f"As of {today.isoformat()}.", ""]
    md += ["| | Urgent | Not urgent |", "|:--|:--|:--|"]
    cell = lambda key: "<br>".join(quadrants[key]["tasks"]) or "—"
    md.append(f"| **Important** | **Do:** {cell('do')} | **Schedule:** {cell('schedule')} |")
    md.append(f"| **Not important** | **Delegate:** {cell('delegate')} | **Eliminate:** {cell('eliminate')} |")
    md += ["", "## Plan", ""]
    for s in plan:
        extra = []
        if "due" in s:
            extra.append(f"due {s['due']}")
        if "effort" in s:
            extra.append(f"effort {s['effort']}")
        md.append(f"{s['step']}. {s['action']}: {s['task']}" + (f" ({', '.join(extra)})" if extra else ""))
    if not plan:
        md.append("Nothing to do.")
    eliminated = quadrants["eliminate"]["tasks"]
    if eliminated and not include4:
        md += ["", "Dropped (neither urgent nor important): " + ", ".join(eliminated) + "."]
    result = {"today": today.isoformat() if today else None,
              "thresholds": {"urgency": u_th, "importance": i_th, "due_within_days": window},
              "quadrants": quadrants, "plan": plan, "tasks": rows,
              "order": "quadrant (do, schedule, delegate), then due date (earliest first, none last), then effort (smallest first), then importance",
              "markdown": "\n".join(md) + "\n"}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
