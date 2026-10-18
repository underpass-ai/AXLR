#!/usr/bin/env python3
"""Turn CSV flashcards (front, back, tags, optional deck and extra fields) into an
Anki-importable tab-separated text file with the file headers of the Anki manual
(https://docs.ankiweb.net/importing/text-files.html, "File Headers", Anki 2.1.54+).

Input (JSON on stdin):
  csv            the CSV text (required)
  delimiter      CSV delimiter, default ","
  header         true | false | "auto" (default: the first row is a header when one cell is "front"
                 and every cell is a short lowercase-able name)
  columns        column names when there is no header row, default ["front", "back", "tags", "deck"]
  deck           deck for every note (#deck:), unless a deck column is present
  notetype       note type for every note (#notetype:)
  tags           tags added to every note (#tags:), list or space-separated string
  html           treat fields as HTML (#html:true), default false
  newlines       "br" (newline -> <br>, needs html) | "quote" (keep newlines inside a quoted field);
                 default br when html is true, else quote
"""
import csv
import io
import json
import re
import sys

KNOWN = ("front", "back", "tags", "deck")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def tag_list(value):
    return [t for t in re.split(r"[\s,;]+", value.strip()) if t]


def quote(field, newlines):
    """Anki text import: a field holding the separator, a newline or a double quote is put in
    double quotes, with each double quote inside doubled."""
    text = field.replace("\r\n", "\n").replace("\r", "\n")
    if newlines == "br":
        text = text.replace("\n", "<br>")
    if any(c in text for c in '\t\n"'):
        return '"' + text.replace('"', '""') + '"'
    return text


def header_value(value, field):
    if not isinstance(value, str) or not value.strip() or any(c in value for c in "\t\r\n"):
        fail(f"{field} must be a non-empty single-line string")
    return value.strip()


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"csv", "delimiter", "header", "columns", "deck", "notetype", "tags", "html", "newlines"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    source = request.get("csv")
    if not isinstance(source, str) or not source.strip():
        fail("csv must be the CSV text")
    if len(source) > 5_000_000:
        fail("csv is larger than 5 MB")
    delimiter = request.get("delimiter", ",")
    if not isinstance(delimiter, str) or len(delimiter) != 1 or delimiter in '"\r\n':
        fail("delimiter must be one character (not a quote or newline)")
    html = request.get("html", False)
    if not isinstance(html, bool):
        fail("html must be true or false")
    newlines = request.get("newlines", "br" if html else "quote")
    if newlines not in ("br", "quote"):
        fail("newlines must be br or quote")
    if newlines == "br" and not html:
        fail("newlines br needs html true (Anki shows <br> literally otherwise)")
    try:
        rows = [r for r in csv.reader(io.StringIO(source, newline=""), delimiter=delimiter) if any(c.strip() for c in r)]
    except csv.Error as error:
        fail(f"csv is not valid: {error}")
    if not rows:
        fail("csv has no rows")
    header = request.get("header", "auto")
    if header not in (True, False, "auto"):
        fail("header must be true, false or auto")
    first = [c.strip().lower() for c in rows[0]]
    if header == "auto":
        header = "front" in first and all(re.fullmatch(r"[a-z][a-z0-9_ ]{0,39}", c) for c in first)
    if header:
        names, rows = first, rows[1:]
    else:
        names = request.get("columns", list(KNOWN))
        if not isinstance(names, list) or not all(isinstance(n, str) and n.strip() for n in names):
            fail("columns must be a list of column names")
        names = [n.strip().lower() for n in names]
    if len(set(names)) != len(names):
        fail("column names must be unique")
    if "front" not in names:
        fail("there must be a front column")
    width = max(len(r) for r in rows) if rows else 0
    if width > len(names):
        fail(f"a row has {width} fields but only {len(names)} columns are named")
    fields = ["front"] + (["back"] if "back" in names else []) + [n for n in names if n not in KNOWN]
    has_tags, has_deck = "tags" in names, "deck" in names
    if has_deck and request.get("deck") is not None:
        fail("give a deck column or a deck for every note, not both")
    out_columns = fields + (["tags"] if has_tags else []) + (["deck"] if has_deck else [])
    lines = ["#separator:Tab", f"#html:{'true' if html else 'false'}"]
    if request.get("notetype") is not None:
        lines.append(f"#notetype:{header_value(request['notetype'], 'notetype')}")
    if request.get("deck") is not None:
        lines.append(f"#deck:{header_value(request['deck'], 'deck')}")
    if request.get("tags") is not None:
        given = request["tags"]
        if isinstance(given, str):
            given = tag_list(given)
        if not isinstance(given, list) or not all(isinstance(t, str) for t in given):
            fail("tags must be a list of strings or a space-separated string")
        given = [t for g in given for t in tag_list(g)]
        if given:
            lines.append("#tags:" + " ".join(given))
    lines.append("#columns:" + "\t".join(c.capitalize() for c in out_columns))
    if has_tags:
        lines.append(f"#tags column:{out_columns.index('tags') + 1}")
    if has_deck:
        lines.append(f"#deck column:{out_columns.index('deck') + 1}")
    header_count = len(lines)
    skipped, notes = [], 0
    offset = 2 if header else 1
    for number, row in enumerate(rows):
        cells = dict(zip(names, row + [""] * (len(names) - len(row))))
        if not cells["front"].strip():
            skipped.append({"row": number + offset, "reason": "empty front"})
            continue
        values = [quote(cells[f], newlines) for f in fields]
        if has_tags:
            values.append(" ".join(tag_list(cells["tags"])))
        if has_deck:
            values.append(quote(cells["deck"].strip(), newlines))
        lines.append("\t".join(values))
        notes += 1
    if not notes:
        fail("no notes: every row has an empty front")
    print(json.dumps({"tsv": "\n".join(lines) + "\n", "notes": notes, "header_lines": lines[:header_count],
                      "columns": out_columns, "skipped": skipped,
                      "import": "Anki: File > Import, choose the .txt file; the header lines preset the options "
                                "(Anki 2.1.54 or later)"}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
