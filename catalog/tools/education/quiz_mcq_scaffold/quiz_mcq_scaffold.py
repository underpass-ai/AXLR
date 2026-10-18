#!/usr/bin/env python3
"""Multiple-choice scaffolds from a text: for each target term, the sentence that first uses it
becomes the stem with the term blanked, and the distractors are words or phrases of the same
text that look like the answer (same number of words, same capitalisation, similar length, same
word ending), chosen and ordered reproducibly from a seed.

Input (JSON on stdin):
  text      the passage (required)
  terms     the target terms, words or phrases (required)
  options   choices per question including the answer, default 4 (2-6)
  seed      integer, default 0
  language  "en" | "es" stopword list, default en
  blank     gap text, default "_____"
"""
import json
import random
import re
import sys

WORD = re.compile(r"[^\W_]+(?:['’-][^\W_]+)*")
SENTENCE = re.compile(r"[^.!?\n]+(?:[.!?]+|$)")
STOP = {
    "en": set("""a about above after again against all also am an and any are as at be because been before being
below between both but by can could did do does doing down during each few for from further had has have having he
her here hers herself him himself his how i if in into is it its itself just me more most my myself no nor not now
of off on once only or other our ours ourselves out over own same she should so some such than that the their
theirs them themselves then there these they this those through to too under until up very was we were what when
where which while who whom why will with would you your yours yourself yourselves upon within without however
therefore thus""".split()),
    "es": set("""a al algo algunas algunos ante antes como con contra cual cuando de del desde donde durante e el
ella ellas ellos en entre era eran es esa esas ese eso esos esta estaba estado estan estar este esto estos fue
fueron ha han hasta hay la las le les lo los mas me mi mis mucho muy nada ni no nos nosotros o os otra otro para
pero poco por porque que quien se sea segun ser si sin sino sobre son su sus tambien tanto te tiene tienen todo
todos tu tus un una unas uno unos y ya más está están también según así cuál él sí sólo solo""".split()),
}
SUFFIXES = ["tion", "sion", "ment", "ness", "ity", "ing", "ous", "ive", "ism", "ist", "ly", "ed", "al", "er", "es", "s"]
LABELS = "ABCDEF"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def caps(text):
    letters = [c for c in text if c.isalpha()]
    if len(letters) > 1 and all(c.isupper() for c in letters):
        return "upper"
    words = text.split()
    if all(w[:1].isupper() for w in words):
        return "title"
    if not any(w[:1].isupper() for w in words):
        return "lower"
    return "mixed"


def ending(text):
    last = text.split()[-1].casefold()
    return next((s for s in SUFFIXES if last.endswith(s) and len(last) > len(s) + 2), "")


def related(a, b):
    a, b = a.casefold(), b.casefold()
    if a in b or b in a:
        return True
    n = max(4, min(len(a), len(b)) - 2)
    return a[:n] == b[:n]


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"text", "terms", "options", "seed", "language", "blank"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip() or len(text) > 200_000:
        fail("text must be a non-empty string of at most 200,000 characters")
    terms = request.get("terms")
    if (not isinstance(terms, list) or not 1 <= len(terms) <= 200
            or not all(isinstance(t, str) and t.strip() and len(t) <= 100 for t in terms)):
        fail("terms must be a list of 1 to 200 words or phrases")
    options = request.get("options", 4)
    if isinstance(options, bool) or not isinstance(options, int) or not 2 <= options <= 6:
        fail("options must be an integer from 2 to 6")
    seed = request.get("seed", 0)
    if isinstance(seed, bool) or not isinstance(seed, int):
        fail("seed must be an integer")
    language = request.get("language", "en")
    if not isinstance(language, str) or language not in STOP:
        fail("language must be en or es")
    blank = request.get("blank", "_____")
    if not isinstance(blank, str) or not 1 <= len(blank) <= 40:
        fail("blank must be a string of 1 to 40 characters")
    stop = STOP[language]
    rng = random.Random(seed)

    sentences = [(m.start(), m.group()) for m in SENTENCE.finditer(text) if m.group().strip()]
    # candidate n-grams: (text as it appears, sentence index, is it sentence-initial)
    grams = {}
    for si, (_, sentence) in enumerate(sentences):
        words = WORD.findall(sentence)
        for size in range(1, 5):
            for i in range(len(words) - size + 1):
                chunk = words[i:i + size]
                if chunk[0].casefold() in stop or chunk[-1].casefold() in stop:
                    continue
                phrase = " ".join(chunk)
                entry = grams.setdefault(phrase.casefold(), {"forms": {}, "initial_only": True})
                entry["forms"][phrase] = entry["forms"].get(phrase, 0) + (0 if i == 0 else 1)
                if i > 0:
                    entry["initial_only"] = False

    questions, not_found = [], []
    targets = [t.strip() for t in terms]
    for term in targets:
        pattern = re.compile(r"(?<![\w’'-])" + r"\s+".join(re.escape(p) for p in term.split()) + r"(?![\w’'-])",
                             re.IGNORECASE)
        found = next(((s, sent) for s, sent in sentences if pattern.search(sent)), None)
        if not found:
            not_found.append(term)
            continue
        stem = pattern.sub(blank, found[1].strip())
        match = pattern.search(found[1])
        answer = match.group()
        if not found[1][:match.start()].strip():
            answer = term  # opens the sentence: its capital may be positional, so use the term as given
        size, length, cls, end = len(answer.split()), len(answer), caps(answer), ending(answer)
        has_digit = any(c.isdigit() for c in answer)
        stem_words = [w.casefold() for w in WORD.findall(found[1])]
        in_stem = {" ".join(stem_words[i:i + size]) for i in range(len(stem_words) - size + 1)}
        pool = []
        for key, entry in grams.items():
            form = max(entry["forms"], key=lambda f: (entry["forms"][f], f))
            if len(form.split()) != size or related(form, answer) or key in stop or key in in_stem:
                continue
            if any(c.isdigit() for c in form) != has_digit:
                continue
            form_cls = caps(form)
            if entry["initial_only"] and form_cls in ("title", "mixed"):
                continue  # capitalised only because it opens a sentence: its true case is unknown
            pool.append((form, form_cls))
        needed = options - 1
        relaxed, chosen = [], []
        stages = [("length and capitalisation", True, True), ("length", False, True), ("capitalisation", False, False)]
        for name, keep_length, keep_caps in stages:
            fits = [(f, c) for f, c in pool if (c == cls or not keep_caps)
                    and (abs(len(f) - length) <= max(2, length // 3) or not keep_length)
                    and f not in chosen]
            fits.sort(key=lambda fc: (abs(len(fc[0]) - length) + (0 if ending(fc[0]) == end else 2), fc[0].casefold()))
            shortlist = [f for f, _ in fits[:max(2 * (needed - len(chosen)), 1)]]
            shortlist.sort(key=str.casefold)
            rng.shuffle(shortlist)
            take = shortlist[:needed - len(chosen)]
            if take and name != stages[0][0]:
                relaxed.append(f"relaxed: {name}")
            chosen += take
            if len(chosen) == needed:
                break
        choices = sorted([answer] + chosen, key=str.casefold)
        rng.shuffle(choices)
        labelled = [{"label": LABELS[i], "text": c} for i, c in enumerate(choices)]
        correct = LABELS[choices.index(answer)]
        q = {"n": len(questions) + 1, "term": term, "stem": stem, "choices": labelled, "answer": correct,
             "answer_text": answer, "notes": relaxed}
        if len(chosen) < needed:
            q["short_of_distractors"] = needed - len(chosen)
        questions.append(q)
    if not questions:
        fail("none of the terms occurs in the text")
    print(json.dumps({"questions": questions, "answer_key": [f"{q['n']}-{q['answer']}" for q in questions],
                      "not_found": not_found, "seed": seed,
                      "rules": "distractors: from other sentences of the text, same word count and capitalisation, length "
                               "within max(2, len/3), same ending preferred, not a variant of the answer; length then "
                               "capitalisation relaxed when short"},
                     ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
