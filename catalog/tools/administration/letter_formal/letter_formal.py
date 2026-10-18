#!/usr/bin/env python3
"""Formal letter skeleton in Spanish, English (UK or US), French, German, Portuguese or Italian,
in a formal, very formal or administrative register: sender and recipient blocks, the date
written the way the language writes it, subject line, the salutation that fits the recipient
(known name and gender, name only, or nobody in particular) and the closing that goes with that
salutation. The date comes from the input; the clock is never read.

Input (JSON on stdin):
  language   "es" | "en" | "fr" | "de" | "pt" | "it"
  register   "formal" (default) | "very_formal" | "administrative"
  variant    "uk" (default) | "us"   (English only)
  date       "YYYY-MM-DD";  place  optional city for the date line
  sender     {"name", "position"?, "organisation"?, "address"?: [lines], "email"?, "phone"?}
  recipient  {"name"?, "surname"?, "gender"?: "female"|"male"|"unspecified", "title"?,
              "position"?, "organisation"?, "address"?: [lines]}
  subject, reference?, body? (text or list of paragraphs), request? (what is asked), enclosures?
"""
import datetime as dt
import json
import re
import sys

MONTHS = {
    "es": ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"],
    "en": ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"],
    "fr": ["janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"],
    "de": ["Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"],
    "pt": ["janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"],
    "it": ["gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"],
}
LABELS = {
    "es": {"subject": "Asunto: ", "reference": "Referencia: ", "enclosures": "Anexos: ", "body": "[Texto de la carta]"},
    "en": {"subject": "Subject: ", "reference": "Our reference: ", "enclosures": "Enclosures: ", "body": "[Text of the letter]"},
    "fr": {"subject": "Objet : ", "reference": "Référence : ", "enclosures": "Pièces jointes : ", "body": "[Texte de la lettre]"},
    "de": {"subject": "", "reference": "Unser Zeichen: ", "enclosures": "Anlagen: ", "body": "[Text des Briefes]"},
    "pt": {"subject": "Assunto: ", "reference": "Referência: ", "enclosures": "Anexos: ", "body": "[Texto da carta]"},
    "it": {"subject": "Oggetto: ", "reference": "Riferimento: ", "enclosures": "Allegati: ", "body": "[Testo della lettera]"},
}
SIGNATURE = {"es": "[Firma]", "en": "[Signature]", "fr": "[Signature]", "de": "[Unterschrift]", "pt": "[Assinatura]", "it": "[Firma]"}
SOURCES = {
    "es": "RAE, Diccionario panhispánico de dudas and Ortografía 2010 (colon after the salutation; months in lower case); Manual de documentos administrativos (MAP, 1994) for EXPONE / SOLICITA",
    "en": "Cambridge Dictionary and Oxford Learner's Dictionaries, 'Yours faithfully' / 'Yours sincerely' (UK rule); The Gregg Reference Manual (US: colon after the salutation, 'Sincerely')",
    "fr": "service-public.fr model letters and Le Bon Usage (Grevisse): the closing repeats the salutation, 'Paris, le 1er octobre'",
    "de": "DIN 5008 and Duden, Briefe gut und richtig schreiben: 'Sehr geehrte Damen und Herren,' and 'Mit freundlichen Grüßen' without a comma",
    "pt": "Portuguese administrative usage (Exmo./Exma.; requerimento closing 'Pede deferimento'); Acordo Ortográfico 1990 (months in lower case)",
    "it": "Italian commercial correspondence usage (Gentile/Egregio, Spettabile, Cordiali/Distinti saluti)",
}


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def text(obj, key, field, limit=500):
    value = obj.get(key)
    if value is None:
        return None
    if not isinstance(value, str):
        fail(f"{field}.{key} must be a string")
    value = value.strip()
    if len(value) > limit:
        fail(f"{field}.{key} must be at most {limit} characters")
    return value or None


def lines(obj, key, field):
    value = obj.get(key)
    if value is None:
        return []
    if isinstance(value, str):
        value = value.splitlines()
    if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
        fail(f"{field}.{key} must be a list of lines")
    return [v.strip() for v in value if v.strip()]


def person(obj, field, keys):
    if obj is None:
        return {}
    if not isinstance(obj, dict):
        fail(f"{field} must be an object")
    bad = sorted(set(obj) - keys)
    if bad:
        fail(f"{field} has unknown field(s): {', '.join(bad)}")
    return obj


def date_line(lang, variant, day, place, register):
    d, m, y = day.day, MONTHS[lang][day.month - 1], day.year
    if lang == "en":
        core = f"{m} {d}, {y}" if variant == "us" else f"{d} {m} {y}"
        return f"{place}, {core}" if place and variant == "us" else core
    if lang == "es":
        core = f"{d} de {m} de {y}"
        if register == "administrative":
            return f"En {place}, a {core}." if place else f"A {core}."
        return f"{place}, {core}" if place else core
    if lang == "fr":
        core = f"le {'1er' if d == 1 else d} {m} {y}"
        return f"{place}, {core}" if place else core[0].upper() + core[1:]
    if lang == "de":
        core = f"{d}. {m} {y}"
        return f"{place}, {core}" if place else core
    if lang == "pt":
        core = f"{d} de {m} de {y}"
        return f"{place}, {core}" if place else core
    core = f"{d} {m} {y}"
    return f"{place}, {core}" if place else core


def salutation(lang, variant, register, r):
    """Returns (salutation, closing, name line for the address block, notes)."""
    name, gender, title = r.get("name"), r.get("gender", "unspecified"), r.get("title")
    surname = r.get("surname") or (name.split()[-1] if name else None)
    organisation, position = r.get("organisation"), r.get("position")
    known = bool(name)
    notes = []
    if lang == "en":
        us = variant == "us"
        dot = "." if us else ""
        honor = title or {"female": f"Ms{dot}", "male": f"Mr{dot}"}.get(gender)
        if title and us and not title.endswith(".") and title in ("Dr", "Prof", "Mr", "Ms", "Mrs"):
            honor = title + "."
        if title and not us:
            honor = title.rstrip(".") if title.rstrip(".") in ("Dr", "Prof", "Mr", "Ms", "Mrs") else title
        end = ":" if us else ","
        if not known:
            sal = f"Dear Sir or Madam{end}"
            close = ("Respectfully," if register == "very_formal" else "Sincerely,") if us else "Yours faithfully,"
            notes.append("No named recipient: 'Dear Sir or Madam' closes with 'Yours faithfully' (UK)." if not us else
                         "US usage: a colon after the salutation; 'Sincerely' whether or not the recipient is named.")
        else:
            sal = f"Dear {honor} {surname}{end}" if honor else f"Dear {name}{end}"
            close = ("Respectfully," if register == "very_formal" else "Sincerely,") if us else "Yours sincerely,"
            if not us:
                notes.append("Named recipient: 'Dear Ms/Mr Surname' closes with 'Yours sincerely' (UK); British style writes Mr, Ms, Dr without a full stop.")
            if not honor:
                notes.append("Gender not given: the full name is used instead of a title.")
        address_name = f"{honor} {name}" if known and honor else name
        return sal, close, address_name, notes
    if lang == "es":
        very = register == "very_formal"
        if register == "administrative":
            return None, None, name, ["Escrito administrativo: identificación, EXPONE (hechos) y SOLICITA (petición), lugar y fecha, firma y, al pie, el órgano al que se dirige."]
        adj = "Distinguid" if very else "Estimad"
        honor = title or {"female": "Sra.", "male": "Sr."}.get(gender)
        if not known:
            sal = f"{adj}os señores:"
            close = "Les saluda atentamente," if very else "Atentamente,"
        else:
            ending = {"female": "a", "male": "o"}.get(gender, "o/a")
            sal = f"{adj}{ending} {honor} {surname}:" if honor else f"{adj}{ending} {name}:"
            close = "Le saluda atentamente," if very else "Atentamente,"
        notes.append("En español el saludo lleva dos puntos y el texto empieza en la línea siguiente con mayúscula (RAE).")
        return sal, close, (f"{honor} {name}" if known and honor else name), notes
    if lang == "fr":
        if known and gender in ("female", "male"):
            appellation = "Madame" if gender == "female" else "Monsieur"
        else:
            appellation = "Madame, Monsieur"
        if register == "very_formal" and position and appellation != "Madame, Monsieur":
            appellation = f"{appellation} {position}"
        sal = f"{appellation},"
        if register == "very_formal":
            close = f"Je vous prie de croire, {appellation}, à l'assurance de ma haute considération."
        elif register == "administrative":
            close = f"Je vous prie d'agréer, {appellation}, l'expression de mes salutations distinguées."
        else:
            close = f"Veuillez agréer, {appellation}, l'expression de mes salutations distinguées."
        notes.append("En français, l'appel ne porte pas le nom de famille et la formule finale reprend l'appel.")
        address = (("Madame " if gender == "female" else "Monsieur " if gender == "male" else "") + name) if known else None
        return sal, close, address, notes
    if lang == "de":
        t = f"{title} " if title else ""
        if not known:
            sal = "Sehr geehrte Damen und Herren,"
        elif gender == "female":
            sal = f"Sehr geehrte Frau {t}{surname},"
        elif gender == "male":
            sal = f"Sehr geehrter Herr {t}{surname},"
        else:
            sal = f"Guten Tag {t}{name},"
            notes.append("Geschlecht unbekannt: 'Guten Tag' mit vollem Namen.")
        close = "Mit vorzüglicher Hochachtung" if register == "very_formal" else "Mit freundlichen Grüßen"
        if register == "very_formal":
            notes.append("'Mit vorzüglicher Hochachtung' ist hohen Amtsträgern vorbehalten; sonst 'Mit freundlichen Grüßen'.")
        notes.append("Nach der Anrede mit Komma beginnt der Text klein (außer Substantive); nach dem Gruß steht kein Komma (DIN 5008).")
        prefix = {"female": "Frau ", "male": "Herrn "}.get(gender, "")
        return sal, close, (f"{prefix}{t}{name}" if known else None), notes
    if lang == "pt":
        honor = {"female": "Exma. Senhora", "male": "Exmo. Senhor"}.get(gender, "Exmo.(a) Senhor(a)")
        if title:
            honor = f"{honor} {title}"
        sal = f"{honor} {name}," if known else "Exmos. Senhores,"
        if register == "administrative":
            close = "Pede deferimento."
            notes.append("Requerimento: termina com 'Pede deferimento', data e assinatura.")
        elif register == "very_formal":
            close = "Com os meus respeitosos cumprimentos,"
        else:
            close = "Com os melhores cumprimentos,"
        return sal, close, (f"{honor} {name}" if known else None), notes
    # Italian
    very = register in ("very_formal", "administrative")
    if not known:
        sal = f"Spettabile {organisation}," if organisation else "Gentili Signori,"
    else:
        if title:
            honor = title
        else:
            honor = {"female": "Signora", "male": "Signor"}.get(gender)
        if gender == "male":
            adj = "Egregio" if very else "Gentile"
        elif gender == "female":
            adj = "Gentilissima" if very else "Gentile"
        else:
            adj = "Gentile"
        sal = f"{adj} {honor} {surname}," if honor else f"{adj} {name},"
    close = "Distinti saluti" if very else "Cordiali saluti"
    address = None
    if known:
        address = {"female": "Sig.ra ", "male": "Sig. "}.get(gender, "") + name if not title else f"{title} {name}"
    return sal, close, address, notes


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"language", "register", "variant", "date", "place", "sender", "recipient", "subject", "reference", "body", "request", "enclosures"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    lang = request.get("language")
    if lang not in MONTHS:
        fail("language must be es, en, fr, de, pt or it")
    register = request.get("register", "formal")
    if register not in ("formal", "very_formal", "administrative"):
        fail("register must be formal, very_formal or administrative")
    variant = request.get("variant", "uk")
    if variant not in ("uk", "us"):
        fail("variant must be uk or us")
    if variant == "us" and lang != "en":
        fail("variant us applies only to language en")
    raw_date = request.get("date")
    if not isinstance(raw_date, str) or not re.match(r"^\d{4}-\d{2}-\d{2}$", raw_date):
        fail("date is required as YYYY-MM-DD (the tool does not read the clock)")
    try:
        day = dt.date.fromisoformat(raw_date)
    except ValueError as error:
        fail(f"date is not valid: {error}")
    place = text(request, "place", "input", 100)
    sender = person(request.get("sender"), "sender", {"name", "position", "organisation", "address", "email", "phone"})
    recipient = person(request.get("recipient"), "recipient", {"name", "surname", "gender", "title", "position", "organisation", "address"})
    r = {k: text(recipient, k, "recipient") for k in ("name", "surname", "title", "position", "organisation")}
    gender = recipient.get("gender", "unspecified")
    if gender not in ("female", "male", "unspecified"):
        fail("recipient.gender must be female, male or unspecified")
    r["gender"] = gender
    if r["surname"] and not r["name"]:
        fail("recipient.surname needs recipient.name")
    subject = text(request, "subject", "input", 300)
    reference = text(request, "reference", "input", 100)
    body = request.get("body")
    if body is None:
        paragraphs = [LABELS[lang]["body"]]
    elif isinstance(body, str):
        paragraphs = [p.strip() for p in re.split(r"\n\s*\n", body) if p.strip()] or [LABELS[lang]["body"]]
    elif isinstance(body, list) and all(isinstance(p, str) for p in body):
        paragraphs = [p.strip() for p in body if p.strip()] or [LABELS[lang]["body"]]
    else:
        fail("body must be a string or a list of paragraphs")
    ask = text(request, "request", "input", 5000)
    enclosures = request.get("enclosures")
    if enclosures is not None and (not isinstance(enclosures, list) or not all(isinstance(e, str) for e in enclosures)):
        fail("enclosures must be a list of strings")

    sal, close, address_name, notes = salutation(lang, variant, register, r)
    s_name = text(sender, "name", "sender")
    s_block = [x for x in [s_name, text(sender, "position", "sender"), text(sender, "organisation", "sender")] if x]
    s_block += lines(sender, "address", "sender")
    s_block += [x for x in [text(sender, "email", "sender"), text(sender, "phone", "sender")] if x]
    position_line = r["position"]
    if lang == "fr" and position_line:
        position_line = re.sub(r"^(le|la|l')\s*", "", position_line, flags=re.I)
        position_line = position_line[:1].upper() + position_line[1:]
    r_block = [x for x in [address_name, position_line, r["organisation"]] if x]
    r_block += lines(recipient, "address", "recipient")
    when = date_line(lang, variant, day, place, register)
    labels = LABELS[lang]
    out = []

    if lang == "es" and register == "administrative":
        ident = s_name or "[Nombre y apellidos]"
        ident_line = ident + ", con domicilio en " + (", ".join(lines(sender, "address", "sender")) or "[domicilio]")
        if text(sender, "email", "sender"):
            ident_line += " y correo electrónico " + text(sender, "email", "sender")
        out += [ident_line + ","]
        if subject:
            out += ["", labels["subject"] + subject]
        if reference:
            out += [labels["reference"] + reference]
        out += ["", "EXPONE:", ""] + [f"{i + 1}. {p}" for i, p in enumerate(paragraphs)]
        out += ["", "SOLICITA:", "", ask or "[Lo que se pide]"]
        if enclosures:
            out += ["", labels["enclosures"] + "; ".join(enclosures)]
        out += ["", when, "", "[Firma]", "", ident]
        organ = " ".join(x for x in [r["position"], r["organisation"]] if x)
        out += ["", ("SR./SRA. " + organ).upper() if organ else "[ÓRGANO AL QUE SE DIRIGE]"]
        result_sal, result_close = None, None
    else:
        if lang == "en" and variant == "uk":
            out += s_block + ([""] if s_block else []) + [when, ""] + r_block + ([""] if r_block else [])
        else:
            out += s_block + ([""] if s_block else []) + r_block + ([""] if r_block else []) + [when, ""]
        if reference:
            out += [labels["reference"] + reference]
        if subject:
            out += [labels["subject"] + subject, ""]
        elif reference:
            out += [""]
        out += [sal, ""]
        body_paragraphs = list(paragraphs) + ([ask] if ask else [])
        for p in body_paragraphs:
            out += [p, ""]
        out += [close, "", SIGNATURE[lang]]
        out += [x for x in [s_name, text(sender, "position", "sender")] if x]
        if enclosures:
            out += ["", labels["enclosures"] + "; ".join(enclosures)]
        result_sal, result_close = sal, close
    if lang == "de" and subject:
        notes.append("DIN 5008: Betreff ohne das Wort 'Betreff', fett gesetzt.")
    letter = "\n".join(out).strip() + "\n"
    result = {"language": lang, "register": register, **({"variant": variant} if lang == "en" else {}),
              "date_line": when, "salutation": result_sal, "closing": result_close,
              "subject_line": (labels["subject"] + subject) if subject else None,
              "letter": letter, "notes": notes, "source": SOURCES[lang]}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
