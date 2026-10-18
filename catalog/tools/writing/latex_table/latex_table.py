#!/usr/bin/env python3
"""CSV text to a LaTeX tabular: the ten special characters escaped as the LaTeX reference
manual prescribes, column alignment (numeric columns right-aligned automatically), plain
\\hline rules or booktabs, optional siunitx S columns, caption and label in a table float.

Input (JSON on stdin):
  csv        the CSV text (required)
  delimiter  one character (default ",")
  header     first row is a header (default true)
  align      "lrc..." one letter per column (l, c, r, or S with siunitx); default automatic
  booktabs   \\toprule / \\midrule / \\bottomrule instead of \\hline (default false)
  siunitx    numeric columns as S columns (default false)
  caption, label   put the tabular in a table float with them
  position   float placement (default "htbp")
  escape     escape special characters in cells (default true)
"""
import csv
import io
import json
import re
import sys

SPECIAL = {"#": r"\#", "$": r"\$", "%": r"\%", "&": r"\&", "{": r"\{", "}": r"\}", "_": r"\_",
           "~": r"\~{}", "^": r"\^{}", "\\": r"\textbackslash{}"}
NUMBER = re.compile(r"[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def escape(text):
    return "".join(SPECIAL.get(c, c) for c in text)


def get_bool(request, field, default):
    value = request.get(field, default)
    if not isinstance(value, bool):
        fail(f"{field} must be true or false")
    return value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    text = request.get("csv")
    if not isinstance(text, str) or not text.strip():
        fail("csv must be a non-empty string")
    if len(text) > 2000000:
        fail("csv is longer than 2000000 characters")
    delimiter = request.get("delimiter", ",")
    if not isinstance(delimiter, str) or len(delimiter) != 1 or delimiter in "\"\r\n":
        fail("delimiter must be one character other than a quote or a newline")
    header = get_bool(request, "header", True)
    booktabs = get_bool(request, "booktabs", False)
    siunitx = get_bool(request, "siunitx", False)
    do_escape = get_bool(request, "escape", True)
    caption, label = request.get("caption"), request.get("label")
    if caption is not None and not isinstance(caption, str):
        fail("caption must be a string")
    if label is not None and (not isinstance(label, str) or not re.fullmatch(r"[A-Za-z0-9:._\-]+", label)):
        fail("label must be letters, digits and : . _ - only")
    position = request.get("position", "htbp")
    if not isinstance(position, str) or not re.fullmatch(r"[htbpH!]{1,5}", position):
        fail("position must be made of h t b p H !")
    try:
        rows = [row for row in csv.reader(io.StringIO(text), delimiter=delimiter) if row]
    except csv.Error as error:
        fail(f"csv: {error}")
    if not rows:
        fail("csv has no rows")
    width = max(len(r) for r in rows)
    ragged = sum(len(r) != width for r in rows)
    rows = [r + [""] * (width - len(r)) for r in rows]
    head, body = (rows[0], rows[1:]) if header else (None, rows)
    numeric = []
    for k in range(width):
        cells = [r[k].strip() for r in body if r[k].strip()]
        numeric.append(bool(cells) and all(NUMBER.fullmatch(c) for c in cells))
    align = request.get("align")
    if align is None:
        spec = ["S" if n and siunitx else ("r" if n else "l") for n in numeric]
    else:
        if not isinstance(align, str) or len(align) != width or not re.fullmatch(r"[lcrS]+", align):
            fail(f"align must have one of l, c, r, S for each of the {width} columns")
        if "S" in align and not siunitx:
            fail("S columns need siunitx: true")
        spec = list(align)

    def cell(value, k, is_header):
        value = value.strip()
        out = escape(value) if do_escape else value
        if spec[k] == "S" and (is_header or not NUMBER.fullmatch(value)):
            out = "{" + out + "}" if out else out  # siunitx: protect non-numbers
        elif spec[k] == "S":
            out = value
        return out

    def line(row, is_header=False):
        return "    " + " & ".join(cell(v, k, is_header) for k, v in enumerate(row)) + r" \\"

    top, mid, bottom = (r"\toprule", r"\midrule", r"\bottomrule") if booktabs else (r"\hline", r"\hline", r"\hline")
    lines = [r"\begin{tabular}{" + "".join(spec) + "}", "    " + top]
    if head is not None:
        lines += [line(head, True), "    " + mid]
    lines += [line(r) for r in body]
    lines += ["    " + bottom, r"\end{tabular}"]
    float_env = caption is not None or label is not None
    if float_env:
        inner = ["  " + x for x in lines]
        out = [r"\begin{table}[" + position + "]", r"  \centering"]
        if caption is not None:
            out.append(r"  \caption{" + escape(caption) + "}")
        if label is not None:
            out.append(r"  \label{" + label + "}")
        lines = out + inner + [r"\end{table}"]
    packages = (["booktabs"] if booktabs else []) + (["siunitx"] if "S" in spec else [])
    result = {"latex": "\n".join(lines) + "\n", "columns": width, "rows": len(body), "header": header,
              "alignment": "".join(spec), "numeric_columns": [k + 1 for k, n in enumerate(numeric) if n],
              "packages": packages, "float": float_env}
    if ragged:
        result["warning"] = f"{ragged} rows had fewer cells than {width} and were padded with empty cells"
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
