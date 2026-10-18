#!/usr/bin/env python3
"""Parse, deduplicate, re-key and check a BibTeX file, and write it back clean.

Input (JSON on stdin):
  bibtex       the .bib text
  operations   any of "dedupe", "normalize_keys", "check", applied in that order;
               default all three; [] only parses and rewrites
  merge        on dedupe, copy fields the kept entry lacks from its duplicates (default true)
  entries      include the parsed entries as JSON in the output (default true)

Parsing follows BibTeX: entries @type{key, field = value, ...} or @type(key, ...), values in
braces (nested), in double quotes, bare numbers or @string macros, joined with #; the twelve
month macros jan..dec are predefined; @comment and text outside entries are ignored;
@preamble is kept. Required fields are those of btxdoc ("BibTeXing", Patashnik 1988).
"""
import json
import re
import sys
import unicodedata

MAX_TEXT = 2_000_000
MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September",
          "October", "November", "December"]
MONTH_MACROS = {m[:3].lower(): m for m in MONTHS}
# btxdoc, section 3.1 ("Entry types"): required fields; a tuple means "one of these"
REQUIRED = {
    "article": ["author", "title", "journal", "year"],
    "book": [("author", "editor"), "title", "publisher", "year"],
    "booklet": ["title"],
    "conference": ["author", "title", "booktitle", "year"],
    "inbook": [("author", "editor"), "title", ("chapter", "pages"), "publisher", "year"],
    "incollection": ["author", "title", "booktitle", "publisher", "year"],
    "inproceedings": ["author", "title", "booktitle", "year"],
    "manual": ["title"],
    "mastersthesis": ["author", "title", "school", "year"],
    "misc": [],
    "phdthesis": ["author", "title", "school", "year"],
    "proceedings": ["title", "year"],
    "techreport": ["author", "title", "institution", "year"],
    "unpublished": ["author", "title", "note"],
}
STOPWORDS = {"a", "an", "the", "on", "of", "in", "for", "and", "to", "with", "at", "by", "from", "into", "is",
             "are", "toward", "towards", "via", "as", "or", "its", "how", "what", "why", "when", "do", "does"}
SPECIAL = {"ss": "ss", "o": "o", "O": "O", "ae": "ae", "AE": "AE", "oe": "oe", "OE": "OE", "aa": "a", "AA": "A",
           "l": "l", "L": "L", "i": "i", "j": "j"}
OPEN_CLOSE = {"{": "}", "(": ")"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


class BibError(Exception):
    pass


class Scanner:
    def __init__(self, text):
        self.text, self.pos = text, 0

    def line(self, pos=None):
        return self.text.count("\n", 0, self.pos if pos is None else pos) + 1

    def skip_ws(self):
        while self.pos < len(self.text) and self.text[self.pos].isspace():
            self.pos += 1

    def peek(self):
        return self.text[self.pos] if self.pos < len(self.text) else ""

    def ident(self):
        m = re.compile(r"[^\s\"#%'(),={}]+").match(self.text, self.pos)
        if not m:
            raise BibError(f"expected a name at line {self.line()}")
        self.pos = m.end()
        return m.group()

    def braced(self):
        """At '{': return the content up to the matching '}'."""
        depth, start = 0, self.pos
        while self.pos < len(self.text):
            ch = self.text[self.pos]
            if ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    self.pos += 1
                    return self.text[start + 1:self.pos - 1]
            self.pos += 1
        raise BibError(f"unbalanced braces from line {self.line(start)}")

    def quoted(self):
        """At '"': the content up to the next '"' outside braces."""
        depth, start = 0, self.pos
        self.pos += 1
        while self.pos < len(self.text):
            ch = self.text[self.pos]
            if ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth < 0:
                    raise BibError(f"unbalanced braces in a quoted value at line {self.line(start)}")
            elif ch == '"' and depth == 0:
                self.pos += 1
                return self.text[start + 1:self.pos - 1]
            self.pos += 1
        raise BibError(f"unterminated quoted value from line {self.line(start)}")

    def value(self, strings, warnings):
        parts = []
        while True:
            self.skip_ws()
            ch = self.peek()
            if ch == "{":
                parts.append(self.braced())
            elif ch == '"':
                parts.append(self.quoted())
            elif ch.isdigit():
                m = re.compile(r"\d+").match(self.text, self.pos)
                self.pos = m.end()
                parts.append(m.group())
            elif ch and ch not in ",}){(#=":
                name = self.ident()
                low = name.lower()
                if low in strings:
                    parts.append(strings[low])
                elif low in MONTH_MACROS:
                    parts.append(MONTH_MACROS[low])
                else:
                    warnings.append(f"line {self.line()}: undefined macro '{name}' kept as text")
                    parts.append(name)
            else:
                raise BibError(f"expected a value at line {self.line()}")
            self.skip_ws()
            if self.peek() == "#":
                self.pos += 1
                continue
            return "".join(parts)


def parse(text):
    sc = Scanner(text)
    strings, entries, preambles, warnings = {}, [], [], []
    comments = 0
    while True:
        at = text.find("@", sc.pos)
        if at < 0:
            break
        sc.pos = at + 1
        start_line = sc.line(at)
        try:
            sc.skip_ws()
            kind = sc.ident().lower()
            sc.skip_ws()
            opener = sc.peek()
            if opener not in OPEN_CLOSE:
                raise BibError(f"expected '{{' or '(' after @{kind} at line {start_line}")
            closer = OPEN_CLOSE[opener]
            if kind == "comment":
                if opener == "{":
                    sc.braced()
                else:
                    end = text.find(")", sc.pos)
                    sc.pos = len(text) if end < 0 else end + 1
                comments += 1
                continue
            sc.pos += 1
            if kind == "preamble":
                preambles.append(sc.value(strings, warnings))
                sc.skip_ws()
                if sc.peek() != closer:
                    raise BibError(f"expected '{closer}' closing @preamble at line {sc.line()}")
                sc.pos += 1
                continue
            if kind == "string":
                sc.skip_ws()
                name = sc.ident().lower()
                sc.skip_ws()
                if sc.peek() != "=":
                    raise BibError(f"expected '=' in @string at line {sc.line()}")
                sc.pos += 1
                strings[name] = sc.value(strings, warnings)
                sc.skip_ws()
                if sc.peek() != closer:
                    raise BibError(f"expected '{closer}' closing @string at line {sc.line()}")
                sc.pos += 1
                continue
            sc.skip_ws()
            m = re.compile(r"[^,\s{}()]*").match(text, sc.pos)
            key = m.group()
            sc.pos = m.end()
            sc.skip_ws()
            fields = {}
            if sc.peek() == ",":
                sc.pos += 1
            elif sc.peek() != closer:
                raise BibError(f"expected ',' after the key of @{kind} at line {start_line}")
            while True:
                sc.skip_ws()
                if sc.peek() == closer:
                    sc.pos += 1
                    break
                if sc.peek() == "":
                    raise BibError(f"@{kind}{{{key} at line {start_line} is not closed")
                name = sc.ident().lower()
                sc.skip_ws()
                if sc.peek() != "=":
                    raise BibError(f"expected '=' after field '{name}' at line {sc.line()}")
                sc.pos += 1
                value = sc.value(strings, warnings)
                if name in fields:
                    warnings.append(f"line {start_line}: {key}: repeated field '{name}', the first is kept")
                else:
                    fields[name] = value
                sc.skip_ws()
                if sc.peek() == ",":
                    sc.pos += 1
                elif sc.peek() != closer:
                    raise BibError(f"expected ',' or '{closer}' after field '{name}' at line {sc.line()}")
            if not key:
                warnings.append(f"line {start_line}: @{kind} without a key")
            entries.append({"type": kind, "key": key, "fields": fields, "line": start_line})
        except BibError as error:
            warnings.append(f"skipped an entry: {error}")
            # resume after the failed '@'
            sc.pos = at + 1
    return entries, strings, preambles, comments, warnings


def plain(value):
    """LaTeX-free ASCII: accents and special letters folded, commands and braces dropped."""
    s = re.sub(r"\\[\'\"`^~=.]\s*\{?\s*\\?([A-Za-z])\}?", r"\1", value)
    s = re.sub(r"\\[uvHckbdrt](?:\s*\{\s*\\?([A-Za-z])\s*\}|\s+([A-Za-z]))", lambda m: m.group(1) or m.group(2), s)
    s = re.sub(r"\\([A-Za-z]+)\b\s*", lambda m: SPECIAL.get(m.group(1), " "), s)
    s = s.replace("{", "").replace("}", "").replace("~", " ")
    s = unicodedata.normalize("NFKD", s)
    return "".join(ch for ch in s if not unicodedata.combining(ch)).encode("ascii", "ignore").decode()


def split_top(text, separator):
    """Split on a regex separator at brace depth 0."""
    out, depth, start, i = [], 0, 0, 0
    pattern = re.compile(separator, re.I)
    while i < len(text):
        ch = text[i]
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
        elif depth == 0:
            m = pattern.match(text, i)
            if m and m.end() > i:
                out.append(text[start:i])
                start = i = m.end()
                continue
        i += 1
    out.append(text[start:])
    return [part.strip() for part in out if part.strip()]


def last_name(name):
    """The last name of a BibTeX name, without the von part ('van Beethoven, L.' -> 'Beethoven')."""
    parts = split_top(name, r",")
    if len(parts) > 1:
        words = split_top(parts[0], r"\s+")
    else:
        words = split_top(name, r"\s+")
        if len(words) > 1:
            von = [i for i, w in enumerate(words[:-1]) if w[:1].islower()]
            words = words[von[-1] + 1:] if von else words[-1:]
    kept = [w for w in words if not w[:1].islower()] or words
    return re.sub(r"[^a-z0-9]", "", plain(" ".join(kept)).lower())


def base_key(entry):
    fields = entry["fields"]
    people = fields.get("author") or fields.get("editor") or ""
    names = split_top(people, r"\s+and\s+")
    who = last_name(names[0]) if names else ""
    year = re.search(r"\d{4}", fields.get("year", ""))
    words = [w for w in re.split(r"[^a-z0-9]+", plain(fields.get("title", "")).lower()) if w]
    first = next((w for w in words if w not in STOPWORDS), words[0] if words else "")
    return (who or "anon") + (year.group() if year else "nd") + first


def suffix(n):
    letters = ""
    n += 1
    while n:
        n, r = divmod(n - 1, 26)
        letters = chr(97 + r) + letters
    return letters


def norm_doi(value):
    v = value.strip().lower()
    v = re.sub(r"^(https?://)?(dx\.)?doi\.org/", "", v)
    v = re.sub(r"^doi:\s*", "", v)
    return v.rstrip(".,; ")


def norm_title(value):
    return re.sub(r"[^a-z0-9]", "", plain(value).lower())


def dedupe(entries, merge):
    kept, by_doi, by_title, removed = [], {}, {}, []
    for entry in entries:
        f = entry["fields"]
        doi = norm_doi(f["doi"]) if f.get("doi", "").strip() else None
        title = norm_title(f.get("title", ""))
        year = re.search(r"\d{4}", f.get("year", ""))
        tkey = (title, year.group() if year else "") if title else None
        match, reason = None, None
        if doi and doi in by_doi:
            match, reason = by_doi[doi], f"same DOI {doi}"
        elif tkey and tkey in by_title:
            other = by_title[tkey]
            other_doi = norm_doi(other["fields"]["doi"]) if other["fields"].get("doi", "").strip() else None
            if not (doi and other_doi and doi != other_doi):
                match, reason = other, "same normalised title and year"
        if match is None:
            kept.append(entry)
            if doi:
                by_doi[doi] = entry
            if tkey:
                by_title.setdefault(tkey, entry)
            continue
        merged = []
        if merge:
            for name, value in f.items():
                if not match["fields"].get(name, "").strip() and value.strip():
                    match["fields"][name] = value
                    merged.append(name)
            if doi and doi not in by_doi:
                by_doi[doi] = match
        removed.append({"kept": match["key"], "removed": entry["key"], "line": entry["line"], "reason": reason,
                        "fields_merged": merged})
    return kept, removed


def normalize_keys(entries):
    bases = [base_key(e) for e in entries]
    groups = {}
    for i, b in enumerate(bases):
        groups.setdefault(b, []).append(i)
    taken = set(b for b, members in groups.items() if len(members) == 1)
    changes = []
    for b, members in groups.items():
        if len(members) == 1:
            continue
        n = 0
        for i in members:
            while b + suffix(n) in taken:
                n += 1
            bases[i] = b + suffix(n)
            taken.add(bases[i])
            n += 1
    for entry, new in zip(entries, bases):
        changes.append({"old": entry["key"], "new": new})
        entry["original_key"] = entry["key"]
        entry["key"] = new
    return changes


def check(entries):
    report = []
    for entry in entries:
        rules = REQUIRED.get(entry["type"])
        f = entry["fields"]
        present = lambda name: bool(f.get(name, "").strip())
        if rules is None:
            report.append({"key": entry["key"], "type": entry["type"], "missing": [],
                           "note": "not one of the 14 standard BibTeX entry types; nothing checked"})
            continue
        missing = []
        for rule in rules:
            if isinstance(rule, tuple):
                if not any(present(r) for r in rule):
                    missing.append(" or ".join(rule))
            elif not present(rule):
                missing.append(rule)
        if missing:
            report.append({"key": entry["key"], "type": entry["type"], "missing": missing})
    return report


def month_macro(value):
    v = value.strip().lower().rstrip(".")
    for i, m in enumerate(MONTHS):
        if v in (m.lower(), m[:3].lower(), str(i + 1), f"{i + 1:02d}"):
            return m[:3].lower()
    return None


def write(entries, preambles):
    blocks = [f"@preamble{{{{{p}}}}}" for p in preambles]
    for entry in entries:
        lines = [f"@{entry['type']}{{{entry['key']},"]
        for name, value in entry["fields"].items():
            macro = month_macro(value) if name == "month" else None
            lines.append(f"  {name} = {macro}," if macro else f"  {name} = {{{value}}},")
        lines.append("}")
        blocks.append("\n".join(lines))
    return "\n\n".join(blocks) + ("\n" if blocks else "")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"bibtex", "operations", "merge", "entries"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    text = request.get("bibtex")
    if not isinstance(text, str):
        fail("bibtex must be a string with the .bib text")
    if len(text) > MAX_TEXT:
        fail(f"bibtex may be at most {MAX_TEXT} characters")
    operations = request.get("operations", ["dedupe", "normalize_keys", "check"])
    if not isinstance(operations, list) or any(op not in ("dedupe", "normalize_keys", "check") for op in operations):
        fail("operations must be a list of dedupe, normalize_keys and check")
    for flag in ("merge", "entries"):
        if not isinstance(request.get(flag, True), bool):
            fail(f"{flag} must be true or false")
    entries, strings, preambles, comments, warnings = parse(text)
    out = {"counts": {"entries_in": len(entries), "strings": len(strings), "preambles": len(preambles),
                      "comments": comments}}
    if "dedupe" in operations:
        entries, removed = dedupe(entries, request.get("merge", True))
        out["duplicates"] = removed
    if "normalize_keys" in operations:
        out["keys"] = normalize_keys(entries)
    else:
        seen = {}
        for e in entries:
            seen[e["key"]] = seen.get(e["key"], 0) + 1
        clashes = sorted(k for k, n in seen.items() if n > 1)
        if clashes:
            warnings.append(f"repeated keys: {', '.join(clashes)}")
    if "check" in operations:
        out["missing_fields"] = check(entries)
        out["missing_fields_rule"] = "required fields of the standard entry types, btxdoc (Patashnik, BibTeXing, 1988)"
    out["counts"]["entries_out"] = len(entries)
    if request.get("entries", True):
        out["entries"] = [{k: e[k] for k in ("type", "key", "original_key", "line", "fields") if k in e} for e in entries]
    out["warnings"] = warnings
    out["bibtex"] = write(entries, preambles)
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
