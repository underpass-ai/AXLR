#!/usr/bin/env python3
"""Heuristic style checks for English and Spanish prose: passive voice (be + participle;
ser/estar + participio), filler words, adverbs in -ly / -mente, a word repeated within a few
words, very long sentences and the spread of sentence lengths. Every finding carries
code-point offsets into the text. Pattern matching, not parsing: expect misses and false alarms.

Input (JSON on stdin):
  text                 the prose (required)
  language             "en" (default) | "es"
  checks               subset of ["passive", "filler", "adverb", "repetition", "long_sentence", "sentence_variety"]
  long_sentence_words  default 30
  repeat_window        words after an occurrence in which a repeat is flagged (default 10)
  min_repeat_length    shortest word checked for repeats (default 4 letters)
"""
import json
import math
import re
import sys

CHECKS = ["passive", "filler", "adverb", "repetition", "long_sentence", "sentence_variety"]

EN_BE = set("am is are was were be been being".split())
EN_IRREGULAR = set("""
arisen awoken beaten become begun bent bitten blown born borne bought bound bred brought built burnt
caught chosen come dealt done drawn driven drunk dug eaten fallen fed felt fought found fled flown
forbidden forgiven forgotten frozen given gone ground grown hung heard held hidden hit hurt kept
known laid led left lent lost made meant met paid proven put quit read ridden risen run said seen
sent set sewn shaken shaped shed shown shot shut sold sought spent split spoken spread stolen stood
struck stuck sung sunk swept sworn taken taught thought thrown told torn understood upset woken
won worn written wound
""".split())
EN_NOT_PARTICIPLE = set("bed red need seed speed feed weed shed indeed embed hundred".split())
EN_BETWEEN = set("not also always never often still already just then being".split())
EN_FILLER = ["very", "really", "just", "actually", "basically", "literally", "quite", "rather", "somewhat",
             "simply", "totally", "definitely", "certainly", "truly", "extremely", "kind of", "sort of",
             "in order to", "a lot", "stuff", "things", "needless to say", "it is important to note"]
EN_NOT_ADVERB = set("""
only early family reply supply apply july italy belly rely fly holy ugly likely daily friendly
lonely lovely silly ally bully jelly rally imply comply multiply assembly anomaly monopoly
butterfly elderly costly deadly lively orderly ugly curly hilly chilly smelly sly
""".split())
ES_SER = set("""
ser sido siendo soy eres es somos sois son era eras éramos erais eran fui fuiste fue fuimos fuisteis
fueron seré serás será seremos seréis serán sería serías seríamos seríais serían sea seas seamos seáis
sean fuera fueras fuéramos fuerais fueran fuese fueses fuésemos fueseis fuesen
""".split())
ES_ESTAR = set("""
estar estado estando estoy estás está estamos estáis están estaba estabas estábamos estabais estaban
estuve estuviste estuvo estuvimos estuvisteis estuvieron estaré estarás estará estaremos estaréis
estarán estaría estarían esté estés estemos estéis estén estuviera estuvieran
""".split())
ES_IRREGULAR_STEMS = ["hech", "dich", "escrit", "vist", "puest", "vuelt", "abiert", "rot", "muert", "cubiert",
                      "impres", "frit", "resuelt", "devuelt", "descubiert", "compuest", "propuest", "satisfech"]
ES_NOT_PARTICIPLE = set("nada cada vida nido ruido todo toda todos todas ida idas".split())
ES_BETWEEN = set("no ya muy también siempre nunca bien mal".split())
ES_FILLER = ["realmente", "básicamente", "literalmente", "simplemente", "prácticamente", "totalmente",
             "absolutamente", "bastante", "un poco", "en realidad", "de alguna manera", "o sea", "la verdad",
             "cosa", "cosas", "algo así", "en cierto modo", "hay que decir que", "cabe destacar que"]
ES_NOT_ADVERB = set("mente demente clemente vehemente".split())
STOP = {
    "en": set("""the and that this with from have has had were was are for not but you your they their them
there then than what when where which while will would could should about into over also been being
these those such some very just more most other only""".split()),
    "es": set("""que los las del con por para una unos unas como pero más este esta estos estas ese esa esos
esas sobre entre cuando donde también muy sin hay ser son está están era fue todo toda todos todas
porque desde hasta según cada otro otra otros otras""".split()),
}
WORD = re.compile(r"[^\W\d_]+(?:['’\-][^\W\d_]+)*")
SENTENCE_END = re.compile(r"[.!?…]+[\"'”’»)\]]*(?=\s|$)")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def get_int(request, field, default, low, high):
    value = request.get(field, default)
    if not isinstance(value, int) or isinstance(value, bool) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def sentences_of(text):
    spans = []
    for para in re.finditer(r"\S(?:.|\n(?!\s*\n))*", text):
        start = para.start()
        for end in SENTENCE_END.finditer(text, para.start(), para.end()):
            spans.append((start, end.end()))
            start = end.end()
        spans.append((start, para.end()))
    out = []
    for s, e in spans:
        while s < e and text[s].isspace():
            s += 1
        if s < e and WORD.search(text, s, e):
            out.append((s, e))
    return out


def is_en_participle(word):
    if word in EN_IRREGULAR:
        return True
    return len(word) > 3 and word.endswith("ed") and word not in EN_NOT_PARTICIPLE


def is_es_participle(word):
    if word in ES_NOT_PARTICIPLE:
        return False
    if re.fullmatch(r"[^\W\d_]{2,}(?:ad|id)[oa]s?", word):
        return True
    return any(re.fullmatch(stem + r"[oa]s?", word) for stem in ES_IRREGULAR_STEMS)


def finding(check, text, start, end, message):
    return {"check": check, "start": start, "end": end, "text": text[start:end], "message": message}


def passive(text, words, language):
    out = []
    for i, (w, s, e) in enumerate(words):
        aux = w in EN_BE if language == "en" else (w in ES_SER or w in ES_ESTAR)
        if not aux:
            continue
        j = i + 1
        between = EN_BETWEEN if language == "en" else ES_BETWEEN
        while j < len(words) and j <= i + 2 and (words[j][0] in between or
                                                  (language == "en" and words[j][0].endswith("ly")) or
                                                  (language == "es" and words[j][0].endswith("mente"))):
            j += 1
        if j < len(words) and (is_en_participle(words[j][0]) if language == "en" else is_es_participle(words[j][0])):
            if language == "en":
                message = "possible passive voice (be + past participle)"
            elif w in ES_SER:
                message = "posible voz pasiva (ser + participio)"
            else:
                message = "posible pasiva de estado (estar + participio)"
            out.append(finding("passive", text, s, words[j][2], message))
    return out


def phrase_matches(text, phrases, check, message):
    out = []
    lower = text.lower()
    for phrase in phrases:
        for m in re.finditer(r"(?<![^\W_])" + re.escape(phrase) + r"(?![^\W_])", lower):
            out.append(finding(check, text, m.start(), m.end(), message.format(phrase)))
    return out


def adverbs(text, words, language):
    out = []
    for w, s, e in words:
        if w in EN_FILLER or w in ES_FILLER:
            continue  # reported as filler
        if language == "en" and len(w) > 4 and w.endswith("ly") and w not in EN_NOT_ADVERB:
            out.append(finding("adverb", text, s, e, "adverb in -ly: is a stronger verb or adjective available?"))
        if language == "es" and len(w) > 6 and w.endswith("mente") and w not in ES_NOT_ADVERB:
            out.append(finding("adverb", text, s, e, "adverbio en -mente: ¿hay un verbo o adjetivo más preciso?"))
    return out


def repetition(text, words, language, window, min_len):
    out, last_seen = [], {}
    stop = STOP[language]
    for i, (w, s, e) in enumerate(words):
        if len(w) < min_len or w in stop:
            continue
        if w in last_seen and i - last_seen[w] <= window:
            out.append(dict(finding("repetition", text, s, e, f"'{w}' repeated {i - last_seen[w]} words after its last use"),
                            distance=i - last_seen[w]))
        last_seen[w] = i
    return out


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
    if len(text) > 1000000:
        fail("text is longer than 1000000 characters")
    language = request.get("language", "en")
    if language not in ("en", "es"):
        fail("language must be en or es")
    checks = request.get("checks", CHECKS)
    if not isinstance(checks, list) or not checks or not all(c in CHECKS for c in checks):
        fail("checks must be a non-empty list from " + ", ".join(CHECKS))
    long_words = get_int(request, "long_sentence_words", 30, 5, 500)
    window = get_int(request, "repeat_window", 10, 1, 1000)
    min_len = get_int(request, "min_repeat_length", 4, 1, 50)

    words = [(m.group(0).lower(), m.start(), m.end()) for m in WORD.finditer(text)]
    findings = []
    if "passive" in checks:
        findings += passive(text, words, language)
    if "filler" in checks:
        findings += phrase_matches(text, EN_FILLER if language == "en" else ES_FILLER, "filler",
                                   "filler: '{}' can often be cut" if language == "en" else "muletilla: '{}' suele sobrar")
    if "adverb" in checks:
        findings += adverbs(text, words, language)
    if "repetition" in checks:
        findings += repetition(text, words, language, window, min_len)
    sentences = sentences_of(text)
    lengths = [len(WORD.findall(text, s, e)) for s, e in sentences]
    if "long_sentence" in checks:
        for (s, e), n in zip(sentences, lengths):
            if n > long_words:
                findings.append(dict(finding("long_sentence", text, s, e, f"{n} words (more than {long_words})"), words=n))
    result = {"language": language, "heuristic": True}
    if "sentence_variety" in checks and lengths:
        mean = sum(lengths) / len(lengths)
        sd = math.sqrt(sum((n - mean) ** 2 for n in lengths) / len(lengths))
        cv = sd / mean if mean else 0
        result["sentence_lengths"] = {"count": len(lengths), "min": min(lengths), "max": max(lengths),
                                      "mean": f"{mean:.2f}", "stdev": f"{sd:.2f}", "variation": f"{cv:.2f}",
                                      "lengths": lengths}
        if len(lengths) >= 5 and cv < 0.25:
            findings.append(finding("sentence_variety", text, sentences[0][0], sentences[-1][1],
                                    f"sentence lengths vary little (coefficient of variation {cv:.2f} < 0.25)"))
    findings.sort(key=lambda f: (f["start"], f["end"], f["check"]))
    summary = {c: sum(f["check"] == c for f in findings) for c in checks}
    result.update({"findings": findings, "summary": summary, "total": len(findings),
                   "offsets": "code-point indices into text; end is exclusive",
                   "note": "heuristic pattern matching, not grammatical analysis: review each finding"})
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
