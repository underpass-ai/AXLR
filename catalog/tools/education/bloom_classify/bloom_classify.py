#!/usr/bin/env python3
"""Classify learning objectives by the level of the revised Bloom taxonomy (Anderson &
Krathwohl 2001, cognitive process dimension) from the main verb, in English or Spanish.

Input (JSON on stdin):
  objectives   ["Students will be able to compare ...", ...]  (1-200 strings)
  language     "en" | "es" | "auto"   default auto (per objective)

The verb table below is bundled in the program. Entries written verb:process are the 19
cognitive processes of the revised taxonomy and their alternative names (Anderson & Krathwohl
2001, Table 5.1; Krathwohl 2002, Theory Into Practice 41(4) 212-218); the other verbs are common
usage, compiled for this tool (MIT). A verb listed under more than one level is "ambiguous";
its first level is the one of the taxonomy's own process when it has one.
"""
import json
import re
import sys
import unicodedata

LEVELS = ["remember", "understand", "apply", "analyze", "evaluate", "create"]
LEVEL_NAMES = {"en": ["Remember", "Understand", "Apply", "Analyze", "Evaluate", "Create"],
               "es": ["Recordar", "Comprender", "Aplicar", "Analizar", "Evaluar", "Crear"]}
EN = {
    "remember": "recognize:recognizing identify:recognizing recall:recalling retrieve:recalling list name define state "
                "memorize repeat label reproduce locate quote describe",
    "understand": "interpret:interpreting clarify:interpreting paraphrase:interpreting represent:interpreting "
                  "translate:interpreting exemplify:exemplifying illustrate:exemplifying instantiate:exemplifying "
                  "classify:classifying categorize:classifying subsume:classifying summarize:summarizing "
                  "abstract:summarizing generalize:summarizing infer:inferring conclude:inferring "
                  "extrapolate:inferring interpolate:inferring predict:inferring compare:comparing "
                  "contrast:comparing map:comparing match:comparing explain:explaining describe discuss restate "
                  "rephrase report outline demonstrate relate",
    "apply": "execute:executing carry_out:executing implement:implementing use:implementing apply solve calculate "
             "compute demonstrate operate perform practise practice modify prepare show employ conduct construct test",
    "analyze": "differentiate:differentiating discriminate:differentiating distinguish:differentiating "
               "focus:differentiating select:differentiating organize:organizing find_coherence:organizing "
               "integrate:organizing outline:organizing parse:organizing structure:organizing "
               "attribute:attributing deconstruct:attributing analyze analyse examine investigate break_down "
               "diagram dissect inspect compare contrast classify identify relate",
    "evaluate": "check:checking coordinate:checking detect:checking monitor:checking test:checking "
                "critique:critiquing judge:critiquing evaluate assess appraise argue defend justify rank rate "
                "recommend prioritize verify support conclude select",
    "create": "generate:generating hypothesize:generating plan:planning design:planning produce:producing "
              "construct:producing create compose invent formulate develop devise build author write propose "
              "organize integrate",
}
ES = {
    "remember": "reconocer:recognizing identificar:recognizing recordar:recalling recuperar:recalling enumerar listar "
                "nombrar definir citar memorizar repetir etiquetar reproducir localizar describir",
    "understand": "interpretar:interpreting aclarar:interpreting parafrasear:interpreting representar:interpreting "
                  "traducir:interpreting ejemplificar:exemplifying ilustrar:exemplifying instanciar:exemplifying "
                  "clasificar:classifying categorizar:classifying subsumir:classifying resumir:summarizing "
                  "abstraer:summarizing generalizar:summarizing inferir:inferring concluir:inferring "
                  "extrapolar:inferring interpolar:inferring predecir:inferring comparar:comparing "
                  "contrastar:comparing emparejar:comparing explicar:explaining describir discutir reformular "
                  "exponer esquematizar demostrar relacionar",
    "apply": "ejecutar:executing implementar:implementing usar:implementing utilizar:implementing aplicar resolver "
             "calcular computar demostrar operar realizar practicar modificar preparar mostrar emplear construir probar",
    "analyze": "diferenciar:differentiating discriminar:differentiating distinguir:differentiating "
               "focalizar:differentiating seleccionar:differentiating organizar:organizing integrar:organizing "
               "esquematizar:organizing estructurar:organizing atribuir:attributing deconstruir:attributing "
               "analizar examinar investigar descomponer diagramar inspeccionar comparar contrastar clasificar "
               "identificar relacionar",
    "evaluate": "comprobar:checking coordinar:checking detectar:checking monitorizar:checking probar:checking "
                "criticar:critiquing juzgar:critiquing evaluar valorar argumentar defender justificar jerarquizar "
                "recomendar priorizar verificar apoyar concluir seleccionar",
    "create": "generar:generating hipotetizar:generating planificar:planning diseñar:planning producir:producing "
              "construir:producing crear componer inventar formular desarrollar idear elaborar redactar proponer "
              "organizar integrar",
}
VAGUE_EN = "understand know learn appreciate comprehend realize realise grasp believe internalize internalise " \
           "be_aware become_aware be_familiar become_familiar familiarize think feel"
VAGUE_ES = "entender comprender saber conocer aprender apreciar darse_cuenta familiarizarse creer interiorizar " \
           "pensar sentir ser_consciente tomar_conciencia"
IRREGULAR_ES = {  # forms the regular rules miss: 3rd person present, future, subjunctive
    "recordar": "recuerda recuerdan recuerde", "predecir": "predice predicen predira prediga", "concluir": "concluye concluyen",
    "construir": "construye construyen construya", "atribuir": "atribuye atribuyen", "deconstruir": "deconstruye",
    "componer": "compondra compondran componga", "proponer": "propondra propondran proponga", "resolver": "resuelve resuelven resuelva",
    "demostrar": "demuestra demuestran demuestre", "probar": "prueba prueban pruebe", "mostrar": "muestra muestran muestre",
    "defender": "defiende defienden defienda", "comprobar": "comprueba comprueban compruebe", "distinguir": "distinga",
    "identificar": "identifique identifiquen", "explicar": "explique expliquen", "aplicar": "aplique apliquen",
    "clasificar": "clasifique", "planificar": "planifique", "justificar": "justifique", "verificar": "verifique",
    "ejemplificar": "ejemplifique", "criticar": "critique", "practicar": "practique", "modificar": "modifique",
    "reconocer": "reconozca reconozcan", "producir": "produzca produzcan", "traducir": "traduzca",
    "conocer": "conozca conozcan", "saber": "sabe sabra sepa sepan", "entender": "entiende entienden entienda entiendan",
    "pensar": "piensa piense", "sentir": "siente sienta", "darse_cuenta": "se_da_cuenta se_den_cuenta",
    "ser_consciente": "sea_consciente sera_consciente", "elegir": "elige",
}
PROMPT_EN = {"remember": "list, define, recall", "understand": "explain, summarize, classify",
             "apply": "use, solve, implement"}
PROMPT_ES = {"remember": "enumerar, definir, recordar", "understand": "explicar, resumir, clasificar",
             "apply": "usar, resolver, aplicar"}
STOP_EN = {"the", "and", "of", "to", "will", "be", "able", "students", "student", "with", "a", "an", "in", "their", "how"}
STOP_ES = {"el", "la", "los", "las", "de", "del", "y", "que", "será", "sera", "capaz", "alumno", "alumnado", "estudiante",
           "estudiantes", "con", "en", "un", "una", "podrá", "podra", "sus", "para"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def plain(text):
    return "".join(c for c in unicodedata.normalize("NFD", text.lower()) if unicodedata.category(c) != "Mn")


def forms_en(verb):
    words = verb.split("_")
    head, tail = words[0], words[1:]
    out = {head, head + "s", head + "es", head + "ed", head + "d", head + "ing"}
    if head.endswith("e"):
        out |= {head[:-1] + "ing", head[:-1] + "ed"}
    if head.endswith("y") and head[-2:-1] not in "aeiou":
        out |= {head[:-1] + "ies", head[:-1] + "ied"}
    if re.fullmatch(r"[^aeiou]*[aeiou][^aeiouwxy]", head):
        out |= {head + head[-1] + "ed", head + head[-1] + "ing"}
    if head.endswith("ize"):
        out |= {f.replace("iz", "is", 1) for f in list(out) if "iz" in f}
    return {"_".join([f] + tail) for f in out}


def forms_es(verb):
    words = verb.split("_")
    head, tail = words[0], words[1:]
    out = {head}
    if head.endswith("se"):  # pronominal infinitive: familiarizarse -> familiarice, se familiariza
        head = head[:-2]
    stem, ending = head[:-2], head[-2:]
    if ending == "ar":
        out |= {stem + e for e in ("a", "an", "ara", "aran", "ando", "ado", "e", "en", "aremos")}
    elif ending in ("er", "ir"):
        vowel = ending[0]
        out |= {stem + e for e in ("e", "en", vowel + "ra", vowel + "ran", "iendo", "ido", "a", "an", vowel + "remos")}
    out.add(head)
    out |= set(IRREGULAR_ES.get(verb, "").split())
    return {"_".join([f] + tail) if "_" not in f else f for f in out}


def build(table, vague, forms):
    entries = {}  # base verb -> {"levels": [...], "process": str|None}
    for level in LEVELS:
        for item in table[level].split():
            verb, _, process = item.partition(":")
            entry = entries.setdefault(verb, {"levels": [], "process": None})
            if process:
                entry["process"] = process
                entry["levels"].insert(0, level)
            elif level not in entry["levels"]:
                entry["levels"].append(level)
    lookup = {}
    for verb, entry in entries.items():
        for form in forms(verb):
            lookup.setdefault(plain(form), (verb, entry))
    vague_lookup = {}
    for verb in vague.split():
        for form in forms(verb):
            vague_lookup.setdefault(plain(form), verb)
    return lookup, vague_lookup


TABLES = {"en": build(EN, VAGUE_EN, forms_en), "es": build(ES, VAGUE_ES, forms_es)}


def detect(text):
    words = re.findall(r"[^\W\d_]+", text.lower())
    es = sum(w in STOP_ES for w in words) + sum(1 for w in words if re.search(r"[áéíóúñ]", w))
    en = sum(w in STOP_EN for w in words)
    return "es" if es > en else "en"


def classify(objective, language):
    lang = detect(objective) if language == "auto" else language
    lookup, vague = TABLES[lang]
    names = LEVEL_NAMES[lang]
    tokens = re.findall(r"[a-z]+", plain(objective))
    found, vague_found, i = [], [], 0
    while i < len(tokens):
        hit = None
        for size in (3, 2, 1):
            key = "_".join(tokens[i:i + size])
            if len(tokens[i:i + size]) == size and (key in lookup or key in vague):  # the table wins over vague
                hit = key, size
                break
        if hit:
            key, size = hit
            if key not in lookup:
                vague_found.append({"verb": vague[key].replace("_", " "), "matched": key.replace("_", " "), "position": len(found) + len(vague_found)})
            else:
                verb, entry = lookup[key]
                found.append({"verb": verb.replace("_", " "), "matched": key.replace("_", " "), "levels": entry["levels"],
                              "process": entry["process"], "position": len(found) + len(vague_found)})
            i += size
        else:
            i += 1
    result = {"objective": objective, "language": lang}
    flags = []
    if vague_found and (not found or vague_found[0]["position"] < found[0]["position"]):
        flags.append("non_measurable")
    if not found:
        result.update({"verb": None, "level": None, "level_name": None, "level_number": None, "ambiguous": False,
                       "non_measurable": bool(vague_found), "process": None})
        if vague_found:
            v = vague_found[0]["verb"]
            hint = PROMPT_ES if lang == "es" else PROMPT_EN
            result["message"] = (f"'{v}' is not observable; restate with a measurable verb, e.g. "
                                 f"{hint['understand']} (Understand) or {hint['remember']} (Remember)")
        else:
            result["message"] = "no verb of the table found; start the objective with an action verb"
        result["non_measurable_verbs"] = [v["verb"] for v in vague_found]
        result["other_verbs"] = []
        return result
    main = found[0]
    level = main["levels"][0]
    result.update({
        "verb": main["verb"], "matched": main["matched"], "level": level, "level_name": names[LEVELS.index(level)],
        "level_number": LEVELS.index(level) + 1, "process": main["process"],
        "ambiguous": len(main["levels"]) > 1, "non_measurable": "non_measurable" in flags,
    })
    if len(main["levels"]) > 1:
        result["also_listed_at"] = [names[LEVELS.index(lv)] for lv in main["levels"][1:]]
        result["message"] = (f"'{main['verb']}' is listed at more than one level; check what the learner does "
                             f"with the content")
    if "non_measurable" in flags:
        result["message"] = (f"starts with the non-measurable '{vague_found[0]['verb']}'; classified by "
                             f"'{main['verb']}', the first measurable verb")
    others = [{"verb": f["verb"], "level": f["levels"][0]} for f in found[1:]]
    result["other_verbs"] = others
    result["highest_level"] = max([level] + [o["level"] for o in others], key=LEVELS.index)
    result["non_measurable_verbs"] = [v["verb"] for v in vague_found]
    return result


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"objectives", "language"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    objectives = request.get("objectives")
    if isinstance(objectives, str):
        objectives = [objectives]
    if (not isinstance(objectives, list) or not 1 <= len(objectives) <= 200
            or not all(isinstance(o, str) and o.strip() and len(o) <= 2000 for o in objectives)):
        fail("objectives must be a list of 1 to 200 non-empty strings (each at most 2000 characters)")
    language = request.get("language", "auto")
    if language not in ("en", "es", "auto"):
        fail("language must be en, es or auto")
    results = [classify(o.strip(), language) for o in objectives]
    counts = {lv: sum(r["level"] == lv for r in results) for lv in LEVELS}
    print(json.dumps({
        "results": results,
        "summary": {"by_level": counts, "unclassified": sum(r["level"] is None for r in results),
                    "ambiguous": sum(r["ambiguous"] for r in results),
                    "non_measurable": sum(r["non_measurable"] for r in results)},
        "source": "Revised Bloom taxonomy, cognitive process dimension (Anderson & Krathwohl 2001, Table 5.1); "
                  "common-usage verbs compiled for this tool",
    }, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
