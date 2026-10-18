#!/usr/bin/env python3
"""Metadata for a DOI from the Crossref REST API, normalised to CSL-JSON-like fields, with an
APA 7 reference.

Input (JSON on stdin):
  doi       "10.1037/ppm0000185" (also "doi:...", "https://doi.org/...")
  mailto    optional contact e-mail for Crossref's polite pool (query string and User-Agent)
  response  optional: the exact body Crossref returned for this DOI (offline or cached use);
            when given, no request is made
One request per call: GET https://api.crossref.org/works/{doi}, timeout 15 s.
"""
import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

API = "https://api.crossref.org/works/"
UA = "AXLR-catalog-tool/1.0 (+https://axlr.underpassai.com)"
TIMEOUT = 15
DOI_RE = re.compile(r"^10\.\d{4,9}/\S+$")
MAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[^@\s]+$")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def normalise_doi(value):
    if not isinstance(value, str) or not value.strip():
        fail("doi must be a non-empty string")
    doi = value.strip()
    doi = re.sub(r"^(https?://(dx\.)?doi\.org/|doi:\s*)", "", doi, flags=re.I)
    if not DOI_RE.match(doi):
        fail("doi must look like 10.NNNN/suffix")
    return doi


def fetch(doi, mailto):
    url = API + urllib.parse.quote(doi, safe="/")
    agent = UA
    if mailto:
        url += "?" + urllib.parse.urlencode({"mailto": mailto})
        agent += f" mailto:{mailto}"
    if not url.startswith("https://"):
        fail("only HTTPS is allowed")
    request = urllib.request.Request(url, headers={"User-Agent": agent, "Accept": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=TIMEOUT) as reply:
            return url, reply.read().decode("utf-8")
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", "replace")
        if error.code == 404:
            return url, body
        fail(f"Crossref answered HTTP {error.code}: {body.strip()[:200]}")
    except (urllib.error.URLError, OSError, ValueError) as error:
        fail(f"request to Crossref failed: {error}")


def first(values):
    if isinstance(values, list):
        return values[0] if values else None
    return values


def clean(text):
    if not isinstance(text, str):
        return None
    text = re.sub(r"<[^>]+>", "", text)  # JATS markup such as <i>…</i> in titles
    return re.sub(r"\s+", " ", text).strip() or None


def date_parts(work):
    for key in ("issued", "published", "published-print", "published-online", "created"):
        parts = (work.get(key) or {}).get("date-parts") or []
        if parts and parts[0] and parts[0][0] is not None:
            return [int(p) for p in parts[0] if p is not None]
    return None


def initials(given):
    out = []
    for word in given.split():
        pieces = [p.strip(".") for p in word.split("-") if p.strip(".")]
        if pieces:
            out.append("-".join(p[0].upper() + "." for p in pieces))
    return " ".join(out)


def apa_author(person):
    if person.get("literal"):
        return person["literal"]
    family, given = person.get("family", ""), person.get("given", "")
    if not given:
        return family
    return f"{family}, {initials(given)}"


def apa_authors(authors):
    names = [apa_author(a) for a in authors if apa_author(a)]
    if not names:
        return ""
    if len(names) == 1:
        return names[0]
    if len(names) <= 20:
        return ", ".join(names[:-1]) + ", & " + names[-1]
    return ", ".join(names[:19]) + ", . . . " + names[-1]


def end_with_period(text):
    return text if text.endswith((".", "?", "!")) else text + "."


def apa(csl):
    authors = apa_authors(csl["author"])
    year = csl["issued"]["date-parts"][0][0] if csl["issued"] else "n.d."
    title = csl["title"] or "[Untitled]"
    head = f"{end_with_period(authors)} ({year}). " if authors else f"{end_with_period(title)} ({year}). "
    plain = html = head
    if authors:
        plain += end_with_period(title) + " "
        html += end_with_period(title) + " "
    container = csl["container-title"]
    if container:
        tail_plain, tail_html = container, f"<i>{container}</i>"
        if csl["volume"]:
            tail_plain += f", {csl['volume']}"
            tail_html += f", <i>{csl['volume']}</i>"
        if csl["issue"]:
            tail_plain += f"({csl['issue']})"
            tail_html += f"({csl['issue']})"
        if csl["page"]:
            tail_plain += f", {csl['page']}"
            tail_html += f", {csl['page']}"
        plain += tail_plain + ". "
        html += tail_html + ". "
    elif csl["publisher"] and csl["type"] in ("book", "monograph", "report", "edited-book"):
        plain += csl["publisher"] + ". "
        html += csl["publisher"] + ". "
    link = f"https://doi.org/{csl['DOI']}"
    return (plain + link).strip(), (html + link).strip()


CSL_TYPES = {"journal-article": "article-journal", "proceedings-article": "paper-conference",
             "book-chapter": "chapter", "posted-content": "article", "dataset": "dataset",
             "book": "book", "edited-book": "book", "monograph": "book", "report": "report",
             "dissertation": "thesis", "reference-entry": "entry"}


def parse(text, doi):
    try:
        payload = json.loads(text)
    except ValueError:
        if "not found" in text.lower():
            fail(f"DOI not found in Crossref: {doi}")
        fail("response is not Crossref JSON: " + text.strip()[:120])
    if not isinstance(payload, dict) or payload.get("status") != "ok" or not isinstance(payload.get("message"), dict):
        fail("Crossref response has no work (status is not ok)")
    work = payload["message"]
    authors = []
    for person in work.get("author") or []:
        entry = {}
        if person.get("family"):
            entry["family"] = person["family"]
        if person.get("given"):
            entry["given"] = person["given"]
        if person.get("name") and not entry:
            entry["literal"] = person["name"]
        if person.get("ORCID"):
            entry["ORCID"] = person["ORCID"]
        if entry:
            authors.append(entry)
    parts = date_parts(work)
    page = work.get("page")
    if isinstance(page, str):
        page = re.sub(r"\s*[-‐‑‒–—]\s*", "–", page.strip())
    title = clean(first(work.get("title")))
    subtitle = clean(first(work.get("subtitle")))
    if title and subtitle and subtitle.lower() not in title.lower():
        title = f"{title.rstrip('.')}: {subtitle}"
    csl = {
        "type": CSL_TYPES.get(work.get("type"), work.get("type")),
        "crossref_type": work.get("type"),
        "title": title,
        "author": authors,
        "container-title": clean(first(work.get("container-title"))),
        "issued": {"date-parts": [parts]} if parts else None,
        "volume": work.get("volume"),
        "issue": work.get("issue"),
        "page": page,
        "DOI": work.get("DOI", doi),
        "URL": work.get("URL") or f"https://doi.org/{doi}",
        "publisher": work.get("publisher"),
        "ISSN": work.get("ISSN") or [],
    }
    plain, html = apa(csl)
    return {"doi": csl["DOI"], "csl": csl, "apa": plain, "apa_html": html}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    try:
        json.dumps(request, ensure_ascii=False).encode("utf-8")
    except UnicodeEncodeError:
        fail("input contains an unpaired surrogate (not valid Unicode text)")
    unknown = set(request) - {"doi", "mailto", "response"}
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    doi = normalise_doi(request.get("doi"))
    mailto = request.get("mailto")
    if mailto is not None and (not isinstance(mailto, str) or not MAIL_RE.match(mailto)):
        fail("mailto must be an e-mail address")
    response = request.get("response")
    if response is not None:
        if not isinstance(response, str) or not response.strip():
            fail("response must be the non-empty text Crossref returned")
        source = {"api": "Crossref REST API", "url": API + urllib.parse.quote(doi, safe="/"), "fetched": False}
        text = response
    else:
        url, text = fetch(doi, mailto)
        source = {"api": "Crossref REST API", "url": url, "fetched": True}
    result = parse(text, doi)
    result["source"] = source
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
