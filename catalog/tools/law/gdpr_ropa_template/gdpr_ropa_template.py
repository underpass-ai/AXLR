#!/usr/bin/env python3
"""GDPR Article 30 record of processing activities: the skeleton for a controller (30(1)(a)-(g))
or a processor (30(2)(a)-(d)), filled with what the input gives, with the fields still missing
(mandatory, or "where applicable" / "where possible"), as JSON and as a Markdown table, in English
or Spanish. Not legal advice.

Input (JSON on stdin):
  role          "controller" | "processor"
  language      "en" | "es"   (default en)
  organisation  {"name", "contact", "representative", "dpo", "joint_controller"}
  activities    [{"name": "Payroll", "purposes": [...], "data_subjects": [...], "data_categories": [...],
                  "recipients": [...], "transfers": "...", "erasure_time_limits": "...", "security_measures": "...",
                  "legal_basis": "..."}]                                  controller
                [{"name": "...", "controller": "name and contact", "processing_categories": [...],
                  "transfers": "...", "security_measures": "..."}]        processor
  Values are a string or a list of strings; write "none" or "not applicable" to record that an item does not apply.
"""
import json
import sys

DISCLAIMER = {"en": "Template filled from the input, not legal advice: the record must reflect the actual processing and be kept up to date.",
              "es": "Plantilla rellenada con los datos facilitados, no es asesoramiento jurídico: el registro debe reflejar el tratamiento real y mantenerse actualizado."}
TEXT = {
    "controller": {
        "en": [("30(1)(a)", "the name and contact details of the controller and, where applicable, the joint controller, the controller's representative and the data protection officer"),
               ("30(1)(b)", "the purposes of the processing"),
               ("30(1)(c)", "a description of the categories of data subjects and of the categories of personal data"),
               ("30(1)(d)", "the categories of recipients to whom the personal data have been or will be disclosed including recipients in third countries or international organisations"),
               ("30(1)(e)", "where applicable, transfers of personal data to a third country or an international organisation, including the identification of that third country or international organisation and, in the case of transfers referred to in the second subparagraph of Article 49(1), the documentation of suitable safeguards"),
               ("30(1)(f)", "where possible, the envisaged time limits for erasure of the different categories of data"),
               ("30(1)(g)", "where possible, a general description of the technical and organisational security measures referred to in Article 32(1)")],
        "es": [("30(1)(a)", "el nombre y los datos de contacto del responsable y, en su caso, del corresponsable, del representante del responsable, y del delegado de protección de datos"),
               ("30(1)(b)", "los fines del tratamiento"),
               ("30(1)(c)", "una descripción de las categorías de interesados y de las categorías de datos personales"),
               ("30(1)(d)", "las categorías de destinatarios a quienes se comunicaron o comunicarán los datos personales, incluidos los destinatarios en terceros países u organizaciones internacionales"),
               ("30(1)(e)", "en su caso, las transferencias de datos personales a un tercer país o una organización internacional, incluida la identificación de dicho tercer país u organización internacional y, en el caso de las transferencias indicadas en el artículo 49, apartado 1, párrafo segundo, la documentación de garantías adecuadas"),
               ("30(1)(f)", "cuando sea posible, los plazos previstos para la supresión de las diferentes categorías de datos"),
               ("30(1)(g)", "cuando sea posible, una descripción general de las medidas técnicas y organizativas de seguridad a que se refiere el artículo 32, apartado 1")],
    },
    "processor": {
        "en": [("30(2)(a)", "the name and contact details of the processor or processors and of each controller on behalf of which the processor is acting, and, where applicable, of the controller's or the processor's representative, and the data protection officer"),
               ("30(2)(b)", "the categories of processing carried out on behalf of each controller"),
               ("30(2)(c)", "where applicable, transfers of personal data to a third country or an international organisation, including the identification of that third country or international organisation and, in the case of transfers referred to in the second subparagraph of Article 49(1), the documentation of suitable safeguards"),
               ("30(2)(d)", "where possible, a general description of the technical and organisational security measures referred to in Article 32(1)")],
        "es": [("30(2)(a)", "el nombre y los datos de contacto del encargado o encargados y de cada responsable por cuenta del cual actúe el encargado, y, en su caso, del representante del responsable o del encargado, y del delegado de protección de datos"),
               ("30(2)(b)", "las categorías de tratamientos efectuados por cuenta de cada responsable"),
               ("30(2)(c)", "en su caso, las transferencias de datos personales a un tercer país u organización internacional, incluida la identificación de dicho tercer país u organización internacional y, en el caso de las transferencias indicadas en el artículo 49, apartado 1, párrafo segundo, la documentación de garantías adecuadas"),
               ("30(2)(d)", "cuando sea posible, una descripción general de las medidas técnicas y organizativas de seguridad a que se refiere el artículo 32, apartado 1")],
    },
}
# field: (item, requirement, label en, label es); "org." fields come from organisation
FIELDS = {
    "controller": [("org.name", "30(1)(a)", "mandatory", "Controller", "Responsable"),
                   ("org.contact", "30(1)(a)", "mandatory", "Controller contact details", "Datos de contacto del responsable"),
                   ("joint_controller", "30(1)(a)", "where_applicable", "Joint controller", "Corresponsable"),
                   ("org.representative", "30(1)(a)", "where_applicable", "Representative", "Representante"),
                   ("org.dpo", "30(1)(a)", "where_applicable", "Data protection officer", "Delegado de protección de datos"),
                   ("purposes", "30(1)(b)", "mandatory", "Purposes", "Fines"),
                   ("data_subjects", "30(1)(c)", "mandatory", "Categories of data subjects", "Categorías de interesados"),
                   ("data_categories", "30(1)(c)", "mandatory", "Categories of personal data", "Categorías de datos personales"),
                   ("recipients", "30(1)(d)", "mandatory", "Categories of recipients", "Categorías de destinatarios"),
                   ("transfers", "30(1)(e)", "where_applicable", "Transfers to third countries or international organisations", "Transferencias internacionales"),
                   ("erasure_time_limits", "30(1)(f)", "where_possible", "Time limits for erasure", "Plazos de supresión"),
                   ("security_measures", "30(1)(g)", "where_possible", "Technical and organisational security measures", "Medidas técnicas y organizativas de seguridad")],
    "processor": [("org.name", "30(2)(a)", "mandatory", "Processor", "Encargado"),
                  ("org.contact", "30(2)(a)", "mandatory", "Processor contact details", "Datos de contacto del encargado"),
                  ("controller", "30(2)(a)", "mandatory", "Controller on whose behalf (name and contact)", "Responsable por cuenta del cual se actúa (nombre y contacto)"),
                  ("org.representative", "30(2)(a)", "where_applicable", "Representative", "Representante"),
                  ("org.dpo", "30(2)(a)", "where_applicable", "Data protection officer", "Delegado de protección de datos"),
                  ("processing_categories", "30(2)(b)", "mandatory", "Categories of processing", "Categorías de tratamientos"),
                  ("transfers", "30(2)(c)", "where_applicable", "Transfers to third countries or international organisations", "Transferencias internacionales"),
                  ("security_measures", "30(2)(d)", "where_possible", "Technical and organisational security measures", "Medidas técnicas y organizativas de seguridad")],
}
EXTRA = {"legal_basis": ("Legal basis (Art. 6, and Art. 9 for special categories; not in Art. 30, recommended)",
                         "Base jurídica (art. 6 y, en su caso, art. 9; no exigida por el art. 30, recomendada)")}
HEAD = {"controller": {"en": ["Controller and contacts", "Purposes", "Data subjects and personal data", "Recipients", "Transfers", "Erasure time limits", "Security measures"],
                       "es": ["Responsable y contactos", "Fines", "Interesados y datos personales", "Destinatarios", "Transferencias", "Plazos de supresión", "Medidas de seguridad"]},
        "processor": {"en": ["Processor, controller and contacts", "Categories of processing", "Transfers", "Security measures"],
                      "es": ["Encargado, responsable y contactos", "Categorías de tratamientos", "Transferencias", "Medidas de seguridad"]}}
ORG_KEYS = {"name", "contact", "representative", "dpo", "joint_controller"}
ACTIVITY_KEYS = {"controller": {"name", "joint_controller", "purposes", "data_subjects", "data_categories", "recipients", "transfers",
                                "erasure_time_limits", "security_measures", "legal_basis", "notes"},
                 "processor": {"name", "controller", "processing_categories", "transfers", "security_measures", "notes"}}
WORDS = {"en": {"activity": "Processing activity", "missing": "**MISSING**", "conditional": "_(where applicable / possible)_", "title": "Record of processing activities (GDPR Art. 30)"},
         "es": {"activity": "Actividad de tratamiento", "missing": "**FALTA**", "conditional": "_(en su caso / cuando sea posible)_", "title": "Registro de actividades de tratamiento (RGPD art. 30)"}}
NOTES = {"en": ["The record is kept in writing, including in electronic form (Art. 30(3)), and made available to the supervisory authority on request (Art. 30(4)).",
                "Art. 30(5): organisations with fewer than 250 employees are exempt unless the processing is likely to result in a risk, is not occasional, or includes special categories (Art. 9(1)) or criminal-offence data (Art. 10).",
                "Spain: public bodies listed in LOPDGDD art. 77.1 must publish an inventory of their processing activities with the Art. 30 information and the legal basis (LOPDGDD art. 31.2)."],
         "es": ["El registro consta por escrito, inclusive en formato electrónico (art. 30.3), y se pone a disposición de la autoridad de control que lo solicite (art. 30.4).",
                "Art. 30.5: las organizaciones con menos de 250 empleados están exentas salvo que el tratamiento pueda entrañar un riesgo, no sea ocasional o incluya categorías especiales (art. 9.1) o datos penales (art. 10).",
                "España: las entidades del art. 77.1 de la LOPDGDD deben publicar un inventario de sus actividades de tratamiento con la información del art. 30 y su base legal (LOPDGDD art. 31.2)."]}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def value(raw, field):
    if raw is None:
        return None
    if isinstance(raw, str):
        return raw.strip() or None
    if isinstance(raw, list) and all(isinstance(x, str) for x in raw):
        items = [x.strip() for x in raw if x.strip()]
        return items or None
    fail(f"{field} must be a string or a list of strings")


def cell(v):
    if isinstance(v, list):
        v = "; ".join(v)
    return v.replace("\\", "\\\\").replace("|", "\\|").replace("\r", " ").replace("\n", "<br>")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    role = request.get("role")
    if role not in FIELDS:
        fail("role must be controller or processor")
    lang = request.get("language", "en")
    if lang not in ("en", "es"):
        fail("language must be en or es")
    org = request.get("organisation", {})
    if not isinstance(org, dict):
        fail("organisation must be an object")
    if set(org) - ORG_KEYS:
        fail(f"organisation has unknown keys: {', '.join(sorted(set(org) - ORG_KEYS))}")
    org = {k: value(v, f"organisation.{k}") for k, v in org.items()}
    activities = request.get("activities", [])
    if not isinstance(activities, list):
        fail("activities must be a list")
    if not activities:
        activities = [{}]
    lab = 4 if lang == "es" else 3
    rows, missing = [], []
    uses_extra = False
    for i, act in enumerate(activities):
        if not isinstance(act, dict):
            fail(f"activities[{i}] must be an object")
        unknown = set(act) - ACTIVITY_KEYS[role]
        if unknown:
            fail(f"activities[{i}] has unknown keys for a {role}: {', '.join(sorted(unknown))}")
        name = value(act.get("name"), f"activities[{i}].name")
        if isinstance(name, list):
            name = ", ".join(name)
        name = name or f"#{i + 1}"
        fields = {}
        for spec in FIELDS[role]:
            key, item, need = spec[0], spec[1], spec[2]
            if key.startswith("org."):
                v = org.get(key[4:])
            elif key == "joint_controller":
                v = value(act.get(key), f"activities[{i}].{key}") or org.get("joint_controller")
            else:
                v = value(act.get(key), f"activities[{i}].{key}")
            fields[key.replace("org.", "")] = {"item": item, "label": spec[lab], "requirement": need, "value": v}
            if v is None:
                missing.append({"activity": name, "item": item, "field": key.replace("org.", "organisation."), "requirement": need})
        if role == "controller":
            lb = value(act.get("legal_basis"), f"activities[{i}].legal_basis")
            if lb is not None:
                uses_extra = True
                fields["legal_basis"] = {"item": None, "label": EXTRA["legal_basis"][lang == "es"], "requirement": "recommended", "value": lb}
        notes = value(act.get("notes"), f"activities[{i}].notes")
        rows.append({"activity": name, "fields": fields, **({"notes": notes} if notes else {})})
    # Markdown: one row per activity, one column per Art. 30 item.
    items = TEXT[role][lang]
    w = WORDS[lang]
    header = [w["activity"]] + [f"{ref} {HEAD[role][lang][n]}" for n, (ref, _) in enumerate(items)]
    if uses_extra:
        header.append(EXTRA["legal_basis"][lang == "es"].split(" (")[0])
    lines = [f"# {w['title']}", "", "| " + " | ".join(cell(h) for h in header) + " |", "|" + "---|" * len(header)]
    for row in rows:
        cells = [cell(row["activity"])]
        for ref, _ in items:
            fields = [f for f in row["fields"].values() if f["item"] == ref]
            parts = []
            for f in fields:
                if f["value"] is not None:
                    parts.append(cell(f["value"]) if len(fields) == 1 else f"{f['label']}: {cell(f['value'])}")
                elif f["requirement"] == "mandatory":
                    parts.append(w["missing"] if len(fields) == 1 else f"{f['label']}: {w['missing']}")
            cells.append("<br>".join(parts) if parts else w["conditional"])
        if uses_extra:
            lb = row["fields"].get("legal_basis")
            cells.append(cell(lb["value"]) if lb else "")
        lines.append("| " + " | ".join(cells) + " |")
    mandatory = [m for m in missing if m["requirement"] == "mandatory"]
    conditional = [m for m in missing if m["requirement"] != "mandatory"]
    out = {"role": role, "language": lang, "article": "30(1)" if role == "controller" else "30(2)",
           "items": [{"ref": ref, "text": t} for ref, t in items],
           "record": rows, "missing_mandatory": mandatory, "missing_conditional": conditional,
           "complete": not mandatory, "markdown": "\n".join(lines) + "\n", "notes": NOTES[lang], "disclaimer": DISCLAIMER[lang]}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
