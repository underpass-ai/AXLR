#!/usr/bin/env python3
"""Outline an OpenAPI 3.0/3.1 or Swagger 2.0 document given inline as JSON or YAML: endpoints,
methods, operationIds, parameters, request bodies and responses, with $ref resolved inside the
document, as a JSON outline and as Markdown.

Input (JSON on stdin):
  text     the document (JSON, or the YAML subset described in the README)
  format   "both" (default), "json" or "markdown"
"""
import json
import re
import sys

MAX_TEXT = 2_000_000
METHODS = ["get", "put", "post", "delete", "options", "head", "patch", "trace"]


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


# ---- a YAML subset ------------------------------------------------------------------------

class YamlError(Exception):
    pass


def strip_comment(s):
    """Remove a trailing # comment that is outside quotes."""
    quote = None
    i = 0
    while i < len(s):
        ch = s[i]
        if quote == "'":
            if ch == "'":
                if i + 1 < len(s) and s[i + 1] == "'":
                    i += 2
                    continue
                quote = None
        elif quote == '"':
            if ch == "\\":
                i += 2
                continue
            if ch == '"':
                quote = None
        else:
            if ch in "'\"" and (i == 0 or s[i - 1] in " \t[{,:"):
                quote = ch
            elif ch == "#" and (i == 0 or s[i - 1] in " \t"):
                return s[:i].rstrip()
        i += 1
    return s.rstrip()


PLAIN_INT = re.compile(r"[-+]?[0-9]+$")
PLAIN_FLOAT = re.compile(r"[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$")


def resolve_plain(s):
    if s in ("", "~", "null", "Null", "NULL"):
        return None
    if s in ("true", "True", "TRUE"):
        return True
    if s in ("false", "False", "FALSE"):
        return False
    if PLAIN_INT.match(s):
        return int(s)
    if re.fullmatch(r"0x[0-9a-fA-F]+", s):
        return int(s, 16)
    if re.fullmatch(r"0o[0-7]+", s):
        return int(s[2:], 8)
    if PLAIN_FLOAT.match(s) and any(c.isdigit() for c in s):
        return float(s)
    return s


ESCAPES = {"n": "\n", "t": "\t", "r": "\r", "0": "\0", "\\": "\\", '"': '"', "/": "/", " ": " ", "b": "\b",
           "f": "\f", "e": "\x1b", "a": "\a", "v": "\v", "N": "\u0085", "_": " "}


def unquote(s, where):
    """s starts with a quote; return (value, rest after the closing quote)."""
    q = s[0]
    out, i = [], 1
    while i < len(s):
        ch = s[i]
        if q == "'" and ch == "'":
            if i + 1 < len(s) and s[i + 1] == "'":
                out.append("'")
                i += 2
                continue
            return "".join(out), s[i + 1:]
        if q == '"' and ch == "\\":
            nxt = s[i + 1:i + 2]
            if nxt in ESCAPES:
                out.append(ESCAPES[nxt])
                i += 2
                continue
            m = re.match(r"(x[0-9a-fA-F]{2}|u[0-9a-fA-F]{4}|U[0-9a-fA-F]{8})", s[i + 1:])
            if m:
                out.append(chr(int(m.group(1)[1:], 16)))
                i += 1 + len(m.group(1))
                continue
            raise YamlError(f"{where}: unknown escape \\{nxt}")
        if q == '"' and ch == '"':
            return "".join(out), s[i + 1:]
        out.append(ch)
        i += 1
    raise YamlError(f"{where}: unterminated quoted string")


def refuse_special(s, where):
    if s[:1] == "&":
        raise YamlError(f"{where}: anchors (&name) are not supported; expand them")
    if s[:1] == "*":
        raise YamlError(f"{where}: aliases (*name) are not supported; expand them")
    if s[:1] == "!":
        raise YamlError(f"{where}: tags (!tag) are not supported")
    if s[:1] in ("%", "@", "`"):
        raise YamlError(f"{where}: a plain value cannot start with {s[0]}")


class Flow:
    """Flow collections on one logical line: [a, b], {k: v}."""

    def __init__(self, text, where):
        self.s, self.i, self.where = text, 0, where

    def ws(self):
        while self.i < len(self.s) and self.s[self.i] in " \t\n":
            self.i += 1

    def value(self):
        self.ws()
        if self.i >= len(self.s):
            raise YamlError(f"{self.where}: unexpected end of a flow collection")
        ch = self.s[self.i]
        if ch == "[":
            self.i += 1
            items = []
            while True:
                self.ws()
                if self.s[self.i:self.i + 1] == "]":
                    self.i += 1
                    return items
                items.append(self.value())
                self.ws()
                if self.s[self.i:self.i + 1] == ",":
                    self.i += 1
                elif self.s[self.i:self.i + 1] != "]":
                    raise YamlError(f"{self.where}: expected , or ] in a flow sequence")
        if ch == "{":
            self.i += 1
            out = {}
            while True:
                self.ws()
                if self.s[self.i:self.i + 1] == "}":
                    self.i += 1
                    return out
                key = self.scalar(key=True)
                self.ws()
                if self.s[self.i:self.i + 1] == ":":
                    self.i += 1
                    val = self.value()
                else:
                    val = None
                out[str(key) if not isinstance(key, str) else key] = val
                self.ws()
                if self.s[self.i:self.i + 1] == ",":
                    self.i += 1
                elif self.s[self.i:self.i + 1] != "}":
                    raise YamlError(f"{self.where}: expected , or }} in a flow mapping")
        return self.scalar()

    def scalar(self, key=False):
        self.ws()
        rest = self.s[self.i:]
        if rest[:1] in ("'", '"'):
            value, after = unquote(rest, self.where)
            self.i = len(self.s) - len(after)
            return value
        refuse_special(rest, self.where)
        m = re.match(r"([^,\[\]{}]*?)(?=\s*(?:,|\]|\}|:\s|:$)|$)" if key else r"([^,\[\]{}]*?)(?=\s*(?:,|\]|\})|$)", rest)
        text = m.group(1).strip()
        self.i += len(m.group(1))
        return resolve_plain(text)


class Yaml:
    def __init__(self, text):
        self.raw = text.replace("\r\n", "\n").replace("\r", "\n").split("\n")
        self.pos = 0
        self.override = None

    def where(self, pos=None):
        return f"YAML line {(self.pos if pos is None else pos) + 1}"

    def line(self, pos):
        if self.override and self.override[0] == pos:
            return self.override[1], self.override[2]
        raw = self.raw[pos]
        stripped = raw.lstrip(" ")
        if stripped.startswith("\t") or (raw[:len(raw) - len(stripped)].count("\t")):
            raise YamlError(f"{self.where(pos)}: tabs are not allowed for indentation")
        return len(raw) - len(stripped), strip_comment(stripped)

    def peek(self):
        while self.pos < len(self.raw):
            indent, content = self.line(self.pos)
            if content == "":
                self.pos += 1
                continue
            if indent == 0 and content in ("---", "..."):
                if content == "---" and self.pos > self.first_content:
                    raise YamlError(f"{self.where()}: several YAML documents in one text are not supported")
                self.pos += 1
                continue
            if indent == 0 and content.startswith("%"):
                raise YamlError(f"{self.where()}: YAML directives are not supported")
            return indent, content
        return None

    def advance(self):
        self.pos += 1
        self.override = None

    def parse(self):
        self.first_content = 0
        while self.first_content < len(self.raw) and (not self.raw[self.first_content].strip()
                                                      or self.raw[self.first_content].lstrip().startswith("#")):
            self.first_content += 1
        top = self.peek()
        if top is None:
            return None
        value = self.block(top[0])
        if self.peek() is not None:
            raise YamlError(f"{self.where()}: unexpected content (check the indentation)")
        return value

    @staticmethod
    def is_item(content):
        return content == "-" or content.startswith("- ")

    def key_split(self, content):
        if content.startswith("? "):
            raise YamlError(f"{self.where()}: complex keys (? ) are not supported")
        if content[:1] in ("'", '"'):
            key, rest = unquote(content, self.where())
            rest = rest.lstrip(" ")
            if not rest.startswith(":") or (len(rest) > 1 and rest[1] != " "):
                return None
            return key, rest[1:].strip()
        refuse_special(content, self.where())
        m = re.match(r"^([^#]*?)\s*:(?:\s+(.*))?$", content)
        if not m or m.group(1) == "" or m.group(1)[0] in "[{":
            return None
        key = resolve_plain(m.group(1))
        return ("null" if key is None else str(key).lower() if isinstance(key, bool) else str(key)), (m.group(2) or "").strip()

    def block(self, indent):
        ln = self.peek()
        if ln is None or ln[0] < indent:
            return None
        if self.is_item(ln[1]):
            return self.seq(ln[0])
        if self.key_split(ln[1]) is None:
            # a lone scalar or flow value on its own line
            self.advance()
            return self.scalar_value(ln[1], ln[0] - 1)
        return self.mapping(ln[0])

    def seq(self, indent):
        items = []
        while True:
            ln = self.peek()
            if ln is None or ln[0] < indent:
                break
            if ln[0] > indent:
                raise YamlError(f"{self.where()}: bad indentation in a sequence")
            if not self.is_item(ln[1]):
                break
            rest = ln[1][1:].lstrip(" ")
            if rest == "":
                self.advance()
                nxt = self.peek()
                items.append(self.block(indent + 1) if nxt and nxt[0] > indent else None)
                continue
            col = indent + len(ln[1]) - len(rest)
            if self.is_item(rest) or self.key_split(rest) is not None:
                self.override = (self.pos, col, rest)
                items.append(self.block(col))
            else:
                self.advance()
                items.append(self.scalar_value(rest, indent))
        return items

    def mapping(self, indent):
        out = {}
        while True:
            ln = self.peek()
            if ln is None or ln[0] < indent:
                break
            if ln[0] > indent:
                raise YamlError(f"{self.where()}: bad indentation (expected column {indent + 1})")
            if self.is_item(ln[1]):
                break
            split = self.key_split(ln[1])
            if split is None:
                raise YamlError(f"{self.where()}: expected 'key: value'")
            key, rest = split
            if key in out:
                raise YamlError(f"{self.where()}: duplicate key {key!r}")
            self.advance()
            if rest == "":
                nxt = self.peek()
                if nxt and (nxt[0] > indent or (nxt[0] == indent and self.is_item(nxt[1]))):
                    out[key] = self.seq(indent) if nxt[0] == indent else self.block(nxt[0])
                else:
                    out[key] = None
            elif rest[0] in "|>":
                out[key] = self.block_scalar(rest, indent)
            else:
                out[key] = self.scalar_value(rest, indent)
        return out

    def scalar_value(self, rest, indent):
        where = self.where(self.pos - 1)
        refuse_special(rest, where)
        if rest[0] in "[{":
            text = rest
            while not balanced(text):
                if self.pos >= len(self.raw):
                    raise YamlError(f"{where}: unterminated flow collection")
                text += " " + strip_comment(self.raw[self.pos].strip())
                self.pos += 1
            f = Flow(text, where)
            value = f.value()
            f.ws()
            if f.i != len(f.s):
                raise YamlError(f"{where}: unexpected text after a flow collection")
            return value
        if rest[0] in "'\"":
            text, blank = rest, False
            while True:
                try:
                    value, after = unquote(text, where)
                    break
                except YamlError as error:
                    if "unterminated" not in str(error) or self.pos >= len(self.raw):
                        raise
                    nxt = self.raw[self.pos].strip()
                    self.pos += 1
                    if not nxt:
                        text += "\n"
                        blank = True
                    else:
                        text += ("" if blank else " ") + nxt
                        blank = False
            if strip_comment(after).strip():
                raise YamlError(f"{where}: unexpected text after a quoted string")
            return value
        parts = [rest]
        while self.pos < len(self.raw):
            raw = self.raw[self.pos]
            if not raw.strip():
                break
            ind = len(raw) - len(raw.lstrip(" "))
            if ind <= indent:
                break
            parts.append(strip_comment(raw.strip()))
            self.pos += 1
        text = " ".join(p for p in parts if p)
        return resolve_plain(text) if len(parts) == 1 else text

    def block_scalar(self, header, indent):
        m = re.fullmatch(r"([|>])([1-9]?)([+-]?)([1-9]?)\s*(#.*)?", header)
        if not m:
            raise YamlError(f"{self.where(self.pos - 1)}: bad block scalar header {header!r}")
        style, chomp = m.group(1), m.group(3)
        explicit = m.group(2) or m.group(4)
        lines = []
        content_indent = indent + int(explicit) if explicit else None
        while self.pos < len(self.raw):
            raw = self.raw[self.pos]
            if raw.strip() == "":
                lines.append("")
                self.pos += 1
                continue
            ind = len(raw) - len(raw.lstrip(" "))
            if content_indent is None:
                if ind <= indent:
                    break
                content_indent = ind
            if ind < content_indent:
                break
            lines.append(raw[content_indent:])
            self.pos += 1
        trailing = 0
        while lines and lines[-1] == "":
            lines.pop()
            trailing += 1
        if style == "|":
            body = "\n".join(lines)
        else:
            out = []
            for i, l in enumerate(lines):
                prev = lines[i - 1] if i else None
                if i == 0:
                    out.append(l)
                elif l == "":
                    out.append("\n")
                elif prev == "":
                    out.append(l)
                elif prev.startswith(" ") or l.startswith(" "):
                    out.append("\n" + l)
                else:
                    out.append(" " + l)
            body = "".join(out)
        if not lines:
            return ""
        if chomp == "-":
            return body
        if chomp == "+":
            return body + "\n" * (1 + trailing)
        return body + "\n"


def balanced(text):
    depth, quote, i = 0, None, 0
    while i < len(text):
        ch = text[i]
        if quote:
            if ch == "\\" and quote == '"':
                i += 2
                continue
            if ch == quote:
                quote = None
        elif ch in "'\"":
            quote = ch
        elif ch in "[{":
            depth += 1
        elif ch in "]}":
            depth -= 1
        i += 1
    return depth <= 0


def parse_document(text):
    stripped = text.lstrip()
    if stripped[:1] in ("{", "["):
        try:
            return json.loads(text), "json"
        except ValueError:
            pass  # maybe a YAML flow document
    try:
        return Yaml(text).parse(), "yaml"
    except YamlError as error:
        fail(f"cannot read the document: {error}")
    except RecursionError:
        fail("the document is nested too deeply")


# ---- OpenAPI ------------------------------------------------------------------------------

class Outline:
    def __init__(self, doc):
        self.doc = doc
        self.warnings = []
        self.unresolved = set()

    def resolve(self, node, seen=()):
        """Follow $ref chains inside the document; returns (node, ref name or None)."""
        name = None
        hops = 0
        while isinstance(node, dict) and isinstance(node.get("$ref"), str):
            ref = node["$ref"]
            if ref in seen or hops > 50:
                self.warnings.append(f"circular $ref {ref}")
                return {}, name
            seen = seen + (ref,)
            if not ref.startswith("#"):
                self.unresolved.add(ref)
                return node, ref
            target = self.doc
            for token in ref[1:].split("/")[1:]:
                token = token.replace("~1", "/").replace("~0", "~")
                token = re.sub(r"%([0-9A-Fa-f]{2})", lambda m: chr(int(m.group(1), 16)), token)
                if isinstance(target, dict) and token in target:
                    target = target[token]
                elif isinstance(target, list) and token.isdigit() and int(token) < len(target):
                    target = target[int(token)]
                else:
                    self.unresolved.add(ref)
                    return node, ref
            name = name or ref.rsplit("/", 1)[-1]
            node = target
            hops += 1
        return node, name

    def type_of(self, schema, depth=0):
        """A compact type string: Pet, array<Pet>, string(date-time), object{a,b}, oneOf<A|B>."""
        if schema is None:
            return None
        if isinstance(schema, bool):
            return "any" if schema else "never"
        if not isinstance(schema, dict):
            return "?"
        if "$ref" in schema:
            target, name = self.resolve(schema)
            return name if name else "?"
        if depth > 6:
            return "…"
        for key in ("oneOf", "anyOf", "allOf"):
            if isinstance(schema.get(key), list):
                return f"{key}<" + "|".join(self.type_of(s, depth + 1) or "?" for s in schema[key]) + ">"
        t = schema.get("type")
        if isinstance(t, list):
            t = "|".join(str(x) for x in t)
        if t == "array" or (t is None and "items" in schema):
            return f"array<{self.type_of(schema.get('items'), depth + 1) or 'any'}>"
        if t == "object" or (t is None and "properties" in schema):
            props = schema.get("properties") or {}
            if props:
                return "object{" + ",".join(props) + "}"
            add = schema.get("additionalProperties")
            if isinstance(add, dict):
                return f"map<string,{self.type_of(add, depth + 1)}>"
            return "object"
        text = str(t) if t else "any"
        if schema.get("format"):
            text += f"({schema['format']})"
        if isinstance(schema.get("enum"), list):
            text += " enum[" + ",".join(json.dumps(v) if not isinstance(v, str) else v for v in schema["enum"][:12]) + "]"
        if schema.get("nullable") is True:
            text += "|null"
        return text

    def parameter(self, raw, swagger2):
        p, ref = self.resolve(raw)
        if not isinstance(p, dict):
            return None
        out = {"name": p.get("name"), "in": p.get("in"), "required": bool(p.get("required", p.get("in") == "path"))}
        if swagger2 and p.get("in") != "body":
            t = p.get("type")
            out["type"] = (f"array<{self.type_of(p.get('items'))}>" if t == "array" else
                           f"{t}({p['format']})" if p.get("format") else t)
        elif "schema" in p:
            out["type"] = self.type_of(p["schema"])
        elif isinstance(p.get("content"), dict) and p["content"]:
            media = next(iter(p["content"].values())) or {}
            out["type"] = self.type_of(media.get("schema"))
        if p.get("description"):
            out["description"] = first_line(p["description"])
        if p.get("deprecated"):
            out["deprecated"] = True
        if ref:
            out["ref"] = ref
        return out

    def content(self, content):
        out = {}
        if isinstance(content, dict):
            for media, body in content.items():
                body = body if isinstance(body, dict) else {}
                out[media] = self.type_of(body.get("schema"))
        return out

    def build(self):
        doc = self.doc
        if not isinstance(doc, dict):
            fail("the document is not an object: not an OpenAPI description")
        if "openapi" in doc:
            version, swagger2 = str(doc["openapi"]), False
            if not version.startswith("3."):
                self.warnings.append(f"openapi {version}: only 3.0 and 3.1 are known; read as 3.x")
        elif "swagger" in doc:
            version, swagger2 = str(doc["swagger"]), True
            if version != "2.0":
                self.warnings.append(f"swagger {version}: read as 2.0")
        else:
            fail("no 'openapi' or 'swagger' field: not an OpenAPI or Swagger document")
        info = doc.get("info") if isinstance(doc.get("info"), dict) else {}
        outline = {"spec": ("Swagger " if swagger2 else "OpenAPI ") + version,
                   "title": info.get("title"), "version": None if info.get("version") is None else str(info.get("version"))}
        if info.get("description"):
            outline["description"] = first_line(info["description"])
        if swagger2:
            schemes = doc.get("schemes") or ["https"]
            if doc.get("host"):
                outline["servers"] = [f"{s}://{doc['host']}{doc.get('basePath', '')}" for s in schemes]
            elif doc.get("basePath"):
                outline["servers"] = [doc["basePath"]]
        elif isinstance(doc.get("servers"), list):
            outline["servers"] = [s.get("url") for s in doc["servers"] if isinstance(s, dict)]
        operations, ids = [], {}
        paths = doc.get("paths") or {}
        if not isinstance(paths, dict):
            fail("paths must be an object")
        hooks = doc.get("webhooks") if not swagger2 and isinstance(doc.get("webhooks"), dict) else {}
        for path, item, webhook in [(p, i, False) for p, i in paths.items()] + [(n, i, True) for n, i in hooks.items()]:
            item, _ = self.resolve(item)
            if not isinstance(item, dict):
                continue
            shared = [x for x in (self.parameter(p, swagger2) for p in as_list(item.get("parameters"))) if x]
            for method in METHODS:
                op = item.get(method)
                if not isinstance(op, dict):
                    continue
                own = [x for x in (self.parameter(p, swagger2) for p in as_list(op.get("parameters"))) if x]
                keys = {(p["name"], p["in"]) for p in own}
                params = [p for p in shared if (p["name"], p["in"]) not in keys] + own
                entry = {"method": method.upper(), "path": path, "operationId": text_or_none(op.get("operationId")),
                         "summary": text_or_none(op.get("summary")) or (first_line(op["description"]) if op.get("description") else None),
                         "tags": [str(t) for t in as_list(op.get("tags"))]}
                if op.get("deprecated"):
                    entry["deprecated"] = True
                if webhook:
                    entry["webhook"] = True
                body = None
                if swagger2:
                    body_params = [p for p in params if p["in"] == "body"]
                    form = [p for p in params if p["in"] == "formData"]
                    consumes = op.get("consumes") or doc.get("consumes") or ["application/json"]
                    if body_params:
                        body = {"required": body_params[0]["required"], "content": {c: body_params[0].get("type") for c in consumes}}
                    elif form:
                        body = {"required": any(p["required"] for p in form),
                                "content": {c: "object{" + ",".join(p["name"] for p in form) + "}" for c in consumes}}
                    params = [p for p in params if p["in"] not in ("body", "formData")]
                elif "requestBody" in op:
                    rb, ref = self.resolve(op["requestBody"])
                    if isinstance(rb, dict):
                        body = {"required": bool(rb.get("required", False)), "content": self.content(rb.get("content"))}
                        if ref:
                            body["ref"] = ref
                entry["parameters"] = params
                entry["request_body"] = body
                responses = {}
                for code, resp in as_dict(op.get("responses")).items():
                    resp, ref = self.resolve(resp)
                    if not isinstance(resp, dict):
                        continue
                    r = {"description": first_line(resp.get("description") or "")}
                    if swagger2:
                        if "schema" in resp:
                            produces = op.get("produces") or doc.get("produces") or ["application/json"]
                            r["content"] = {c: self.type_of(resp["schema"]) for c in produces}
                    elif resp.get("content"):
                        r["content"] = self.content(resp["content"])
                    if as_dict(resp.get("headers")):
                        r["headers"] = list(resp["headers"])
                    if ref:
                        r["ref"] = ref
                    responses[str(code)] = r
                entry["responses"] = responses
                if entry["operationId"]:
                    ids.setdefault(entry["operationId"], []).append(f"{method.upper()} {path}")
                else:
                    self.warnings.append(f"{method.upper()} {path} has no operationId")
                operations.append(entry)
        for op_id, where in ids.items():
            if len(where) > 1:
                self.warnings.append(f"operationId {op_id} is used {len(where)} times: {', '.join(where)}")
        outline["paths"] = list(paths)
        if hooks:
            outline["webhooks"] = list(hooks)
        outline["operation_count"] = len(operations)
        outline["operations"] = operations
        schemas = as_dict(doc.get("definitions") if swagger2 else as_dict(doc.get("components")).get("schemas"))
        outline["schemas"] = {}
        for name, schema in schemas.items():
            target, _ = self.resolve(schema)
            entry = {"type": self.type_of(schema) if isinstance(schema, dict) and "$ref" in schema else self.type_of(target)}
            if isinstance(target, dict):
                if isinstance(target.get("required"), list) and target["required"]:
                    entry["required"] = [str(x) for x in target["required"] if isinstance(x, (str, int, float))]
                if isinstance(target.get("properties"), dict):
                    entry["properties"] = {k: self.type_of(v) for k, v in target["properties"].items()}
            outline["schemas"][name] = entry
        tags = {}
        for op in operations:
            for t in op["tags"] or ["(untagged)"]:
                tags[t] = tags.get(t, 0) + 1
        outline["tags"] = tags
        if self.unresolved:
            self.warnings.append("unresolved or external $ref (not followed): " + ", ".join(sorted(self.unresolved)))
        outline["warnings"] = self.warnings
        return outline


def as_dict(v):
    return v if isinstance(v, dict) else {}


def as_list(v):
    return v if isinstance(v, list) else []


def text_or_none(v):
    return v if isinstance(v, str) else (None if v is None else json.dumps(v))


def first_line(text):
    text = str(text).strip()
    return text.split("\n", 1)[0][:300]


def markdown(o):
    lines = [f"# {o.get('title') or 'API'} {o.get('version') or ''}".rstrip(), "", f"{o['spec']}"]
    if o.get("description"):
        lines += ["", o["description"]]
    if o.get("servers"):
        lines += ["", "Servers: " + ", ".join(f"`{s}`" for s in o["servers"])]
    lines += ["", f"## Endpoints ({o['operation_count']})", "",
              "| Method | Path | operationId | Summary |", "|:--|:--|:--|:--|"]
    for op in o["operations"]:
        cell = lambda v: (v or "").replace("|", "\\|")
        where = f"webhook {op['path']}" if op.get("webhook") else f"`{op['path']}`"
        lines.append(f"| {op['method']} | {where} | {cell(op['operationId'])} | {cell(op['summary'])} |")
    for op in o["operations"]:
        lines += ["", f"### {op['method']} {op['path']}" + (" (deprecated)" if op.get("deprecated") else "")]
        if op.get("summary"):
            lines.append(op["summary"])
        if op["parameters"]:
            lines += ["", "Parameters:"]
            for p in op["parameters"]:
                lines.append(f"- `{p['name']}` ({p['in']}{', required' if p['required'] else ''}): {p.get('type') or 'any'}")
        if op["request_body"]:
            rb = op["request_body"]
            lines += ["", "Request body" + (" (required)" if rb["required"] else "") + ": " +
                      ", ".join(f"{k} {v}" for k, v in rb["content"].items())]
        if op["responses"]:
            lines += ["", "Responses:"]
            for code, r in op["responses"].items():
                content = ", ".join(f"{k} {v}" for k, v in r.get("content", {}).items())
                lines.append(f"- {code}: {r['description']}" + (f" ({content})" if content else ""))
    if o["schemas"]:
        lines += ["", f"## Schemas ({len(o['schemas'])})", ""]
        for name, s in o["schemas"].items():
            props = s.get("properties")
            req = set(s.get("required") or [])
            detail = ", ".join(f"{k}{'*' if k in req else ''}: {v}" for k, v in props.items()) if props else s["type"]
            lines.append(f"- **{name}**: {detail}")
    if o["warnings"]:
        lines += ["", "## Warnings", ""] + [f"- {w}" for w in o["warnings"]]
    return "\n".join(lines) + "\n"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - {"text", "format"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be the OpenAPI document (JSON or YAML) as a string")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    fmt = request.get("format", "both")
    if fmt not in ("both", "json", "markdown"):
        fail("format must be both, json or markdown")
    doc, syntax = parse_document(text)
    try:
        outline = Outline(doc).build()
    except (AttributeError, TypeError, KeyError, ValueError) as error:
        fail(f"the document does not have the structure OpenAPI expects somewhere ({type(error).__name__}: {error})")
    outline["syntax"] = syntax
    result = {}
    if fmt in ("both", "json"):
        result["outline"] = outline
    if fmt in ("both", "markdown"):
        try:
            result["markdown"] = markdown(outline)
        except (AttributeError, TypeError, KeyError, ValueError) as error:
            fail(f"the document does not have the structure OpenAPI expects somewhere ({type(error).__name__}: {error})")
    if fmt == "markdown":
        result["operation_count"] = outline["operation_count"]
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
