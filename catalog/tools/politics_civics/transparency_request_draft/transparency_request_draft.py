#!/usr/bin/env python3
"""Draft an access-to-information request: Spain (Ley 19/2013), the EU institutions
(Regulation 1049/2001), the US federal government (FOIA, 5 U.S.C. 552) or the UK (FOIA 2000).
The draft carries the elements each law requires, the legal basis, where to submit it, the
response time counted from the submission date, and what to do if there is no answer.
Not legal advice.

Input (JSON on stdin):
  jurisdiction  "es" | "eu" | "us" | "uk"
  authority     the public body, e.g. "Ministerio de Hacienda"
  requester     {"name": "...", "email": "...", "address": "..."}   name and email or address required
  information   "what is requested", or a list of items
  format        "electronic" (default) | "paper" | "inspection"
  date          "YYYY-MM-DD" submission date, for the response deadline (optional)
  fee_category  US only: "news_media" | "educational" | "commercial" | "other" (default other)
  fee_waiver    US only: true to ask for one (give fee_waiver_reason)
"""
import calendar
import datetime as dt
import json
import sys

DISCLAIMER = {"es": "Borrador orientativo, no es asesoramiento jurídico: compruebe la norma vigente y los requisitos del organismo.",
              "en": "Draft for guidance only, not legal advice: check the law in force and the authority's own requirements."}

LAWS = {
    "es": {"language": "es", "law": "Ley 19/2013, de 9 de diciembre, de transparencia, acceso a la información pública y buen gobierno",
           "submit_via": "Portal de la Transparencia de la Administración General del Estado (transparencia.gob.es), la sede electrónica del organismo o su registro; para comunidades autónomas y entidades locales, su propio portal",
           "deadline": ("months", 1), "deadline_text": "un mes desde la recepción por el órgano competente, ampliable un mes más (art. 20.1)",
           "next_steps": ["Si no hay respuesta en plazo, la solicitud se entiende desestimada (silencio negativo, art. 20.4).",
                          "Contra la resolución expresa o presunta cabe reclamación ante el Consejo de Transparencia y Buen Gobierno o el órgano autonómico equivalente en el plazo de un mes (art. 24), previa al recurso contencioso-administrativo.",
                          "Si se deniega en parte, la resolución debe motivar el límite aplicado (arts. 14, 15 y 20.2) y conceder el acceso parcial cuando sea posible (art. 16)."]},
    "eu": {"language": "en", "law": "Regulation (EC) No 1049/2001 regarding public access to European Parliament, Council and Commission documents",
           "submit_via": "the institution's access-to-documents service (for the Commission, its online portal for access to documents), or in writing by post or e-mail",
           "deadline": ("working_days", 15), "deadline_text": "15 working days from registration, extendable by 15 in exceptional cases (Article 7(1) and 7(3))",
           "next_steps": ["If access is refused in whole or in part, or there is no reply in time, make a confirmatory application within 15 working days (Article 7(2) and 7(4)).",
                          "A refusal of the confirmatory application can be challenged before the General Court or by a complaint to the European Ombudsman (Article 8(1))."]},
    "us": {"language": "en", "law": "Freedom of Information Act, 5 U.S.C. § 552",
           "submit_via": "the agency's FOIA office or portal, or the National FOIA Portal (foia.gov)",
           "deadline": ("working_days", 20), "deadline_text": "20 working days to determine whether to comply, extendable by 10 in unusual circumstances (5 U.S.C. § 552(a)(6)(A)(i) and (B)(i))",
           "next_steps": ["Agencies must give a tracking number for requests taking more than 10 days (§ 552(a)(7)).",
                          "An adverse determination can be appealed to the agency head within the period the agency states (at least 90 days); the Office of Government Information Services (OGIS) offers mediation.",
                          "After an appeal is denied, or if the deadlines pass, the requester may sue in federal district court (§ 552(a)(4)(B))."]},
    "uk": {"language": "en", "law": "Freedom of Information Act 2000",
           "submit_via": "the public authority's FOI contact (in writing: letter, e-mail or web form); in Scotland, the Freedom of Information (Scotland) Act 2002 applies to Scottish authorities instead",
           "deadline": ("working_days", 20), "deadline_text": "promptly and in any event within 20 working days (section 10(1))",
           "next_steps": ["If refused, ask the authority for an internal review, normally within 40 working days of its response.",
                          "After the internal review, complain to the Information Commissioner's Office (section 50)."]},
}
FORMATS = {"es": {"electronic": "por medios electrónicos, a la dirección de correo indicada", "paper": "en papel, en la dirección postal indicada", "inspection": "mediante consulta presencial"},
           "en": {"electronic": "in electronic form, to the e-mail address above", "paper": "on paper, to the postal address above", "inspection": "by inspection in person"}}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def deadline(start, kind, amount):
    if kind == "months":
        month_index = start.month - 1 + amount
        year, month = start.year + month_index // 12, month_index % 12 + 1
        return dt.date(year, month, min(start.day, calendar.monthrange(year, month)[1]))
    current, left = start, amount
    while left:
        current += dt.timedelta(1)
        if current.isoweekday() < 6:
            left -= 1
    return current


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    code = request.get("jurisdiction")
    if code not in LAWS:
        fail("jurisdiction must be es, eu, us or uk")
    law, lang = LAWS[code], LAWS[code]["language"]
    authority = request.get("authority")
    requester = request.get("requester")
    info = request.get("information")
    if not isinstance(authority, str) or not authority.strip():
        fail("authority must name the public body")
    if not isinstance(requester, dict) or not requester.get("name") or not (requester.get("email") or requester.get("address")):
        fail("requester needs a name and an email or postal address")
    if isinstance(info, str):
        items = [info.strip()] if info.strip() else []
    elif isinstance(info, list):
        items = [str(i).strip() for i in info if str(i).strip()]
    else:
        items = []
    if not items:
        fail("information must describe what is requested")
    fmt = request.get("format", "electronic")
    if fmt not in FORMATS["en"]:
        fail("format must be electronic, paper or inspection")
    name, email, address = requester["name"], requester.get("email"), requester.get("address")
    contact = ", ".join(x for x in (address, email) if x)
    if fmt == "electronic" and not email:
        FORMATS["es"]["electronic"], FORMATS["en"]["electronic"] = "por medios electrónicos", "in electronic form"
    listing = "\n".join(f"{n}. {item}" for n, item in enumerate(items, 1))
    if lang == "es":
        subject = "Solicitud de acceso a información pública (Ley 19/2013)"
        body = (f"A: {authority}\n\n{name}, con dirección a efectos de notificaciones en {contact}, al amparo del artículo 105.b) de la Constitución "
                f"y de los artículos 12 y 17 de la Ley 19/2013, de 9 de diciembre, de transparencia, acceso a la información pública y buen gobierno,\n\n"
                f"SOLICITA el acceso a la siguiente información pública:\n\n{listing}\n\n"
                f"Modalidad de acceso preferida: {FORMATS['es'][fmt]} (art. 17.2.d).\n\n"
                "De conformidad con el artículo 17.3, el solicitante no está obligado a motivar la solicitud. Si alguna parte de la información estuviera "
                "afectada por un límite legal, se solicita el acceso parcial previsto en el artículo 16 y la indicación de la parte omitida.\n\n"
                f"Firmado: {name}\nFecha: {request.get('date', '[fecha]')}")
    else:
        subject = {"eu": "Request for access to documents under Regulation (EC) No 1049/2001", "us": "Freedom of Information Act request",
                   "uk": "Request for information under the Freedom of Information Act 2000"}[code]
        basis = {"eu": "Under Article 2 and Article 6 of Regulation (EC) No 1049/2001, I request access to the following documents",
                 "us": "Under the Freedom of Information Act, 5 U.S.C. § 552, I request copies of the following records",
                 "uk": "Under section 1 of the Freedom of Information Act 2000, I request the following information"}[code]
        extra = ""
        if code == "eu":
            extra = "\n\nUnder Article 6(1) I am not obliged to state reasons. If any part is covered by an exception in Article 4, please release the remainder (Article 4(6))."
        elif code == "us":
            category = request.get("fee_category", "other")
            if category not in ("news_media", "educational", "commercial", "other"):
                fail("fee_category must be news_media, educational, commercial or other")
            extra = (f"\n\nFor fee purposes I am a requester in the {category.replace('_', ' ')} category. Please notify me before incurring fees above $25.")
            if request.get("fee_waiver"):
                reason = request.get("fee_waiver_reason")
                if not isinstance(reason, str) or not reason.strip():
                    fail("fee_waiver needs fee_waiver_reason: why disclosure is in the public interest")
                extra += f"\n\nI request a fee waiver under 5 U.S.C. § 552(a)(4)(A)(iii): {reason.strip()}"
            extra += "\n\nIf any portion is withheld, please release all reasonably segregable portions and cite the exemption for each withholding (§ 552(b))."
        else:
            extra = "\n\nIf any of the information is exempt, please provide the rest and say which exemption applies (section 17)."
        preference = f"I would like to receive the information {FORMATS['en'][fmt]}" + (" (section 11)." if code == "uk" else ".")
        body = (f"To: {authority}\n\n{basis}:\n\n{listing}\n\n{preference}{extra}"
                f"\n\nYours faithfully,\n{name}\n{contact}\nDate: {request.get('date', '[date]')}")
    result = {"jurisdiction": code, "language": lang, "law": law["law"], "subject": subject, "body": body,
              "submit_via": law["submit_via"], "response_time": law["deadline_text"], "next_steps": law["next_steps"]}
    if "date" in request:
        try:
            start = dt.date.fromisoformat(request["date"])
        except (TypeError, ValueError):
            fail("date must be YYYY-MM-DD")
        kind, amount = law["deadline"]
        result["response_due"] = deadline(start, kind, amount).isoformat()
        result["response_due_note"] = "counted from the given date, weekends excluded for working days; public holidays not excluded" if lang == "en" else "contado desde la fecha indicada; el plazo corre desde la entrada en el órgano competente"
    result["disclaimer"] = DISCLAIMER[lang]
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
