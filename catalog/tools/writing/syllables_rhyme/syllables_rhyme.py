#!/usr/bin/env python3
"""Syllables, stress and rhyme. Spanish follows the orthographic rules of the RAE exactly
(diphthongs, triphthongs, hiatus, written accents; stress by the aguda / llana / esdrujula
rules); English is a heuristic (vowel groups) and says so.

Input (JSON on stdin):
  operation  "syllables" | "rhyme" | "scheme"
  language   "es" (default) | "en"
  words      ["...", ...] or text "..."   (syllables)
  a, b       two words                     (rhyme)
  lines      ["...", ...] or text with one line per row   (scheme)
  kind       scheme: "any" (default: consonant, else assonant) | "consonant" | "assonant"
  case       scheme: "auto" (default: capitals for lines over 8 metric syllables) | "upper" | "lower"
"""
import json
import re
import sys
import unicodedata

# ---- Spanish orthographic syllabification (shared by syllables_rhyme and meter_check) ----
STRONG = set("aeoáéó")
WEAK = set("iuü")
ACCENTED = set("áéíóú")
VOWELS = STRONG | WEAK | {"í", "ú"}
PLAIN = str.maketrans("áéíóúü", "aeiouu")
ONSETS = {"pl", "pr", "bl", "br", "fl", "fr", "gl", "gr", "cl", "cr", "kl", "kr", "dr", "tr"}


def units_of(word):
    """Split a lowercase word into letters/digraphs, each tagged vowel (True) or not (False)."""
    units, i, n = [], 0, len(word)
    while i < n:
        c = word[i]
        nxt = word[i + 1] if i + 1 < n else ""
        after = word[i + 2] if i + 2 < n else ""
        if c in "cl" and nxt == ("h" if c == "c" else "l") or c == "r" and nxt == "r":
            units.append((c + nxt, False))
            i += 2
        elif c == "q" and nxt == "u":
            units.append(("qu", False))
            i += 2
        elif c == "g" and nxt == "u" and after in ("e", "i", "é", "í"):
            units.append(("gu", False))
            i += 2
        elif c == "y":
            units.append(("y", not (nxt in VOWELS)))
            i += 1
        else:
            units.append((c, c in VOWELS))
            i += 1
    return units


def vowel_kind(v):
    if v == "y":
        return "weak"
    if v in ("í", "ú"):
        return "accented_weak"
    return "strong" if v in STRONG else "weak"


def hiatus(a, b):
    """True when two adjacent vowels belong to different syllables (RAE, Ortografia 2010 and
    DPD 'tilde' 2.1.1 and 2.3.1)."""
    ka, kb = vowel_kind(a), vowel_kind(b)
    pa, pb = a.translate(PLAIN).replace("y", "i"), b.translate(PLAIN).replace("y", "i")
    if ka == "strong" and kb == "strong":
        return True
    if pa == pb:
        return True  # two equal vowels: chiita, duunviro
    if ka == "accented_weak" and kb == "strong" or ka == "strong" and kb == "accented_weak":
        return True
    return False  # strong + unstressed weak, or two different weak vowels: diphthong


def syllabify(word):
    """Orthographic syllables of one lowercase Spanish word (letters only)."""
    units = units_of(word)
    # nuclei: lists of unit indexes; an h between two vowels that form a diphthong stays inside
    nuclei, i = [], 0
    while i < len(units):
        if not units[i][1]:
            i += 1
            continue
        nucleus = [i]
        j = i + 1
        while j < len(units):
            last = units[nucleus[-1]][0]
            if units[j][1]:
                if hiatus(last, units[j][0]):
                    break
                strong_in = sum(vowel_kind(units[k][0]) != "weak" for k in nucleus if units[k][1])
                if strong_in and vowel_kind(units[j][0]) != "weak":
                    break
                nucleus.append(j)
                j += 1
            elif units[j][0] == "h" and j + 1 < len(units) and units[j + 1][1] and not hiatus(last, units[j + 1][0]) \
                    and not (vowel_kind(last) != "weak" and vowel_kind(units[j + 1][0]) != "weak"):
                nucleus += [j, j + 1]
                j += 2
            else:
                break
        nuclei.append(nucleus)
        i = j
    if not nuclei:
        return [word]
    bounds = []  # index of the first unit of each syllable after the first
    for left, right in zip(nuclei, nuclei[1:]):
        cons = list(range(left[-1] + 1, right[0]))
        if not cons:
            bounds.append(right[0])
        elif len(cons) == 1:
            bounds.append(cons[0])
        else:
            pair = units[cons[-2]][0] + units[cons[-1]][0]
            bounds.append(cons[-2] if pair in ONSETS else cons[-1])
    syllables, start = [], 0
    for b in bounds + [len(units)]:
        syllables.append("".join(u for u, _ in units[start:b]))
        start = b
    return syllables


def stressed_index(word, syllables):
    """Index of the stressed syllable by the RAE accentuation rules (DPD 'tilde' 1.2)."""
    if word.endswith("mente") and len(syllables) > 2:
        return len(syllables) - 2  # adverb in -mente: main stress on "men"
    for k, syl in enumerate(syllables):
        if any(c in ACCENTED for c in syl):
            return k
    if len(syllables) == 1:
        return 0
    last = word[-1]
    if last in "aeiou" or last in "ns" and len(word) > 1 and word[-2] in VOWELS:
        return len(syllables) - 2
    return len(syllables) - 1


def stress_type(position_from_end):
    return ["aguda", "llana", "esdrújula"][position_from_end] if position_from_end < 3 else "sobresdrújula"


def stressed_vowel(syllable):
    """Position, in the syllable string, of its stressed vowel: the accented one, else the
    strong one, else the second of two weak vowels (DPD 'tilde' 2.1.3)."""
    units, pos, found = units_of(syllable), 0, []
    for u, is_vowel in units:
        if is_vowel:
            found.append((pos, u))
        pos += len(u)
    if not found:
        return None
    for p, u in found:
        if u in ACCENTED:
            return p
    for p, u in found:
        if u in STRONG:
            return p
    return found[-1][0] if len(found) > 1 and found[-1][1] != "y" else found[0][0]


# ---- Spanish rhyme ----
def es_word(word):
    syllables = syllabify(word)
    k = stressed_index(word, syllables)
    offset = sum(len(s) for s in syllables[:k])
    vowel = stressed_vowel(syllables[k])
    tail = word[offset + (vowel or 0):]
    return syllables, k, tail


def phonetic(tail):
    """Castilian sounds of a rhyme tail: h is silent, b = v, c before e/i = z, qu = c = k,
    g before e/i = j, gu before e/i = g, u-dieresis = u, x = ks, final y = i."""
    out, i, t = [], 0, tail.translate(PLAIN)
    while i < len(t):
        c, nxt = t[i], t[i + 1:i + 2]
        after = t[i + 2:i + 3]
        if c == "h" and not (out and out[-1] == "c"):
            i += 1
            continue
        if c == "v":
            out.append("b")
        elif c == "c":
            out.append("z" if nxt in ("e", "i") else ("c" if nxt == "h" else "k"))
        elif c == "q":
            out.append("k")
            i += 1 if nxt == "u" else 0
        elif c == "g" and nxt in ("e", "i"):
            out.append("j")
        elif c == "g" and nxt == "u" and after in ("e", "i") and tail[i + 1:i + 2] != "ü":
            out.append("g")
            i += 1
        elif c == "x":
            out.append("ks")
        elif c == "y" and (not nxt or nxt not in "aeiou"):
            out.append("i")
        elif c == "w":
            out.append("u")
        else:
            out.append(c)
        i += 1
    return "".join(out)


def assonance(word):
    """Stressed vowel and last vowel: post-tonic middle vowels and the unstressed weak vowel
    of a diphthong do not count; a final unstressed i counts as e and u as o."""
    syllables, k, tail = es_word(word)
    sv = tail[0].translate(PLAIN) if tail else ""
    if k == len(syllables) - 1:
        return sv
    vowels = [c.translate(PLAIN) for c, is_vowel in units_of(syllables[-1]) if is_vowel]
    strong = [v for v in vowels if v in "aeo"]
    last = strong[-1] if strong else (vowels[-1] if vowels else "")
    last = {"i": "e", "u": "o", "y": "e"}.get(last, last)
    return sv + "-" + last


def es_rhyme(a, b):
    ta, tb = es_word(a)[2], es_word(b)[2]
    pa, pb = phonetic(ta), phonetic(tb)
    if pa and pa == pb:
        kind = "consonant"
    elif assonance(a) == assonance(b):
        kind = "assonant"
    else:
        kind = "none"
    return {"rhyme": kind, "tails": [ta, tb], "sounds": [pa, pb], "assonance": [assonance(a), assonance(b)],
            "same_word": a == b}


# ---- natural metric count of a Spanish line (for capital or small scheme letters) ----
def starts_with_vowel(word):
    if word[:1] == "h":
        word = word[1:]
        if word[:1] in ("i", "u") and word[1:2] in VOWELS:
            return False  # hie-, hue-: consonantal, no synalepha (hielo, huevo)
    return word[:1] in VOWELS or word == "y"


def natural_metric_count(words):
    total = 0
    for i, word in enumerate(words):
        total += len(syllabify(word))
        if i and (words[i - 1][-1:] in VOWELS or words[i - 1][-1:] == "y") and starts_with_vowel(word):
            total -= 1  # synalepha
    last = words[-1]
    syllables = syllabify(last)
    from_end = len(syllables) - 1 - stressed_index(last, syllables)
    return total + (1 if from_end == 0 else 0) - (from_end - 1 if from_end >= 2 else 0)


# ---- English heuristic ----
EN_VOWELS = "aeiouy"


def en_clean(word):
    return re.sub(r"[^a-z]", "", fold(word).replace("'s", "").replace("’s", ""))


def en_syllables(word):
    w = en_clean(word)
    if not w:
        return 0
    if len(w) <= 3:
        return 1
    groups = re.findall(r"[aeiouy]+", w)
    n = len(groups)
    if w.endswith("e") and not w.endswith(("ee", "ye")) and not (w.endswith("le") and w[-3] not in EN_VOWELS):
        n -= 1
    elif w.endswith("ed") and w[-3] not in "td" + EN_VOWELS:
        n -= 1
    elif w.endswith("es") and w[-3] not in "sxzhgc" + EN_VOWELS:
        n -= 1
    n -= len(re.findall(r"[^aeiouy]e(?:ly|ment|ful|ness|less)$", w))  # lovely, statement: silent e
    n += len(re.findall(r"[^tscxg]i[aou]", w))  # li-on, vi-a; but -tion, -sion, -cian one syllable
    return max(n, 1)


def en_rhyme_key(word):
    w = en_clean(word)
    plural = len(w) > 3 and w.endswith("s") and not w.endswith(("ss", "us", "is"))
    if plural:
        w = w[:-1]  # days / makes: rhyme on day / make, then add the s back
    spans = [m.span() for m in re.finditer(r"[aeiouy]+", w)]
    if not spans:
        return w + ("s" if plural else "")
    start = spans[-1][0]
    if len(spans) > 1 and (w.endswith("e") and spans[-1] == (len(w) - 1, len(w))
                           or w.endswith("y") and spans[-1] == (len(w) - 1, len(w))):
        start = spans[-2][0]  # silent final e (make, take -> "ake"); unstressed -y (lovely -> "ely")
    if start == 0 and w[0] == "y" and len(w) > 1:
        start = 1
    return w[start:] + ("s" if plural else "")


def en_rhyme(a, b):
    ka, kb = en_rhyme_key(a), en_rhyme_key(b)
    return {"rhyme": "rhyme" if ka == kb and ka else "none", "keys": [ka, kb], "same_word": en_clean(a) == en_clean(b),
            "approximate": True}


# ---- input ----
def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fold(text):
    return unicodedata.normalize("NFC", text.lower())


def words_in(text):
    return [fold(w) for w in re.findall(r"[^\W\d_]+(?:['\u2019][^\W\d_]+)*", text)]


def get_words(request):
    if "words" in request:
        words = request["words"]
        if not isinstance(words, list) or not words or not all(isinstance(w, str) for w in words):
            fail("words must be a non-empty list of strings")
        out = []
        for w in words:
            found = words_in(w)
            if len(found) != 1:
                fail(f"words: {w!r} is not one word")
            out.append(found[0])
    else:
        text = request.get("text")
        if not isinstance(text, str):
            fail("give words (a list) or text")
        out = words_in(text)
    if not out:
        fail("no words found")
    if len(out) > 5000:
        fail("more than 5000 words")
    return out


def one_word(request, field):
    value = request.get(field)
    found = words_in(value) if isinstance(value, str) else []
    if len(found) != 1:
        fail(f"{field} must be one word")
    return found[0]


def syllables_op(request, language):
    out = []
    for w in get_words(request):
        if language == "es":
            syllables = syllabify(w)
            k = stressed_index(w, syllables)
            out.append({"word": w, "syllables": syllables, "count": len(syllables), "stressed": k + 1,
                        "stressed_syllable": syllables[k], "type": stress_type(len(syllables) - 1 - k),
                        "written_accent": any(c in ACCENTED for c in w)})
        else:
            out.append({"word": w, "count": en_syllables(w), "rhyme_key": en_rhyme_key(w)})
    result = {"operation": "syllables", "language": language, "words": out, "total": sum(x["count"] for x in out)}
    if language == "en":
        result["approximate"] = True
        result["method"] = "English heuristic: vowel groups, silent final e, -ed and -es; not a dictionary"
    else:
        result["method"] = "RAE orthographic syllabification and accentuation rules (Ortografia 2010, DPD 'tilde')"
    return result


def rhyme_op(request, language):
    a, b = one_word(request, "a"), one_word(request, "b")
    result = es_rhyme(a, b) if language == "es" else en_rhyme(a, b)
    result.update({"operation": "rhyme", "language": language, "a": a, "b": b})
    return result


def scheme_op(request, language):
    lines = request.get("lines")
    if lines is None and isinstance(request.get("text"), str):
        lines = [x for x in request["text"].splitlines() if x.strip()]
    if not isinstance(lines, list) or not lines or not all(isinstance(x, str) for x in lines):
        fail("give lines (a list of strings) or text with one line per row")
    if len(lines) > 500:
        fail("more than 500 lines")
    kind = request.get("kind", "any")
    if kind not in ("any", "consonant", "assonant"):
        fail("kind must be any, consonant or assonant")
    case = request.get("case", "auto")
    if case not in ("auto", "upper", "lower"):
        fail("case must be auto, upper or lower")
    info = []
    for line in lines:
        words = words_in(line)
        if not words:
            fail(f"line {line!r} has no words")
        count = natural_metric_count(words) if language == "es" else sum(en_syllables(w) for w in words)
        info.append({"line": line, "last_word": words[-1], "syllables": count, "group": None, "rhyme": None})
    groups = []

    def join(item, kind_of_rhyme, test):
        for g in groups:
            if test(g["words"][0], item["last_word"]):
                g["words"].append(item["last_word"])
                item["group"], item["rhyme"] = g, kind_of_rhyme
                return
        item["group"] = {"words": [item["last_word"]]}
        groups.append(item["group"])

    if language == "en":
        for item in info:
            join(item, "rhyme", lambda a, b: en_rhyme(a, b)["rhyme"] == "rhyme")
    elif kind == "assonant":
        for item in info:
            join(item, "assonant", lambda a, b: assonance(a) == assonance(b))
    else:
        for item in info:  # first pass: consonant rhyme (same sounds from the stressed vowel)
            join(item, "consonant", lambda a, b: phonetic(es_word(a)[2]) == phonetic(es_word(b)[2]))
        if kind == "any":  # second pass: a line rhyming with no other joins a group it assonates with
            for item in info:
                g = item["group"]
                if len(g["words"]) > 1:
                    continue
                for other in groups:
                    if other is not g and (len(other["words"]) > 1 or groups.index(other) < groups.index(g)) \
                            and assonance(other["words"][0]) == assonance(item["last_word"]):
                        groups.remove(g)
                        other["words"].append(item["last_word"])
                        item["group"], item["rhyme"] = other, "assonant"
                        break
    letters, next_letter = {}, 0
    for g in groups:
        if len(g["words"]) > 1:
            letters[id(g)] = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"[next_letter % 26]
            next_letter += 1
    scheme, out = [], []
    for item in info:
        letter = letters.get(id(item["group"]), "-")
        upper = case == "upper" or case == "auto" and item["syllables"] > 8
        letter = letter if upper else letter.lower()
        scheme.append(letter)
        out.append({"line": item["line"], "last_word": item["last_word"], "letter": letter,
                    "rhyme_with_earlier": item["rhyme"],
                    ("metric_syllables_natural" if language == "es" else "syllables"): item["syllables"]})
    result = {"operation": "scheme", "language": language, "scheme": "".join(scheme), "lines": out, "kind": kind,
              "notation": "a letter per rhyme group in order of appearance, '-' for an unrhymed line; capitals for arte mayor (more than 8 syllables) when case is auto; metric_syllables_natural applies every synalepha (meter_check weighs the licences)"}
    if language == "en":
        result["approximate"] = True
    return result


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    language = request.get("language", "es")
    if language not in ("es", "en"):
        fail("language must be es or en")
    operations = {"syllables": syllables_op, "rhyme": rhyme_op, "scheme": scheme_op}
    operation = request.get("operation")
    if not isinstance(operation, str) or operation not in operations:
        fail("operation must be one of syllables, rhyme, scheme")
    print(json.dumps(operations[operation](request, language), ensure_ascii=False))


if __name__ == "__main__":
    main()
