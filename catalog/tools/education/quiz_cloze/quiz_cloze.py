#!/usr/bin/env python3
"""Cloze (gap-fill) questions from a text, with an answer key and Anki cloze syntax.

Input (JSON on stdin):
  text             the passage (required)
  mode             "auto" | "terms" | "nth"      default auto
  count            auto: how many distinct words to blank, default 5
  rank_by          auto: "frequency" (then length) | "length" (then frequency), default frequency
  min_length       auto: shortest word considered, default 4
  terms            terms: the words or phrases to blank (case-insensitive, whole words)
  every_n          nth: blank every n-th word
  offset           nth: position of the first blank (1-based), default n (or seeded when seed is given)
  content_only     nth: count only content words (skip stopwords), default false
  all_occurrences  auto, terms: blank every occurrence, not just the first, default false
  language         "en" | "es" stopword list, default en
  seed             optional integer: shuffles ties in auto and picks the nth offset, reproducibly
  blank            gap text, default "_____"
  anki_single_card true: every gap is c1 (one card); default false (c1, c2, ... one card each)
"""
import bisect
import json
import random
import re
import sys

WORD = re.compile(r"[^\W\d_]+(?:['’-][^\W\d_]+)*")
STOP = {
    "en": set("""a about above after again against all also am an and any are as at be because been before being
below between both but by can could did do does doing down during each few for from further had has have having he
her here hers herself him himself his how i if in into is it its itself just me more most my myself no nor not now
of off on once only or other our ours ourselves out over own same she should so some such than that the their
theirs them themselves then there these they this those through to too under until up very was we were what when
where which while who whom why will with would you your yours yourself yourselves upon within without also
however therefore thus there's it's they're""".split()),
    "es": set("""a al algo algunas algunos ante antes como con contra cual cuando de del desde donde durante e el
ella ellas ellos en entre era eran es esa esas ese eso esos esta estaba estado estan estar este esto estos fue
fueron ha han hasta hay la las le les lo los mas me mi mis mucho muy nada ni no nos nosotros o os otra otro para
pero poco por porque que quien se sea segun ser si sin sino sobre son su sus tambien tanto te tiene tienen todo
todos tu tus un una unas uno unos y ya más está están también según así cuál él sí sólo solo""".split()),
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def integer(request, field, default, low, high):
    value = request.get(field, default)
    if isinstance(value, bool) or not isinstance(value, int) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def boolean(request, field, default=False):
    value = request.get(field, default)
    if not isinstance(value, bool):
        fail(f"{field} must be true or false")
    return value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"text", "mode", "count", "rank_by", "min_length", "terms", "every_n", "offset",
                                   "content_only", "all_occurrences", "language", "seed", "blank", "anki_single_card"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be a non-empty string")
    if len(text) > 200_000:
        fail("text is longer than 200,000 characters")
    mode = request.get("mode", "auto")
    if mode not in ("auto", "terms", "nth"):
        fail("mode must be auto, terms or nth")
    language = request.get("language", "en")
    if not isinstance(language, str) or language not in STOP:
        fail("language must be en or es")
    stop = STOP[language]
    seed = request.get("seed")
    if seed is not None and (isinstance(seed, bool) or not isinstance(seed, int)):
        fail("seed must be an integer")
    rng = random.Random(seed) if seed is not None else None
    blank = request.get("blank", "_____")
    if not isinstance(blank, str) or not 1 <= len(blank) <= 40:
        fail("blank must be a string of 1 to 40 characters")
    single = boolean(request, "anki_single_card")
    every = boolean(request, "all_occurrences")
    words = [(m.start(), m.end(), m.group()) for m in WORD.finditer(text)]
    if not words:
        fail("text has no words")

    spans, not_found, chosen_words = [], [], []
    if mode == "auto":
        count = integer(request, "count", 5, 1, 200)
        min_length = integer(request, "min_length", 4, 1, 30)
        rank_by = request.get("rank_by", "frequency")
        if rank_by not in ("frequency", "length"):
            fail("rank_by must be frequency or length")
        freq = {}
        for _, _, w in words:
            key = w.casefold()
            if len(w) >= min_length and key not in stop and key.replace("’", "'") not in stop:
                freq[key] = freq.get(key, 0) + 1
        score = (lambda k: (freq[k], len(k))) if rank_by == "frequency" else (lambda k: (len(k), freq[k]))
        groups = {}
        for key in sorted(freq):
            groups.setdefault(score(key), []).append(key)
        ranked = []
        for s in sorted(groups, reverse=True):
            group = groups[s]
            if rng:
                rng.shuffle(group)
            ranked += group
        chosen_words = ranked[:count]
        targets = set(chosen_words)
        seen = set()
        for start, end, w in words:
            key = w.casefold()
            if key in targets and (every or key not in seen):
                spans.append((start, end))
                seen.add(key)
    elif mode == "terms":
        terms = request.get("terms")
        if not isinstance(terms, list) or not terms or not all(isinstance(t, str) and t.strip() for t in terms):
            fail("terms must be a non-empty list of words or phrases")
        taken = []
        for term in terms:
            parts = [re.escape(p) for p in term.split()]
            pattern = re.compile(r"(?<![\w’'-])" + r"\s+".join(parts) + r"(?![\w’'-])", re.IGNORECASE)
            hits = [m.span() for m in pattern.finditer(text)
                    if not any(m.start() < e and s < m.end() for s, e in taken)]
            if not hits:
                not_found.append(term)
                continue
            for hit in (hits if every else hits[:1]):
                taken.append(hit)
                spans.append(hit)
            chosen_words.append(term)
    else:
        n = request.get("every_n")
        if isinstance(n, bool) or not isinstance(n, int) or not 2 <= n <= 100:
            fail("every_n must be an integer from 2 to 100")
        content_only = boolean(request, "content_only")
        pool = [w for w in words if not content_only or w[2].casefold() not in stop]
        if request.get("offset") is not None:
            offset = integer(request, "offset", None, 1, n)
        else:
            offset = rng.randint(1, n) if rng else n
        spans = [(s, e) for s, e, _ in pool[offset - 1::n]]
        chosen_words = [text[s:e] for s, e in spans]
        request["offset"] = offset
    spans.sort()
    if not spans:
        fail("nothing to blank: no word matched the selection")

    starts = [w[0] for w in words]
    out_text, anki, key = [], [], []
    last = 0
    for i, (s, e) in enumerate(spans, 1):
        answer = text[s:e]
        out_text.append(text[last:s] + f"{blank} ({i})")
        c = 1 if single else i
        anki.append(text[last:s] + "{{c%d::%s}}" % (c, answer.replace("}}", "} }").replace("::", ": :")))
        word_index = bisect.bisect_left(starts, s) + 1
        key.append({"n": i, "answer": answer, "char_offset": s, "word_number": word_index})
        last = e
    out_text.append(text[last:])
    anki.append(text[last:])
    result = {"mode": mode, "cloze_text": "".join(out_text), "answer_key": key,
              "answers": [k["answer"] for k in key], "anki": "".join(anki), "blanks": len(key), "words": len(words)}
    if mode == "auto":
        result["selected_words"] = chosen_words
    if mode == "terms":
        result["not_found"] = not_found
    if mode == "nth":
        result["offset"] = request["offset"]
    if seed is not None:
        result["seed"] = seed
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
