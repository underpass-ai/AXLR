#!/usr/bin/env python3
"""Conjugate a Spanish verb in every simple and compound tense, the imperative and the
non-personal forms, after the RAE models: regular -ar (amar), -er (temer), -ir (partir), the
automatic spelling changes, and the irregular verbs bundled in irregulares.json.

Input (JSON on stdin):
  verb        infinitive, e.g. "tener" (required unless list is true)
  voseo       boolean: add the vos forms (present indicative and imperative differ)
  compound    boolean: include the compound tenses with haber (default true)
  list        boolean: list the bundled irregular verbs by model instead
"""
import json
import pathlib
import re
import sys

PERSONS = ["yo", "tú", "él", "nosotros", "vosotros", "ellos"]
STRESSED = (0, 1, 2, 5)
PRES = {"ar": ["o", "as", "a", "amos", "áis", "an"], "er": ["o", "es", "e", "emos", "éis", "en"], "ir": ["o", "es", "e", "imos", "ís", "en"]}
SUBJ = {"ar": ["e", "es", "e", "emos", "éis", "en"], "er": ["a", "as", "a", "amos", "áis", "an"], "ir": ["a", "as", "a", "amos", "áis", "an"]}
IMPF = {"ar": ["aba", "abas", "aba", "ábamos", "abais", "aban"], "er": ["ía", "ías", "ía", "íamos", "íais", "ían"]}
PRET = {"ar": ["é", "aste", "ó", "amos", "asteis", "aron"], "er": ["í", "iste", "ió", "imos", "isteis", "ieron"]}
STRONG = ["e", "iste", "o", "imos", "isteis", "ieron"]
FUT = ["é", "ás", "á", "emos", "éis", "án"]
COND = ["ía", "ías", "ía", "íamos", "íais", "ían"]
ACCENT = str.maketrans("aeiou", "áéíóú")
PLAIN = str.maketrans("áéíóú", "aeiou")
VOWELS = "aeiouáéíóú"
WEAK_KINDS = ("e>ie/i", "o>ue/u", "e>i")
STEM_KINDS = ("e>ie", "o>ue", "u>ue", "i>ie", "e>ie/i", "o>ue/u", "e>i")
TENSE_NAMES = {
    "presente": "presente (Bello: presente)",
    "preterito_imperfecto": "pretérito imperfecto (copretérito)",
    "preterito_perfecto_simple": "pretérito perfecto simple (pretérito)",
    "futuro_simple": "futuro simple (futuro)",
    "condicional_simple": "condicional simple (pospretérito)",
    "preterito_perfecto_compuesto": "pretérito perfecto compuesto (antepresente)",
    "preterito_pluscuamperfecto": "pretérito pluscuamperfecto (antecopretérito)",
    "preterito_anterior": "pretérito anterior (antepretérito)",
    "futuro_compuesto": "futuro compuesto (antefuturo)",
    "condicional_compuesto": "condicional compuesto (antepospretérito)",
}


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def load_table():
    try:
        return json.loads((pathlib.Path(__file__).resolve().parent / "irregulares.json").read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        fail(f"cannot read irregulares.json: {error}")


def accent_last_vowel(text):
    for i in range(len(text) - 1, -1, -1):
        if text[i] in "aeiou":
            return text[:i] + text[i].translate(ACCENT) + text[i + 1:]
    return text


def accent_weak(stem):
    """envi -> enví, actu -> actú, reun -> reún, prohib -> prohíb."""
    for i in range(len(stem) - 1, -1, -1):
        if stem[i] in "iu":
            return stem[:i] + stem[i].translate(ACCENT) + stem[i + 1:]
    return stem


def change(stem, kind, strong):
    if not kind:
        return stem
    if strong:
        source, target = {"e>ie": ("e", "ie"), "o>ue": ("o", "ue"), "u>ue": ("u", "ue"), "i>ie": ("i", "ie"),
                          "e>ie/i": ("e", "ie"), "o>ue/u": ("o", "ue"), "e>i": ("e", "i")}[kind]
    elif kind in WEAK_KINDS:
        source, target = {"e>ie/i": ("e", "i"), "o>ue/u": ("o", "u"), "e>i": ("e", "i")}[kind]
    else:
        return stem
    # in gu/qu the u is silent, so jugar's u is the only u that counts and seguir changes its e
    i = stem.rfind(source)
    if source == "u":
        i = stem.rfind("u", 0, len(stem) - 1) if stem.endswith(("gu", "qu")) else stem.rfind("u")
    if i < 0:
        return stem
    out = stem[:i] + target + stem[i + 1:]
    if out.startswith("ue"):
        out = "h" + out
    elif out.startswith("ie"):
        out = "y" + out[1:]
    return out


class Verb:
    def __init__(self, infinitive, table):
        self.infinitive = infinitive
        plain = infinitive.translate(PLAIN)
        self.plain = plain
        self.klass = plain[-2:]
        self.stem = plain[:-2]
        self.ending_class = "ar" if self.klass == "ar" else "er"
        self.features, self.model, self.known = self.lookup(infinitive, table)
        self.kind = self.features.get("stem")
        self.notes = []

    @staticmethod
    def lookup(infinitive, table):
        verbs = table["verbs"]
        entry = verbs.get(infinitive)
        if entry is None:
            return {}, None, False
        if "like" in entry:
            base_name = entry["like"]
            base = verbs[base_name]
            n = 0
            while n < min(len(base_name), len(infinitive)) and base_name[-1 - n] == infinitive[-1 - n]:
                n += 1
            base_pre, verb_pre = base_name[:len(base_name) - n], infinitive[:len(infinitive) - n]

            def shift(value):
                if isinstance(value, str) and value.startswith(base_pre):
                    return verb_pre + value[len(base_pre):]
                if isinstance(value, list):
                    return [shift(v) for v in value]
                if isinstance(value, dict):
                    return {k: shift(v) for k, v in value.items()}
                return value

            features = {k: (shift(v) if k in ("yo", "pret", "fut", "imp_tu", "imp_vos", "part", "ger", "subj_stem", "forms",
                                              "vos_pres", "part_alt") else v) for k, v in base.items() if k not in ("model", "note")}
            features.update({k: v for k, v in entry.items() if k != "like"})
            model = base.get("model", base_name) + f"; conjugated like {base_name}"
            return features, model, True
        return dict(entry), entry.get("model"), True

    # ------------------------------------------------------------ building blocks
    def join(self, stem, ending, orth=True):
        k, plain = self.klass, self.plain
        first = ending[:1]
        if orth and k == "ar" and first in ("e", "é"):
            if stem.endswith("c"):
                stem = stem[:-1] + "qu"
            elif stem.endswith("gu"):
                stem = stem[:-2] + "gü"
            elif stem.endswith("g"):
                stem = stem[:-1] + "gu"
            elif stem.endswith("z"):
                stem = stem[:-1] + "c"
        if orth and k in ("er", "ir") and first in ("a", "á", "o", "ó"):
            if stem.endswith("c"):
                if self.features.get("c_z") or len(stem) < 2 or stem[-2] not in VOWELS:
                    stem = stem[:-1] + "z"
                else:
                    stem = stem[:-1] + "zc"
            elif stem.endswith("gu") and plain.endswith("guir"):
                stem = stem[:-2] + "g"
            elif stem.endswith("qu") and plain.endswith("quir"):
                stem = stem[:-2] + "c"
            elif stem.endswith("g"):
                stem = stem[:-1] + "j"
        if k in ("er", "ir") and stem:
            silent_u = stem.endswith(("gu", "qu"))
            if plain.endswith("uir") and not silent_u and stem.endswith("u") and first in "aeoáéó":
                return stem + "y" + ending
            if stem[-1] in "aeou" and not silent_u and first == "i":
                if len(ending) > 1 and ending[1] in VOWELS:
                    return stem + "y" + ending[1:]
                if stem[-1] in "aeo":
                    return stem + "í" + ending[1:]
            if (stem.endswith("ñ") or stem.endswith("ll")) and first == "i" and len(ending) > 1 and ending[1] in VOWELS:
                return stem + ending[1:]
        return stem + ending

    def strong_stem(self):
        stem = change(self.stem, self.kind, True)
        return accent_weak(stem) if self.features.get("accent") else stem

    def weak_stem(self):
        return change(self.stem, self.kind, False)

    # ------------------------------------------------------------ tenses
    def presente(self):
        forms = self.features.get("forms", {}).get("presente")
        if forms:
            return list(forms)
        out = []
        for p in range(6):
            if p == 0 and self.features.get("yo"):
                out.append(self.features["yo"])
                continue
            stem = self.strong_stem() if p in STRESSED else self.stem
            out.append(self.join(stem, PRES[self.klass][p]))
        return out

    def subj_presente(self):
        forms = self.features.get("forms", {}).get("subj_presente")
        if forms:
            return list(forms)
        f = self.features
        stem_all = f.get("subj_stem") or (f["yo"][:-1] if f.get("yo", "").endswith("o") else None)
        if stem_all is not None:
            return [stem_all + e for e in SUBJ[self.klass]]
        out = []
        for p in range(6):
            if p in STRESSED:
                stem = self.strong_stem()
            elif self.klass == "ir" and self.kind in WEAK_KINDS:
                stem = self.weak_stem()
            else:
                stem = self.stem
            out.append(self.join(stem, SUBJ[self.klass][p]))
        return out

    def imperfecto(self):
        forms = self.features.get("forms", {}).get("imperfecto")
        return list(forms) if forms else [self.join(self.stem, e) for e in IMPF[self.ending_class]]

    def preterito(self):
        forms = self.features.get("forms", {}).get("preterito")
        if forms:
            return list(forms)
        strong = self.features.get("pret")
        if strong:
            out = []
            for p, e in enumerate(STRONG):
                if p == 5 and strong.endswith("j"):
                    e = "eron"
                if p == 2 and strong.endswith("c"):
                    out.append(strong[:-1] + "zo")
                    continue
                out.append(strong + e)
            return out
        out = []
        for p, e in enumerate(PRET[self.ending_class]):
            stem = self.weak_stem() if p in (2, 5) and self.klass == "ir" else self.stem
            out.append(self.join(stem, e))
        return out

    def future_stem(self):
        return self.features.get("fut") or self.plain

    def futuro(self):
        return [self.future_stem() + e for e in FUT]

    def condicional(self):
        return [self.future_stem() + e for e in COND]

    def from_preterite(self, endings, first_plural):
        base = self.preterito()[5][:-3]  # amaron -> ama, tuvieron -> tuvie
        out = [base + e for e in endings]
        out[3] = accent_last_vowel(base) + first_plural
        return out

    def gerundio(self):
        if self.features.get("ger"):
            return self.features["ger"]
        if self.klass == "ar":
            return self.stem + "ando"
        stem = self.weak_stem() if self.klass == "ir" else self.stem
        return self.join(stem, "iendo")

    def participio(self):
        if self.features.get("part"):
            return self.features["part"]
        return self.stem + "ado" if self.klass == "ar" else self.join(self.stem, "ido")

    def vos_presente(self):
        if "vos_pres" in self.features:
            return self.features["vos_pres"]
        return accent_last_vowel(self.plain[:-1]) + "s"

    # ------------------------------------------------------------ notes
    def spelling_notes(self):
        notes, s, plain = [], self.stem, self.plain
        if self.klass == "ar":
            for end, text in (("c", "c → qu before e (busqué)"), ("gu", "gu → gü before e (averigüé)"),
                              ("g", "g → gu before e (pagué)"), ("z", "z → c before e (cacé)")):
                if s.endswith(end):
                    notes.append(text)
                    break
        else:
            if s.endswith("c"):
                if self.features.get("c_z") or len(s) < 2 or s[-2] not in VOWELS:
                    notes.append("c → z before a and o (venzo)")
                else:
                    notes.append("-zc- before a and o (conozco)")
            elif s.endswith("gu") and plain.endswith("guir"):
                notes.append("gu → g before a and o (sigo)")
            elif s.endswith("qu") and plain.endswith("quir"):
                notes.append("qu → c before a and o (delinco)")
            elif s.endswith("g"):
                notes.append("g → j before a and o (cojo)")
            if plain.endswith("uir") and not s.endswith(("gu", "qu")):
                notes.append("y before a, e and o in -uir verbs (construyo)")
            elif s and s[-1] in "aeo":
                notes.append("unstressed i between vowels → y (leyó, leyendo); stressed í in hiatus (leíste, leído)")
            if s.endswith(("ñ", "ll")):
                notes.append("unstressed i dropped after ñ and ll (tañó, bulló)")
        return notes


def describe(v):
    regular = {"ar": "amar (regular -ar)", "er": "temer (regular -er)", "ir": "partir (regular -ir)"}[v.klass]
    f, extra = v.features, []
    if f.get("stem"):
        extra.append(f"stem change {f['stem']} (" + {"e>ie": "pensar, entender", "o>ue": "contar, mover", "u>ue": "jugar",
                     "i>ie": "adquirir", "e>ie/i": "sentir", "o>ue/u": "dormir", "e>i": "pedir"}[f["stem"]] + ")")
    if f.get("accent"):
        extra.append("written accent on the stressed weak vowel (enviar, actuar, reunir)")
    if f.get("part"):
        extra.append(f"irregular participle {f['part']}")
    if f.get("c_z"):
        extra.append("c → z before a and o, without -zc- (mecer)")
    return regular + (", with " + "; ".join(extra) if extra else "")


def conjugate(verb, voseo, compound, table, haber=None):
    v = Verb(verb, table)
    pres, subj, impf, pret = v.presente(), v.subj_presente(), v.imperfecto(), v.preterito()
    fut, cond = v.futuro(), v.condicional()
    ra = v.from_preterite(["ra", "ras", "ra", "ramos", "rais", "ran"], "ramos")
    se = v.from_preterite(["se", "ses", "se", "semos", "seis", "sen"], "semos")
    fs = v.from_preterite(["re", "res", "re", "remos", "reis", "ren"], "remos")
    part, ger = v.participio(), v.gerundio()

    def table6(forms, vos=None):
        out = {}
        for i, person in enumerate(PERSONS):
            out[person] = forms[i]
            if voseo and i == 1:
                out["vos"] = vos if vos is not None else forms[1]
        return out

    f = v.features
    no_imperative = "imp_tu" in f and f["imp_tu"] is None and "imp_vosotros" in f
    imperative = {}
    if not no_imperative:
        tu = f["imp_tu"] if "imp_tu" in f else pres[2]
        vos = f["imp_vos"] if "imp_vos" in f else accent_last_vowel(v.plain[:-1])
        vosotros = f.get("imp_vosotros", v.infinitive[:-1] + "d")
        affirmative = {"tú": tu}
        if voseo:
            affirmative["vos"] = vos
        affirmative.update({"usted": subj[2], "nosotros": f.get("imp_nosotros", subj[3]), "vosotros": vosotros, "ustedes": subj[5]})
        negative = {"tú": "no " + subj[1]}
        if voseo:
            negative["vos"] = "no " + subj[1]
        negative.update({"usted": "no " + subj[2], "nosotros": "no " + subj[3], "vosotros": "no " + subj[4], "ustedes": "no " + subj[5]})
        imperative = {"afirmativo": affirmative, "negativo": negative}

    result = {
        "verb": verb,
        "model": v.model or describe(v),
        "irregular": bool(v.known and set(f) - {"model", "note", "c_z"} or (v.known and v.model)),
        "infinitivo": verb,
        "gerundio": ger,
        "participio": part,
        "indicativo": {
            "presente": table6(pres, v.vos_presente()),
            "preterito_imperfecto": table6(impf),
            "preterito_perfecto_simple": table6(pret),
            "futuro_simple": table6(fut),
            "condicional_simple": table6(cond),
        },
        "subjuntivo": {
            "presente": table6(subj),
            "preterito_imperfecto_ra": table6(ra),
            "preterito_imperfecto_se": table6(se),
            "futuro_simple": table6(fs),
        },
        "imperativo": imperative or None,
    }
    if f.get("part_alt"):
        result["participio_regular"] = f["part_alt"]
    if compound and haber is not None:
        h = haber
        result["infinitivo_compuesto"] = "haber " + part
        result["gerundio_compuesto"] = "habiendo " + part
        hi, hs = h["indicativo"], h["subjuntivo"]
        result["indicativo"].update({
            "preterito_perfecto_compuesto": {p: x + " " + part for p, x in hi["presente"].items()},
            "preterito_pluscuamperfecto": {p: x + " " + part for p, x in hi["preterito_imperfecto"].items()},
            "preterito_anterior": {p: x + " " + part for p, x in hi["preterito_perfecto_simple"].items()},
            "futuro_compuesto": {p: x + " " + part for p, x in hi["futuro_simple"].items()},
            "condicional_compuesto": {p: x + " " + part for p, x in hi["condicional_simple"].items()},
        })
        result["subjuntivo"].update({
            "preterito_perfecto_compuesto": {p: x + " " + part for p, x in hs["presente"].items()},
            "preterito_pluscuamperfecto_ra": {p: x + " " + part for p, x in hs["preterito_imperfecto_ra"].items()},
            "preterito_pluscuamperfecto_se": {p: x + " " + part for p, x in hs["preterito_imperfecto_se"].items()},
            "futuro_compuesto": {p: x + " " + part for p, x in hs["futuro_simple"].items()},
        })
    notes = []
    if not v.known:
        notes.append("conjugated on the regular model with the automatic spelling rules (the verb is not in the bundled "
                     "irregular list; a stem-changing or otherwise irregular verb missing from it would come out wrong)")
    notes += v.spelling_notes()
    if f.get("note"):
        notes.append(f["note"])
    if voseo:
        notes.append("vos takes its own forms in the present indicative and the imperative; elsewhere, including the present "
                     "subjunctive, the tú forms given are the norm (forms such as amés are also heard)")
    result["notes"] = notes
    return result


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = set(request) - {"verb", "voseo", "compound", "list"}
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    for name in ("voseo", "compound", "list"):
        if name in request and not isinstance(request[name], bool):
            fail(f"{name} must be a boolean")
    table = load_table()
    if request.get("list"):
        models = {}
        for name, entry in table["verbs"].items():
            if "like" in entry:
                model = f"like {entry['like']}"
            else:
                model = entry.get("model") or ("stem " + entry["stem"] if "stem" in entry else
                                               "accent on the weak vowel" if entry.get("accent") else
                                               "irregular participle" if "part" in entry else "c → z (no -zc-)")
            models.setdefault(model, []).append(name)
        print(json.dumps({"count": len(table["verbs"]), "source": table["source"], "generated": table["generated"],
                          "models": dict(sorted(models.items()))}, ensure_ascii=False))
        return
    verb = request.get("verb")
    if not isinstance(verb, str):
        fail("verb must be a string (an infinitive such as amar, temer, partir)")
    verb = verb.strip().lower()
    if re.fullmatch(r"[a-zñáéíóúü]+(ar|er|ir|ír)(se|me|te|nos|os)", verb):
        fail("pronominal verbs are not conjugated with their pronouns; conjugate the infinitive without -se and add me, te, se, nos, os")
    if not re.fullmatch(r"[a-zñáéíóúü]*(ar|er|ir|ír)", verb) or len(verb) < 2 or len(verb) > 40:
        fail("verb must be a Spanish infinitive ending in -ar, -er or -ir")
    compound = request.get("compound", True)
    haber = conjugate("haber", request.get("voseo", False), False, table) if compound else None
    result = conjugate(verb, request.get("voseo", False), compound, table, haber)
    result["tense_names"] = TENSE_NAMES
    result["persons"] = {"él": "él, ella, usted", "ellos": "ellos, ellas, ustedes", "nosotros": "nosotros, nosotras",
                         "vosotros": "vosotros, vosotras"}
    result["source"] = table["source"]
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
