#!/usr/bin/env python3
"""DPIA screening under GDPR Article 35: the nine criteria of the Article 29 Working Party
guidelines on DPIA (WP248 rev.01, endorsed by the EDPB), the "two or more criteria" rule of
thumb, the mandatory cases of Article 35(3), the Recital 91 exception and a note on the
supervisory authority's Article 35(4) list. Criteria are given as booleans or hinted from a
description by keywords (English and Spanish). Not legal advice.

Input (JSON on stdin):
  criteria     {"evaluation_scoring": true, "automated_decision": false, "systematic_monitoring": ...,
                "sensitive_data": ..., "large_scale": ..., "matching_combining": ..., "vulnerable_subjects": ...,
                "innovative_technology": ..., "prevents_right_or_service": ...}
  art35_3      {"a": bool, "b": bool, "c": bool}
  description  free text; keyword hints count for criteria not set explicitly
  recital_91   true for personal data of patients or clients processed by an individual physician,
               other health care professional or lawyer
  country      ISO code of the lead or competent supervisory authority, for the national-list note
"""
import json
import re
import sys
import unicodedata

DISCLAIMER = ("Screening aid, not legal advice: WP248's criteria are indicators, the controller must assess the specific processing, "
              "and the competent supervisory authority's Article 35(4) list may require a DPIA in other cases.")
CRITERIA = [
    ("evaluation_scoring", "Evaluation or scoring, including profiling and predicting",
     [r"scor", r"profil", r"credit rating", r"evaluat", r"predict", r"puntuacion", r"perfil", r"solvencia", r"prediccion", r"evaluacion"]),
    ("automated_decision", "Automated decision-making with legal or similar significant effect",
     [r"automated decision", r"decided automatically", r"automatic(ally)? (refus|reject|den)", r"decision(es)? automatizad", r"decisiones individuales automatizadas"]),
    ("systematic_monitoring", "Systematic monitoring",
     [r"monitor", r"surveillance", r"cctv", r"camera", r"video", r"tracking", r"geolocat", r"vigilancia", r"videovigilancia", r"camara", r"monitoriz", r"seguimiento", r"geolocaliz"]),
    ("sensitive_data", "Sensitive data or data of a highly personal nature",
     [r"health", r"medical", r"genetic", r"biometric", r"religio", r"ethnic", r"sexual", r"political", r"trade union", r"criminal", r"financial", r"location data",
      r"salud", r"medic", r"genetic", r"biometric", r"religi", r"etnic", r"politic", r"sindical", r"penal", r"financier", r"datos de localizacion"]),
    ("large_scale", "Data processed on a large scale",
     [r"large[- ]scale", r"nationwide", r"national level", r"millions?", r"gran escala", r"nivel nacional", r"millones", r"masiv"]),
    ("matching_combining", "Matching or combining datasets",
     [r"combin", r"matching", r"merg", r"cross-referenc", r"enrich", r"data broker", r"cruce", r"cruzar", r"combinacion", r"enriquec"]),
    ("vulnerable_subjects", "Data concerning vulnerable data subjects",
     [r"employee", r"staff", r"child", r"minors?\b", r"patient", r"elderly", r"asylum", r"mentally ill", r"pupil", r"student", r"empleado", r"trabajador",
      r"plantilla", r"menores", r"ninos", r"paciente", r"ancian", r"personas mayores", r"solicitantes de asilo", r"alumn"]),
    ("innovative_technology", "Innovative use or applying new technological or organisational solutions",
     [r"artificial intelligence", r"\bai\b", r"machine learning", r"facial recognition", r"fingerprint", r"internet of things", r"\biot\b", r"plate recognition",
      r"inteligencia artificial", r"\bia\b", r"aprendizaje automatico", r"reconocimiento facial", r"huella", r"internet de las cosas", r"reconocimiento de matriculas"]),
    ("prevents_right_or_service", "Processing that prevents data subjects from exercising a right or using a service or a contract",
     [r"\bcredit\b", r"\bloan", r"insurance", r"deny access", r"refus", r"eligib", r"blacklist", r"fraud database", r"credito", r"prestamo", r"\bseguro",
      r"denegar", r"excluir", r"morosos", r"fraude"]),
]
ART35_3 = {
    "a": "a systematic and extensive evaluation of personal aspects relating to natural persons which is based on automated processing, including profiling, and on which decisions are based that produce legal effects concerning the natural person or similarly significantly affect the natural person",
    "b": "processing on a large scale of special categories of data referred to in Article 9(1), or of personal data relating to criminal convictions and offences referred to in Article 10",
    "c": "a systematic monitoring of a publicly accessible area on a large scale",
}
AUTHORITIES = {"es": "Agencia Española de Protección de Datos (AEPD)",
               "fr": "Commission nationale de l'informatique et des libertés (CNIL)", "de": "Datenschutzkonferenz (DSK) and the Land authorities",
               "it": "Garante per la protezione dei dati personali", "ie": "Data Protection Commission (DPC)", "nl": "Autoriteit Persoonsgegevens",
               "pt": "Comissão Nacional de Proteção de Dados (CNPD)", "be": "Autorité de protection des données / Gegevensbeschermingsautoriteit"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fold(text):
    out = []
    for ch in text:
        base = unicodedata.normalize("NFD", ch)[0]
        low = base.lower()
        out.append(low if len(low) == 1 else base)
    return "".join(out)


def word_end(text, i):
    while i < len(text) and text[i].isalpha():
        i += 1
    return i


def bools(obj, keys, field):
    if obj is None:
        return {}
    if not isinstance(obj, dict):
        fail(f"{field} must be an object of true/false values")
    unknown = set(obj) - set(keys)
    if unknown:
        fail(f"{field} has unknown keys: {', '.join(sorted(unknown))}; known: {', '.join(keys)}")
    for k, v in obj.items():
        if not isinstance(v, bool):
            fail(f"{field}.{k} must be true or false")
    return obj


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    keys = [c[0] for c in CRITERIA]
    explicit = bools(request.get("criteria"), keys, "criteria")
    art = bools(request.get("art35_3"), ["a", "b", "c"], "art35_3")
    description = request.get("description")
    if description is not None and (not isinstance(description, str) or not description.strip()):
        fail("description must be a non-empty string")
    recital_91 = request.get("recital_91", False)
    if not isinstance(recital_91, bool):
        fail("recital_91 must be true or false")
    country = request.get("country")
    if country is not None and (not isinstance(country, str) or not re.fullmatch(r"[A-Za-z]{2}", country)):
        fail("country must be a two-letter code")
    if not explicit and not art and description is None:
        fail("give criteria, art35_3 or a description to screen")
    folded = fold(description) if description else ""
    rows, met = [], []
    for key, title, patterns in CRITERIA:
        hints = sorted({description[m.start():word_end(folded, m.end())] for p in patterns for m in re.finditer(r"(?<![a-z0-9])" + p, folded)}) if description else []
        row = {"id": key, "criterion": title, "hint_keywords": hints[:8]}
        if key in explicit:
            row["met"], row["source"] = explicit[key], "input"
        elif hints:
            row["met"], row["source"] = True, "description (keyword hint: confirm)"
        else:
            row["met"], row["source"] = False, "not given"
        if row["met"]:
            met.append(key)
        rows.append(row)
    art_met = [k for k in "abc" if art.get(k)]
    possible = []
    if not art_met:
        if "evaluation_scoring" in met and "automated_decision" in met:
            possible.append("a")
        if "sensitive_data" in met and "large_scale" in met:
            possible.append("b")
        if "systematic_monitoring" in met and "large_scale" in met:
            possible.append("c")
    count = len(met)
    if art_met:
        verdict = "required"
        reason = "the processing falls under Article 35(3)(" + "), (".join(art_met) + "), where a DPIA is always required"
    elif recital_91:
        verdict = "not_mandatory_recital_91"
        reason = ("Recital 91: processing of patients' or clients' data by an individual physician, other health care professional or lawyer "
                  "is not considered large scale, and a DPIA should not be mandatory (WP248 gives this as an example of no DPIA)")
    elif count >= 2:
        verdict = "likely_required"
        reason = f"{count} WP248 criteria are met: in most cases a processing meeting two criteria requires a DPIA"
    elif count == 1:
        verdict = "not_likely_required"
        reason = ("one WP248 criterion is met: a DPIA is usually not needed, but WP248 notes that in some cases a processing meeting only "
                  "one criterion requires one; assess and document")
    else:
        verdict = "not_indicated"
        reason = "no WP248 criterion is met"
    out = {"verdict": verdict, "reason": reason, "criteria_met": met, "criteria_count": count, "criteria": rows,
           "art35_3_met": art_met, "art35_3": [{"point": k, "text": v, "met": k in art_met} for k, v in ART35_3.items()],
           "rule": "WP248 rev.01: the more criteria are met, the more likely a high risk; in most cases two or more criteria mean a DPIA is required. "
                   "If the controller concludes that a processing meeting two or more criteria is not high risk, it should document the reasons.",
           "notes": ["Article 35(1): a DPIA is required where a type of processing, in particular using new technologies, is likely to result in a high risk "
                     "to the rights and freedoms of natural persons; the criteria are indicators of that risk.",
                     "Article 35(4) and (5): supervisory authorities publish lists of processing that requires a DPIA (and may publish lists that do not); "
                     "check the competent authority's list: a processing on its Article 35(4) list requires a DPIA whatever the criteria count."]}
    if possible:
        out["art35_3_possible"] = possible
        out["notes"].append("The criteria met suggest checking Article 35(3)(" + "), (".join(possible) + "): if it applies, a DPIA is mandatory. "
                            "WP248's 'sensitive data' is wider than Article 9 special categories.")
    if description:
        out["notes"].append("Criteria marked as keyword hints come from the description by a heuristic word search; confirm or override them in criteria.")
    if country:
        cc = country.lower()
        out["national_list"] = {"authority": AUTHORITIES.get(cc, f"the supervisory authority of {country.upper()}"),
                                "note": "check its Article 35(4) list of processing operations that require a DPIA, and any Article 35(5) list of those that do not"}
    out["disclaimer"] = DISCLAIMER
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
