#!/usr/bin/env python3
"""Guess the language of a text with character n-grams (Cavnar & Trenkle 1994, "N-Gram-Based
Text Categorization"): the text's 1-3-grams ranked by frequency are compared with the bundled
ranked profiles of 20 languages (profiles.json, counted from Unicode CLDR locale data by
tools/catalog/data/language_detect.py), and the "out-of-place" distance picks the language.
A heuristic: short texts and closely related languages (Danish/Norwegian, Spanish/Catalan)
are unreliable, and a text in a language without a profile is still matched to the nearest one
unless it is too far from all of them.

Input (JSON on stdin):
  text          the text (required)
  top_k         how many candidates to return, 1-20 (default 3)
  languages     restrict the candidates to these codes, e.g. ["es", "ca", "pt"]
  max_distance  "unknown" when the best distance is above this (default 0.6)
  min_margin    "unknown" when the best two distances differ by less than this (default 0.01)
"""
import json
import pathlib
import sys
import unicodedata

MAX_CHARS = 100000
DEFAULTS = {"top_k": 3, "max_distance": 0.6, "min_margin": 0.01}
RELIABLE_LETTERS = 30


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def words(text):
    text = unicodedata.normalize("NFC", text).lower()
    return "".join(c if c.isalpha() else " " for c in text).split()


def ngrams(word_list):
    for word in word_list:
        padded = f" {word} "
        for n in (1, 2, 3):
            for i in range(len(padded) - n + 1):
                gram = padded[i:i + n]
                if gram.strip():
                    yield gram


def ranked(counts, size):
    return [g for g, _ in sorted(counts.items(), key=lambda item: (-item[1], item[0]))[:size]]


def number(request, key, low, high, integer=False):
    value = request.get(key, DEFAULTS[key])
    if isinstance(value, bool) or not isinstance(value, (int, float)) or (integer and not isinstance(value, int)):
        fail(f"{key} must be {'an integer' if integer else 'a number'} from {low} to {high}")
    if not low <= value <= high:
        fail(f"{key} must be from {low} to {high}")
    return value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"text", "top_k", "languages", "max_distance", "min_margin"})
    if extra:
        fail(f"unknown field: {extra[0]}")
    text = request.get("text")
    if not isinstance(text, str):
        fail("text must be a string")
    if len(text) > MAX_CHARS:
        fail(f"text is longer than {MAX_CHARS} characters")
    top_k = number(request, "top_k", 1, 20, integer=True)
    max_distance = number(request, "max_distance", 0, 1)
    min_margin = number(request, "min_margin", 0, 1)
    data = json.loads((pathlib.Path(__file__).parent / "profiles.json").read_text(encoding="utf-8"))
    profiles = data["profiles"]
    chosen = list(profiles)
    if "languages" in request:
        wanted = request["languages"]
        if not isinstance(wanted, list) or not wanted or not all(isinstance(c, str) for c in wanted):
            fail("languages must be a non-empty list of language codes")
        unknown = [c for c in wanted if c not in profiles]
        if unknown:
            fail(f"no profile for {unknown[0]!r}; profiles: {', '.join(profiles)}")
        chosen = list(dict.fromkeys(wanted))

    word_list = words(text)
    letters = sum(len(w) for w in word_list)
    counts = {}
    for gram in ngrams(word_list):
        counts[gram] = counts.get(gram, 0) + 1
    size = max(len(p["ngrams"]) for p in profiles.values())
    document = ranked(counts, size)
    candidates = []
    for code in chosen:
        rank = {g: i for i, g in enumerate(ranked(profiles[code]["ngrams"], size))}
        if document:
            total = sum(abs(rank[g] - i) if g in rank else size for i, g in enumerate(document))
            distance = total / (len(document) * size)
        else:
            distance = 1.0
        candidates.append((distance, code))
    candidates.sort()

    best_distance, best = candidates[0]
    margin = candidates[1][0] - best_distance if len(candidates) > 1 else 1.0
    reason = None
    if letters < 3:
        reason = "too short: fewer than 3 letters"
    elif best_distance >= 1.0:
        reason = "none of the text's n-grams is in any profile (another script or language)"
    elif best_distance > max_distance:
        reason = f"best distance {best_distance:.4f} is above max_distance {max_distance}: probably a language without a profile"
    elif margin < min_margin:
        reason = f"{best} and {candidates[1][1]} are too close (margin {margin:.4f} < min_margin {min_margin})"
    language = "unknown" if reason else best
    result = {
        "language": language,
        "name": profiles[best]["name"] if not reason else None,
        "reliable": not reason and letters >= RELIABLE_LETTERS,
    }
    if reason:
        result["reason"] = reason
    elif letters < RELIABLE_LETTERS:
        result["reason"] = f"short text ({letters} letters): treat the guess with caution"
    result.update({
        "margin": round(margin, 4),
        "candidates": [{"language": c, "name": profiles[c]["name"], "distance": round(d, 4), "score": round(1 - d, 4)} for d, c in candidates[:top_k]],
        "letters": letters,
        "ngrams_compared": len(document),
        "thresholds": {"max_distance": max_distance, "min_margin": min_margin},
        "method": "Cavnar & Trenkle (1994) out-of-place distance between ranked character 1-3-gram profiles; distance 0 = same ranking, 1 = nothing in common; score = 1 - distance",
        "note": "a heuristic: short texts, mixed-language texts and close languages (da/no, es/ca/pt) are unreliable; only the 20 profiled languages can be named",
        "source": {"corpus": data["corpus"], "licence": data["corpus_licence"], "generated": data["generated"], "generator": data["generator"],
                   "udhr_not_used": data["udhr_not_used"]},
    })
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
