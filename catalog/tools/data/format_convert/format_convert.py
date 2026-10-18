#!/usr/bin/env python3
"""Convert configuration and data files between JSON, TOML, INI and a YAML subset. TOML is
read with tomllib (Python 3.11+) and written by this program (a subset: tables, arrays of
tables, inline arrays and scalars); INI goes through configparser; YAML is a block-style
subset read and written here (mappings, sequences, scalars, quoted strings, block scalars,
simple flow collections, comments), refusing anchors, aliases, tags and multi-document
streams with a clear message. Every conversion reports what does not survive a round trip.

Input (JSON on stdin):
  text    the input text (or data: a JSON value, which implies from "json")
  from    "json" | "toml" | "ini" | "yaml"
  to      "json" | "toml" | "ini" | "yaml"
  indent  JSON indentation, default 2
"""
import configparser
import datetime as dt
import json
import math
import re
import sys

try:
    import tomllib
except ImportError:  # Python 3.10
    tomllib = None

FORMATS = ("json", "toml", "ini", "yaml")


class ConvertError(Exception):
    pass


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


# ---------------------------------------------------------------- TOML

def toml_plain(value, notes):
    """tomllib values to JSON: dates and times become ISO strings."""
    if isinstance(value, dict):
        return {k: toml_plain(v, notes) for k, v in value.items()}
    if isinstance(value, list):
        return [toml_plain(v, notes) for v in value]
    if isinstance(value, (dt.datetime, dt.date, dt.time)):
        notes.add("TOML dates and times became ISO 8601 strings (JSON has no date type)")
        return value.isoformat()
    if isinstance(value, float) and not math.isfinite(value):
        notes.add("TOML inf and nan have no JSON equivalent: written as the strings \"inf\", \"-inf\", \"nan\"")
        return "nan" if value != value else ("inf" if value > 0 else "-inf")
    return value


BARE_KEY = re.compile(r"^[A-Za-z0-9_-]+$")


def toml_key(key):
    return key if BARE_KEY.match(key) else json.dumps(key, ensure_ascii=False)


def toml_value(value, path):
    if value is None:
        raise ConvertError(f"{path or 'the value'} is null: TOML has no null (remove it or use a string)")
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        if value != value:
            return "nan"
        if math.isinf(value):
            return "inf" if value > 0 else "-inf"
        text = repr(value)
        return text if any(c in text for c in ".eEn") else text + ".0"
    if isinstance(value, str):
        return json.dumps(value, ensure_ascii=False).replace("\\/", "/")
    if isinstance(value, list):
        return "[" + ", ".join(toml_value(v, f"{path}[{i}]") for i, v in enumerate(value)) + "]"
    if isinstance(value, dict):
        return "{" + ", ".join(f"{toml_key(k)} = {toml_value(v, f'{path}.{k}')}" for k, v in value.items()) + "}"
    raise ConvertError(f"{path}: unsupported value")


def is_table_array(value):
    return isinstance(value, list) and value and all(isinstance(v, dict) for v in value)


def write_toml(data):
    if not isinstance(data, dict):
        raise ConvertError("TOML needs a table (a JSON object) at the top level")
    lines = []

    def table(obj, prefix):
        scalars = [(k, v) for k, v in obj.items() if not isinstance(v, dict) and not is_table_array(v)]
        for k, v in scalars:
            lines.append(f"{toml_key(k)} = {toml_value(v, '.'.join(prefix + [k]))}")
        for k, v in obj.items():
            name = ".".join(toml_key(p) for p in prefix + [k])
            if isinstance(v, dict):
                lines.append("")
                lines.append(f"[{name}]")
                table(v, prefix + [k])
            elif is_table_array(v):
                for item in v:
                    lines.append("")
                    lines.append(f"[[{name}]]")
                    table(item, prefix + [k])

    table(data, [])
    return "\n".join(lines).lstrip("\n") + "\n"


# ---------------------------------------------------------------- INI

def read_ini(text):
    parser = configparser.ConfigParser(interpolation=None, default_section="\0no default section\0", strict=True)
    parser.optionxform = str
    try:
        parser.read_string(text)
    except configparser.MissingSectionHeaderError:
        try:
            parser.read_string("[DEFAULT]\n" + text)
        except configparser.Error as error:
            raise ConvertError(f"INI: {error}")
    except configparser.Error as error:
        raise ConvertError(f"INI: {' '.join(str(error).split())}")
    return {section: dict(parser.items(section, raw=True)) for section in parser.sections()}


def ini_scalar(value, path):
    if value is None:
        return ""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float, str)):
        text = str(value)
        if "\n" in text:
            return text.replace("\n", "\n    ")
        return text
    raise ConvertError(f"{path}: INI values must be scalars (lists and deeper nesting are not representable)")


def write_ini(data, notes):
    if not isinstance(data, dict):
        raise ConvertError("INI needs an object of sections")
    loose = {k: v for k, v in data.items() if not isinstance(v, dict)}
    sections = {k: v for k, v in data.items() if isinstance(v, dict)}
    lines = []
    if loose:
        notes.add("top-level scalars were put in the [DEFAULT] section")
        sections = {"DEFAULT": {**sections.get("DEFAULT", {}), **loose}, **{k: v for k, v in sections.items() if k != "DEFAULT"}}
    for name, values in sections.items():
        if "]" in name or "\n" in name:
            raise ConvertError(f"section name {name!r} cannot be written in INI")
        lines.append(f"[{name}]")
        for key, value in values.items():
            if any(c in str(key) for c in "=:\n[") or str(key).strip() != str(key) or not str(key):
                raise ConvertError(f"key {key!r} cannot be written in INI")
            lines.append(f"{key} = {ini_scalar(value, f'{name}.{key}')}")
        lines.append("")
    notes.add("INI has no types: every value is read back as a string")
    return "\n".join(lines)


# ---------------------------------------------------------------- YAML subset

NULLS = {"", "~", "null", "Null", "NULL"}
TRUES = {"true", "True", "TRUE"}
FALSES = {"false", "False", "FALSE"}
YAML11_BOOLS = {"yes", "Yes", "YES", "no", "No", "NO", "on", "On", "ON", "off", "Off", "OFF", "y", "Y", "n", "N"}
INT_RE = re.compile(r"^[-+]?[0-9]+$")
OCT_RE = re.compile(r"^0o[0-7]+$")
HEX_RE = re.compile(r"^0x[0-9a-fA-F]+$")
FLOAT_RE = re.compile(r"^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$")


class YamlReader:
    def __init__(self, text, notes):
        self.notes = notes
        self.lines = []
        seen_content = False
        for number, raw in enumerate(text.replace("\r\n", "\n").replace("\r", "\n").split("\n"), 1):
            if "\t" in raw[:len(raw) - len(raw.lstrip())]:
                raise ConvertError(f"YAML line {number}: tabs are not allowed for indentation")
            stripped = self.strip_comment(raw, number).rstrip()
            if not stripped.strip():
                self.lines.append((None, "", number))
                continue
            body = stripped.strip()
            if body.startswith("%"):
                raise ConvertError(f"YAML line {number}: directives (%YAML, %TAG) are not supported")
            if body == "---" or body.startswith("--- "):
                if seen_content:
                    raise ConvertError(f"YAML line {number}: a second document (---): multi-document streams are not supported")
                if body.startswith("--- "):
                    raise ConvertError(f"YAML line {number}: content after --- is not supported")
                continue
            if body == "...":
                continue
            seen_content = True
            indent = len(stripped) - len(stripped.lstrip(" "))
            self.lines.append((indent, body, number))

    @staticmethod
    def strip_comment(line, number):
        quote = None
        for index, char in enumerate(line):
            if quote:
                if char == quote:
                    if quote == "'" and index + 1 < len(line) and line[index + 1] == "'":
                        continue
                    if quote == '"' and line[index - 1] == "\\" and not line[max(0, index - 2):index] == "\\\\":
                        continue
                    quote = None
            elif char in "'\"" and (index == 0 or line[index - 1] in " -:[{,?"):
                quote = char
            elif char == "#" and (index == 0 or line[index - 1] in " \t"):
                return line[:index]
        return line

    def content(self, start):
        index = start
        while index < len(self.lines) and self.lines[index][0] is None:
            index += 1
        return index

    def parse(self):
        index = self.content(0)
        if index >= len(self.lines):
            return None
        value, index = self.block(index, self.lines[index][0])
        index = self.content(index)
        if index < len(self.lines):
            raise ConvertError(f"YAML line {self.lines[index][2]}: unexpected content (check the indentation)")
        return value

    def block(self, index, indent):
        line_indent, body, number = self.lines[index]
        if body == "-" or body.startswith("- "):
            return self.sequence(index, line_indent)
        if self.mapping_key(body, number) is not None:
            return self.mapping(index, line_indent)
        return self.scalar_lines(index, line_indent)

    def check_node_properties(self, text, number):
        if text[:1] in ("&", "*"):
            raise ConvertError(f"YAML line {number}: anchors (&) and aliases (*) are not supported")
        if text[:1] == "!":
            raise ConvertError(f"YAML line {number}: tags (!) are not supported")
        if text[:1] in ("?",) and (len(text) == 1 or text[1] == " "):
            raise ConvertError(f"YAML line {number}: complex mapping keys (?) are not supported")
        if text[:1] in ("@", "`"):
            raise ConvertError(f"YAML line {number}: a plain scalar cannot start with {text[0]}")

    def mapping_key(self, body, number):
        """(key, rest) when the line is 'key: value' or 'key:'; None otherwise."""
        if body[:1] in ('"', "'"):
            quote = body[0]
            end = 1
            while end < len(body):
                if body[end] == "\\" and quote == '"':
                    end += 2
                    continue
                if body[end] == quote:
                    if quote == "'" and end + 1 < len(body) and body[end + 1] == "'":
                        end += 2
                        continue
                    break
                end += 1
            after = body[end + 1:]
            if after.startswith(":") and (len(after) == 1 or after[1] == " "):
                return self.quoted(body[:end + 1], number), after[1:].strip()
            return None
        if body[:1] in "#[]{},|>" or body.startswith("- ") or body == "-":
            return None
        match = re.match(r"^(.+?):(?: +(.*)|)$", body)
        if not match:
            return None
        key = match.group(1).strip()
        if not key:
            return None
        self.check_node_properties(key, number)
        return key, (match.group(2) or "").strip()

    def mapping(self, index, indent):
        result = {}
        while True:
            index = self.content(index)
            if index >= len(self.lines) or self.lines[index][0] < indent:
                break
            line_indent, body, number = self.lines[index]
            if line_indent > indent:
                raise ConvertError(f"YAML line {number}: unexpected indentation")
            pair = self.mapping_key(body, number)
            if pair is None:
                raise ConvertError(f"YAML line {number}: expected 'key: value' in this mapping")
            key, rest = pair
            if key in result:
                raise ConvertError(f"YAML line {number}: duplicate key {key!r}")
            index += 1
            if rest:
                value, index = self.inline_value(rest, number, index, indent)
            else:
                following = self.content(index)
                if following < len(self.lines) and (self.lines[following][0] > indent or
                                                    (self.lines[following][0] == indent and (self.lines[following][1] == "-" or self.lines[following][1].startswith("- ")))):
                    value, index = self.block(following, self.lines[following][0])
                else:
                    value = None
            result[key] = value
        return result, index

    def sequence(self, index, indent):
        result = []
        while True:
            index = self.content(index)
            if index >= len(self.lines) or self.lines[index][0] < indent:
                break
            line_indent, body, number = self.lines[index]
            if line_indent > indent:
                raise ConvertError(f"YAML line {number}: unexpected indentation")
            if not (body == "-" or body.startswith("- ")):
                break
            rest = body[1:].lstrip()
            if not rest:
                following = self.content(index + 1)
                if following < len(self.lines) and self.lines[following][0] > indent:
                    value, index = self.block(following, self.lines[following][0])
                else:
                    value, index = None, index + 1
            elif rest == "-" or rest.startswith("- ") or (self.mapping_key(rest, number) is not None and rest[:1] not in "[{"):
                # "- key: value" or "- - item": the item is a block that starts after the dash.
                offset = len(body) - len(rest)
                self.lines[index] = (line_indent + offset, rest, number)
                value, index = self.block(index, line_indent + offset)
            else:
                value, index = self.inline_value(rest, number, index + 1, indent)
            result.append(value)
        return result, index

    def inline_value(self, text, number, index, indent):
        """A value written after 'key:' or '- ', and the index of the next line to read."""
        self.check_node_properties(text, number)
        if text[:1] in ("|", ">") and re.match(r"^[|>][-+]?[1-9]?[-+]?$", text):
            return self.block_scalar(text, index, indent)
        if text[:1] in ("[", "{"):
            parser = FlowParser(text, number, self)
            value = parser.parse()
            return value, index
        if text[:1] in ('"', "'"):
            closing = self.quoted_end(text)
            if closing is None:
                raise ConvertError(f"YAML line {number}: quoted strings spanning several lines are not supported")
            if text[closing + 1:].strip():
                raise ConvertError(f"YAML line {number}: unexpected text after the quoted string")
            return self.quoted(text, number), index
        # Plain scalar, possibly continued on more-indented lines (folded with spaces).
        parts = [text]
        while True:
            following = self.content(index)
            if following < len(self.lines) and self.lines[following][0] > indent and not self.lines[following][1].startswith("- ") \
                    and self.mapping_key(self.lines[following][1], self.lines[following][2]) is None:
                parts.append(self.lines[following][1])
                index = following + 1
            else:
                break
        return self.resolve(" ".join(parts), number), index

    def scalar_lines(self, index, indent):
        line_indent, body, number = self.lines[index]
        return self.inline_value(body, number, index + 1, indent - 1)

    def block_scalar(self, header, index, indent):
        style, chomp = header[0], ("-" if "-" in header else "+" if "+" in header else "")
        digits = re.findall(r"[1-9]", header)
        raw = []
        while index < len(self.lines) and (self.lines[index][0] is None or self.lines[index][0] > indent):
            raw.append(index)
            index += 1
        # Block scalars keep their text: reread the original lines (comments inside are text).
        texts = []
        block_indent = None
        for position in raw:
            line_indent, body, number = self.lines[position]
            if line_indent is None:
                texts.append(None)
                continue
            if block_indent is None:
                block_indent = indent + int(digits[0]) if digits else line_indent
            texts.append(" " * (line_indent - block_indent) + body)
        while texts and texts[-1] is None:
            texts.pop()
        lines = ["" if t is None else t for t in texts]
        if style == "|":
            value = "\n".join(lines)
        else:
            value, previous = "", None
            for line in lines:
                if line == "":
                    value += "\n"
                    previous = "blank"
                elif line.startswith(" "):
                    value += ("\n" if previous == "text" else "") + line + "\n"
                    previous = "indented"
                else:
                    value += (" " if previous == "text" else "") + line
                    previous = "text"
        if chomp == "-":
            value = value.rstrip("\n")
        elif lines:
            value = value.rstrip("\n") + "\n"
        self.notes.add("block scalars are read with simplified rules: comments inside them are removed and trailing blank lines are not kept")
        return value, index

    @staticmethod
    def quoted_end(text):
        quote, end = text[0], 1
        while end < len(text):
            if quote == '"' and text[end] == "\\":
                end += 2
                continue
            if text[end] == quote:
                if quote == "'" and end + 1 < len(text) and text[end + 1] == "'":
                    end += 2
                    continue
                return end
            end += 1
        return None

    @staticmethod
    def quoted(text, number):
        if text[0] == "'":
            return text[1:-1].replace("''", "'")
        inner = text[1:-1]
        escapes = {"0": "\0", "a": "\a", "b": "\b", "t": "\t", "\t": "\t", "n": "\n", "v": "\v", "f": "\f", "r": "\r", "e": "\x1b",
                   " ": " ", '"': '"', "/": "/", "\\": "\\", "N": "\u0085", "_": " ", "L": " ", "P": " "}
        out, i = [], 0
        while i < len(inner):
            char = inner[i]
            if char != "\\":
                out.append(char)
                i += 1
                continue
            if i + 1 >= len(inner):
                raise ConvertError(f"YAML line {number}: a backslash ends the string")
            code = inner[i + 1]
            if code in escapes:
                out.append(escapes[code])
                i += 2
            elif code in "xuU":
                width = {"x": 2, "u": 4, "U": 8}[code]
                digits = inner[i + 2:i + 2 + width]
                if len(digits) != width or not re.fullmatch(r"[0-9a-fA-F]+", digits):
                    raise ConvertError(f"YAML line {number}: bad \\{code} escape")
                out.append(chr(int(digits, 16)))
                i += 2 + width
            else:
                raise ConvertError(f"YAML line {number}: unknown escape \\{code}")
        return "".join(out)

    def resolve(self, text, number):
        """YAML 1.2 core schema."""
        if text in NULLS:
            return None
        if text in TRUES:
            return True
        if text in FALSES:
            return False
        if text in YAML11_BOOLS:
            self.notes.add(f"{text!r} is a string in YAML 1.2 (the core schema used here) but a boolean in YAML 1.1 readers such as PyYAML")
        if INT_RE.match(text):
            return int(text)
        if OCT_RE.match(text):
            return int(text[2:], 8)
        if HEX_RE.match(text):
            return int(text[2:], 16)
        if FLOAT_RE.match(text):
            return float(text)
        if re.fullmatch(r"[-+]?\.(inf|Inf|INF)", text):
            self.notes.add("YAML .inf and .nan have no JSON equivalent")
            return float("-inf") if text.startswith("-") else float("inf")
        if text in (".nan", ".NaN", ".NAN"):
            return float("nan")
        if re.match(r"^\d{4}-\d{2}-\d{2}", text):
            self.notes.add("dates are kept as strings (the YAML 1.2 core schema has no timestamp type)")
        if text[:1] in ("&", "*", "!", "|", ">", "%", "@", "`"):
            self.check_node_properties(text, number)
        return text


class FlowParser:
    """Single-line flow collections: [a, b] and {k: v}, nested, with quoted or plain scalars."""

    def __init__(self, text, number, reader):
        self.text, self.number, self.reader, self.pos = text, number, reader, 0

    def error(self, message):
        raise ConvertError(f"YAML line {self.number}: {message}")

    def skip(self):
        while self.pos < len(self.text) and self.text[self.pos] == " ":
            self.pos += 1

    def parse(self):
        value = self.value()
        self.skip()
        if self.pos != len(self.text):
            self.error("unexpected text after the flow collection (flow collections must fit on one line)")
        return value

    def value(self):
        self.skip()
        if self.pos >= len(self.text):
            self.error("flow collection not closed on this line (multi-line flow collections are not supported)")
        char = self.text[self.pos]
        if char == "[":
            self.pos += 1
            items = []
            while True:
                self.skip()
                if self.pos < len(self.text) and self.text[self.pos] == "]":
                    self.pos += 1
                    return items
                items.append(self.value())
                self.skip()
                if self.pos < len(self.text) and self.text[self.pos] == ",":
                    self.pos += 1
                elif self.pos < len(self.text) and self.text[self.pos] == "]":
                    continue
                else:
                    self.error("expected ',' or ']' in a flow sequence")
        if char == "{":
            self.pos += 1
            result = {}
            while True:
                self.skip()
                if self.pos < len(self.text) and self.text[self.pos] == "}":
                    self.pos += 1
                    return result
                key = self.value()
                self.skip()
                if self.pos < len(self.text) and self.text[self.pos] == ":":
                    self.pos += 1
                    item = self.value()
                else:
                    item = None
                if isinstance(key, (dict, list)):
                    self.error("collection keys are not supported")
                key = key if isinstance(key, str) else json.dumps(key)
                if key in result:
                    self.error(f"duplicate key {key!r}")
                result[key] = item
                self.skip()
                if self.pos < len(self.text) and self.text[self.pos] == ",":
                    self.pos += 1
                elif self.pos < len(self.text) and self.text[self.pos] == "}":
                    continue
                else:
                    self.error("expected ',' or '}' in a flow mapping")
        if char in "'\"":
            end = YamlReader.quoted_end(self.text[self.pos:])
            if end is None:
                self.error("unterminated quoted string")
            token = self.text[self.pos:self.pos + end + 1]
            self.pos += end + 1
            return YamlReader.quoted(token, self.number)
        start = self.pos
        while self.pos < len(self.text) and self.text[self.pos] not in ",]}" and not (self.text[self.pos] == ":" and (self.pos + 1 == len(self.text) or self.text[self.pos + 1] in " ,]}")):
            self.pos += 1
        token = self.text[start:self.pos].strip()
        self.reader.check_node_properties(token, self.number)
        return self.reader.resolve(token, self.number)


YAML_PLAIN = re.compile(r"^[A-Za-z_/.][A-Za-z0-9_ ./@()+-]*$")


def yaml_scalar(value):
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        if value != value:
            return ".nan"
        if math.isinf(value):
            return ".inf" if value > 0 else "-.inf"
        return repr(value)
    text = str(value)
    ambiguous = text in NULLS or text in TRUES or text in FALSES or text in YAML11_BOOLS or INT_RE.match(text) or FLOAT_RE.match(text) \
        or text.startswith((".inf", ".nan", "-.inf")) or HEX_RE.match(text) or OCT_RE.match(text)
    if YAML_PLAIN.match(text) and not ambiguous and text == text.strip() and ": " not in text and " #" not in text and not text.endswith(":"):
        return text
    return json.dumps(text, ensure_ascii=False)


def yaml_key(key):
    key = str(key)
    return yaml_scalar(key) if key else '""'


def write_yaml(data, indent=0):
    pad = " " * indent
    if isinstance(data, dict):
        if not data:
            return pad + "{}\n"
        out = ""
        for key, value in data.items():
            if isinstance(value, dict) and value:
                out += f"{pad}{yaml_key(key)}:\n" + write_yaml(value, indent + 2)
            elif isinstance(value, list) and value:
                out += f"{pad}{yaml_key(key)}:\n" + write_yaml(value, indent + 2)
            else:
                out += f"{pad}{yaml_key(key)}: {yaml_inline(value)}\n"
        return out
    if isinstance(data, list):
        if not data:
            return pad + "[]\n"
        out = ""
        for item in data:
            if isinstance(item, (dict, list)) and item:
                block = write_yaml(item, indent + 2)
                out += pad + "- " + block[indent + 2:]
            else:
                out += f"{pad}- {yaml_inline(item)}\n"
        return out
    return pad + yaml_scalar(data) + "\n"


def yaml_inline(value):
    if isinstance(value, dict):
        return "{}"
    if isinstance(value, list):
        return "[]"
    return yaml_scalar(value)


# ---------------------------------------------------------------- main

def json_safe(value, notes):
    if isinstance(value, float) and not math.isfinite(value):
        notes.add("inf and nan have no JSON equivalent: written as strings")
        return "nan" if value != value else ("inf" if value > 0 else "-inf")
    if isinstance(value, dict):
        return {k: json_safe(v, notes) for k, v in value.items()}
    if isinstance(value, list):
        return [json_safe(v, notes) for v in value]
    return value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"text", "data", "from", "to", "indent"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    if ("text" in request) == ("data" in request):
        fail("give exactly one of text (with from) or data (a JSON value)")
    source = request.get("from", "json" if "data" in request else None)
    target = request.get("to")
    if source not in FORMATS:
        fail("from must be json, toml, ini or yaml")
    if target not in FORMATS:
        fail("to must be json, toml, ini or yaml")
    if "data" in request and source != "json":
        fail("data is a JSON value: from must be json (or omitted)")
    indent = request.get("indent", 2)
    if isinstance(indent, bool) or not isinstance(indent, int) or not 0 <= indent <= 8:
        fail("indent must be an integer from 0 to 8")
    notes = set()
    try:
        if "data" in request:
            data = request["data"]
        else:
            text = request["text"]
            if not isinstance(text, str):
                fail("text must be a string")
            if len(text) > 10_000_000:
                fail("text is longer than 10 MB")
            if source == "json":
                try:
                    data = json.loads(text)
                except ValueError as error:
                    raise ConvertError(f"JSON: {error}")
            elif source == "toml":
                if tomllib is None:
                    fail("reading TOML needs Python 3.11 or later (tomllib)")
                try:
                    data = toml_plain(tomllib.loads(text), notes)
                except tomllib.TOMLDecodeError as error:
                    raise ConvertError(f"TOML: {error}")
            elif source == "ini":
                data = read_ini(text)
                notes.add("INI has no types: every value is a string")
            else:
                data = YamlReader(text, notes).parse()
        if target == "json":
            data = json_safe(data, notes)
            output = json.dumps(data, indent=indent or None, ensure_ascii=False) + "\n"
        elif target == "toml":
            output = write_toml(data)
            if source == "json" or source == "yaml":
                notes.add("TOML cannot hold null; strings that look like dates stay strings")
        elif target == "ini":
            output = write_ini(data, notes)
        else:
            output = write_yaml(data)
    except ConvertError as error:
        fail(str(error))
    except RecursionError:
        fail("the input is nested too deeply")
    check = None
    if target == "toml" and tomllib is not None:
        check = toml_plain(tomllib.loads(output), set()) == json_safe(data, set())
    elif target == "yaml":
        try:
            check = YamlReader(output, set()).parse() == data
        except ConvertError:
            check = False
    elif target == "json":
        check = True
    result = {"from": source, "to": target, "text": output, "round_trip_ok": check, "round_trip_notes": sorted(notes)}
    if target == "json":
        result["data"] = json.loads(output)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
