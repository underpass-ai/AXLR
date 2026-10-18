#!/usr/bin/env python3
"""SuperMemo 2 spaced repetition: the next interval, repetition count and easiness factor of
one card after a review graded 0-5, or a whole review history replayed from the start.

Input (JSON on stdin), either one review:
  quality       0-5 (5 perfect, 3 correct with difficulty, below 3 a lapse)
  repetitions   successful reviews in a row so far (default 0)
  easiness      easiness factor so far (default 2.5)
  interval      days of the last interval (default 0)
  review_date   "YYYY-MM-DD" (optional; gives next_review)
or a history:
  reviews       [{"date": "YYYY-MM-DD", "quality": 4}, ...] in order
"""
import datetime as dt
import json
import math
import sys
from fractions import Fraction


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def step(quality, repetitions, easiness, interval):
    """One SM-2 review. EF' = EF + (0.1 - (5 - q)(0.08 + (5 - q) 0.02)), floored at 1.3; a grade
    below 3 restarts the repetitions without changing EF; I(1) = 1, I(2) = 6, I(n) = I(n-1) EF',
    rounded up to whole days."""
    if quality < 3:
        return 0, easiness, 1
    q = 5 - quality
    easiness = max(Fraction(13, 10), easiness + Fraction(1, 10) - q * (Fraction(8, 100) + q * Fraction(2, 100)))
    repetitions += 1
    if repetitions == 1:
        interval = 1
    elif repetitions == 2:
        interval = 6
    else:
        interval = math.ceil(interval * easiness)
    return repetitions, easiness, interval


def number(value, field, default):
    value = default if value is None else value
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number")
    try:
        return Fraction(str(value))
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number")


def grade(value, field):
    if not isinstance(value, int) or isinstance(value, bool) or not 0 <= value <= 5:
        fail(f"{field} must be an integer from 0 to 5")
    return value


def date(value, field):
    try:
        return dt.date.fromisoformat(value)
    except (TypeError, ValueError):
        fail(f"{field} must be a date YYYY-MM-DD")


def state(repetitions, easiness, interval, when):
    out = {"repetitions": repetitions, "easiness": float(round(easiness, 4)), "interval_days": interval}
    if when:
        out["next_review"] = (when + dt.timedelta(interval)).isoformat()
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if "reviews" in request:
        reviews = request["reviews"]
        if not isinstance(reviews, list) or not reviews:
            fail("reviews must be a non-empty list")
        repetitions, easiness, interval, history = 0, Fraction(5, 2), 0, []
        for i, r in enumerate(reviews):
            if not isinstance(r, dict):
                fail(f"reviews[{i}] must be an object with date and quality")
            when = date(r.get("date"), f"reviews[{i}].date")
            repetitions, easiness, interval = step(grade(r.get("quality"), f"reviews[{i}].quality"), repetitions, easiness, interval)
            history.append({"date": when.isoformat(), "quality": r["quality"], **state(repetitions, easiness, interval, when)})
        print(json.dumps({"algorithm": "SM-2", **{k: v for k, v in history[-1].items() if k not in ("date", "quality")}, "history": history}))
        return
    quality = grade(request.get("quality"), "quality")
    repetitions = request.get("repetitions", 0)
    if not isinstance(repetitions, int) or isinstance(repetitions, bool) or repetitions < 0:
        fail("repetitions must be a non-negative integer")
    easiness = number(request.get("easiness"), "easiness", "2.5")
    interval = number(request.get("interval"), "interval", 0)
    if easiness < Fraction(13, 10) or interval < 0:
        fail("easiness must be at least 1.3 and interval non-negative")
    when = date(request["review_date"], "review_date") if "review_date" in request else None
    print(json.dumps({"algorithm": "SM-2", **state(*step(quality, repetitions, easiness, interval), when)}))


if __name__ == "__main__":
    main()
