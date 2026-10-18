#!/usr/bin/env python3
"""Text banners in a bundled 5-row block font (A-Z, 0-9 and common punctuation, an original
font of this tool, MIT), with an optional border, or plain text in a box.

Input (JSON on stdin):
  text       the text; "\\n" starts a new line
  mode       "banner" (default) or "box" (the text itself inside a border)
  fill       banner: the character drawing the letters, default "#" ("block" for U+2588)
  spacing    banner: blank columns between letters, default 1 (0-4)
  max_width  banner: wrap words onto further banner lines past this many columns, default 120
  border     "none" (default for banner) | "ascii" | "single" | "double" | "rounded" | "heavy"
             (box mode default: "ascii")
  padding    spaces between the border and the content, default 1 (0-4)
  align      box: "left" (default) | "center" | "right"
  unknown    "replace" (default: a character the font lacks becomes ?) or "error"
"""
import json
import sys

ROWS = 5
# Each glyph: five strings of equal width; '#' marks ink. Original design for this tool.
FONT = {
    "A": [" ### ", "#   #", "#####", "#   #", "#   #"],
    "B": ["#### ", "#   #", "#### ", "#   #", "#### "],
    "C": [" ####", "#    ", "#    ", "#    ", " ####"],
    "D": ["#### ", "#   #", "#   #", "#   #", "#### "],
    "E": ["#####", "#    ", "#### ", "#    ", "#####"],
    "F": ["#####", "#    ", "#### ", "#    ", "#    "],
    "G": [" ####", "#    ", "#  ##", "#   #", " ####"],
    "H": ["#   #", "#   #", "#####", "#   #", "#   #"],
    "I": ["###", " # ", " # ", " # ", "###"],
    "J": ["  ###", "   # ", "   # ", "#  # ", " ##  "],
    "K": ["#   #", "#  # ", "###  ", "#  # ", "#   #"],
    "L": ["#    ", "#    ", "#    ", "#    ", "#####"],
    "M": ["#   #", "## ##", "# # #", "#   #", "#   #"],
    "N": ["#   #", "##  #", "# # #", "#  ##", "#   #"],
    "O": [" ### ", "#   #", "#   #", "#   #", " ### "],
    "P": ["#### ", "#   #", "#### ", "#    ", "#    "],
    "Q": [" ### ", "#   #", "# # #", "#  # ", " ## #"],
    "R": ["#### ", "#   #", "#### ", "#  # ", "#   #"],
    "S": [" ####", "#    ", " ### ", "    #", "#### "],
    "T": ["#####", "  #  ", "  #  ", "  #  ", "  #  "],
    "U": ["#   #", "#   #", "#   #", "#   #", " ### "],
    "V": ["#   #", "#   #", "#   #", " # # ", "  #  "],
    "W": ["#   #", "#   #", "# # #", "## ##", "#   #"],
    "X": ["#   #", " # # ", "  #  ", " # # ", "#   #"],
    "Y": ["#   #", " # # ", "  #  ", "  #  ", "  #  "],
    "Z": ["#####", "   # ", "  #  ", " #   ", "#####"],
    "0": [" ### ", "#  ##", "# # #", "##  #", " ### "],
    "1": [" # ", "## ", " # ", " # ", "###"],
    "2": [" ### ", "#   #", "  ## ", " #   ", "#####"],
    "3": ["#### ", "    #", " ### ", "    #", "#### "],
    "4": ["#   #", "#   #", "#####", "    #", "    #"],
    "5": ["#####", "#    ", "#### ", "    #", "#### "],
    "6": [" ### ", "#    ", "#### ", "#   #", " ### "],
    "7": ["#####", "    #", "   # ", "  #  ", "  #  "],
    "8": [" ### ", "#   #", " ### ", "#   #", " ### "],
    "9": [" ### ", "#   #", " ####", "    #", " ### "],
    " ": ["   ", "   ", "   ", "   ", "   "],
    ".": [" ", " ", " ", " ", "#"],
    ",": ["  ", "  ", "  ", " #", "# "],
    "!": ["#", "#", "#", " ", "#"],
    "?": [" ### ", "#   #", "  ## ", "     ", "  #  "],
    "-": ["   ", "   ", "###", "   ", "   "],
    "+": ["   ", " # ", "###", " # ", "   "],
    "=": ["   ", "###", "   ", "###", "   "],
    ":": [" ", "#", " ", "#", " "],
    ";": ["  ", " #", "  ", " #", "# "],
    "'": ["#", "#", " ", " ", " "],
    '"': ["# #", "# #", "   ", "   ", "   "],
    "/": ["    #", "   # ", "  #  ", " #   ", "#    "],
    "\\": ["#    ", " #   ", "  #  ", "   # ", "    #"],
    "(": [" #", "# ", "# ", "# ", " #"],
    ")": ["# ", " #", " #", " #", "# "],
    "[": ["##", "# ", "# ", "# ", "##"],
    "]": ["##", " #", " #", " #", "##"],
    "_": ["    ", "    ", "    ", "    ", "####"],
    "*": ["     ", "# # #", " ### ", "# # #", "     "],
    "#": [" # # ", "#####", " # # ", "#####", " # # "],
    "<": ["  #", " # ", "#  ", " # ", "  #"],
    ">": ["#  ", " # ", "  #", " # ", "#  "],
    "%": ["#   #", "   # ", "  #  ", " #   ", "#   #"],
}
BORDERS = {
    "ascii": ("+", "-", "+", "|", "|", "+", "-", "+"),
    "single": ("┌", "─", "┐", "│", "│", "└", "─", "┘"),
    "double": ("╔", "═", "╗", "║", "║", "╚", "═", "╝"),
    "rounded": ("╭", "─", "╮", "│", "│", "╰", "─", "╯"),
    "heavy": ("┏", "━", "┓", "┃", "┃", "┗", "━", "┛"),
}


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def integer(request, field, default, low, high):
    value = request.get(field, default)
    if isinstance(value, bool) or not isinstance(value, int) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def render_line(text, fill, spacing):
    rows = [""] * ROWS
    for i, ch in enumerate(text):
        glyph = FONT[ch]
        for r in range(ROWS):
            rows[r] += ("" if i == 0 else " " * spacing) + glyph[r].replace("#", fill)
    return rows


def glyph_width(word, spacing):
    return sum(len(FONT[ch][0]) for ch in word) + spacing * max(0, len(word) - 1)


def wrap(line, spacing, max_width):
    """Split a line into pieces whose banners fit max_width, at spaces where possible."""
    words, pieces, current = line.split(" "), [], ""
    for word in words:
        candidate = word if not current else current + " " + word
        if glyph_width(candidate, spacing) <= max_width:
            current = candidate
            continue
        if current:
            pieces.append(current)
        current = ""
        while glyph_width(word, spacing) > max_width:  # a single word too wide: cut it
            cut = len(word)
            while cut > 1 and glyph_width(word[:cut], spacing) > max_width:
                cut -= 1
            pieces.append(word[:cut])
            word = word[cut:]
        current = word
    if current or not pieces:
        pieces.append(current)
    return pieces


def frame(lines, border, padding, align="left"):
    width = max((len(line) for line in lines), default=0)
    if align == "center":
        body = [line.center(width) for line in lines]
    elif align == "right":
        body = [line.rjust(width) for line in lines]
    else:
        body = [line.ljust(width) for line in lines]
    if border == "none":
        return body
    tl, top, tr, left, right, bl, bottom, br = BORDERS[border]
    inner = width + 2 * padding
    out = [tl + top * inner + tr]
    out += [left + " " * inner + right for _ in range(padding // 2)]
    out += [left + " " * padding + line + " " * padding + right for line in body]
    out += [left + " " * inner + right for _ in range(padding // 2)]
    out.append(bl + bottom * inner + br)
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"text", "mode", "fill", "spacing", "max_width", "border", "padding", "align", "unknown"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be a non-empty string")
    if len(text) > 2000:
        fail("text may be at most 2000 characters")
    mode = request.get("mode", "banner")
    if mode not in ("banner", "box"):
        fail("mode must be banner or box")
    border = request.get("border", "ascii" if mode == "box" else "none")
    if border not in ("none", *BORDERS):
        fail("border must be none, ascii, single, double, rounded or heavy")
    padding = integer(request, "padding", 1, 0, 4)
    lines = text.replace("\r\n", "\n").replace("\t", "    ").split("\n")
    warnings = []
    if mode == "box":
        align = request.get("align", "left")
        if align not in ("left", "center", "right"):
            fail("align must be left, center or right")
        out = frame(lines, border, padding, align)
    else:
        fill = request.get("fill", "#")
        if fill == "block":
            fill = "█"
        if not isinstance(fill, str) or len(fill) != 1 or fill.isspace():
            fail("fill must be one visible character, or \"block\"")
        spacing = integer(request, "spacing", 1, 0, 4)
        max_width = integer(request, "max_width", 120, 10, 1000)
        unknown = request.get("unknown", "replace")
        if unknown not in ("replace", "error"):
            fail("unknown must be replace or error")
        clean = []
        for line in lines:
            line = line.upper()
            missing = sorted({ch for ch in line if ch not in FONT})
            if missing:
                if unknown == "error":
                    fail(f"the font has no glyph for: {' '.join(missing)}")
                warnings.append(f"replaced with ?: {' '.join(missing)}")
                line = "".join(ch if ch in FONT else "?" for ch in line)
            clean.append(line)
        rows = []
        for n, line in enumerate(clean):
            for k, piece in enumerate(wrap(line.strip() or " ", spacing, max_width)):
                if rows:
                    rows.append("")
                rows += render_line(piece, fill, spacing)
        width = max(len(r) for r in rows)
        rows = [r.ljust(width) for r in rows]
        out = frame(rows, border, padding)
    print(json.dumps({"mode": mode, "lines": out, "text": "\n".join(out) + "\n", "width": max(len(r) for r in out),
                      "height": len(out), "warnings": warnings, "font": "AXLR block 5 (original, MIT)"}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
