#!/usr/bin/env python3
"""Scan a line of verse. Spanish: metric syllables with synalepha between words and the
final-word rule (+1 aguda, -1 esdrujula), fitted to a meter (octosilabo, endecasilabo with its
accent types, alejandrino in two hemistichs of 7, or any count) by choosing where a hiatus
breaks a synalepha. English: an iambic pentameter heuristic.

Input (JSON on stdin):
  line      the verse (or lines: a list, scanned one by one)
  language  "es" (default) | "en"
  meter     es: "auto" (default) | "octosilabo" | "endecasilabo" | "alejandrino" ; en: "iambic_pentameter"
  syllables es: instead of meter, any target number of metric syllables (2 to 20)
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


# ---- words without their own stress (RAE, Nueva gramatica 2009, and Ortografia 2010: articles,
# prepositions except segun, conjunctions, unstressed pronouns and possessives, relatives) ----
ATONIC = set("""
el la lo los las al del
me te se nos os le les
mi mis tu tus su sus
a ante bajo cabe con contra de desde en entre hacia hasta para por pro sin so sobre tras
y e ni o u que pero mas aunque sino porque pues si
como cuando donde adonde quien quienes cuyo cuya cuyos cuyas cuanto cuanta cuantos cuantas aun
""".split())

METERS = {"octosilabo": 8, "endecasilabo": 11, "alejandrino": 14}
NAMES = {2: "bisílabo", 3: "trisílabo", 4: "tetrasílabo", 5: "pentasílabo", 6: "hexasílabo", 7: "heptasílabo",
         8: "octosílabo", 9: "eneasílabo", 10: "decasílabo", 11: "endecasílabo", 12: "dodecasílabo",
         13: "tridecasílabo", 14: "alejandrino (tetradecasílabo)"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


WORD = re.compile(r"[^\W\d_]+")


def starts_with_vowel(word):
    if word[:1] == "h":
        word = word[1:]
        if word[:1] in ("i", "u") and word[1:2] in VOWELS:
            return False  # hie-, hue-: consonantal onset (hielo, hierba, huevo), no synalepha
    return word[:1] in VOWELS


def ends_with_vowel(word):
    return word[-1:] in VOWELS or word[-1:] == "y"


def analyse_words(line):
    originals = WORD.findall(line)
    if not originals:
        fail("the line has no words")
    if len(originals) > 40:
        fail("a line of more than 40 words")
    words = []
    for original in originals:
        lower = unicodedata.normalize("NFC", original).lower()
        syllables = syllabify(lower)
        k = stressed_index(lower, syllables)
        shown, pos = [], 0
        for s in syllables:  # keep the original capitals when the lengths agree
            shown.append(original[pos:pos + len(s)] if len(original) == len(lower) else s)
            pos += len(s)
        words.append({"word": lower, "syllables": syllables, "shown": shown, "stressed": k,
                      "tonic": lower not in ATONIC or len(originals) == 1})
    return words


def scan(words, hiatus):
    """Metric scansion of words with every possible synalepha except at the boundaries in hiatus."""
    glued, index, positions, stresses, text = [], 0, [], [], []
    for j, w in enumerate(words):
        merge = j > 0 and j in candidates(words) and j not in hiatus
        start = index if merge else index + 1
        for s_i, syl in enumerate(w["shown"]):
            pos = start + s_i
            if s_i == 0 and merge:
                text[-1] += "_" + syl
            else:
                text.append(syl)
            if s_i == w["stressed"] and w["tonic"]:
                stresses.append(pos)
        index = start + len(w["syllables"]) - 1
        if merge:
            glued.append(j)
    last = words[-1]
    from_end = len(last["syllables"]) - 1 - last["stressed"]
    adjustment = 1 if from_end == 0 else -(from_end - 1)
    final_stress = index - from_end
    if final_stress not in stresses:
        stresses.append(final_stress)
    return {"phonological": index, "metric": index + adjustment, "adjustment": adjustment,
            "final_type": stress_type(from_end), "stresses": sorted(set(stresses)), "scansion": "-".join(text),
            "synalephas": glued}


def candidates(words):
    return [j for j in range(1, len(words)) if ends_with_vowel(words[j - 1]["word"]) and starts_with_vowel(words[j]["word"])]


def hiatus_rank(words, j):
    """Where a hiatus is preferred: before a stressed vowel, then after one, then leftmost."""
    nxt, prev = words[j], words[j - 1]
    if nxt["tonic"] and nxt["stressed"] == 0:
        return (0, j)
    if prev["tonic"] and prev["stressed"] == len(prev["syllables"]) - 1:
        return (1, j)
    return (2, j)


def fit(words, target):
    """Scansion with the fewest hiatus that reaches target metric syllables, or None."""
    cands = candidates(words)
    natural = scan(words, set())
    need = target - natural["metric"]
    if need < 0 or need > len(cands):
        return None
    chosen = sorted(cands, key=lambda j: hiatus_rank(words, j))[:need]
    return scan(words, set(chosen)), sorted(chosen)


def boundary(words, j):
    return words[j - 1]["word"] + " " + words[j]["word"]


def describe(words, result, hiatus):
    return {"metric_syllables": result["metric"], "phonological_syllables": result["phonological"],
            "final_word": {"word": words[-1]["word"], "type": result["final_type"], "adjustment": result["adjustment"]},
            "scansion": result["scansion"], "stresses": result["stresses"],
            "synalephas": [boundary(words, j) for j in result["synalephas"]],
            "hiatus": [boundary(words, j) for j in hiatus]}


def endecasilabo_types(stresses):
    """Accent types as defined in es.wikipedia 'Endecasilabo' (Ritmos y acentos)."""
    s = set(p for p in stresses if p <= 10)
    types = []
    if 1 in s and 6 in s and not s & {2, 3, 4, 5}:
        types.append("enfático")
    if 2 in s and 6 in s:
        types.append("heroico")
    if 3 in s and 6 in s:
        types.append("melódico")
    if 4 in s and 8 in s:
        types.append("sáfico")
    if 4 in s and 7 in s:
        types.append("dactílico")
    if 4 in s and 6 in s:
        types.append("mixto")
    if {2, 4, 6, 8, 10} <= s:
        types.append("yámbico")
    if 6 in s and not s & {1, 2, 3, 4, 5}:
        types.append("vacío")
    warnings = []
    if 9 in s:
        warnings.append("stress on the 9th syllable (antirhythmic next to the 10th)")
    if not s & {4, 6}:
        warnings.append("no rhythmic accent on the 4th or 6th syllable")
    return {"types": types, "a_maiore": 6 in s, "a_minore": 6 not in s and bool(s & {4, 8}), "warnings": warnings,
            "note": "every type whose accents are present is listed; the source sets no precedence between them"}


def scan_spanish(line, meter, target):
    words = analyse_words(line)
    natural = scan(words, set())
    out = {"line": line, "language": "es", "natural": describe(words, natural, [])}
    out["natural"]["name"] = NAMES.get(natural["metric"], f"{natural['metric']} syllables")
    reachable = {}
    for name, n in METERS.items():
        reachable[name] = fit(words, n) is not None
    out["reachable_by_hiatus"] = reachable
    if meter == "auto" and target is None:
        out.update({"meter": "auto", "fits": True, **describe(words, natural, [])})
        return out
    if meter == "alejandrino":
        best = None
        for cut in range(1, len(words)):
            a, b = fit(words[:cut], 7), fit(words[cut:], 7)
            if a and b:
                cost = (len(a[1]) + len(b[1]), abs(cut - len(words) / 2))
                if best is None or cost < best[0]:
                    best = (cost, cut, a, b)
        out["meter"] = "alejandrino"
        if best is None:
            out.update({"fits": False, "reason": "no word boundary splits the line into two hemistichs of 7 metric syllables"})
            return out
        _, cut, (ra, ha), (rb, hb) = best
        first, second = describe(words[:cut], ra, ha), describe(words[cut:], rb, hb)
        out.update({"fits": True, "metric_syllables": 14, "hemistichs": [first, second],
                    "scansion": first["scansion"] + " // " + second["scansion"],
                    "stresses": first["stresses"] + [7 + p for p in second["stresses"]],
                    "note": "two heptasyllables; no synalepha across the caesura, each hemistich takes the final-word rule"})
        return out
    n = target if target is not None else METERS[meter]
    out["meter"] = meter if target is None else f"{n} syllables"
    found = fit(words, n)
    if found is None:
        out.update({"fits": False, "metric_syllables": natural["metric"],
                    "reason": (f"even with every synalepha the line has {natural['metric']} metric syllables (more than {n}); "
                               "syneresis inside words is not considered") if natural["metric"] > n else
                              f"even with a hiatus at every vowel contact the line has fewer than {n} metric syllables"})
        return out
    result, hiatus = found
    out.update({"fits": True, **describe(words, result, hiatus)})
    if n == 11:
        out["endecasilabo"] = endecasilabo_types(result["stresses"])
    return out


# ---- English iambic pentameter heuristic ----
EN_VOWELS = "aeiouy"
UNSTRESSED_PREFIXES = ("a", "be", "de", "re", "com", "con", "ex", "em", "en", "im", "ob", "pre", "sus")


def en_clean(word):
    return re.sub(r"[^a-z]", "", word.lower().replace("'s", "").replace("’s", ""))


def en_syllables(word):
    w = en_clean(word)
    if not w:
        return 0
    if len(w) <= 3:
        return 1
    n = len(re.findall(r"[aeiouy]+", w))
    if w.endswith("e") and not w.endswith(("ee", "ye")) and not (w.endswith("le") and w[-3] not in EN_VOWELS):
        n -= 1
    elif w.endswith("ed") and w[-3] not in "td" + EN_VOWELS:
        n -= 1
    elif w.endswith("es") and w[-3] not in "sxzhgc" + EN_VOWELS:
        n -= 1
    n -= len(re.findall(r"[^aeiouy]e(?:ly|ment|ful|ness|less)$", w))
    n += len(re.findall(r"[^tscxg]i[aou]", w))
    return max(n, 1)


def en_stress(word, n):
    """Guessed stressed syllable of a polysyllable (0-based), or None when no rule applies."""
    w = en_clean(word)
    if n < 2:
        return None
    if re.search(r"(tion|sion|cian|ic)s?$", w):
        return n - 2
    if n == 2:
        rest = [p for p in UNSTRESSED_PREFIXES if w.startswith(p) and len(w) > len(p) + 2 and w[len(p)] not in EN_VOWELS]
        return 1 if rest else 0
    return None


def scan_english(line):
    words = re.findall(r"[A-Za-z]+(?:['’][A-Za-z]+)*", line)
    if not words:
        fail("the line has no words")
    position, conflicts, guesses = 0, [], []
    for w in words:
        n = en_syllables(w)
        k = en_stress(w, n)
        if k is not None:
            p = position + k + 1
            guesses.append({"word": w, "syllables": n, "stressed": k + 1, "position": p})
            if p % 2 == 1:
                kind = "initial inversion (trochee in the first foot)" if p == 1 else "stress on a weak (odd) position"
                conflicts.append({"word": w, "position": p, "issue": kind})
        else:
            guesses.append({"word": w, "syllables": n, "stressed": None})
        position += n
    feminine = position == 11 and guesses[-1]["stressed"] is not None and guesses[-1]["position"] == 10
    real = [c for c in conflicts if c["position"] != 1]
    fits = (position == 10 or feminine) and not real
    return {"line": line, "language": "en", "meter": "iambic_pentameter", "fits": fits, "syllables": position,
            "feminine_ending": feminine, "expected": "x/x/x/x/x/", "words": guesses, "conflicts": conflicts,
            "heuristic": True,
            "method": "syllables by vowel groups; stress guessed only for polysyllables (prefix and suffix rules); "
                      "monosyllables may take either stress"}


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
    if "lines" in request:
        lines = request["lines"]
        if not isinstance(lines, list) or not lines or not all(isinstance(x, str) for x in lines):
            fail("lines must be a non-empty list of strings")
        if len(lines) > 200:
            fail("more than 200 lines")
    else:
        line = request.get("line")
        if not isinstance(line, str) or not line.strip():
            fail("line must be a non-empty string (or give lines)")
        lines = [line]
    target = request.get("syllables")
    if target is not None and (not isinstance(target, int) or isinstance(target, bool) or not 2 <= target <= 20):
        fail("syllables must be an integer from 2 to 20")
    if language == "en":
        meter = request.get("meter", "iambic_pentameter")
        if meter != "iambic_pentameter":
            fail("meter for English must be iambic_pentameter")
        results = [scan_english(x) for x in lines]
    else:
        meter = request.get("meter", "auto")
        if meter not in ("auto", *METERS):
            fail("meter must be auto, octosilabo, endecasilabo or alejandrino")
        if target is not None and meter != "auto":
            fail("give meter or syllables, not both")
        results = [scan_spanish(x, meter, target) for x in lines]
    out = results[0] if "lines" not in request else {"lines": results, "all_fit": all(r["fits"] for r in results)}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
