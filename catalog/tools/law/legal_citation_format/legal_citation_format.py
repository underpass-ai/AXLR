#!/usr/bin/env python3
"""Format and parse legal citations: Spanish legislation in the BOE (with BOE-A identifiers and
ELI URLs), EU acts in EUR-Lex (CELEX numbers, act names, Official Journal references, ELI),
European Case Law Identifiers (ECLI, Council conclusions 2011/C 127/01) and US case citations
in Bluebook form. Not legal advice.

Input (JSON on stdin):
  parse   a citation or identifier to read: "ECLI:ES:TS:2020:1234", "32016R0679",
          "Regulation (EU) 2016/679", "BOE-A-2015-10565", "Ley 39/2015, de 1 de octubre, ...",
          "Brown v. Board of Education, 347 U.S. 483 (1954)"
  system  "boe" | "eurlex" | "ecli" | "us_case"   (optional with parse: detected)
  parts   the pieces to format a citation from (see the README for each system)
  language "en" | "es"   EUR-Lex wording (default en)
"""
import datetime as dt
import json
import re
import sys

DISCLAIMER = "Citation formatting aid, not legal advice: check the citation against the official publication."
MESES = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"]
MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"]
BOE_TYPES = {"ley": ("Ley", "l"), "ley orgánica": ("Ley Orgánica", "lo"), "ley organica": ("Ley Orgánica", "lo"),
             "real decreto": ("Real Decreto", "rd"), "real decreto-ley": ("Real Decreto-ley", "rdl"),
             "real decreto legislativo": ("Real Decreto Legislativo", "rdlg"), "orden": ("Orden", None)}
EU_TYPES = {"R": ("regulation", "reg"), "L": ("directive", "dir"), "D": ("decision", "dec")}
EU_NAMES = {"en": {"regulation": "Regulation", "directive": "Directive", "decision": "Decision"},
            "es": {"regulation": "Reglamento", "directive": "Directiva", "decision": "Decisión"}}
DOMAINS = {"EU": {"en": "EU", "es": "UE"}, "EC": {"en": "EC", "es": "CE"}, "EEC": {"en": "EEC", "es": "CEE"},
           "Euratom": {"en": "Euratom", "es": "Euratom"}}
DOMAIN_IN = {"eu": "EU", "ue": "EU", "ec": "EC", "ce": "EC", "eec": "EEC", "cee": "EEC", "euratom": "Euratom"}
CELEX_SECTORS = {"1": "treaties", "2": "international agreements", "3": "legal acts", "4": "complementary legislation",
                 "5": "preparatory acts", "6": "case law", "7": "national transposition", "8": "national case law",
                 "9": "parliamentary questions", "0": "consolidated texts", "C": "OJ C documents", "E": "EFTA documents"}
ECLI_COURTS = {("ES", "TS"): "Tribunal Supremo", ("ES", "AN"): "Audiencia Nacional", ("ES", "TC"): "Tribunal Constitucional",
               ("EU", "C"): "Court of Justice", ("EU", "T"): "General Court", ("EU", "F"): "Civil Service Tribunal"}
ECLI_COUNTRIES = set("AT BE BG CY CZ DE DK EE EL ES FI FR HR HU IE IT LT LU LV MT NL PL PT RO SE SI SK EU".split())
SCOTUS_REPORTERS = {"U.S.", "S. Ct.", "L. Ed.", "L. Ed. 2d"}
KNOWN_REPORTERS = SCOTUS_REPORTERS | {"F.", "F.2d", "F.3d", "F.4th", "F. Supp.", "F. Supp. 2d", "F. Supp. 3d", "F. App'x",
                                      "B.R.", "Fed. Cl.", "A.", "A.2d", "A.3d", "N.E.", "N.E.2d", "N.E.3d", "P.", "P.2d", "P.3d",
                                      "So.", "So. 2d", "So. 3d", "S.W.", "S.W.2d", "S.W.3d", "N.W.", "N.W.2d", "S.E.", "S.E.2d",
                                      "Cal. Rptr.", "N.Y.S.", "N.Y.S.2d", "N.Y.S.3d"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def date(value, field):
    try:
        return dt.date.fromisoformat(value)
    except (TypeError, ValueError):
        fail(f"{field} must be a date YYYY-MM-DD")


def text(parts, key, required=False):
    value = parts.get(key)
    if value is None:
        if required:
            fail(f"parts.{key} is required")
        return None
    if isinstance(value, bool) or not isinstance(value, (str, int)):
        fail(f"parts.{key} must be a string or number")
    value = str(value).strip()
    if required and not value:
        fail(f"parts.{key} is required")
    return value or None


def es_date(d, year=True):
    return f"{d.day} de {MESES[d.month - 1]}" + (f" de {d.year}" if year else "")


def boe_urls(boe_id):
    return {"boe_id": boe_id, "consolidated_url": f"https://www.boe.es/buscar/act.php?id={boe_id}",
            "original_url": f"https://www.boe.es/buscar/doc.php?id={boe_id}"}


def boe_format(parts, warnings):
    kind_in = text(parts, "type", True)
    kind = BOE_TYPES.get(kind_in.lower())
    if kind is None:
        fail("parts.type must be Ley, Ley Orgánica, Real Decreto, Real Decreto-ley, Real Decreto Legislativo or Orden")
    name, eli_code = kind
    number = text(parts, "number", True)
    m = re.fullmatch(r"(?:([A-Z]{2,4})/)?(\d{1,5})/(\d{4})", number)
    if not m:
        fail("parts.number must look like 39/2015 (or HAC/1148/2018 for an Orden)")
    adopted = date(text(parts, "date", True), "parts.date")
    if int(m.group(3)) != adopted.year:
        warnings.append(f"the number's year {m.group(3)} differs from the date's year {adopted.year}")
    title = text(parts, "title")
    citation = f"{name} {number}, de {es_date(adopted, year=False)}"
    if title:
        citation += f", {title}"
    out = {"system": "boe", "type": name, "number": number, "date": adopted.isoformat()}
    boe_number, boe_date = text(parts, "boe_number"), text(parts, "boe_date")
    if boe_number or boe_date:
        if not (boe_number and boe_date):
            fail("parts.boe_number and parts.boe_date go together")
        if not boe_number.isdigit():
            fail("parts.boe_number must be the BOE issue number")
        published = date(boe_date, "parts.boe_date")
        citation += f". «BOE» núm. {boe_number}, de {es_date(published)}"
        pages = text(parts, "pages")
        if pages:
            pm = re.fullmatch(r"(\d+)(?:\s*[-–]\s*(\d+))?", pages)
            if not pm:
                fail("parts.pages must be a page or a range like 89343-89410")
            citation += f", pp. {pm.group(1)}-{pm.group(2)}" if pm.group(2) else f", p. {pm.group(1)}"
        out["boe"] = {"number": int(boe_number), "date": published.isoformat()}
    out["citation"] = citation + "."
    if eli_code and not m.group(1):
        eli = f"https://www.boe.es/eli/es/{eli_code}/{adopted.year}/{adopted.month:02d}/{adopted.day:02d}/{m.group(2)}"
        out["eli"] = eli
        out["eli_consolidated"] = eli + "/con"
    elif eli_code is None:
        warnings.append("no ELI built for an Orden: its ELI includes the department code; take it from the BOE page")
    boe_id = text(parts, "boe_id")
    if boe_id:
        if not re.fullmatch(r"BOE-[ABT]-\d{4}-\d{1,6}", boe_id):
            fail("parts.boe_id must look like BOE-A-2015-10565")
        out.update(boe_urls(boe_id))
    return out


def boe_parse(source, warnings):
    s = source.strip()
    if re.fullmatch(r"BOE-[ABT]-\d{4}-\d{1,6}", s):
        section = {"A": "provisions and acts", "B": "announcements", "T": "Constitutional Court supplement"}[s[4]]
        out = {"system": "boe", "identifier_kind": "BOE identifier", "year": int(s[6:10]), "sequence": int(s[11:]), "series": section}
        out.update(boe_urls(s))
        return out
    m = re.match(r"(Ley Orgánica|Ley Organica|Real Decreto-ley|Real Decreto Legislativo|Real Decreto|Ley|Orden)\s+((?:[A-Z]{2,4}/)?\d{1,5}/(\d{4})),\s*de\s+(\d{1,2})\s+de\s+([a-záéíóú]+)(.*)$", s, re.I)
    if not m:
        fail("could not read a BOE citation: expected e.g. 'Ley 39/2015, de 1 de octubre, ...' or a BOE-A identifier")
    month = m.group(5).lower()
    if month not in MESES:
        fail(f"unknown month {m.group(5)!r}")
    try:
        adopted = dt.date(int(m.group(3)), MESES.index(month) + 1, int(m.group(4)))
    except ValueError:
        fail("the date in the citation does not exist")
    parts = {"type": m.group(1), "number": m.group(2), "date": adopted.isoformat()}
    rest = m.group(6)
    bm = re.search(r"«?BOE»?\s*n[úu]m\.\s*(\d+),\s*de\s+(\d{1,2})\s+de\s+([a-záéíóú]+)\s+de\s+(\d{4})(?:,\s*p(?:p|ágs?)?\.\s*(\d+(?:\s*[-–]\s*\d+)?))?", rest, re.I)
    title_part = rest[:bm.start()] if bm else rest
    title = title_part.strip().lstrip(",").strip().rstrip(".").strip()
    if title:
        parts["title"] = title
    if bm:
        if bm.group(3).lower() not in MESES:
            fail(f"unknown month {bm.group(3)!r}")
        parts["boe_number"] = bm.group(1)
        parts["boe_date"] = dt.date(int(bm.group(4)), MESES.index(bm.group(3).lower()) + 1, int(bm.group(2))).isoformat()
        if bm.group(5):
            parts["pages"] = re.sub(r"\s*[-–]\s*", "-", bm.group(5))
    out = boe_format(parts, warnings)
    out["parts"] = parts
    return out


def eu_label(kind, year, number, domain, lang):
    name = EU_NAMES[lang][kind]
    dom = DOMAINS[domain][lang]
    if year >= 2015:
        return f"{name} ({dom}) {year}/{number}"
    yy = f"{year % 100:02d}" if year < 1999 else str(year)
    if kind == "regulation":
        return f"{name} ({dom}) {'No' if lang == 'en' else 'n.º'} {number}/{yy}"
    return f"{name} {yy}/{number}/{dom}"


def default_domain(year, warnings):
    if year >= 2010:
        domain = "EU"
    elif year >= 1993:
        domain = "EC"
    else:
        domain = "EEC"
    if year < 2015:
        warnings.append(f"domain {domain} inferred from the year; give parts.domain (EU, EC, EEC, Euratom) to override")
    return domain


def eu_urls(celex, kind, year, number, lang):
    return {"celex": celex, "eurlex_url": f"https://eur-lex.europa.eu/legal-content/{lang.upper()}/TXT/?uri=CELEX:{celex}",
            "eli": f"http://data.europa.eu/eli/{EU_TYPES[{'regulation': 'R', 'directive': 'L', 'decision': 'D'}[kind]][1]}/{year}/{number}/oj"}


def eurlex_format(parts, lang, warnings):
    kind = text(parts, "act_type", True).lower()
    if kind not in ("regulation", "directive", "decision"):
        fail("parts.act_type must be regulation, directive or decision")
    try:
        year, number = int(text(parts, "year", True)), int(text(parts, "number", True))
    except ValueError:
        fail("parts.year and parts.number must be integers")
    if not (1952 <= year <= 2100) or not (0 < number < 100000):
        fail("parts.year or parts.number out of range")
    dom_in = text(parts, "domain")
    if dom_in:
        domain = DOMAIN_IN.get(dom_in.lower())
        if domain is None:
            fail("parts.domain must be EU, EC, EEC or Euratom")
    else:
        domain = default_domain(year, warnings)
    label = eu_label(kind, year, number, domain, lang)
    citation = label
    author = text(parts, "author")
    if author:
        citation += f" {author}"
    adopted = text(parts, "date")
    if adopted:
        d = date(adopted, "parts.date")
        citation += f" of {d.day} {MONTHS[d.month - 1]} {d.year}" if lang == "en" else f", de {es_date(d)},"
    title = text(parts, "title")
    if title:
        citation += f" {title}"
    citation = citation.rstrip(",")
    oj = parts.get("oj")
    if oj is not None:
        if not isinstance(oj, dict):
            fail("parts.oj must be an object")
        series = str(oj.get("series", "")).upper()
        if series not in ("L", "C"):
            fail("parts.oj.series must be L or C")
        when = date(oj.get("date"), "parts.oj.date")
        stamp = f"{when.day}.{when.month}.{when.year}"
        journal = "OJ" if lang == "en" else "DO"
        if oj.get("act"):
            if not re.fullmatch(r"\d{4}/\d{1,5}", str(oj["act"])):
                fail("parts.oj.act must look like 2023/2854")
            ref = f"{journal} {series}, {oj['act']}, {stamp}"
        else:
            num, page = oj.get("number"), oj.get("page")
            if not isinstance(num, int) or isinstance(num, bool) or not isinstance(page, int) or isinstance(page, bool):
                fail("parts.oj needs integer number and page (or act for the act-by-act Official Journal since October 2023)")
            ref = f"{journal} {series} {num}, {stamp}, p. {page}" if lang == "en" else f"{journal} {series} {num} de {stamp}, p. {page}"
        citation += f", {ref}"
        out_oj = ref
    else:
        out_oj = None
    letter = {"regulation": "R", "directive": "L", "decision": "D"}[kind]
    celex = f"3{year}{letter}{number:04d}"
    out = {"system": "eurlex", "act_type": kind, "year": year, "number": number, "domain": domain, "label": label,
           "citation": citation + "."}
    if out_oj:
        out["oj"] = out_oj
    out.update(eu_urls(celex, kind, year, number, lang))
    return out


def celex_parse(source, lang, warnings):
    m = re.fullmatch(r"([0-9CE])(\d{4})([A-Z]{1,2})(\d{4})(\(\d{2}\))?", source.strip().upper())
    if not m:
        fail("not a CELEX number: expected e.g. 32016R0679")
    sector, year, letter, number = m.group(1), int(m.group(2)), m.group(3), int(m.group(4))
    out = {"system": "eurlex", "celex": m.group(0), "sector": sector, "sector_name": CELEX_SECTORS[sector], "year": year,
           "type_letter": letter, "number": number}
    if m.group(5):
        out["suffix"] = m.group(5)
        warnings.append("the suffix marks a corrigendum or a related document")
    if sector == "3" and letter in EU_TYPES:
        kind = EU_TYPES[letter][0]
        domain = "EU" if year >= 2015 else default_domain(year, warnings)
        out.update({"act_type": kind, "domain": domain, "label": eu_label(kind, year, number, domain, lang)})
        out.update(eu_urls(m.group(0), kind, year, number, lang))
    else:
        out["eurlex_url"] = f"https://eur-lex.europa.eu/legal-content/{lang.upper()}/TXT/?uri=CELEX:{m.group(0)}"
        warnings.append("only regulations (R), directives (L) and decisions (D) of sector 3 get an act label")
    return out


def eurlex_parse(source, lang, warnings):
    s = source.strip()
    if re.fullmatch(r"[0-9CE]\d{4}[A-Za-z]{1,2}\d{4}(\(\d{2}\))?", s):
        return celex_parse(s, lang, warnings)
    m = re.search(r"\b(Regulation|Directive|Decision|Reglamento|Directiva|Decisi[oó]n)\s*(?:\((EU|EC|EEC|Euratom|UE|CE|CEE)\))?\s*(No\.?|n\.?\s?º|nº)?\s*(\d{1,4})/(\d{1,4})(?:/(EU|EC|EEC|Euratom|UE|CE|CEE))?", s, re.I)
    if not m:
        fail("could not read an EU act: expected e.g. 'Regulation (EU) 2016/679', 'Directive 95/46/EC' or a CELEX number")
    word = m.group(1).lower()
    kind = "regulation" if word.startswith("reg") else "directive" if word.startswith("dir") else "decision"
    a, b = int(m.group(4)), int(m.group(5))
    dom = m.group(2) or m.group(6)
    if m.group(3):            # "No 1049/2001": number / year
        number, year = a, b
    else:                     # "(EU) 2016/679" or "95/46/EC": year / number
        year, number = a, b
    if year < 100:
        year += 1900
    parts = {"act_type": kind, "year": year, "number": number}
    if dom:
        parts["domain"] = dom
    out = eurlex_format(parts, lang, warnings)
    om = re.search(r"\b(?:OJ|DO)\s+([LC])\s+(\d+)(?:,|\s+de)\s*(\d{1,2})\.(\d{1,2})\.(\d{4}),\s*p\.\s*(\d+)", s)
    if om:
        out["oj_parsed"] = {"series": om.group(1), "number": int(om.group(2)),
                            "date": dt.date(int(om.group(5)), int(om.group(4)), int(om.group(3))).isoformat(), "page": int(om.group(6))}
    del out["citation"]
    return out


def ecli_check(raw, warnings):
    s = raw.strip()
    if s != s.upper():
        warnings.append("ECLI is case-insensitive; capitals are the preferred form and are used here")
    s = s.upper()
    pieces = s.split(":")
    problems = []
    if len(pieces) != 5:
        problems.append(f"an ECLI has five components separated by colons, found {len(pieces)}")
        pieces += [""] * (5 - len(pieces))
    prefix, country, court, year, ordinal = pieces[:5]
    if prefix != "ECLI":
        problems.append("the first component must be 'ECLI'")
    if not re.fullmatch(r"[A-Z]{2}", country):
        problems.append("the country code must be two letters")
    elif country not in ECLI_COUNTRIES:
        warnings.append(f"{country} is not an EU Member State code or EU; other bodies have been given codes (e.g. CE for the ECtHR)")
    if country in ("GR", "GB"):
        warnings.append("ECLI uses EL for Greece and UK for the United Kingdom")
    if not re.fullmatch(r"[A-Z0-9]{1,7}", court):
        problems.append("the court code must be 1 to 7 letters or digits")
    if not re.fullmatch(r"\d{4}", year):
        problems.append("the year must be four digits")
    if not re.fullmatch(r"[A-Z0-9.]{1,25}", ordinal):
        problems.append("the ordinal number must be 1 to 25 letters, digits or dots, without other punctuation")
    out = {"system": "ecli", "ecli": s, "valid": not problems, "problems": problems,
           "components": {"prefix": prefix, "country": country, "court": court, "year": year, "ordinal": ordinal}}
    if (country, court) in ECLI_COURTS:
        out["court_name"] = ECLI_COURTS[(country, court)]
    return out


def us_format(parts, warnings):
    name = text(parts, "case_name")
    if not name:
        first, second = text(parts, "plaintiff"), text(parts, "defendant")
        if not (first and second):
            fail("parts needs case_name, or plaintiff and defendant")
        name = f"{first} v. {second}"
    volume, page = text(parts, "volume", True), text(parts, "page", True)
    if not volume.isdigit() or not page.isdigit():
        fail("parts.volume and parts.page must be numbers")
    reporter = text(parts, "reporter", True)
    year = text(parts, "year", True)
    if not re.fullmatch(r"\d{4}", year):
        fail("parts.year must be four digits")
    if reporter not in KNOWN_REPORTERS:
        warnings.append(f"reporter {reporter!r} is not in the tool's list; check its Bluebook abbreviation (table T1)")
    court = text(parts, "court")
    if reporter in SCOTUS_REPORTERS and court:
        warnings.append("the court is omitted for Supreme Court reporters (U.S., S. Ct., L. Ed.)")
        court = None
    if reporter not in SCOTUS_REPORTERS and not court:
        warnings.append("give parts.court (e.g. '9th Cir.', 'S.D.N.Y.'): the parenthetical names the court for reporters other than U.S.")
    pin = text(parts, "pincite")
    if pin and not re.fullmatch(r"\d+(?:[-–]\d+)?", pin):
        fail("parts.pincite must be a page or page range")
    tail = f", {volume} {reporter} {page}" + (f", {pin}" if pin else "") + f" ({court + ' ' if court else ''}{year})"
    return {"system": "us_case", "case_name": name, "volume": int(volume), "reporter": reporter, "page": int(page),
            "pincite": pin, "court": court, "year": int(year), "citation": name + tail, "citation_markdown": f"*{name}*" + tail}


def us_parse(source, warnings):
    m = re.fullmatch(r"\s*(?P<name>.+?),\s+(?P<vol>\d+)\s+(?P<rep>[A-Z][A-Za-z0-9.' ]*?)\s+(?P<page>\d+)(?:,\s*(?P<pin>\d+(?:[-–]\d+)?))?\s*\((?P<court>[^()]*?)\s*(?P<year>\d{4})\)\.?\s*", source)
    if not m:
        fail("could not read a US case citation: expected 'Name v. Name, 347 U.S. 483 (1954)'")
    parts = {"case_name": m.group("name").strip("*_ "), "volume": m.group("vol"), "reporter": m.group("rep").strip(),
             "page": m.group("page"), "year": m.group("year")}
    if m.group("pin"):
        parts["pincite"] = m.group("pin")
    if m.group("court").strip():
        parts["court"] = m.group("court").strip()
    out = us_format(parts, warnings)
    if " v. " in out["case_name"]:
        out["parties"] = out["case_name"].split(" v. ", 1)
    return out


def detect(source):
    s = source.strip()
    if s.upper().startswith("ECLI:"):
        return "ecli"
    if re.fullmatch(r"[0-9CE]\d{4}[A-Za-z]{1,2}\d{4}(\(\d{2}\))?", s):
        return "eurlex"
    if re.fullmatch(r"BOE-[ABT]-\d{4}-\d{1,6}", s) or "«BOE»" in s or re.match(r"(Ley|Real Decreto|Orden)\b", s):
        return "boe"
    if re.search(r"\b(Regulation|Directive|Decision|Reglamento|Directiva|Decisi[oó]n)\b", s):
        return "eurlex"
    if re.search(r",\s+\d+\s+[A-Z][A-Za-z0-9.' ]*\s+\d+.*\([^()]*\d{4}\)", s):
        return "us_case"
    fail("could not tell the citation system; give system: boe, eurlex, ecli or us_case")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    lang = request.get("language", "en")
    if lang not in ("en", "es"):
        fail("language must be en or es")
    system = request.get("system")
    if system is not None and system not in ("boe", "eurlex", "ecli", "us_case"):
        fail("system must be boe, eurlex, ecli or us_case")
    source, parts = request.get("parse"), request.get("parts")
    if (source is None) == (parts is None):
        fail("give either parse (a citation to read) or parts (pieces to format)")
    warnings = []
    if source is not None:
        if not isinstance(source, str) or not source.strip():
            fail("parse must be a non-empty string")
        system = system or detect(source)
        if system == "boe":
            out = boe_parse(source, warnings)
        elif system == "eurlex":
            out = eurlex_parse(source, lang, warnings)
        elif system == "ecli":
            out = ecli_check(source, warnings)
        else:
            out = us_parse(source, warnings)
    else:
        if not isinstance(parts, dict):
            fail("parts must be an object")
        if system is None:
            fail("system is required with parts")
        if system == "boe":
            out = boe_format(parts, warnings)
        elif system == "eurlex":
            out = eurlex_format(parts, lang, warnings)
        elif system == "ecli":
            raw = ":".join(["ECLI"] + [text(parts, k, True) for k in ("country", "court", "year", "ordinal")])
            out = ecli_check(raw, warnings)
        else:
            out = us_format(parts, warnings)
    out["warnings"] = warnings
    out["disclaimer"] = DISCLAIMER
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
