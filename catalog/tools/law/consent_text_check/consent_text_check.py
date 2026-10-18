#!/usr/bin/env python3
"""Check a privacy notice or consent text (English or Spanish, given inline) against the
information items of GDPR Article 13 (data collected from the data subject) or Article 14 (data
obtained elsewhere), by keyword heuristics per item: found, partial or missing, with the matching
snippet, and a completeness score. Heuristic, not legal advice.

Input (JSON on stdin):
  text         the notice, inline
  source       "data_subject" (Article 13, default) | "third_party" (Article 14)
  language     "en" | "es"   item labels and disclaimer (default en)
  applies      {"dpo", "legitimate_interests", "transfers", "consent", "automated_decisions", "recipients"}: false marks
               the conditional item as not applicable (excluded from the score); unknown items are checked
"""
import json
import re
import sys
import unicodedata
from decimal import ROUND_HALF_UP, Decimal
from fractions import Fraction

DISCLAIMER = {"en": "Heuristic keyword check, not legal advice: 'found' means a related word was seen, not that the information is adequate, clear or accurate.",
              "es": "Comprobación heurística por palabras clave, no es asesoramiento jurídico: 'found' indica que aparece una palabra relacionada, no que la información sea suficiente, clara o exacta."}
EMAIL = r"[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}"
P = {
    "identity": [r"data controller", r"\bcontroller\b", r"who we are", r"responsable del tratamiento", r"responsable:", r"identidad del responsable",
                 r"quienes somos", r"\btitular\b"],
    "contact": [EMAIL, r"contact", r"address", r"phone", r"telephone", r"contacto", r"direccion", r"domicilio", r"telefono", r"correo electronico"],
    "dpo": [r"data protection officer", r"\bdpo\b", r"delegad[oa] de proteccion de datos", r"\bdpd\b"],
    "purpose": [r"purposes?\b", r"we use your", r"in order to", r"finalidad", r"\bfines\b", r"para que (usamos|utilizamos)"],
    "basis": [r"legal basis", r"lawful basis", r"legal ground", r"article 6", r"art\.? ?6", r"legitimacion", r"base juridica", r"base legal", r"articulo 6",
              r"your consent", r"performance of (a|the) contract", r"legal obligation", r"su consentimiento", r"ejecucion de(l| un) contrato", r"obligacion legal"],
    "legitimate_interests": [r"legitimate interests?", r"interes(es)? legitimos?"],
    "recipients": [r"recipients?", r"share your", r"disclose", r"third parties", r"service providers", r"processors", r"destinatari", r"cesion", r"cederan",
                   r"se comunicaran", r"terceros", r"encargados del tratamiento"],
    "transfers": [r"international transfers?", r"transfer[a-z]* [a-z ,]{0,40}(third countr|outside the)", r"third countr", r"outside the (eea|european)",
                  r"adequacy decision", r"standard contractual clauses", r"transferencias? internacional", r"terceros paises", r"fuera del (espacio economico europeo|eee|ue)",
                  r"decision de adecuacion", r"clausulas contractuales tipo"],
    "retention": [r"retain", r"retention", r"stored for", r"keep your", r"kept for", r"storage period", r"as long as", r"conservar", r"conservacion", r"conservaran",
                  r"plazo de conservacion", r"durante el tiempo"],
    "withdraw": [r"withdraw (your )?consent", r"withdraw it", r"revoke", r"retirar (el|su) consentimiento", r"revocar"],
    "complaint": [r"supervisory authority", r"lodge a complaint", r"data protection authority", r"complaint (with|to)", r"autoridad de control", r"reclamacion ante",
                  r"agencia espanola de proteccion de datos", r"\baepd\b", r"\bico\b"],
    "requirement": [r"obliged to provide", r"mandatory", r"required to provide", r"statutory requirement", r"contractual requirement", r"failure to provide",
                    r"if you do not provide", r"obligatori", r"requisito (legal|contractual)", r"no (facilitar|proporcionar|aportar)", r"consecuencias de no"],
    "automated": [r"automated decision", r"profiling", r"decisiones automatizadas", r"decisiones individuales automatizadas", r"elaboracion de perfiles"],
    "categories": [r"categories of (personal )?data", r"types of (personal )?data", r"categorias de datos", r"tipos de datos"],
    "origin": [r"source", r"obtained from", r"publicly (available|accessible)", r"\borigen\b", r"procedencia", r"\bfuentes?\b", r"obtenid[oa]s? de"],
}
RIGHTS = {"access": [r"right of access", r"right to access", r"access (to )?your", r"derecho de acceso", r"\bacceso\b", r"\baccess\b"],
          "rectification": [r"rectif"],
          "erasure": [r"erasure", r"delet", r"right to be forgotten", r"supresion", r"cancelacion", r"derecho al olvido"],
          "restriction": [r"restrict", r"limitacion"],
          "portability": [r"portab"],
          "objection": [r"object to", r"right to object", r"objection", r"oposicion", r"oponerse"]}
# (ref, key, groups, applies flag, label en, label es)
ART13 = [
    ("13(1)(a)", ["identity", "contact"], None, "Identity and contact details of the controller (and its representative)", "Identidad y datos de contacto del responsable (y su representante)"),
    ("13(1)(b)", ["dpo"], "dpo", "Contact details of the data protection officer, where applicable", "Datos de contacto del delegado de protección de datos, en su caso"),
    ("13(1)(c)", ["purpose", "basis"], None, "Purposes of the processing and its legal basis", "Fines del tratamiento y base jurídica"),
    ("13(1)(d)", ["legitimate_interests"], "legitimate_interests", "Legitimate interests pursued, where based on Art. 6(1)(f)", "Intereses legítimos perseguidos, si la base es el art. 6.1.f)"),
    ("13(1)(e)", ["recipients"], "recipients", "Recipients or categories of recipients, if any", "Destinatarios o categorías de destinatarios, en su caso"),
    ("13(1)(f)", ["transfers"], "transfers", "Transfers to a third country or international organisation and the safeguards, where applicable", "Transferencias a terceros países u organizaciones internacionales y garantías, en su caso"),
    ("13(2)(a)", ["retention"], None, "Storage period, or the criteria used to determine it", "Plazo de conservación o criterios para determinarlo"),
    ("13(2)(b)", ["rights"], None, "Rights of access, rectification, erasure, restriction, objection and portability", "Derechos de acceso, rectificación, supresión, limitación, oposición y portabilidad"),
    ("13(2)(c)", ["withdraw"], "consent", "Right to withdraw consent at any time, where based on consent", "Derecho a retirar el consentimiento en cualquier momento, si la base es el consentimiento"),
    ("13(2)(d)", ["complaint"], None, "Right to lodge a complaint with a supervisory authority", "Derecho a presentar una reclamación ante una autoridad de control"),
    ("13(2)(e)", ["requirement"], None, "Whether providing the data is a statutory or contractual requirement, whether it is obligatory and the consequences of not providing it", "Si la comunicación de datos es un requisito legal o contractual, si es obligatoria y las consecuencias de no facilitarlos"),
    ("13(2)(f)", ["automated"], "automated_decisions", "Existence of automated decision-making, including profiling, with meaningful information about the logic and consequences", "Existencia de decisiones automatizadas, incluida la elaboración de perfiles, con información sobre la lógica y las consecuencias"),
]
ART14 = [
    ("14(1)(a)", ["identity", "contact"], None, ART13[0][3], ART13[0][4]),
    ("14(1)(b)", ["dpo"], "dpo", ART13[1][3], ART13[1][4]),
    ("14(1)(c)", ["purpose", "basis"], None, ART13[2][3], ART13[2][4]),
    ("14(1)(d)", ["categories"], None, "Categories of personal data concerned", "Categorías de datos personales de que se trate"),
    ("14(1)(e)", ["recipients"], "recipients", ART13[4][3], ART13[4][4]),
    ("14(1)(f)", ["transfers"], "transfers", ART13[5][3], ART13[5][4]),
    ("14(2)(a)", ["retention"], None, ART13[6][3], ART13[6][4]),
    ("14(2)(b)", ["legitimate_interests"], "legitimate_interests", ART13[3][3], ART13[3][4]),
    ("14(2)(c)", ["rights"], None, ART13[7][3], ART13[7][4]),
    ("14(2)(d)", ["withdraw"], "consent", ART13[8][3], ART13[8][4]),
    ("14(2)(e)", ["complaint"], None, ART13[9][3], ART13[9][4]),
    ("14(2)(f)", ["origin"], None, "Source of the personal data and whether it came from publicly accessible sources", "Fuente de la que proceden los datos y, en su caso, si proceden de fuentes de acceso público"),
    ("14(2)(g)", ["automated"], "automated_decisions", ART13[11][3], ART13[11][4]),
]
FLAGS = ("dpo", "legitimate_interests", "transfers", "consent", "automated_decisions", "recipients")


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


def snippet(text, start, end, width=200):
    left = max(text.rfind("\n", 0, start), text.rfind(". ", 0, start))
    left = left + 1 if left >= 0 else 0
    if start - left > 80:
        left = start - 80
    ends = [i for i in (text.find("\n", end), text.find(". ", end)) if i >= 0]
    right = min(ends) + 1 if ends else len(text)
    if right - left > width:
        right = left + width
    return " ".join(text[left:right].split())


def first(text, folded, patterns):
    best = None
    for pattern in patterns:
        m = re.search(r"(?<![a-z0-9])" + pattern, folded)
        if m and (best is None or m.start() < best.start()):
            best = m
    return best


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be the notice as a non-empty string")
    if len(text) > 2000000:
        fail("text is longer than 2,000,000 characters")
    source = request.get("source", "data_subject")
    if source not in ("data_subject", "third_party"):
        fail("source must be data_subject (Article 13) or third_party (Article 14)")
    lang = request.get("language", "en")
    if lang not in ("en", "es"):
        fail("language must be en or es")
    applies = request.get("applies", {})
    if not isinstance(applies, dict) or set(applies) - set(FLAGS) or not all(isinstance(v, bool) for v in applies.values()):
        fail("applies takes true/false for: " + ", ".join(FLAGS))
    folded = fold(text)
    spec = ART13 if source == "data_subject" else ART14
    items, by_ref = [], {}
    points = Fraction(0)
    counted = 0
    for ref, groups, flag, en, es in spec:
        row = {"ref": ref, "label": es if lang == "es" else en, "conditional": flag is not None}
        if flag and applies.get(flag) is False:
            row["status"] = "not_applicable"
        elif groups == ["rights"]:
            rights = {}
            for name, patterns in RIGHTS.items():
                m = first(text, folded, patterns)
                rights[name] = bool(m)
                if m and "snippet" not in row:
                    row["matched"], row["snippet"] = text[m.start():word_end(folded, m.end())], snippet(text, m.start(), m.end())
            row["rights"] = rights
            have = sum(rights.values())
            row["status"] = "found" if have == len(RIGHTS) else "partial" if have else "missing"
            if 0 < have < len(RIGHTS):
                row["missing_rights"] = [k for k, v in rights.items() if not v]
        else:
            hits = [first(text, folded, P[g]) for g in groups]
            found = [m for m in hits if m]
            if found:
                m = found[0]
                row["matched"], row["snippet"] = text[m.start():word_end(folded, m.end())], snippet(text, m.start(), m.end())
            row["status"] = "found" if len(found) == len(groups) else "partial" if found else "missing"
            if row["status"] == "partial":
                row["missing_parts"] = [g for g, m in zip(groups, hits) if not m]
        if row["status"] != "not_applicable":
            counted += 1
            points += {"found": 1, "partial": Fraction(1, 2), "missing": 0}[row["status"]]
        if row["status"] == "missing" and row["conditional"]:
            row["note"] = "conditional item: required only where it applies; set applies to exclude it"
        items.append(row)
        by_ref[ref] = row["status"]
    percent = Decimal(0) if not counted else (Decimal(points.numerator * 100) / Decimal(points.denominator * counted)).quantize(Decimal("0.1"), ROUND_HALF_UP)
    out = {"article": "13" if source == "data_subject" else "14", "language": lang, "items": items, "by_ref": by_ref,
           "missing": [i["ref"] for i in items if i["status"] == "missing"], "partial": [i["ref"] for i in items if i["status"] == "partial"],
           "score": {"points": str(points), "applicable_items": counted, "completeness_percent": str(percent)},
           "method": "found = 1 point, partial = 1/2 (some but not all parts or rights), missing = 0; not-applicable items are excluded",
           "heuristic_note": "Keyword heuristics in English and Spanish on an accent-free lower-case copy of the text; read the notice to confirm.",
           "disclaimer": DISCLAIMER[lang]}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
