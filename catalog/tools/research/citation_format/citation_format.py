#!/usr/bin/env python3
"""Format references in APA 7, MLA 9, Chicago author-date (17th ed.), IEEE or Vancouver from
CSL-JSON items or a BibTeX string. Output per entry: plain text and Markdown (italics as
*...*); the author-date styles are sorted by author, the numbered ones keep input order.

Input (JSON on stdin):
  style    "apa" | "mla" | "chicago" | "ieee" | "vancouver"
  items    [CSL-JSON objects]  or
  bibtex   "@article{key, author = {...}, ...}"
"""
import json
import re
import sys

STYLES = ("apa", "mla", "chicago", "ieee", "vancouver")
BIBTEX_TYPES = {"article": "article-journal", "book": "book", "incollection": "chapter", "inbook": "chapter",
                "inproceedings": "paper-conference", "conference": "paper-conference", "phdthesis": "thesis",
                "mastersthesis": "thesis", "techreport": "report", "misc": "webpage", "online": "webpage"}
BIBTEX_FIELDS = {"title": "title", "journal": "container-title", "booktitle": "container-title", "volume": "volume",
                 "number": "issue", "pages": "page", "doi": "DOI", "url": "URL", "publisher": "publisher",
                 "address": "publisher-place", "edition": "edition", "school": "publisher", "institution": "publisher"}
MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"]


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def bib_value(text, i):
    if text[i] == "{":
        depth, j = 0, i
        while j < len(text):
            depth += {"{": 1, "}": -1}.get(text[j], 0)
            if depth == 0:
                return text[i + 1:j], j + 1
            j += 1
        fail("unbalanced braces in bibtex")
    if text[i] == '"':
        j = text.index('"', i + 1)
        return text[i + 1:j], j + 1
    m = re.match(r"[A-Za-z0-9_.-]+", text[i:])
    if not m:
        fail(f"cannot read a bibtex value at: {text[i:i + 30]!r}")
    return m.group(0), i + m.end()


def bib_names(text):
    names = []
    for part in re.split(r"\s+and\s+", text.strip()):
        part = part.strip()
        if not part:
            continue
        if part.lower() == "others":
            names.append({"literal": "others"})
        elif "," in part:
            family, given = [p.strip() for p in part.split(",", 1)]
            names.append({"family": family, "given": given})
        elif " " in part:
            given, family = part.rsplit(" ", 1)
            names.append({"family": family, "given": given})
        else:
            names.append({"literal": part})
    return names


def parse_bibtex(text):
    items = []
    for m in re.finditer(r"@(\w+)\s*\{\s*([^,\s]+)\s*,", text):
        kind, key, i = m.group(1).lower(), m.group(2), m.end()
        if kind in ("comment", "string", "preamble"):
            continue
        item, fields = {"id": key, "type": BIBTEX_TYPES.get(kind, "article-journal")}, {}
        while i < len(text):
            fm = re.compile(r"\s*([A-Za-z_-]+)\s*=\s*").match(text, i)
            if not fm:
                break
            value, i = bib_value(text, fm.end())
            fields[fm.group(1).lower()] = re.sub(r"\s+", " ", value.replace("{", "").replace("}", "")).strip()
            sep = re.compile(r"\s*,?\s*").match(text, i)
            i = sep.end()
            if i < len(text) and text[i] == "}":
                break
        for bib, csl in BIBTEX_FIELDS.items():
            if bib in fields and csl not in item:
                item[csl] = fields[bib]
        for role in ("author", "editor"):
            if role in fields:
                item[role] = bib_names(fields[role])
        if "year" in fields:
            parts = [int(fields["year"])] if fields["year"].isdigit() else []
            month = fields.get("month", "")[:3].lower()
            if parts and month in [x[:3].lower() for x in MONTHS]:
                parts.append([x[:3].lower() for x in MONTHS].index(month) + 1)
            item["issued"] = {"date-parts": [parts]} if parts else {"literal": fields["year"]}
        items.append(item)
    if not items:
        fail("no bibtex entries found")
    return items


def initials(given, sep=" ", dots=True):
    out = []
    for word in given.replace(".", ". ").split():
        pieces = word.split("-")
        out.append("-".join(p[0] + ("." if dots else "") for p in pieces if p))
    return sep.join(out) if dots else "".join(out)


def year(item):
    issued = item.get("issued")
    if isinstance(issued, dict):
        parts = issued.get("date-parts", [[]])
        if parts and parts[0]:
            return str(parts[0][0])
        if issued.get("literal"):
            return str(issued["literal"])
    if item.get("year"):
        return str(item["year"])
    return None


def date_parts(item):
    issued = item.get("issued")
    if isinstance(issued, dict) and issued.get("date-parts") and issued["date-parts"][0]:
        return issued["date-parts"][0]
    return []


def pages(item, abbreviate=None):
    """The page range with an en dash; abbreviate "mla" keeps at least two digits of the
    second number (207–17), "nlm" drops every repeated leading digit and uses a hyphen (284-7)."""
    page = str(item.get("page", "")).replace("--", "-").replace("–", "-")
    if "-" not in page:
        return page
    first, last = [p.strip() for p in page.split("-", 1)]
    if abbreviate and first.isdigit() and last.isdigit() and len(first) == len(last):
        common = 0
        while common < len(last) - 1 and first[common] == last[common]:
            common += 1
        if abbreviate == "nlm":
            last = last[common:]
        elif int(first) > 99:
            last = last[min(common, len(last) - 2):]
    return f"{first}{'-' if abbreviate == 'nlm' else '–'}{last}"


def edition(item):
    raw = str(item.get("edition", "")).strip()
    if not raw:
        return ""
    if raw.isdigit():
        n = int(raw)
        suffix = "th" if 10 <= n % 100 <= 20 else {1: "st", 2: "nd", 3: "rd"}.get(n % 10, "th")
        return f"{n}{suffix} ed."
    return raw if raw.endswith(".") else raw + " ed."


def doi_url(item):
    if item.get("DOI"):
        doi = re.sub(r"^(https?://(dx\.)?doi\.org/|doi:)", "", str(item["DOI"]))
        return f"https://doi.org/{doi}"
    return item.get("URL")


def person(name, order):
    if "literal" in name:
        return name["literal"]
    family, given = name.get("family", ""), name.get("given", "")
    if order == "apa":
        return f"{family}, {initials(given)}" if given else family
    if order == "inverted":
        return f"{family}, {given}" if given else family
    if order == "ieee":
        return f"{initials(given)} {family}".strip()
    if order == "vancouver":
        return f"{family} {initials(given, dots=False)}".strip()
    return f"{given} {family}".strip()


def join_names(names, last_sep, two_sep=None):
    if len(names) == 1:
        return names[0]
    if len(names) == 2 and two_sep is not None:
        return f"{names[0]}{two_sep}{names[1]}"
    return ", ".join(names[:-1]) + last_sep + names[-1]


def authors(item, style):
    names = list(item.get("author") or [])
    if not names:
        return ""
    others = names[-1].get("literal") == "others" if names else False
    if others:
        names = names[:-1]
        listed = {"apa": [person(n, "apa") for n in names], "ieee": [person(n, "ieee") for n in names],
                  "vancouver": [person(n, "vancouver") for n in names]}.get(style)
        if style in ("mla", "chicago"):
            return person(names[0], "inverted") + ", et al"
        if style == "vancouver":
            return ", ".join(listed[:6]) + ", et al"
        return ", ".join(listed) + (", et al." if style == "apa" else " et al.")
    if style == "apa":
        listed = [person(n, "apa") for n in names]
        if len(listed) > 20:
            return ", ".join(listed[:19]) + ", . . . " + listed[-1]
        return join_names(listed, ", & ", ", & ")
    if style == "mla":
        if len(names) >= 3:
            return person(names[0], "inverted") + ", et al"
        return join_names([person(names[0], "inverted")] + [person(n, "direct") for n in names[1:]], ", and ", ", and ")
    if style == "chicago":
        listed = [person(names[0], "inverted")] + [person(n, "direct") for n in names[1:]]
        if len(listed) > 10:
            listed = listed[:7] + ["et al"]
            return ", ".join(listed)
        return join_names(listed, ", and ", " and ")
    if style == "ieee":
        listed = [person(n, "ieee") for n in names]
        if len(listed) > 6:
            return listed[0] + " et al."
        return join_names(listed, ", and ", " and ")
    listed = [person(n, "vancouver") for n in names]
    return ", ".join(listed[:6]) + (", et al" if len(listed) > 6 else "")


def end(text, mark="."):
    text = text.rstrip()
    return text if not text or text[-1] in ".?!" else text + mark


def it(text, md):
    return f"*{text}*" if md and text else text


def format_item(item, style, md, number):
    kind = item.get("type", "article-journal")
    title, container = item.get("title", ""), item.get("container-title", "")
    vol, issue, y = item.get("volume"), item.get("issue"), year(item) or "n.d."
    publisher, place, link = item.get("publisher"), item.get("publisher-place"), doi_url(item)
    who = authors(item, style)
    journal = kind in ("article-journal", "article-magazine", "article-newspaper", "article")
    chapter = kind in ("chapter", "paper-conference")
    editors = item.get("editor") or []
    if style == "apa":
        head = f"{end(who)} ({y})." if who else f"{it(title, md) if not journal else title}. ({y})."
        parts = [head]
        if who:
            ed = f" ({edition(item)})" if edition(item) and not (journal or chapter) else ""
            parts.append(end(it(title, md) + ed if not (journal or chapter) else title))
        if journal:
            src = it(container, md)
            if vol:
                src += ", " + it(str(vol), md) + (f"({issue})" if issue else "")
            if item.get("page"):
                src += ", " + pages(item)
            parts.append(end(src))
        elif chapter:
            ed = ""
            if editors:
                names = [f"{initials(n.get('given', ''))} {n.get('family', '')}".strip() if "literal" not in n else n["literal"] for n in editors]
                ed = join_names(names, ", & ", " & ") + (" (Eds.), " if len(names) > 1 else " (Ed.), ")
            pp = f" (pp. {pages(item)})" if item.get("page") else ""
            parts.append(end(f"In {ed}{it(container, md)}{pp}"))
            if publisher:
                parts.append(end(publisher))
        else:
            if publisher:
                parts.append(end(publisher))
            elif container:
                parts.append(end(container))
        text = " ".join(parts) + (f" {link}" if link else "")
    elif style == "mla":
        parts = [end(who)] if who else []
        parts.append(f"“{end(title)}”" if journal or chapter else end(it(title, md)))
        bits = []
        if container:
            bits.append(it(container, md))
        if vol:
            bits.append(f"vol. {vol}")
        if issue:
            bits.append(f"no. {issue}")
        if edition(item) and not journal:
            bits.append(edition(item))
        if publisher and not journal:
            bits.append(publisher)
        bits.append(y)
        if item.get("page"):
            bits.append(("pp. " if "-" in str(item["page"]) else "p. ") + pages(item, abbreviate="mla"))
        parts.append(end(", ".join(bits)))
        if link:
            parts.append(end(link))
        text = " ".join(parts)
    elif style == "chicago":
        parts = [end(who)] if who else []
        parts.append(end(y))
        if journal:
            parts.append(f"“{end(title)}”")
            src = it(container, md) + (f" {vol}" if vol else "") + (f" ({issue})" if issue else "")
            src += f": {pages(item)}" if item.get("page") else ""
            parts.append(end(src))
        elif chapter:
            parts.append(f"“{end(title)}”")
            parts.append(end(f"In {it(container, md)}" + (f", {pages(item)}" if item.get("page") else "")))
            if publisher:
                parts.append(end(f"{place}: {publisher}" if place else publisher))
        else:
            parts.append(end(it(title, md)))
            if edition(item):
                parts.append(end(edition(item)))
            if publisher:
                parts.append(end(f"{place}: {publisher}" if place else publisher))
        if link:
            parts.append(end(link))
        text = " ".join(parts)
    elif style == "ieee":
        prefix = f"[{number}] "
        if journal or chapter:
            bits = [f"{who}, “{title},”" if who else f"“{title},”"]
            src = (("in " + it(container, md)) if chapter else it(container, md))
            tail = [src]
            if vol:
                tail.append(f"vol. {vol}")
            if issue:
                tail.append(f"no. {issue}")
            if item.get("page"):
                tail.append(("pp. " if "-" in str(item["page"]) else "p. ") + pages(item))
            parts = date_parts(item)
            tail.append((MONTHS[parts[1] - 1][:3] + ". " if len(parts) > 1 else "") + y)
            text = prefix + " ".join(bits) + " " + ", ".join(tail)
            if link and item.get("DOI"):
                text += f", doi: {re.sub(r'^https://doi.org/', '', link)}."
            else:
                text = end(text) + (f" [Online]. Available: {link}" if link else "")
        else:
            ed = f", {edition(item)}" if edition(item) else ""
            where = f" {place}: {publisher}, {y}." if place and publisher else f" {publisher}, {y}." if publisher else f" {y}."
            text = prefix + (f"{who}, " if who else "") + it(title, md) + ed + ("" if ed else ".") + where + (f" [Online]. Available: {link}" if link else "")
    else:
        prefix = f"{number}. "
        parts = [end(who)] if who else []
        parts.append(end(title))
        if journal:
            dp = date_parts(item)
            when = y + (f" {MONTHS[dp[1] - 1][:3]}" if len(dp) > 1 else "") + (f" {dp[2]}" if len(dp) > 2 else "")
            src = f"{container}. {when}" + (f";{vol}" if vol else "") + (f"({issue})" if issue else "") + (f":{pages(item, 'nlm')}" if item.get("page") else "")
            parts.append(end(src))
        elif chapter:
            parts.append(end(f"In: {container}"))
            parts.append(end(f"{place}: {publisher}; {y}" if place and publisher else f"{publisher or ''}; {y}".lstrip("; ")))
            if item.get("page"):
                parts.append(end(f"p. {pages(item, 'nlm')}"))
        else:
            if edition(item):
                parts.append(end(edition(item)))
            parts.append(end(f"{place}: {publisher}; {y}" if place and publisher else f"{publisher}; {y}" if publisher else y))
        if item.get("DOI"):
            parts.append(f"doi:{re.sub(r'^https://doi.org/', '', link)}")
        elif link:
            parts.append(f"Available from: {link}")
        text = prefix + " ".join(parts)
    return re.sub(r"\s+", " ", text).replace("..", ".").strip()


def sort_key(item):
    names = item.get("author") or []
    first = names[0] if names else {}
    family = first.get("family") or first.get("literal") or item.get("title", "")
    return (family.lower(), year(item) or "", item.get("title", "").lower())


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    style = request.get("style", "apa")
    if style not in STYLES:
        fail(f"style must be one of {', '.join(STYLES)}")
    if isinstance(request.get("bibtex"), str):
        items = parse_bibtex(request["bibtex"])
    else:
        items = request.get("items")
        if not isinstance(items, list) or not items or not all(isinstance(i, dict) for i in items):
            fail("give items (CSL-JSON objects) or bibtex")
    ordered = sorted(items, key=sort_key) if style in ("apa", "mla", "chicago") else items
    out = []
    for n, item in enumerate(ordered, 1):
        if not item.get("title"):
            fail(f"entry {item.get('id', n)} has no title")
        out.append({"id": item.get("id"), "text": format_item(item, style, False, n), "markdown": format_item(item, style, True, n)})
    print(json.dumps({"style": style, "references": out}, ensure_ascii=False))


if __name__ == "__main__":
    main()
