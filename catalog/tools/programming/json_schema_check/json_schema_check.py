#!/usr/bin/env python3
"""Validate a JSON instance against a JSON Schema (a draft 2020-12 subset) and list every
error with its instance path and schema path. Keywords outside the subset are reported.

Input (JSON on stdin):
  schema         the schema (object or boolean), or schema_text: the schema as a JSON string
  instance       the instance, or instance_text: the instance as a JSON string
  formats        "annotate" (default: format failures are warnings, as 2020-12 says) or "assert"
  max_errors     stop listing after this many errors (default 100)
"""
import datetime as dt
import json
import re
import signal
import sys
from fractions import Fraction

SUPPORTED = {
    "type", "enum", "const", "properties", "required", "additionalProperties", "patternProperties",
    "propertyNames", "minProperties", "maxProperties", "dependentRequired",
    "items", "prefixItems", "contains", "minContains", "maxContains", "minItems", "maxItems", "uniqueItems",
    "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
    "minLength", "maxLength", "pattern", "format",
    "allOf", "anyOf", "oneOf", "not", "if", "then", "else", "$ref",
}
CONTAINERS = {"$defs", "definitions"}
ANNOTATIONS = {"$schema", "$id", "$comment", "title", "description", "default", "examples", "deprecated",
               "readOnly", "writeOnly", "$vocabulary"}
KNOWN_UNSUPPORTED = {
    "unevaluatedProperties", "unevaluatedItems", "dependentSchemas", "$dynamicRef", "$dynamicAnchor",
    "$anchor", "$recursiveRef", "$recursiveAnchor", "contentEncoding", "contentMediaType", "contentSchema",
    "additionalItems", "dependencies"}
FORMATS = {"date", "date-time", "time", "email", "uri", "uuid", "ipv4"}
TIME_LIMIT = 10
MAX_INPUT = 2_000_000


def reject_constant(name):
    raise ValueError(f"{name} is not valid JSON")


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


class Timeout(Exception):
    pass


def escape(token):
    return str(token).replace("~", "~0").replace("/", "~1")


def ptr(path):
    return "".join("/" + escape(p) for p in path)


def json_type(v):
    if v is None:
        return "null"
    if isinstance(v, bool):
        return "boolean"
    if isinstance(v, int):
        return "integer"
    if isinstance(v, float):
        return "integer" if v.is_integer() else "number"
    if isinstance(v, str):
        return "string"
    if isinstance(v, list):
        return "array"
    return "object"


def is_number(v):
    return isinstance(v, (int, float)) and not isinstance(v, bool)


def equal(a, b):
    """JSON equality: 1 == 1.0, but true != 1; objects by key, arrays by position."""
    if is_number(a) and is_number(b):
        return a == b
    if type(a) is not type(b) and not (is_number(a) and is_number(b)):
        return False
    if isinstance(a, list):
        return len(a) == len(b) and all(equal(x, y) for x, y in zip(a, b))
    if isinstance(a, dict):
        return a.keys() == b.keys() and all(equal(a[k], b[k]) for k in a)
    return a == b


def frac(v):
    return Fraction(str(v)) if isinstance(v, float) else Fraction(v)


def check_format(name, v):
    if not isinstance(v, str):
        return True
    try:
        if name == "date":
            return bool(re.fullmatch(r"\d{4}-\d{2}-\d{2}", v)) and bool(dt.date.fromisoformat(v))
        if name in ("date-time", "time"):
            body = v if name == "date-time" else "2000-01-01T" + v
            m = re.fullmatch(r"(\d{4}-\d{2}-\d{2})[Tt](\d{2}):(\d{2}):(\d{2})(\.\d+)?([Zz]|[+-]\d{2}:\d{2})", body)
            if not m:
                return False
            dt.date.fromisoformat(m.group(1))
            h, mi, s = int(m.group(2)), int(m.group(3)), int(m.group(4))
            return h <= 23 and mi <= 59 and s <= 60
        if name == "email":
            return bool(re.fullmatch(r"[^@\s\"(),:;<>\[\\\]]+@[A-Za-z0-9](?:[A-Za-z0-9\-]*[A-Za-z0-9])?"
                                     r"(?:\.[A-Za-z0-9](?:[A-Za-z0-9\-]*[A-Za-z0-9])?)+", v))
        if name == "uri":
            return bool(re.fullmatch(r"[A-Za-z][A-Za-z0-9+.\-]*:[^\s<>\"{}|\\^`]*", v))
        if name == "uuid":
            return bool(re.fullmatch(r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}", v))
        if name == "ipv4":
            parts = v.split(".")
            return len(parts) == 4 and all(re.fullmatch(r"0|[1-9]\d{0,2}", p) and int(p) <= 255 for p in parts)
    except ValueError:
        return False
    return True


class Validator:
    def __init__(self, root, formats, max_errors):
        self.root, self.formats, self.max_errors = root, formats, max_errors
        self.unsupported, self.unknown, self.warnings = {}, {}, []
        self.regex = {}
        self.ref_stack = []

    def rx(self, pattern, spath):
        if pattern not in self.regex:
            try:
                self.regex[pattern] = re.compile(pattern)
            except re.error as error:
                fail(f"schema {ptr(spath)}: invalid regular expression {pattern!r}: {error}")
        return self.regex[pattern]

    def resolve(self, ref, spath):
        if not ref.startswith("#"):
            return None
        target = self.root
        frag = ref[1:]
        if frag and not frag.startswith("/"):
            return None  # an anchor name: not supported
        for raw in frag.split("/")[1:]:
            token = re.sub(r"%([0-9A-Fa-f]{2})", lambda m: chr(int(m.group(1), 16)), raw)
            token = token.replace("~1", "/").replace("~0", "~")
            if isinstance(target, dict) and token in target:
                target = target[token]
            elif isinstance(target, list) and token.isdigit() and int(token) < len(target):
                target = target[int(token)]
            else:
                fail(f"schema {ptr(spath)}: $ref {ref!r} points to nothing in this document")
        return target

    def validate(self, schema, inst, ipath, spath):
        """Return a list of errors (dicts)."""
        if schema is True:
            return []
        if schema is False:
            return [self.err(ipath, spath, "false", "the schema false accepts nothing")]
        if not isinstance(schema, dict):
            fail(f"schema {ptr(spath) or '(root)'} must be an object or a boolean")
        errors = []
        for key in schema:
            if key in SUPPORTED or key in CONTAINERS or key in ANNOTATIONS:
                continue
            where = ptr(spath + [key])
            if key in KNOWN_UNSUPPORTED:
                self.unsupported.setdefault(key, where)
            else:
                self.unknown.setdefault(key, where)
        if "$ref" in schema:
            ref = schema["$ref"]
            if not isinstance(ref, str):
                fail(f"schema {ptr(spath + ['$ref'])} must be a string")
            target = self.resolve(ref, spath + ["$ref"])
            if target is None:
                self.unsupported.setdefault("$ref (non-local: " + ref + ")", ptr(spath + ["$ref"]))
            else:
                marker = (ref, ptr(ipath))
                if marker in self.ref_stack:
                    fail(f"schema {ptr(spath + ['$ref'])}: $ref {ref!r} loops without consuming the instance")
                self.ref_stack.append(marker)
                errors += self.validate(target, inst, ipath, spath + ["$ref"])
                self.ref_stack.pop()
        for key, check in (("type", self.k_type), ("enum", self.k_enum), ("const", self.k_const)):
            if key in schema:
                errors += check(schema[key], inst, ipath, spath + [key])
        if is_number(inst):
            errors += self.numeric(schema, inst, ipath, spath)
        if isinstance(inst, str):
            errors += self.string(schema, inst, ipath, spath)
        if isinstance(inst, list):
            errors += self.array(schema, inst, ipath, spath)
        if isinstance(inst, dict):
            errors += self.obj(schema, inst, ipath, spath)
        errors += self.logic(schema, inst, ipath, spath)
        return errors

    def err(self, ipath, spath, keyword, message, **extra):
        return dict({"instance_path": ptr(ipath), "schema_path": ptr(spath), "keyword": keyword, "message": message}, **extra)

    def k_type(self, want, inst, ipath, spath):
        types = want if isinstance(want, list) else [want]
        valid = {"null", "boolean", "object", "array", "number", "string", "integer"}
        if not types or not all(isinstance(t, str) and t in valid for t in types):
            fail(f"schema {ptr(spath)}: type must be one of {', '.join(sorted(valid))} or a list of them")
        actual = json_type(inst)
        ok = actual in types or (actual == "integer" and "number" in types)
        return [] if ok else [self.err(ipath, spath, "type", f"{actual} is not of type {' or '.join(types)}")]

    def k_enum(self, values, inst, ipath, spath):
        if not isinstance(values, list):
            fail(f"schema {ptr(spath)}: enum must be an array")
        if any(equal(v, inst) for v in values):
            return []
        return [self.err(ipath, spath, "enum", f"{json.dumps(inst)[:80]} is not one of {json.dumps(values)[:200]}")]

    def k_const(self, value, inst, ipath, spath):
        return [] if equal(value, inst) else [self.err(ipath, spath, "const", f"value must be {json.dumps(value)[:200]}")]

    def number_kw(self, schema, key, spath):
        v = schema[key]
        if isinstance(v, bool) and key.startswith("exclusive"):
            self.unsupported.setdefault(f"{key} as a boolean (draft 4)", ptr(spath + [key]))
            return None
        if not is_number(v):
            fail(f"schema {ptr(spath + [key])} must be a number")
        return frac(v)

    def numeric(self, schema, inst, ipath, spath):
        errors = []
        x = frac(inst)
        tests = {"minimum": (lambda b: x >= b, "less than the minimum"),
                 "maximum": (lambda b: x <= b, "greater than the maximum"),
                 "exclusiveMinimum": (lambda b: x > b, "not greater than the exclusive minimum"),
                 "exclusiveMaximum": (lambda b: x < b, "not less than the exclusive maximum")}
        for key, (test, words) in tests.items():
            if key in schema:
                bound = self.number_kw(schema, key, spath)
                if bound is not None and not test(bound):
                    errors.append(self.err(ipath, spath + [key], key, f"{inst} is {words} {schema[key]}"))
        if "multipleOf" in schema:
            m = self.number_kw(schema, "multipleOf", spath)
            if m is None or m <= 0:
                fail(f"schema {ptr(spath + ['multipleOf'])} must be greater than 0")
            if (x / m).denominator != 1:
                errors.append(self.err(ipath, spath + ["multipleOf"], "multipleOf", f"{inst} is not a multiple of {schema['multipleOf']}"))
        return errors

    def count_kw(self, schema, key, spath):
        v = schema[key]
        if isinstance(v, float) and v.is_integer():
            v = int(v)
        if not isinstance(v, int) or isinstance(v, bool) or v < 0:
            fail(f"schema {ptr(spath + [key])} must be a non-negative integer")
        return v

    def string(self, schema, inst, ipath, spath):
        errors = []
        n = len(inst)
        if "minLength" in schema and n < self.count_kw(schema, "minLength", spath):
            errors.append(self.err(ipath, spath + ["minLength"], "minLength", f"string of length {n} is shorter than {schema['minLength']}"))
        if "maxLength" in schema and n > self.count_kw(schema, "maxLength", spath):
            errors.append(self.err(ipath, spath + ["maxLength"], "maxLength", f"string of length {n} is longer than {schema['maxLength']}"))
        if "pattern" in schema:
            if not isinstance(schema["pattern"], str):
                fail(f"schema {ptr(spath + ['pattern'])} must be a string")
            if not self.rx(schema["pattern"], spath + ["pattern"]).search(inst):
                errors.append(self.err(ipath, spath + ["pattern"], "pattern", f"{inst[:80]!r} does not match {schema['pattern']!r}"))
        if "format" in schema:
            name = schema["format"]
            if name not in FORMATS:
                self.warnings.append(f"format {name!r} at {ptr(spath + ['format'])} is not checked")
            elif not check_format(name, inst):
                e = self.err(ipath, spath + ["format"], "format", f"{inst[:80]!r} is not a valid {name}")
                if self.formats == "assert":
                    errors.append(e)
                else:
                    self.warnings.append(f"{e['instance_path'] or '(root)'}: {e['message']} (format is an annotation; use formats: assert)")
        return errors

    def array(self, schema, inst, ipath, spath):
        errors = []
        n = len(inst)
        if "minItems" in schema and n < self.count_kw(schema, "minItems", spath):
            errors.append(self.err(ipath, spath + ["minItems"], "minItems", f"array has {n} items, fewer than {schema['minItems']}"))
        if "maxItems" in schema and n > self.count_kw(schema, "maxItems", spath):
            errors.append(self.err(ipath, spath + ["maxItems"], "maxItems", f"array has {n} items, more than {schema['maxItems']}"))
        if schema.get("uniqueItems") is True:
            for i in range(n):
                for j in range(i):
                    if equal(inst[i], inst[j]):
                        errors.append(self.err(ipath, spath + ["uniqueItems"], "uniqueItems", f"items {j} and {i} are equal"))
                        break
                else:
                    continue
                break
        prefix = schema.get("prefixItems")
        start = 0
        if prefix is not None:
            if not isinstance(prefix, list):
                fail(f"schema {ptr(spath + ['prefixItems'])} must be an array")
            for i, sub in enumerate(prefix[:n]):
                errors += self.validate(sub, inst[i], ipath + [i], spath + ["prefixItems", i])
            start = len(prefix)
        if "items" in schema:
            items = schema["items"]
            if isinstance(items, list):
                self.unsupported.setdefault("items as an array (draft 2019-09 and older; use prefixItems)", ptr(spath + ["items"]))
            else:
                for i in range(start, n):
                    errors += self.validate(items, inst[i], ipath + [i], spath + ["items"])
        if "contains" in schema:
            hits = sum(1 for i, v in enumerate(inst) if not self.validate(schema["contains"], v, ipath + [i], spath + ["contains"]))
            low = self.count_kw(schema, "minContains", spath) if "minContains" in schema else 1
            if hits < low:
                errors.append(self.err(ipath, spath + ["contains"], "contains", f"{hits} item(s) match contains, fewer than {low}"))
            if "maxContains" in schema and hits > self.count_kw(schema, "maxContains", spath):
                errors.append(self.err(ipath, spath + ["maxContains"], "maxContains", f"{hits} items match contains, more than {schema['maxContains']}"))
        return errors

    def obj(self, schema, inst, ipath, spath):
        errors = []
        n = len(inst)
        if "minProperties" in schema and n < self.count_kw(schema, "minProperties", spath):
            errors.append(self.err(ipath, spath + ["minProperties"], "minProperties", f"object has {n} properties, fewer than {schema['minProperties']}"))
        if "maxProperties" in schema and n > self.count_kw(schema, "maxProperties", spath):
            errors.append(self.err(ipath, spath + ["maxProperties"], "maxProperties", f"object has {n} properties, more than {schema['maxProperties']}"))
        if "required" in schema:
            req = schema["required"]
            if not isinstance(req, list) or not all(isinstance(r, str) for r in req):
                fail(f"schema {ptr(spath + ['required'])} must be an array of strings")
            for name in req:
                if name not in inst:
                    errors.append(self.err(ipath, spath + ["required"], "required", f"{name!r} is a required property", property=name))
        if "dependentRequired" in schema:
            dep = schema["dependentRequired"]
            if not isinstance(dep, dict) or not all(isinstance(v, list) for v in dep.values()):
                fail(f"schema {ptr(spath + ['dependentRequired'])} must be an object of string arrays")
            for name, needs in dep.items():
                if name in inst:
                    for other in needs:
                        if other not in inst:
                            errors.append(self.err(ipath, spath + ["dependentRequired", name], "dependentRequired",
                                                   f"{other!r} is required when {name!r} is present"))
        props = schema.get("properties", {})
        pats = schema.get("patternProperties", {})
        if not isinstance(props, dict) or not isinstance(pats, dict):
            fail(f"schema {ptr(spath)}: properties and patternProperties must be objects")
        for name, value in inst.items():
            matched = False
            if name in props:
                matched = True
                errors += self.validate(props[name], value, ipath + [name], spath + ["properties", name])
            for pat, sub in pats.items():
                if self.rx(pat, spath + ["patternProperties", pat]).search(name):
                    matched = True
                    errors += self.validate(sub, value, ipath + [name], spath + ["patternProperties", pat])
            if not matched and "additionalProperties" in schema:
                add = schema["additionalProperties"]
                if add is False:
                    errors.append(self.err(ipath + [name], spath + ["additionalProperties"], "additionalProperties",
                                           f"additional property {name!r} is not allowed"))
                else:
                    errors += self.validate(add, value, ipath + [name], spath + ["additionalProperties"])
            if "propertyNames" in schema:
                for e in self.validate(schema["propertyNames"], name, ipath, spath + ["propertyNames"]):
                    e["message"] = f"property name {name!r}: " + e["message"]
                    errors.append(e)
        return errors

    def logic(self, schema, inst, ipath, spath):
        errors = []
        for key in ("allOf", "anyOf", "oneOf"):
            if key not in schema:
                continue
            subs = schema[key]
            if not isinstance(subs, list) or not subs:
                fail(f"schema {ptr(spath + [key])} must be a non-empty array")
            results = [self.validate(s, inst, ipath, spath + [key, i]) for i, s in enumerate(subs)]
            passing = [i for i, r in enumerate(results) if not r]
            if key == "allOf":
                for r in results:
                    errors += r
            elif key == "anyOf" and not passing:
                errors.append(self.err(ipath, spath + [key], key, f"does not match any of the {len(subs)} schemas",
                                       causes=[e for r in results for e in r][:10]))
            elif key == "oneOf" and len(passing) != 1:
                if passing:
                    msg = f"matches {len(passing)} schemas (indexes {passing}), exactly one is required"
                    errors.append(self.err(ipath, spath + [key], key, msg))
                else:
                    errors.append(self.err(ipath, spath + [key], key, f"does not match any of the {len(subs)} schemas",
                                           causes=[e for r in results for e in r][:10]))
        if "not" in schema and not self.validate(schema["not"], inst, ipath, spath + ["not"]):
            errors.append(self.err(ipath, spath + ["not"], "not", "must not match the schema in not"))
        if "if" in schema:
            branch = "then" if not self.validate(schema["if"], inst, ipath, spath + ["if"]) else "else"
            if branch in schema:
                errors += self.validate(schema[branch], inst, ipath, spath + [branch])
        return errors


def load(request, key):
    if key in request and key + "_text" in request:
        fail(f"give {key} or {key}_text, not both")
    if key + "_text" in request:
        text = request[key + "_text"]
        if not isinstance(text, str):
            fail(f"{key}_text must be a string")
        if len(text) > MAX_INPUT:
            fail(f"{key}_text is longer than {MAX_INPUT} characters")
        try:
            return json.loads(text, parse_constant=reject_constant)
        except ValueError as error:
            fail(f"{key}_text is not JSON: {error}")
    if key not in request:
        fail(f"{key} (or {key}_text) is required")
    return request[key]


def on_alarm(signum, frame):
    raise Timeout()


def main():
    try:
        request = json.load(sys.stdin, parse_constant=reject_constant)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - {"schema", "schema_text", "instance", "instance_text", "formats", "max_errors"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    schema, instance = load(request, "schema"), load(request, "instance")
    if not isinstance(schema, (dict, bool)):
        fail("schema must be a JSON object or a boolean")
    formats = request.get("formats", "annotate")
    if formats not in ("annotate", "assert"):
        fail("formats must be annotate or assert")
    max_errors = request.get("max_errors", 100)
    if not isinstance(max_errors, int) or isinstance(max_errors, bool) or not 1 <= max_errors <= 10000:
        fail("max_errors must be an integer from 1 to 10000")
    v = Validator(schema, formats, max_errors)
    signal.signal(signal.SIGALRM, on_alarm)
    signal.alarm(TIME_LIMIT)
    try:
        errors = v.validate(schema, instance, [], [])
    except Timeout:
        fail(f"validation took longer than {TIME_LIMIT} s (a pattern that backtracks, or a very large instance)")
    except RecursionError:
        fail("the schema or the instance is nested too deeply")
    except (AttributeError, TypeError, KeyError) as error:
        fail(f"the schema is malformed somewhere ({type(error).__name__}: {error})")
    finally:
        signal.alarm(0)
    declared = schema.get("$schema") if isinstance(schema, dict) else None
    if declared and "2020-12" not in str(declared):
        v.warnings.insert(0, f"$schema is {declared}; validated with draft 2020-12 rules")
    result = {"valid": not errors, "error_count": len(errors), "errors": errors[:max_errors],
              "complete": not v.unsupported,
              "unsupported": [{"keyword": k, "schema_path": p} for k, p in v.unsupported.items()],
              "unknown_keywords": [{"keyword": k, "schema_path": p} for k, p in v.unknown.items()],
              "warnings": v.warnings[:100], "formats": formats, "dialect": "JSON Schema draft 2020-12 (subset)"}
    if v.unsupported:
        result["note"] = "some keywords are not supported and were not checked; valid means valid for the rest"
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
