#!/usr/bin/env python3
"""Heuristic gender and number agreement check for Spanish noun phrases: determiner,
adjectives and noun, with a lexicon of nouns whose gender the ending does not give
(el día, la mano, el problema) and the rule for feminine nouns that begin with a stressed
a or ha (el agua fría, las aguas, esta agua, mucha hambre).

Input (JSON on stdin):
  text   string: one or more noun phrases or short sentences (required)
Output: the phrases found, each with its noun, gender (m, f or null), number (s, p), how the
gender was decided, and the issues with a suggested correction.
"""
import json
import re
import sys
import unicodedata

NOTE = ("Heuristic check, not a parser: words are classed from small lists and word endings, so verbs or nouns it does "
        "not know may be taken for adjectives, and nouns of either gender (el/la estudiante) take the determiner's gender.")

# determiner -> (family, gender, number); gender None = either
DETERMINERS = {}
FAMILIES = {
    "el": ("el", "la", "los", "las"), "un": ("un", "una", "unos", "unas"), "este": ("este", "esta", "estos", "estas"),
    "ese": ("ese", "esa", "esos", "esas"), "aquel": ("aquel", "aquella", "aquellos", "aquellas"),
    "nuestro": ("nuestro", "nuestra", "nuestros", "nuestras"), "vuestro": ("vuestro", "vuestra", "vuestros", "vuestras"),
    "algún": ("algún", "alguna", "algunos", "algunas"), "ningún": ("ningún", "ninguna", "ningunos", "ningunas"),
    "otro": ("otro", "otra", "otros", "otras"), "mucho": ("mucho", "mucha", "muchos", "muchas"),
    "poco": ("poco", "poca", "pocos", "pocas"), "todo": ("todo", "toda", "todos", "todas"),
    "tanto": ("tanto", "tanta", "tantos", "tantas"), "cuánto": ("cuánto", "cuánta", "cuántos", "cuántas"),
    "demasiado": ("demasiado", "demasiada", "demasiados", "demasiadas"), "cierto": ("cierto", "cierta", "ciertos", "ciertas"),
    "varios": (None, None, "varios", "varias"), "ambos": (None, None, "ambos", "ambas"),
    "alguno": ("alguno", "alguna", "algunos", "algunas"), "ninguno": ("ninguno", "ninguna", "ningunos", "ningunas"),
}
for family, forms in FAMILIES.items():
    for i, form in enumerate(forms):
        if form and form not in DETERMINERS:
            DETERMINERS[form] = (family, "m" if i % 2 == 0 else "f", "s" if i < 2 else "p")
for word, number in (("mi", "s"), ("tu", "s"), ("su", "s"), ("mis", "p"), ("tus", "p"), ("sus", "p"), ("cada", "s"),
                     ("cualquier", "s"), ("qué", None), ("dos", "p"), ("tres", "p"), ("cuatro", "p"), ("cinco", "p"),
                     ("seis", "p"), ("siete", "p"), ("ocho", "p"), ("nueve", "p"), ("diez", "p"), ("cien", "p"), ("mil", "p")):
    DETERMINERS[word] = (word, None, number)
DETERMINERS["al"] = ("el", "m", "s")
DETERMINERS["del"] = ("el", "m", "s")
DETERMINERS["lo"] = ("lo", None, "s")

STRESSED_A_FORMS = {"el": "el", "un": "un", "algún": "algún", "ningún": "ningún"}

# words that end a phrase: prepositions, conjunctions, pronouns, adverbs, frequent verbs
BREAKERS = set("""a ante bajo cabe con contra de desde durante en entre hacia hasta mediante para por según sin so sobre tras
y e ni o u pero sino que porque como cuando donde si aunque mientras pues
yo tú usted él ella ellos ellas nosotros nosotras vosotros vosotras ustedes me te se nos os le les lo
no sí ya también tampoco aquí allí ahí hoy ayer mañana siempre nunca
es son era eran fue fueron será serán sea sean está están estaba estaban estuvo estará estén esté
hay había hubo habrá tiene tienen tenía tenían tuvo hace hacen hizo va van iba fue parece parecen parecía
queda quedan quedó resulta resultan resultó sigue siguen vive viven come comen ve ven dice dicen
ser estar haber tener hacer ir ver dar""".split())
COPULAS = {"es", "son", "era", "eran", "fue", "fueron", "será", "serán", "sea", "sean", "está", "están", "estaba",
           "estaban", "estuvo", "estará", "parece", "parecen", "parecía", "queda", "quedan", "quedó", "resulta",
           "resultan", "resultó", "sigue", "siguen"}
ADVERBS = {"muy", "más", "menos", "tan", "bastante", "realmente", "sumamente", "algo"}
PRENOMINAL = set("""buen bueno buena buenos buenas mal malo mala malos malas gran grande grandes pequeño pequeña pequeños
pequeñas nuevo nueva nuevos nuevas viejo vieja viejos viejas mejor mejores peor peores primer primero primera primeros
primeras segundo segunda segundos segundas tercer tercero tercera último última últimos últimas próximo próxima próximos
próximas mismo misma mismos mismas propio propia propios propias antiguo antigua antiguos antiguas hermoso hermosa
hermosos hermosas bonito bonita bonitos bonitas bello bella bellos bellas pobre pobres santo santa san único única
únicos únicas verdadero verdadera verdaderos verdaderas largo larga largos largas breve breves medio media
supuesto supuesta supuestos supuestas fría frío fríos frías dulce dulces""".split())

# noun lexicon: gender the ending does not give, or confirms an exception. "c" = common gender (el/la estudiante),
# "b" = either (el/la mar)
GREEK_MA = """problema sistema tema idioma programa clima drama poema esquema dilema emblema enigma fantasma dogma lema
prisma teorema diagrama panorama aroma carisma crucigrama telegrama síntoma trauma estigma plasma holograma axioma
diploma eccema edema fonema morfema lexema cisma magma paradigma pentagrama sofisma monograma organigrama karma
carcinoma melanoma glaucoma hematoma anagrama epigrama histograma cinema""".split()
LEXICON = {w: "m" for w in GREEK_MA}
LEXICON.update({w: "m" for w in """día mediodía mapa planeta cometa tranvía sofá papá pijama yoga koala gorila puma
coche padre hombre nombre golpe parque puente diente accidente aceite café té pie valle tomate juguete bosque cine baile
chiste postre aire mes país pez lápiz arroz maíz disfraz ajedrez avestruz matiz tapiz árbol papel hotel sol animal
hospital pan tren jardín avión camión gorrión sarampión análisis énfasis paréntesis oasis éxtasis chasis virus
lunes martes miércoles jueves viernes sábado domingo césped ataúd huésped alumno libro rey buey menú champú
continente cliente equipo tanque desastre detalle informe paisaje viaje mensaje garaje lenguaje""".split()})
LEXICON.update({w: "f" for w in """mano foto moto libido seo dinamo flor labor coliflor leche noche calle carne clase fe
fuente gente llave muerte nieve nube parte sangre suerte tarde torre base frase especie serie superficie piel sal miel
cárcel señal catedral red pared sed ley voz luz paz cruz vez raíz nariz actriz imagen mujer razón sazón sartén crin
hambre ave clave nave tos mies res crisis tesis hipótesis síntesis dosis sinopsis metamorfosis diócesis tribu
agua águila alma arma área aula hacha hada habla ala ancla ansia asma aria haba álgebra alga alba acta arca arpa hampa
aspa asa ama alza áncora ánima aguja madre""".split()})
LEXICON.update({w: "c" for w in """estudiante cantante presidente paciente agente representante amante adolescente
colega atleta guía policía astronauta homicida suicida patriota idiota psiquiatra pediatra testigo joven modelo
miembro rehén cónyuge intérprete árabe ácrata""".split()})
LEXICON.update({w: "b" for w in "mar azúcar arte radio internet color calor dote linde maratón".split()})
STRESSED_A = set("""agua águila alma arma área aula hacha hada habla ala ancla ansia asma aria haba álgebra alga alba
acta arca arpa hampa aspa asa ama alza áncora ánima hambre ave""".split())
SINGULAR_S = {w for w, g in LEXICON.items() if w.endswith("s")}

ADJ_INVARIABLE_OR = {"mejor", "peor", "mayor", "menor", "superior", "inferior", "exterior", "interior", "anterior",
                     "posterior", "ulterior", "multicolor", "incoloro"}
ESO_ADJECTIVES = {"grueso", "espeso", "tieso", "ileso", "travieso", "obeso", "preso", "confeso", "tenso", "denso",
                  "intenso", "extenso", "propenso", "inmenso", "ingreso", "expreso", "impreso", "converso"}
ADJ_INVARIABLE_S = {"gris", "cortés", "descortés", "isósceles"}


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def strip_accents(word):
    return "".join(c for c in unicodedata.normalize("NFD", word) if unicodedata.category(c) != "Mn")


def singular_of(word):
    """Candidate singulars of a noun or adjective, most likely first; empty when it looks singular."""
    if word in LEXICON or word in SINGULAR_S or not word.endswith("s") or word.endswith(("sis", "itis", "us")):
        return []
    candidates = []
    if word.endswith("ces"):
        candidates.append(word[:-3] + "z")
    if word.endswith("iones"):
        candidates.append(word[:-5] + "ión")
    if word.endswith("ones"):
        candidates.append(word[:-4] + "ón")
    if word.endswith("eses"):
        candidates.append(word[:-4] + "és")
    if word.endswith("es") and len(word) > 3 and strip_accents(word[-3]) not in "aeiou":
        stem = word[:-2]
        candidates.append(stem)
        accentless = strip_accents(stem)
        if accentless != stem:
            candidates.append(accentless)
        if stem == "jóven":
            candidates.append("joven")
        if strip_accents(stem) == "joven":
            candidates.append("joven")
    candidates.append(word[:-1])
    return candidates


def noun_info(word):
    """(gender, number, source); gender m, f, c (either, from the determiner), b (both accepted) or None."""
    number = "s"
    base = word
    if word in LEXICON:
        return LEXICON[word], "s" if not word.endswith("s") or word in SINGULAR_S else "s", "lexicon"
    candidates = singular_of(word)
    if candidates:
        number = "p"
        for candidate in candidates:
            if candidate in LEXICON:
                return LEXICON[candidate], "p", "lexicon"
        base = candidates[0]
    if base.endswith("ista"):
        return "c", number, "ending -ista (either gender)"
    for ending, gender in (("ción", "f"), ("sión", "f"), ("xión", "f"), ("ión", "f"), ("dad", "f"), ("tad", "f"),
                           ("tud", "f"), ("umbre", "f"), ("sis", "f"), ("itis", "f"), ("ie", "f"), ("triz", "f"),
                           ("aje", "m"), ("ón", "m"), ("or", "m"), ("ora", "f"), ("ma", None), ("o", "m"), ("a", "f"),
                           ("án", "m"), ("ambre", "m"), ("ante", "c"), ("ente", "c")):
        if base.endswith(ending):
            if gender is None:
                return None, number, "ending -ma (Greek -ma nouns are masculine; not in the lexicon)"
            return gender, number, f"ending -{ending}"
    return None, number, "unknown ending"


def adjective_info(word):
    """(gender, number): gender m or f when the form marks it, None for invariable adjectives."""
    w = word
    if w in ADJ_INVARIABLE_S:
        return None, "s"
    if w.endswith("es") and w[:-2] in ADJ_INVARIABLE_S:
        return None, "p"
    plural = w.endswith("s") and not w.endswith(("és", "ás", "ís", "ús"))
    base = w
    if plural:
        if w.endswith("es") and len(w) > 3 and w[-3] in "lrnsyzj":
            base = w[:-2]  # azules, trabajadores, mandones, franceses, jóvenes
        elif w.endswith("ces"):
            base = w[:-1]  # dulces (felices -> feliz is also invariable)
        else:
            base = w[:-1]
    number = "p" if plural else "s"
    plain = strip_accents(base)
    if plain in ADJ_INVARIABLE_OR or plain.endswith("ista"):
        return None, number
    if plain.endswith("o"):
        return "m", number
    if plain.endswith("a"):
        return "f", number
    if plain.endswith(("or", "on", "es")) or base.endswith("án") or (plural and plain.endswith("an")):
        return "m", number
    return None, number


def inflect_adjective(word, gender, number):
    w = word
    g_now, n_now = adjective_info(w)
    plain = strip_accents(w)
    # find the singular stem
    if g_now is None:
        singular = w
        if n_now == "p":
            singular = w[:-1] if not w.endswith("es") or w[:-2].endswith(("e",)) else w[:-2]
            if w.endswith("ces"):
                singular = w[:-3] + "z"
        if number == "s":
            return singular
        if singular.endswith(("a", "e", "i", "o", "u", "á", "é", "ó")):
            return singular + "s"
        if singular.endswith("z"):
            return singular[:-1] + "ces"
        if singular == "joven":
            return "jóvenes"
        return strip_accents(singular) + "es" if singular.endswith(("ún", "ín", "án", "ón", "és")) else singular + "es"
    kind, stem = None, w
    for ending, k in (("doras", "or"), ("toras", "or"), ("soras", "or"), ("dores", "or"), ("tores", "or"), ("sores", "or"),
                      ("dora", "or"), ("tora", "or"), ("sora", "or"), ("dor", "or"), ("tor", "or"), ("sor", "or"),
                      ("onas", "on"), ("ones", "on"), ("ona", "on"), ("ón", "on"),
                      ("esas", "es"), ("eses", "es"), ("esa", "es"), ("és", "es"),
                      ("os", "o"), ("as", "o"), ("o", "o"), ("a", "o")):
        if w.endswith(ending):
            kind, stem = k, w[: len(w) - len(ending)] + (ending[0] if k == "or" else "")
            break
    if kind == "on" and len(stem) < 3:
        kind, stem = "o", stem + "on"  # mona, monas -> mono
    if kind == "es" and stem + "eso" in ESO_ADJECTIVES:
        kind, stem = "o", stem + "es"  # gruesa -> grueso
    if kind is None:
        return w
    table = {"o": ("o", "a", "os", "as"), "or": ("or", "ora", "ores", "oras"), "on": ("ón", "ona", "ones", "onas"),
             "es": ("és", "esa", "eses", "esas")}[kind]
    return stem + table[(0 if gender == "m" else 1) + (0 if number == "s" else 2)]


def determiner_form(word, gender, number):
    family = DETERMINERS[word][0]
    forms = FAMILIES.get(family)
    if not forms:
        return word
    index = (0 if gender == "m" else 1) + (0 if number == "s" else 2)
    return forms[index] or word


def split_phrases(text):
    """Runs of words between punctuation and breakers, with the copula that may follow each."""
    tokens = re.findall(r"[^\W\d_]+(?:['’-][^\W\d_]+)*|\d+|[^\w\s]", text)
    runs, current, copula_before = [], [], None
    pending_copula = None
    for raw in tokens:
        word = raw.lower()
        if not re.match(r"[^\W\d_]", word) or word in BREAKERS:
            if current:
                runs.append((current, pending_copula))
                current = []
                pending_copula = None
            pending_copula = word if word in COPULAS and runs else None
            continue
        current.append((raw, word))
    if current:
        runs.append((current, pending_copula))
    return runs


def analyse(run, copula, previous):
    words = [w for w in run if w[1] not in ADVERBS]
    if not words:
        return None
    determiners, i = [], 0
    while i < len(words) and words[i][1] in DETERMINERS:
        determiners.append(words[i])
        i += 1
    content = words[i:]
    issues = []
    phrase = {"text": " ".join(w[0] for w in run)}
    if not content:
        return None
    # predicative adjectives after a copula: agree with the previous phrase's noun
    if copula and not determiners and previous and previous.get("gender") in ("m", "f"):
        phrase.update({"role": "predicate", "of": previous["text"], "copula": copula})
        for raw, w in content:
            g, n = adjective_info(w)
            want_g, want_n = previous["gender"], previous["number"]
            if (g and g != want_g) or n != want_n:
                issues.append({"word": raw, "problem": f"predicate adjective is {describe(g, n)}, subject '{previous['text']}' is "
                               f"{describe(want_g, want_n)}", "suggestion": inflect_adjective(w, want_g, want_n)})
        phrase["issues"] = issues
        return phrase
    noun_index = 0
    while noun_index < len(content) - 1 and content[noun_index][1] in PRENOMINAL:
        noun_index += 1
    noun_raw, noun = content[noun_index]
    gender, number, source = noun_info(noun)
    det_gender = next((DETERMINERS[d[1]][1] for d in reversed(determiners) if DETERMINERS[d[1]][1]), None)
    det_number = next((DETERMINERS[d[1]][2] for d in reversed(determiners) if DETERMINERS[d[1]][2]), None)
    invariable_number = noun.endswith(("sis", "itis", "us")) or (noun in SINGULAR_S and not noun.endswith(("és", "ís", "ás", "ús")))
    if invariable_number and det_number:
        number, source = det_number, source + "; number from the determiner (same form in singular and plural)"
    stressed_a = noun in STRESSED_A and number == "s"
    resolved = gender
    if gender in ("c", "b", None):
        if det_gender and not (stressed_a and gender is None):
            resolved, source = det_gender, source + "; gender from the determiner"
        else:
            adj_genders = [adjective_info(w)[0] for _, w in content if w != noun]
            adj_genders = [g for g in adj_genders if g]
            resolved = adj_genders[0] if adj_genders and gender != "b" else (None if gender != "b" else None)
            if resolved:
                source += "; gender from the adjectives"
    if gender == "b" and det_gender:
        resolved = det_gender
    phrase.update({"noun": noun_raw, "gender": resolved if resolved in ("m", "f") else None, "number": number,
                   "gender_source": source})
    if stressed_a:
        phrase["rule"] = ("feminine noun beginning with stressed a/ha: el, un, algún, ningún directly before it in the "
                          "singular; adjectives and other determiners stay feminine")
    # determiners
    for k, (raw, d) in enumerate(determiners):
        family, dg, dn = DETERMINERS[d]
        directly_before = k == len(determiners) - 1 and noun_index == 0
        if dn and number and dn != number and family not in ("lo", "qué"):
            issues.append({"word": raw, "problem": f"determiner is {describe(None, dn)}, noun '{noun_raw}' is {describe(None, number)}",
                           "suggestion": determiner_form(d, dg or resolved or "m", number) if family in FAMILIES else None})
            continue
        if not dg or resolved not in ("m", "f"):
            continue
        if stressed_a and resolved == "f" and number == "s" and directly_before and family in STRESSED_A_FORMS:
            if dg == "f":
                if d == "la":
                    issues.append({"word": raw, "problem": "before a feminine noun with stressed a/ha the article is el",
                                   "suggestion": "el"})
                else:
                    issues.append({"word": raw, "severity": "note", "problem": f"{STRESSED_A_FORMS[family]} is the usual form "
                                   "before a feminine noun with stressed a/ha; the feminine form is also accepted",
                                   "suggestion": STRESSED_A_FORMS[family]})
            continue
        if dg != resolved:
            fix = determiner_form(d, resolved, number) if family in FAMILIES else None
            problem = f"determiner is {describe(dg, None)}, noun '{noun_raw}' is {describe(resolved, None)}"
            if stressed_a and resolved == "f" and dg == "m" and family in STRESSED_A_FORMS:
                problem += " (el/un are used only directly before the noun)"
            elif stressed_a and resolved == "f":
                problem += " (only el, un, algún, ningún take the masculine-looking form: esta agua, mucha hambre)"
            issues.append({"word": raw, "problem": problem, "suggestion": fix})
    # adjectives
    for k, (raw, w) in enumerate(content):
        if k == noun_index:
            continue
        g, n = adjective_info(w)
        problems = []
        if g and resolved in ("m", "f") and g != resolved:
            problems.append(f"adjective is {describe(g, None)}, noun is {describe(resolved, None)}")
        if n != number:
            problems.append(f"adjective is {describe(None, n)}, noun is {describe(None, number)}")
        if problems:
            issues.append({"word": raw, "problem": "; ".join(problems),
                           "suggestion": inflect_adjective(w, resolved if resolved in ("m", "f") else (g or "m"), number)})
    phrase["issues"] = issues
    return phrase


def describe(gender, number):
    parts = []
    if gender:
        parts.append({"m": "masculine", "f": "feminine"}[gender])
    if number:
        parts.append({"s": "singular", "p": "plural"}[number])
    return " ".join(parts) or "unmarked"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = set(request) - {"text"}
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be a non-empty string")
    if len(text) > 20000:
        fail("text is longer than 20,000 characters")
    phrases, previous = [], None
    for run, copula in split_phrases(text):
        phrase = analyse(run, copula, previous)
        if phrase:
            phrases.append(phrase)
            if phrase.get("role") != "predicate":
                previous = phrase
    errors = sum(1 for p in phrases for i in p["issues"] if i.get("severity", "error") == "error")
    notes = sum(1 for p in phrases for i in p["issues"] if i.get("severity") == "note")
    for p in phrases:
        for i in p["issues"]:
            i.setdefault("severity", "error")
    print(json.dumps({"ok": errors == 0, "errors": errors, "notes": notes, "phrases": phrases, "heuristic": NOTE},
                     ensure_ascii=False))


if __name__ == "__main__":
    main()
