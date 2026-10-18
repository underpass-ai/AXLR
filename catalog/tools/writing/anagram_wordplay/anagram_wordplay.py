#!/usr/bin/env python3
"""Word play: anagrams (single and multi-word) from a word list, palindrome check, lipogram
check, word ladder (shortest chain of one-letter changes) and letter frequencies.

Input (JSON on stdin):
  operation      "anagrams" | "palindrome" | "lipogram" | "word_ladder" | "letter_frequency"
  text           the word, phrase or text (anagrams, palindrome, lipogram, letter_frequency)
  words          ["...", ...]  your word list (anagrams, word_ladder)
  bundled        true to add the bundled English list words_en.txt (SCOWL/ESDB size 35)
  multi_word     anagrams: also look for phrases of several words (default false)
  max_words      anagrams: words per phrase, 2 to 4 (default 3)
  min_word_length anagrams: shortest word allowed in a phrase (default 2)
  max_results    anagrams: cap on results (default 100, at most 1000)
  time_limit     seconds for the multi-word search (default 5, at most 20)
  letters        lipogram: the letters that must not appear, e.g. "e" or ["a", "e"]
  start, end     word_ladder: the two words, same length
Letters are compared without case and without accents (NFKD, combining marks removed).
"""
import json
import sys
import time
import unicodedata
from collections import Counter, deque

BUNDLED = "words_en.txt"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fold(text):
    """Lowercase and strip accents: 'Ñandú' -> 'nandu'. Letters without a decomposition stay."""
    decomposed = unicodedata.normalize("NFKD", text.lower())
    return "".join(c for c in decomposed if not unicodedata.combining(c))


def letters_only(text):
    return "".join(c for c in fold(text) if c.isalpha())


def get_text(request, field="text"):
    value = request.get(field)
    if not isinstance(value, str) or not value.strip():
        fail(f"{field} must be a non-empty string")
    if len(value) > 200000:
        fail(f"{field} is longer than 200000 characters")
    return value


def get_int(request, field, default, low, high):
    value = request.get(field, default)
    if not isinstance(value, int) or isinstance(value, bool) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def load_words(request, required=True):
    words = request.get("words", [])
    if not isinstance(words, list) or not all(isinstance(w, str) for w in words):
        fail("words must be a list of strings")
    if len(words) > 200000:
        fail("words has more than 200000 entries")
    bundled = request.get("bundled", False)
    if not isinstance(bundled, bool):
        fail("bundled must be true or false")
    found = {}
    for word in words:
        key = letters_only(word)
        if key:
            found.setdefault(key, word.strip())
    source = ["input"] if words else []
    if bundled:
        with open(BUNDLED, encoding="utf-8") as handle:
            for line in handle:
                line = line.strip()
                if line and not line.startswith("#"):
                    found.setdefault(line, line)
        source.append("bundled SCOWL/ESDB size-35 English list (words_en.txt, 1-9 letters)")
    if required and not found:
        fail("give words, or bundled: true")
    return found, source


def anagrams(request):
    text = get_text(request)
    target = letters_only(text)
    if not target:
        fail("text has no letters")
    if len(target) > 40:
        fail("text has more than 40 letters")
    words, source = load_words(request)
    multi = request.get("multi_word", False)
    if not isinstance(multi, bool):
        fail("multi_word must be true or false")
    max_words = get_int(request, "max_words", 3, 2, 4)
    min_len = get_int(request, "min_word_length", 2, 1, 20)
    max_results = get_int(request, "max_results", 100, 1, 1000)
    limit = request.get("time_limit", 5)
    if not isinstance(limit, (int, float)) or isinstance(limit, bool) or not 0 < limit <= 20:
        fail("time_limit must be a number of seconds from 0 to 20")
    key = sorted(target)
    single = sorted(display for norm, display in words.items() if sorted(norm) == key and norm != target)
    result = {"operation": "anagrams", "text": text, "letters": "".join(key), "word_list": source,
              "single_word": single[:max_results], "single_word_count": len(single)}
    if multi:
        pool = Counter(target)
        candidates = sorted(norm for norm in words if len(norm) >= min_len and not Counter(norm) - pool)
        deadline = time.monotonic() + limit
        phrases, complete = [], True

        def search(start, remaining, chosen):
            nonlocal complete
            if time.monotonic() > deadline or len(phrases) >= max_results:
                complete = False
                return
            if not remaining:
                if len(chosen) > 1:
                    phrases.append(" ".join(words[w] for w in chosen))
                return
            if len(chosen) == max_words:
                return
            for i in range(start, len(candidates)):
                word = candidates[i]
                need = Counter(word)
                if not need - remaining:
                    search(i, remaining - need, chosen + [word])
                    if not complete:
                        return

        search(0, pool, [])
        result.update({"multi_word": phrases, "multi_word_count": len(phrases), "search_complete": complete,
                       "max_words": max_words, "min_word_length": min_len})
    return result


def palindrome(request):
    text = get_text(request)
    norm = "".join(c for c in fold(text) if c.isalnum())
    if not norm:
        fail("text has no letters or digits")
    words = [letters_only(w) for w in text.split()]
    words = [w for w in words if w]
    return {"operation": "palindrome", "text": text, "normalized": norm, "is_palindrome": norm == norm[::-1],
            "length": len(norm), "word_palindrome": len(words) > 1 and words == words[::-1],
            "rule": "letters and digits only, case and accents ignored"}


def lipogram(request):
    text = get_text(request)
    letters = request.get("letters")
    if isinstance(letters, str):
        letters = list(letters)
    if not isinstance(letters, list) or not letters or not all(isinstance(c, str) and len(c) == 1 for c in letters):
        fail("letters must be a string or a list of single letters")
    banned = {letters_only(c) for c in letters} - {""}
    if not banned:
        fail("letters has no letters")
    violations = []
    for offset, char in enumerate(text):
        folded = letters_only(char)
        if folded and folded in banned:
            violations.append({"offset": offset, "char": char})
    counts = Counter(letters_only(v["char"]) for v in violations)
    return {"operation": "lipogram", "letters": sorted(banned), "is_lipogram": not violations,
            "violation_count": len(violations), "by_letter": dict(sorted(counts.items())),
            "violations": violations[:200], "offsets": "code-point indices into text, from 0"}


def word_ladder(request):
    start, end = letters_only(get_text(request, "start")), letters_only(get_text(request, "end"))
    if len(start) != len(end):
        fail("start and end must have the same number of letters")
    words, source = load_words(request)
    pool = {w for w in words if len(w) == len(start)} | {start, end}
    previous = {start: None}
    queue = deque([start])
    while queue:
        word = queue.popleft()
        if word == end:
            break
        for i in range(len(word)):
            for letter in "abcdefghijklmnopqrstuvwxyz":
                step = word[:i] + letter + word[i + 1:]
                if step in pool and step not in previous:
                    previous[step] = word
                    queue.append(step)
    result = {"operation": "word_ladder", "start": start, "end": end, "word_list": source}
    if end not in previous:
        result.update({"found": False, "ladder": [], "steps": None,
                       "note": "no chain of one-letter changes through the word list"})
        return result
    path, word = [], end
    while word is not None:
        path.append(word)
        word = previous[word]
    path.reverse()
    result.update({"found": True, "ladder": path, "steps": len(path) - 1,
                   "note": "a shortest ladder (breadth-first search); other ladders of the same length may exist"})
    return result


def letter_frequency(request):
    text = get_text(request)
    letters = [c for c in letters_only(text)]
    counts = Counter(letters)
    total = len(letters)
    ascii_letters = "abcdefghijklmnopqrstuvwxyz"
    ranked = sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))
    return {"operation": "letter_frequency", "total_letters": total,
            "counts": {k: v for k, v in ranked},
            "percent": {k: f"{100 * v / total:.2f}" for k, v in ranked} if total else {},
            "missing_letters": [c for c in ascii_letters if c not in counts],
            "is_pangram": all(c in counts for c in ascii_letters),
            "rule": "case and accents ignored; letters outside a-z after folding are counted too"}


OPERATIONS = {"anagrams": anagrams, "palindrome": palindrome, "lipogram": lipogram, "word_ladder": word_ladder,
              "letter_frequency": letter_frequency}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    operation = request.get("operation")
    if not isinstance(operation, str) or operation not in OPERATIONS:
        fail("operation must be one of " + ", ".join(OPERATIONS))
    print(json.dumps(OPERATIONS[operation](request), ensure_ascii=False))


if __name__ == "__main__":
    main()
