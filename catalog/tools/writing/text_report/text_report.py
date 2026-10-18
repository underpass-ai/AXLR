#!/usr/bin/env python3
"""Text statistics: characters, words, sentences, paragraphs, most frequent words (optionally
without English or Spanish stopwords), 2- and 3-grams, reading and speaking time at stated
words per minute, the longest sentences and lexical diversity (type-token ratio, MTLD).

Input (JSON on stdin):
  text          the text (required)
  stopwords     "none" (default) | "en" | "es"   removed from top words and from n-gram edges
  top_n         how many top words and n-grams (default 10, 1 to 100)
  ngrams        sizes, subset of [2, 3] (default [2, 3]); [] for none
  reading_wpm   default 238 (Brysbaert 2019, silent reading of English non-fiction)
  speaking_wpm  default 150
  longest_n     longest sentences listed (default 3, 0 to 50)
  mtld          true to add MTLD (McCarthy and Jarvis 2010, threshold 0.72)
"""
import json
import re
import sys
from collections import Counter
from fractions import Fraction

STOPWORDS = {
    "en": set("""
a about above after again against all am an and any are as at be because been before being
below between both but by can could did do does doing down during each few for from further
had has have having he her here hers herself him himself his how i if in into is it its itself
just me more most my myself no nor not now of off on once only or other our ours ourselves out
over own same she should so some such than that the their theirs them themselves then there
these they this those through to too under until up very was we were what when where which
while who whom why will with would you your yours yourself yourselves
""".split()),
    "es": set("""
a al algo algunas algunos ante antes como con contra cual cuando de del desde donde durante
e el ella ellas ellos en entre era eran es esa esas ese eso esos esta estaba estado estan
estar estas este esto estos fue fueron ha hace hacia han hasta hay la las le les lo los mas
me mi mis mucho muy nada ni no nos nosotros o os otra otras otro otros para pero poco por
porque que quien se sea ser si sido sin sino sobre son su sus también tan te tiene todo
todos tu tus un una unas uno unos usted ustedes y ya yo él más qué sí está están había
""".split()),
}
TOKEN = re.compile(r"\d+(?:[.,]\d+)*|[^\W\d_]+(?:['’\-][^\W\d_]+)*")
SENTENCE_END = re.compile(r"[.!?…]+[\"'”’»)\]]*(?=\s|$)")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def get_int(request, field, default, low, high):
    value = request.get(field, default)
    if not isinstance(value, int) or isinstance(value, bool) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def get_wpm(request, field, default):
    value = request.get(field, default)
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a positive number")
    try:
        value = Fraction(str(value))
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a positive number")
    if not 0 < value <= 2000:
        fail(f"{field} must be from 0 to 2000 words per minute")
    return value


def sentences_of(text):
    """(start, end) spans: a sentence ends at . ! ? or an ellipsis followed by space or the
    end (closing quotes and brackets stay with it), or at a paragraph break."""
    spans = []
    for para in re.finditer(r"\S(?:.|\n(?!\s*\n))*", text):
        start = para.start()
        for end in SENTENCE_END.finditer(text, para.start(), para.end()):
            if text[start:end.end()].strip():
                spans.append((start, end.end()))
            start = end.end()
        if text[start:para.end()].strip():
            spans.append((start, para.end()))
    out = []
    for s, e in spans:
        while s < e and text[s].isspace():
            s += 1
        out.append((s, e))
    return out


def duration(words, wpm):
    seconds = Fraction(words * 60) / wpm
    whole = int(seconds + Fraction(1, 2))
    return {"wpm": format_number(wpm), "minutes": f"{float(seconds / 60):.2f}", "seconds": whole,
            "clock": f"{whole // 60}:{whole % 60:02d}"}


def format_number(value):
    return int(value) if value.denominator == 1 else float(value)


def mtld_pass(tokens, threshold=Fraction(72, 100)):
    factors, types, count = Fraction(0), set(), 0
    for token in tokens:
        types.add(token)
        count += 1
        if Fraction(len(types), count) <= threshold:
            factors += 1
            types, count = set(), 0
    if count:
        factors += (1 - Fraction(len(types), count)) / (1 - threshold)
    return Fraction(len(tokens)) / factors if factors else None


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be a non-empty string")
    if len(text) > 2000000:
        fail("text is longer than 2000000 characters")
    language = request.get("stopwords", "none")
    if language not in ("none", "en", "es"):
        fail("stopwords must be none, en or es")
    stop = STOPWORDS.get(language, set())
    top_n = get_int(request, "top_n", 10, 1, 100)
    longest_n = get_int(request, "longest_n", 3, 0, 50)
    sizes = request.get("ngrams", [2, 3])
    if not isinstance(sizes, list) or not all(s in (2, 3) and not isinstance(s, bool) for s in sizes):
        fail("ngrams must be a list of 2 and/or 3")
    mtld = request.get("mtld", False)
    if not isinstance(mtld, bool):
        fail("mtld must be true or false")
    reading = get_wpm(request, "reading_wpm", 238)
    speaking = get_wpm(request, "speaking_wpm", 150)

    words = [m.group(0).lower() for m in TOKEN.finditer(text)]
    sentences = []
    for s, e in sentences_of(text):
        tokens = [m.group(0).lower() for m in TOKEN.finditer(text, s, e)]
        if tokens:
            sentences.append({"start": s, "end": e, "text": text[s:e], "words": tokens})
    paragraphs = [p for p in re.split(r"\n\s*\n", text) if TOKEN.search(p)]
    counts = Counter(w for w in words if w not in stop)
    top = sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))[:top_n]
    ngram_out = {}
    for n in sorted(set(sizes)):
        grams = Counter()
        for sentence in sentences:
            seq = sentence["words"]
            for i in range(len(seq) - n + 1):
                gram = seq[i:i + n]
                if stop and (gram[0] in stop or gram[-1] in stop):
                    continue
                grams[" ".join(gram)] += 1
        ngram_out[str(n)] = [{"ngram": g, "count": c} for g, c in sorted(grams.items(), key=lambda kv: (-kv[1], kv[0]))[:top_n]]
    lengths = [len(s["words"]) for s in sentences]
    longest = sorted(sentences, key=lambda s: (-len(s["words"]), s["start"]))[:longest_n]
    letters = sum(c.isalpha() for c in text)
    word_letters = sum(sum(c.isalpha() or c.isdigit() for c in w) for w in words)
    types = len(set(words))
    result = {
        "characters": len(text),
        "characters_no_spaces": sum(not c.isspace() for c in text),
        "letters": letters,
        "words": len(words),
        "unique_words": types,
        "sentences": len(sentences),
        "paragraphs": len(paragraphs),
        "average_words_per_sentence": f"{len(words) / len(sentences):.2f}" if sentences else None,
        "average_word_length": f"{word_letters / len(words):.2f}" if words else None,
        "sentence_length": {"min": min(lengths), "max": max(lengths)} if lengths else None,
        "top_words": [{"word": w, "count": c} for w, c in top],
        "stopwords_removed": language,
        "ngrams": ngram_out,
        "reading_time": duration(len(words), reading),
        "speaking_time": duration(len(words), speaking),
        "longest_sentences": [{"text": s["text"], "words": len(s["words"]), "offset": s["start"]} for s in longest],
        "lexical_diversity": {"type_token_ratio": f"{types / len(words):.4f}" if words else None,
                              "type_token_exact": str(Fraction(types, len(words))) if words else None},
        "tokenizer": "words are runs of letters (joined by an inner apostrophe or hyphen) or numbers (with inner . or ,); "
                     "case-folded; offsets are code-point indices",
    }
    if mtld:
        forward, backward = mtld_pass(words), mtld_pass(words[::-1])
        value = (forward + backward) / 2 if forward is not None and backward is not None else None
        result["lexical_diversity"]["mtld"] = f"{float(value):.2f}" if value is not None else None
        result["lexical_diversity"]["mtld_forward"] = f"{float(forward):.2f}" if forward is not None else None
        result["lexical_diversity"]["mtld_backward"] = f"{float(backward):.2f}" if backward is not None else None
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
