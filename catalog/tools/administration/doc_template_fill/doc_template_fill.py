#!/usr/bin/env python3
"""Fill {{placeholders}} in plain text, or in a Word .docx given as hex, with the values given.
A .docx is a zip of XML parts: the tool rewrites the text runs (<w:t>) of the body, headers,
footers, footnotes, endnotes and comments. Word often splits "{{name}}" across several runs
(spell-check marks, formatting, editing history), so the runs of each paragraph are joined
before matching and the replacement goes into the first run of each placeholder, keeping its
formatting. Returns the filled text or the filled .docx as hex, with the placeholders found and
the ones without a value.

Input (JSON on stdin):
  values     {"name": "Ada", "amount": 12.5, ...}   strings, numbers or booleans
  text       plain text                              or
  docx_hex   the .docx bytes as hex
  missing    "keep" (default) | "empty" | "error" (exit 2)
"""
import io
import json
import re
import sys
import zipfile

PLACEHOLDER = re.compile(r"\{\{\s*([^{}|\s][^{}|]*?)\s*(?:\|([^{}]*))?\}\}")
PARTS = re.compile(r"^word/(document|header\d*|footer\d*|footnotes|endnotes|comments)\.xml$")
TOKEN = re.compile(r"<w:p(?=[\s>/])[^>]*>|</w:p>|<w:t(?=[\s>/])[^>]*/>|<w:t(?=[\s>/])([^>]*)>(.*?)</w:t>", re.S)
ENTITY = re.compile(r"&(#x[0-9a-fA-F]+|#[0-9]+|amp|lt|gt|quot|apos);")
NAMED = {"amp": "&", "lt": "<", "gt": ">", "quot": '"', "apos": "'"}
MAX_HEX = 40_000_000
MAX_UNCOMPRESSED = 200_000_000


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def unescape(text):
    def one(m):
        name = m.group(1)
        if name.startswith("#x"):
            return chr(int(name[2:], 16))
        if name.startswith("#"):
            return chr(int(name[1:]))
        return NAMED[name]
    return ENTITY.sub(one, text)


def escape(text):
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


class Filler:
    def __init__(self, values, mode):
        self.values, self.mode = values, mode
        self.found, self.missing, self.defaulted = [], [], []

    def value(self, match):
        field, default = match.group(1), match.group(2)
        if field not in self.found:
            self.found.append(field)
        if field in self.values:
            return self.values[field]
        if default is not None:
            if field not in self.defaulted:
                self.defaulted.append(field)
            return default
        if field not in self.missing:
            self.missing.append(field)
        return match.group(0) if self.mode == "keep" else ""

    def text(self, text):
        return PLACEHOLDER.sub(self.value, text)


def fill_runs(texts, filler):
    """texts: the unescaped text of each <w:t> of one paragraph. Returns the new texts."""
    joined = "".join(texts)
    matches = list(PLACEHOLDER.finditer(joined))
    if not matches:
        return texts
    starts, pos = [], 0
    for t in texts:
        starts.append(pos)
        pos += len(t)

    def run_of(char):
        """Index of the run holding character number char of the joined text."""
        for i, t in enumerate(texts):
            if starts[i] <= char < starts[i] + len(t):
                return i
        raise AssertionError("offset outside the paragraph")

    new = list(texts)
    replacements = [(m, filler.value(m)) for m in matches]  # in reading order, so found/missing keep it
    for m, replacement in reversed(replacements):
        i, j = run_of(m.start()), run_of(m.end() - 1)
        a, b = m.start() - starts[i], m.end() - starts[j]
        if i == j:
            new[i] = new[i][:a] + replacement + new[i][b:]
        else:
            new[i] = new[i][:a] + replacement
            for k in range(i + 1, j):
                new[k] = ""
            new[j] = new[j][b:]
    return new


def run_xml(attrs, text):
    """A <w:t> element for text; newlines become <w:br/> and tabs <w:tab/> inside the same run."""
    if "xml:space" not in attrs:
        attrs = attrs + ' xml:space="preserve"'
    pieces = re.split(r"(\r\n|\n|\r|\t)", text)
    out = []
    for piece in pieces:
        if piece in ("\r\n", "\n", "\r"):
            out.append("</w:t><w:br/><w:t xml:space=\"preserve\">")
        elif piece == "\t":
            out.append("</w:t><w:tab/><w:t xml:space=\"preserve\">")
        else:
            out.append(escape(piece))
    return f"<w:t{attrs}>" + "".join(out) + "</w:t>"


def fill_part(xml, filler):
    """Returns (new xml, paragraph texts after filling)."""
    nodes, paragraphs, stack, order = [], [], [], []
    for m in TOKEN.finditer(xml):
        tag = m.group(0)
        if tag.startswith("</w:p"):
            if stack:
                stack.pop()
        elif tag.startswith("<w:p"):
            if not tag.endswith("/>"):
                paragraphs.append([])
                stack.append(len(paragraphs) - 1)
                order.append(len(paragraphs) - 1)
        else:
            attrs = m.group(1) if m.group(1) is not None else None
            text = unescape(m.group(2)) if m.group(2) is not None else ""
            nodes.append({"start": m.start(), "end": m.end(), "attrs": attrs if attrs is not None else "", "text": text,
                          "self_closing": m.group(2) is None})
            if stack:
                paragraphs[stack[-1]].append(len(nodes) - 1)
            else:
                paragraphs.append([len(nodes) - 1])
                order.append(len(paragraphs) - 1)
    changed = {}
    for para in paragraphs:
        texts = [nodes[n]["text"] for n in para]
        new = fill_runs(texts, filler)
        for n, old, value in zip(para, texts, new):
            if value != old:
                changed[n] = value
    out, pos = [], 0
    for n, node in enumerate(nodes):
        if n in changed:
            out.append(xml[pos:node["start"]])
            out.append(run_xml(node["attrs"], changed[n]))
            pos = node["end"]
    out.append(xml[pos:])
    texts = []
    for p in order:
        text = "".join(changed.get(n, nodes[n]["text"]) for n in paragraphs[p])
        if text:
            texts.append(text)
    return "".join(out), texts


def fill_docx(hex_text, filler):
    if not isinstance(hex_text, str) or not hex_text.strip():
        fail("docx_hex must be the .docx file as a hex string")
    if len(hex_text) > MAX_HEX:
        fail("docx_hex is too large (at most 20 MB of .docx)")
    try:
        data = bytes.fromhex(re.sub(r"\s+", "", hex_text))
    except ValueError:
        fail("docx_hex is not valid hex")
    try:
        zin = zipfile.ZipFile(io.BytesIO(data))
        infos = zin.infolist()
    except (zipfile.BadZipFile, ValueError, OSError):
        fail("docx_hex is not a zip archive (.docx)")
    if len(infos) > 5000 or sum(i.file_size for i in infos) > MAX_UNCOMPRESSED:
        fail("the .docx is too large once uncompressed")
    names = [i.filename for i in infos]
    if "word/document.xml" not in names:
        fail("not a Word document: word/document.xml is missing")
    if any(".." in n.split("/") or n.startswith("/") for n in names):
        fail("the .docx has an unsafe member path")
    out_buffer = io.BytesIO()
    paragraphs, touched = [], []
    try:
        with zipfile.ZipFile(out_buffer, "w") as zout:
            for info in infos:
                content = zin.read(info)
                if PARTS.match(info.filename):
                    try:
                        xml = content.decode("utf-8")
                    except UnicodeDecodeError:
                        fail(f"{info.filename} is not UTF-8 XML")
                    new_xml, texts = fill_part(xml, filler)
                    if info.filename == "word/document.xml":
                        paragraphs = texts
                    if new_xml != xml:
                        touched.append(info.filename)
                        content = new_xml.encode("utf-8")
                zout.writestr(info, content)
    except (zipfile.BadZipFile, zipfile.LargeZipFile, OSError, RuntimeError, NotImplementedError) as error:
        fail(f"the .docx could not be read: {error}")
    return out_buffer.getvalue(), paragraphs, touched


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"values", "text", "docx_hex", "missing"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    raw = request.get("values", {})
    if not isinstance(raw, dict):
        fail("values must be an object of field: value")
    values = {}
    for key, value in raw.items():
        if isinstance(value, bool):
            values[key] = "true" if value else "false"
        elif isinstance(value, (int, float, str)):
            values[key] = str(value)
        elif value is None:
            continue
        else:
            fail(f"values.{key} must be a string, number or boolean")
    mode = request.get("missing", "keep")
    if mode not in ("keep", "empty", "error"):
        fail("missing must be keep, empty or error")
    has_text, has_docx = request.get("text") is not None, request.get("docx_hex") is not None
    if has_text == has_docx:
        fail("give text or docx_hex (exactly one)")
    filler = Filler(values, mode)
    if has_text:
        text = request["text"]
        if not isinstance(text, str):
            fail("text must be a string")
        filled = filler.text(text)
        result = {"format": "text", "text": filled}
    else:
        blob, paragraphs, touched = fill_docx(request["docx_hex"], filler)
        result = {"format": "docx", "docx_hex": blob.hex(), "bytes": len(blob), "paragraphs": paragraphs, "parts_changed": touched}
    if mode == "error" and filler.missing:
        fail(f"no value for: {', '.join(filler.missing)}")
    result.update({"placeholders_found": filler.found, "missing": filler.missing, "defaults_used": filler.defaulted,
                   "values_not_used": [k for k in values if k not in filler.found]})
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
