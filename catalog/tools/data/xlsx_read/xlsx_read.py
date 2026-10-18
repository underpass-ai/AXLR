#!/usr/bin/env python3
"""Read the sheets of an Excel .xlsx workbook into CSV and JSON with the standard library:
the file is opened as a zip and its SpreadsheetML parts are parsed (shared strings, inline
strings, numbers, booleans, errors, formulas' cached values), dates are recognised by their
number format and converted with the workbook's 1900 or 1904 date system, and merged cells
are reported.

Input (JSON on stdin):
  hex        the .xlsx file as hex (or path: a relative file in the working directory)
  sheet      a sheet name or 1-based index (default: every sheet)
  header     true: also return rows as objects keyed by the first row (default true)
  max_rows   rows read per sheet, default 10000
  dates      "iso" (default) converts date-formatted numbers; "serial" keeps the numbers
"""
import csv
import datetime as dt
import io
import json
import pathlib
import re
import sys
import zipfile
import xml.etree.ElementTree as ET

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
      "r": "http://schemas.openxmlformats.org/officeDocument/2006/relationships",
      "rel": "http://schemas.openxmlformats.org/package/2006/relationships"}
STRICT = {"m": "http://purl.oclc.org/ooxml/spreadsheetml/main",
          "r": "http://purl.oclc.org/ooxml/officeDocument/relationships"}
# ECMA-376 Part 1, 18.8.30: built-in number formats that are dates or times.
BUILTIN_DATES = {14: "date", 15: "date", 16: "date", 17: "date", 18: "time", 19: "time", 20: "time", 21: "time",
                 22: "datetime", 27: "date", 30: "date", 36: "date", 45: "time", 46: "duration", 47: "time", 50: "date", 57: "date"}
MAX_BYTES = 20_000_000
MAX_UNZIPPED = 200_000_000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load_bytes(request):
    if ("hex" in request) == ("path" in request):
        fail("give exactly one of hex (the .xlsx as hex) or path (relative)")
    if "hex" in request:
        text = request["hex"]
        if not isinstance(text, str):
            fail("hex must be a string")
        try:
            data = bytes.fromhex(re.sub(r"\s", "", text))
        except ValueError:
            fail("hex is not valid hexadecimal")
    else:
        path = request["path"]
        if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path) or ".." in re.split(r"[\\/]", path):
            fail("path must be a relative path inside the working directory, without '..'")
        target = pathlib.Path(path)
        if not target.is_file():
            fail(f"no such file: {path}")
        if target.stat().st_size > MAX_BYTES:
            fail("the file is larger than 20 MB")
        data = target.read_bytes()
    if len(data) > MAX_BYTES:
        fail("the file is larger than 20 MB")
    return data


def tag(element):
    return element.tag.rsplit("}", 1)[-1]


def children(element, name):
    return [c for c in element if tag(c) == name]


def descendants(element, name):
    return [c for c in element.iter() if tag(c) == name]


def column_index(ref):
    letters = re.match(r"^([A-Z]+)", ref)
    if not letters:
        return None
    index = 0
    for char in letters.group(1):
        index = index * 26 + ord(char) - 64
    return index - 1


def column_letters(index):
    letters = ""
    index += 1
    while index:
        index, rest = divmod(index - 1, 26)
        letters = chr(65 + rest) + letters
    return letters


def format_kind(code):
    """date, time, datetime, duration or None for a custom number format code."""
    cleaned = re.sub(r'"[^"]*"|\\.|\[(?!h\]|m\]|s\])[^\]]*\]', "", code.lower())
    cleaned = cleaned.split(";")[0]
    if re.search(r"\[(h|m|s)\]", cleaned):
        return "duration"
    has_date = re.search(r"[dy]", cleaned) or re.search(r"(^|[^h:])m{3,}", cleaned)
    has_time = re.search(r"[hs]", cleaned) or "am/pm" in cleaned or "a/p" in cleaned
    if has_date and has_time:
        return "datetime"
    if has_date:
        return "date"
    if has_time:
        return "time"
    return None


def serial_to_datetime(serial, date1904):
    """ECMA-376 Part 1, 18.17.4: serial day numbers in the 1900 or 1904 date system."""
    if date1904:
        base = dt.datetime(1904, 1, 1)
        return base + dt.timedelta(days=serial), None
    if serial < 0:
        return None, "negative serial in the 1900 date system"
    if 60 <= serial < 61:
        return None, "serial 60 is 1900-02-29, a day that does not exist (kept for Lotus 1-2-3 compatibility)"
    if serial < 60:
        return dt.datetime(1899, 12, 31) + dt.timedelta(days=serial), None
    return dt.datetime(1899, 12, 30) + dt.timedelta(days=serial), None


def iso(moment, kind):
    seconds = round(moment.second + moment.microsecond / 1e6)
    moment = moment.replace(microsecond=0) + dt.timedelta(seconds=seconds - moment.second)
    if kind == "date":
        return moment.date().isoformat()
    if kind == "time":
        return moment.time().isoformat()
    return moment.isoformat(sep="T")


def read_xml(archive, name):
    try:
        with archive.open(name) as handle:
            return ET.fromstring(handle.read())
    except KeyError:
        return None
    except ET.ParseError as error:
        fail(f"{name} is not well-formed XML: {error}")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"hex", "path", "sheet", "header", "max_rows", "dates"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    header = request.get("header", True)
    if not isinstance(header, bool):
        fail("header must be true or false")
    max_rows = request.get("max_rows", 10000)
    if isinstance(max_rows, bool) or not isinstance(max_rows, int) or not 1 <= max_rows <= 1_000_000:
        fail("max_rows must be an integer from 1 to 1000000")
    dates = request.get("dates", "iso")
    if dates not in ("iso", "serial"):
        fail("dates must be iso or serial")
    data = load_bytes(request)
    try:
        archive = zipfile.ZipFile(io.BytesIO(data))
    except zipfile.BadZipFile:
        fail("not an .xlsx file: it is not a zip archive (an old .xls is not supported)")
    if sum(info.file_size for info in archive.infolist()) > MAX_UNZIPPED:
        fail("the workbook expands to more than 200 MB")
    names = set(archive.namelist())
    workbook = read_xml(archive, "xl/workbook.xml")
    if workbook is None:
        fail("not an .xlsx workbook: xl/workbook.xml is missing")
    pr = descendants(workbook, "workbookPr")
    date1904 = bool(pr) and pr[0].get("date1904", "0").lower() in ("1", "true")
    rels = read_xml(archive, "xl/_rels/workbook.xml.rels")
    targets = {}
    if rels is not None:
        for rel in descendants(rels, "Relationship"):
            target = rel.get("Target", "")
            target = target.lstrip("/") if target.startswith("/") else "xl/" + target
            targets[rel.get("Id")] = target
    sheets = []
    for sheet in descendants(workbook, "sheet"):
        rid = sheet.get(f"{{{NS['r']}}}id") or sheet.get(f"{{{STRICT['r']}}}id")
        sheets.append({"name": sheet.get("name"), "target": targets.get(rid), "state": sheet.get("state", "visible")})
    if not sheets:
        fail("the workbook has no sheets")

    shared = []
    sst = read_xml(archive, "xl/sharedStrings.xml")
    if sst is not None:
        for si in children(sst, "si"):
            # Text of t and of the runs' t, but not phonetic guides (rPh).
            parts = [t.text or "" for t in children(si, "t")]
            for run in children(si, "r"):
                parts += [t.text or "" for t in children(run, "t")]
            shared.append("".join(parts))
    xf_kinds = []
    styles = read_xml(archive, "xl/styles.xml")
    custom = {}
    if styles is not None:
        for fmt in descendants(styles, "numFmt"):
            try:
                custom[int(fmt.get("numFmtId"))] = fmt.get("formatCode", "")
            except (TypeError, ValueError):
                continue
        xfs = descendants(styles, "cellXfs")
        for xf in children(xfs[0], "xf") if xfs else []:
            try:
                fid = int(xf.get("numFmtId", "0"))
            except ValueError:
                fid = 0
            xf_kinds.append(format_kind(custom[fid]) if fid in custom else BUILTIN_DATES.get(fid))

    wanted = request.get("sheet")
    if wanted is None:
        chosen = sheets
    elif isinstance(wanted, int) and not isinstance(wanted, bool):
        if not 1 <= wanted <= len(sheets):
            fail(f"sheet index must be from 1 to {len(sheets)}")
        chosen = [sheets[wanted - 1]]
    elif isinstance(wanted, str):
        chosen = [s for s in sheets if s["name"] == wanted]
        if not chosen:
            fail(f"no sheet named {wanted!r}; sheets: {', '.join(s['name'] for s in sheets)}")
    else:
        fail("sheet must be a name or a 1-based index")

    results = []
    for sheet in chosen:
        target = sheet["target"]
        if not target or target not in names:
            fail(f"sheet {sheet['name']!r} points to a missing part {target!r}")
        root = read_xml(archive, target)
        cells = {}
        notes = []
        truncated = False
        max_col = -1
        row_number = 0
        for row in descendants(root, "row"):
            row_number = int(row.get("r", row_number + 1))
            if row_number > max_rows:
                truncated = True
                break
            next_col = 0
            for c in children(row, "c"):
                ref = c.get("r")
                col = column_index(ref) if ref else next_col
                if col is None:
                    col = next_col
                next_col = col + 1
                kind_attr = c.get("t", "n")
                v = children(c, "v")
                raw = v[0].text if v else None
                value = None
                if kind_attr == "s":
                    try:
                        value = shared[int(raw)]
                    except (TypeError, ValueError, IndexError):
                        value = None
                        notes.append(f"{column_letters(col)}{row_number}: shared string index out of range")
                elif kind_attr == "inlineStr":
                    inline = children(c, "is")
                    if inline:
                        parts = [t.text or "" for t in children(inline[0], "t")]
                        for run in children(inline[0], "r"):
                            parts += [t.text or "" for t in children(run, "t")]
                        value = "".join(parts)
                elif kind_attr == "str":
                    value = raw or ""
                elif kind_attr == "b":
                    value = raw == "1"
                elif kind_attr == "e":
                    value = raw
                elif kind_attr == "d":
                    value = raw
                elif raw is not None:
                    try:
                        number = float(raw)
                    except ValueError:
                        number = None
                        value = raw
                    if number is not None:
                        style = c.get("s")
                        kind = xf_kinds[int(style)] if style and style.isdigit() and int(style) < len(xf_kinds) else None
                        if kind and kind != "duration" and dates == "iso":
                            if kind == "time" and 0 <= number < 1:
                                moment = dt.datetime(1900, 1, 1) + dt.timedelta(days=number)
                                value = iso(moment, "time")
                            else:
                                moment, problem = serial_to_datetime(number, date1904)
                                if moment is None:
                                    value = int(number) if number.is_integer() else number
                                    notes.append(f"{column_letters(col)}{row_number}: {problem}")
                                else:
                                    value = iso(moment, "date" if kind == "date" and number.is_integer() else "datetime")
                        else:
                            value = int(number) if number.is_integer() and abs(number) < 2 ** 53 and "." not in raw and "E" not in raw.upper() else number
                if value is not None:
                    cells[(row_number, col)] = value
                    max_col = max(max_col, col)
        merged = [m.get("ref") for m in descendants(root, "mergeCell")]
        last_row = max((r for r, _ in cells), default=0)
        grid = [[cells.get((r, c)) for c in range(max_col + 1)] for r in range(1, last_row + 1)]
        out = io.StringIO()
        writer = csv.writer(out, lineterminator="\n")
        for line in grid:
            writer.writerow(["" if v is None else ("TRUE" if v is True else "FALSE" if v is False else v) for v in line])
        entry = {"name": sheet["name"], "state": sheet["state"], "rows": len(grid), "columns": max_col + 1,
                 "dimension": f"A1:{column_letters(max_col)}{last_row}" if grid and max_col >= 0 else None,
                 "values": grid, "csv": out.getvalue(), "merged_cells": merged, "truncated": truncated}
        if merged:
            notes.append("merged cells: only the top-left cell of each merged range holds the value; the others are empty")
        if header and grid:
            keys = [str(k) if k is not None else f"column_{i + 1}" for i, k in enumerate(grid[0])]
            entry["records"] = [{keys[i]: v for i, v in enumerate(line)} for line in grid[1:]]
        entry["notes"] = notes
        results.append(entry)
    print(json.dumps({"date_system": 1904 if date1904 else 1900, "sheet_names": [s["name"] for s in sheets],
                      "sheets": results, "formulas": "cached values only; formulas are not recalculated"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
