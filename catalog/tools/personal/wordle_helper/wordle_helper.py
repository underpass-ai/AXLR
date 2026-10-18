#!/usr/bin/env python3
"""Wordle helper: score a guess against an answer with the game's rules for repeated letters,
filter a word list by the feedback of earlier guesses, and rank the remaining candidates by letter
frequency or by expected information (entropy of the feedback pattern).

Input (JSON on stdin):
  guesses   [{"guess": "crane", "feedback": "..y.g"}, ...]   feedback per letter:
            g / G / 2 / 🟩 green (right letter, right place); y / Y / 1 / 🟨 yellow (in the word,
            elsewhere); . / b / x / - / 0 / ⬛ / ⬜ grey (not in the word, or no more copies of it)
  words     list of allowed words (default: the bundled words5.txt, five-letter words from SCOWL)
  rank      "entropy" | "frequency" (default entropy)
  top       how many suggestions (default 10, at most 100)
  hard_mode true: suggest only words that are still possible answers (default false for entropy)
  score     {"guess": "speed", "answer": "abide"}: only compute the feedback of one guess
"""
import json
import math
import sys
from collections import Counter

GREEN, YELLOW, GREY = "g", "y", "."
CODES = {"g": GREEN, "2": GREEN, "🟩": GREEN, "y": YELLOW, "1": YELLOW, "🟨": YELLOW,
         ".": GREY, "b": GREY, "x": GREY, "-": GREY, "0": GREY, "⬛": GREY, "⬜": GREY, "_": GREY}
WORK_LIMIT = 400000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def feedback(guess, answer):
    """Two passes, as in the game: greens first, then yellows from the answer's remaining letters
    left to right; extra copies of a letter are grey."""
    result = [GREY] * len(guess)
    remaining = Counter()
    for i, (g, a) in enumerate(zip(guess, answer)):
        if g == a:
            result[i] = GREEN
        else:
            remaining[a] += 1
    for i, g in enumerate(guess):
        if result[i] != GREEN and remaining[g] > 0:
            result[i] = YELLOW
            remaining[g] -= 1
    return "".join(result)


def parse_feedback(text, length, field):
    if not isinstance(text, str):
        fail(f"{field} must be a string such as \"..y.g\"")
    out = []
    for ch in text.replace("️", "").replace(" ", ""):
        code = CODES.get(ch.lower() if ch.isascii() else ch)
        if code is None:
            fail(f"{field}: {ch!r} is not g, y or . (or 2/1/0, or the coloured squares)")
        out.append(code)
    if len(out) != length:
        fail(f"{field} has {len(out)} marks for a {length}-letter guess")
    return "".join(out)


def word(text, field):
    if not isinstance(text, str) or not text.strip().isalpha() or not text.strip().isascii():
        fail(f"{field} must be a word of letters a-z")
    return text.strip().lower()


def load_bundled():
    try:
        with open("words5.txt", encoding="utf-8") as handle:
            return [line.strip() for line in handle if line.strip() and not line.startswith("#")]
    except OSError as error:
        fail(f"cannot read the bundled words5.txt: {error}")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"guesses", "words", "rank", "top", "hard_mode", "score"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    if "score" in request:
        s = request["score"]
        if not isinstance(s, dict) or set(s) != {"guess", "answer"}:
            fail("score must be {\"guess\": ..., \"answer\": ...}")
        g, a = word(s["guess"], "score.guess"), word(s["answer"], "score.answer")
        if len(g) != len(a):
            fail("score: guess and answer must have the same length")
        fb = feedback(g, a)
        print(json.dumps({"guess": g, "answer": a, "feedback": fb,
                          "squares": fb.replace(GREEN, "🟩").replace(YELLOW, "🟨").replace(GREY, "⬛"),
                          "letters": [{"letter": c, "mark": {"g": "green", "y": "yellow", ".": "grey"}[m]} for c, m in zip(g, fb)]},
                         ensure_ascii=False))
        return
    if "words" in request:
        raw = request["words"]
        if not isinstance(raw, list) or not raw or len(raw) > 50000:
            fail("words must be a list of 1 to 50000 words")
        words = [word(w, f"words[{i}]") for i, w in enumerate(raw)]
        source = "words given in the input"
    else:
        words = load_bundled()
        source = "bundled words5.txt (SCOWL/ESDB size 35, five-letter words; not the official Wordle list)"
    lengths = {len(w) for w in words}
    if len(lengths) != 1:
        fail("all words must have the same length")
    length = lengths.pop()
    words = sorted(set(words))
    guesses = request.get("guesses", [])
    if not isinstance(guesses, list) or len(guesses) > 20:
        fail("guesses must be a list of at most 20 guesses")
    constraints = []
    for i, item in enumerate(guesses):
        if not isinstance(item, dict) or set(item) != {"guess", "feedback"}:
            fail(f"guesses[{i}] must be {{\"guess\": ..., \"feedback\": ...}}")
        g = word(item["guess"], f"guesses[{i}].guess")
        if len(g) != length:
            fail(f"guesses[{i}].guess has {len(g)} letters; the words have {length}")
        constraints.append((g, parse_feedback(item["feedback"], length, f"guesses[{i}].feedback")))
    candidates = [w for w in words if all(feedback(g, w) == fb for g, fb in constraints)]
    rank = request.get("rank", "entropy")
    if rank not in ("entropy", "frequency"):
        fail("rank must be entropy or frequency")
    top = request.get("top", 10)
    if isinstance(top, bool) or not isinstance(top, int) or not 1 <= top <= 100:
        fail("top must be an integer from 1 to 100")
    hard = request.get("hard_mode", False)
    if not isinstance(hard, bool):
        fail("hard_mode must be a boolean")

    result = {"word_list": source, "word_count": len(words), "length": length, "candidates_count": len(candidates)}
    if len(candidates) <= 50:
        result["candidates"] = candidates
    if not candidates:
        result["note"] = "no word in the list fits all the feedback: check the marks, or the answer is not in the list"
        result["suggestions"] = []
        print(json.dumps(result, ensure_ascii=False))
        return
    # letter frequency among the candidates (each letter counted once per word)
    freq = Counter()
    for w in candidates:
        freq.update(set(w))
    pos = [Counter(w[i] for w in candidates) for i in range(length)]
    freq_score = lambda w: sum(freq[c] for c in set(w)) + sum(pos[i][c] for i, c in enumerate(w)) / (len(candidates) + 1)
    result["letter_frequency"] = {c: n for c, n in sorted(freq.items(), key=lambda kv: (-kv[1], kv[0]))}
    candidate_set = set(candidates)
    if rank == "frequency" or len(candidates) <= 2:
        ordered = sorted(candidates, key=lambda w: (-freq_score(w), w))[:top]
        result["rank"] = "frequency" if rank == "frequency" else "entropy (two or fewer candidates: guess one)"
        result["suggestions"] = [{"word": w, "score": sum(freq[c] for c in set(w)), "possible_answer": True} for w in ordered]
    else:
        pool = candidates if hard else words
        n = len(candidates)
        budget = max(top, WORK_LIMIT // n)
        if len(pool) > budget:
            pool = sorted(pool, key=lambda w: (-freq_score(w), w))[:budget]
            result["guesses_evaluated"] = f"{budget} best by letter frequency (of {len(candidates) if hard else len(words)})"
        scored = []
        for g in pool:
            buckets = Counter(feedback(g, w) for w in candidates)
            entropy = -sum(c / n * math.log2(c / n) for c in buckets.values())
            expected = sum(c * c for c in buckets.values()) / n
            scored.append((-round(entropy, 12), g not in candidate_set, g, entropy, expected, len(buckets)))
        scored.sort()
        result["rank"] = "entropy" + (" (hard mode: possible answers only)" if hard else "")
        result["max_entropy_bits"] = round(math.log2(n), 4)
        result["suggestions"] = [{"word": g, "entropy_bits": round(e, 4), "expected_remaining": round(x, 3), "patterns": p,
                                  "possible_answer": g in candidate_set} for _, _, g, e, x, p in scored[:top]]
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
