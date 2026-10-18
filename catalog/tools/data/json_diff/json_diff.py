#!/usr/bin/env python3
"""Structural diff of two JSON values with JSON Pointer paths (RFC 6901) and the RFC 6902
JSON Patch that turns the first into the second; arrays compared by index, by a key field or
by longest common subsequence. Also applies a JSON Patch (add, remove, replace, move, copy,
test) to a document.

Input (JSON on stdin):
  operation  "diff" (default) | "apply"
  a, b       diff: the two JSON values (or a_text, b_text as JSON text)
  arrays     diff: "index" (default), "key" (objects matched by key) or "lcs" (minimal inserts and deletes)
  key        diff with arrays "key": the field that identifies array elements
  doc, patch apply: the document and the RFC 6902 operations
"""
import copy
import json
import sys

MAX_DEPTH = 500


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


class PatchError(Exception):
    pass


def escape(token):
    return str(token).replace("~", "~0").replace("/", "~1")


def pointer(parts):
    return "".join("/" + escape(p) for p in parts)


def same(x, y):
    """JSON equality (RFC 6902 4.6): numbers by value, booleans are not numbers, objects unordered."""
    if isinstance(x, bool) or isinstance(y, bool):
        return isinstance(x, bool) and isinstance(y, bool) and x == y
    if isinstance(x, (int, float)) and isinstance(y, (int, float)):
        return x == y
    if isinstance(x, dict) and isinstance(y, dict):
        return x.keys() == y.keys() and all(same(x[k], y[k]) for k in x)
    if isinstance(x, list) and isinstance(y, list):
        return len(x) == len(y) and all(same(p, q) for p, q in zip(x, y))
    return type(x) is type(y) and x == y


class Differ:
    def __init__(self, arrays, key):
        self.arrays, self.key = arrays, key
        self.changes, self.patch = [], []

    def diff(self, x, y, path, depth=0):
        if depth > MAX_DEPTH:
            fail("the values are nested more than 500 levels deep")
        if isinstance(x, dict) and isinstance(y, dict):
            for k in x:
                if k not in y:
                    self.changes.append({"op": "removed", "path": pointer(path + [k]), "old": x[k]})
                    self.patch.append({"op": "remove", "path": pointer(path + [k])})
            for k in x:
                if k in y:
                    self.diff(x[k], y[k], path + [k], depth + 1)
            for k in y:
                if k not in x:
                    self.changes.append({"op": "added", "path": pointer(path + [k]), "new": y[k]})
                    self.patch.append({"op": "add", "path": pointer(path + [k]), "value": y[k]})
        elif isinstance(x, list) and isinstance(y, list):
            if self.arrays == "key" and self.keyed(x) and self.keyed(y):
                self.diff_keyed(x, y, path, depth)
            elif self.arrays == "lcs":
                self.diff_lcs(x, y, path, depth)
            else:
                self.diff_index(x, y, path, depth)
        elif not same(x, y):
            self.changes.append({"op": "changed", "path": pointer(path), "old": x, "new": y})
            self.patch.append({"op": "replace", "path": pointer(path), "value": y})

    def keyed(self, items):
        values = [item.get(self.key) for item in items if isinstance(item, dict)]
        return len(values) == len(items) and all(v is not None for v in values) and len({json.dumps(v, sort_keys=True) for v in values}) == len(values)

    def diff_index(self, x, y, path, depth):
        for i in range(min(len(x), len(y))):
            self.diff(x[i], y[i], path + [i], depth + 1)
        for i in range(len(x) - 1, len(y) - 1, -1):
            self.changes.append({"op": "removed", "path": pointer(path + [i]), "old": x[i]})
            self.patch.append({"op": "remove", "path": pointer(path + [i])})
        for i in range(len(x), len(y)):
            self.changes.append({"op": "added", "path": pointer(path + [i]), "new": y[i]})
            self.patch.append({"op": "add", "path": pointer(path + [i]), "value": y[i]})

    def diff_lcs(self, x, y, path, depth):
        n, m = len(x), len(y)
        if n * m > 4_000_000:
            fail("arrays too long for the lcs comparison (n x m above 4 million); use arrays index or key")
        table = [[0] * (m + 1) for _ in range(n + 1)]
        for i in range(n - 1, -1, -1):
            for j in range(m - 1, -1, -1):
                table[i][j] = table[i + 1][j + 1] + 1 if same(x[i], y[j]) else max(table[i + 1][j], table[i][j + 1])
        i = j = 0
        script = []
        while i < n and j < m:
            if same(x[i], y[j]):
                script.append(("keep", i, j))
                i, j = i + 1, j + 1
            elif table[i + 1][j] >= table[i][j + 1]:
                script.append(("delete", i, None))
                i += 1
            else:
                script.append(("insert", None, j))
                j += 1
        script += [("delete", k, None) for k in range(i, n)] + [("insert", None, k) for k in range(j, m)]
        # A delete followed by an insert at the same place is a change of that element: diff it in depth.
        merged, k = [], 0
        while k < len(script):
            if k + 1 < len(script) and script[k][0] == "delete" and script[k + 1][0] == "insert":
                merged.append(("change", script[k][1], script[k + 1][2]))
                k += 2
            else:
                merged.append(script[k])
                k += 1
        position = 0  # index in the array as the patch transforms it
        for op, i, j in merged:
            if op == "keep":
                position += 1
            elif op == "delete":
                self.changes.append({"op": "removed", "path": pointer(path + [i]), "old": x[i]})
                self.patch.append({"op": "remove", "path": pointer(path + [position])})
            elif op == "insert":
                self.changes.append({"op": "added", "path": pointer(path + [j]), "new": y[j]})
                self.patch.append({"op": "add", "path": pointer(path + [position]), "value": y[j]})
                position += 1
            else:
                before = len(self.patch)
                self.diff(x[i], y[j], path + [i], depth + 1)
                for entry in self.patch[before:]:
                    entry["path"] = entry["path"].replace(pointer(path + [i]), pointer(path + [position]), 1)
                position += 1

    def diff_keyed(self, x, y, path, depth):
        ident = lambda item: json.dumps(item[self.key], sort_keys=True)
        old = {ident(item): index for index, item in enumerate(x)}
        new = {ident(item): index for index, item in enumerate(y)}
        common_old = [k for k in (ident(item) for item in x) if k in new]
        common_new = [k for k in (ident(item) for item in y) if k in old]
        if common_old != common_new:
            # Elements were reordered: report per key, patch the whole array.
            for k, i in old.items():
                if k not in new:
                    self.changes.append({"op": "removed", "path": pointer(path + [i]), "old": x[i], "key": x[i][self.key]})
            for k, j in new.items():
                if k not in old:
                    self.changes.append({"op": "added", "path": pointer(path + [j]), "new": y[j], "key": y[j][self.key]})
            for k in common_new:
                sub = Differ(self.arrays, self.key)
                sub.diff(x[old[k]], y[new[k]], path + [old[k]], depth + 1)
                for change in sub.changes:
                    change["key"] = x[old[k]][self.key]
                self.changes += sub.changes
            self.changes.append({"op": "reordered", "path": pointer(path), "note": "elements matched by key appear in another order"})
            self.patch.append({"op": "replace", "path": pointer(path), "value": y})
            return
        for i in range(len(x) - 1, -1, -1):
            if ident(x[i]) not in new:
                self.changes.append({"op": "removed", "path": pointer(path + [i]), "old": x[i], "key": x[i][self.key]})
                self.patch.append({"op": "remove", "path": pointer(path + [i])})
        for position, k in enumerate(common_old):
            sub = Differ(self.arrays, self.key)
            sub.diff(x[old[k]], y[new[k]], path + [old[k]], depth + 1)
            for change in sub.changes:
                change["key"] = x[old[k]][self.key]
            self.changes += sub.changes
            for entry in sub.patch:
                entry["path"] = entry["path"].replace(pointer(path + [old[k]]), pointer(path + [position]), 1)
            self.patch += sub.patch
        for j, item in enumerate(y):
            if ident(item) not in old:
                self.changes.append({"op": "added", "path": pointer(path + [j]), "new": item, "key": item[self.key]})
                self.patch.append({"op": "add", "path": pointer(path + [j]), "value": item})


def parse_pointer(path):
    if not isinstance(path, str):
        raise PatchError("path must be a string")
    if path == "":
        return []
    if not path.startswith("/"):
        raise PatchError(f"{path!r} is not a JSON Pointer (it must start with '/')")
    return [token.replace("~1", "/").replace("~0", "~") for token in path[1:].split("/")]


def array_index(token, length, allow_end):
    if token == "-" and allow_end:
        return length
    if not token.isdigit() or (len(token) > 1 and token[0] == "0"):
        raise PatchError(f"{token!r} is not a valid array index")
    index = int(token)
    if index > length or (index == length and not allow_end):
        raise PatchError(f"array index {index} is out of bounds")
    return index


def resolve(doc, tokens):
    node = doc
    for token in tokens:
        if isinstance(node, dict):
            if token not in node:
                raise PatchError(f"{pointer(tokens)} does not exist (no member {token!r})")
            node = node[token]
        elif isinstance(node, list):
            node = node[array_index(token, len(node), False)]
        else:
            raise PatchError(f"{pointer(tokens)} does not exist (a scalar has no members)")
    return node


def add(doc, tokens, value):
    if not tokens:
        return value
    parent = resolve(doc, tokens[:-1])
    last = tokens[-1]
    if isinstance(parent, dict):
        parent[last] = value
    elif isinstance(parent, list):
        parent.insert(array_index(last, len(parent), True), value)
    else:
        raise PatchError(f"cannot add to {pointer(tokens[:-1])}: it is not an object or array")
    return doc


def remove(doc, tokens):
    if not tokens:
        raise PatchError("cannot remove the whole document")
    parent = resolve(doc, tokens[:-1])
    last = tokens[-1]
    if isinstance(parent, dict):
        if last not in parent:
            raise PatchError(f"{pointer(tokens)} does not exist")
        return doc, parent.pop(last)
    if isinstance(parent, list):
        return doc, parent.pop(array_index(last, len(parent), False))
    raise PatchError(f"{pointer(tokens)} does not exist")


def apply(doc, patch):
    if not isinstance(patch, list):
        raise PatchError("patch must be an array of operations")
    doc = copy.deepcopy(doc)
    for number, op in enumerate(patch):
        try:
            if not isinstance(op, dict) or "op" not in op or "path" not in op:
                raise PatchError("each operation needs op and path")
            name, tokens = op["op"], parse_pointer(op["path"])
            if name in ("add", "replace", "test") and "value" not in op:
                raise PatchError(f"{name} needs value")
            if name == "add":
                doc = add(doc, tokens, copy.deepcopy(op["value"]))
            elif name == "remove":
                doc, _ = remove(doc, tokens)
            elif name == "replace":
                resolve(doc, tokens)
                if tokens:
                    doc, _ = remove(doc, tokens)
                doc = add(doc, tokens, copy.deepcopy(op["value"])) if tokens else copy.deepcopy(op["value"])
            elif name in ("move", "copy"):
                source = parse_pointer(op.get("from"))
                if name == "move" and tokens[:len(source)] == source and tokens != source:
                    raise PatchError("cannot move a value into one of its own children")
                value = copy.deepcopy(resolve(doc, source))
                if name == "move":
                    doc, value = remove(doc, source)
                doc = add(doc, tokens, value)
            elif name == "test":
                if not same(resolve(doc, tokens), op["value"]):
                    raise PatchError(f"test failed: the value at {op['path']} is not the expected one")
            else:
                raise PatchError(f"unknown op {name!r} (add, remove, replace, move, copy, test)")
        except PatchError as error:
            raise PatchError(f"operation {number} ({op.get('op') if isinstance(op, dict) else '?'}): {error}")
    return doc


def value_of(request, name):
    if name in request and f"{name}_text" in request:
        fail(f"give {name} or {name}_text, not both")
    if name in request:
        return request[name]
    if f"{name}_text" in request:
        text = request[f"{name}_text"]
        if not isinstance(text, str):
            fail(f"{name}_text must be a string")
        try:
            return json.loads(text)
        except ValueError as error:
            fail(f"{name}_text is not JSON: {error}")
    fail(f"{name} is required")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"operation", "a", "b", "a_text", "b_text", "arrays", "key", "doc", "patch"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    operation = request.get("operation", "diff")
    if operation == "apply":
        if "doc" not in request or "patch" not in request:
            fail("apply needs doc and patch")
        try:
            result = apply(request["doc"], request["patch"])
        except PatchError as error:
            print(json.dumps({"operation": "apply", "applied": False, "failure": str(error), "result": None,
                              "note": "the patch was not applied: RFC 6902 applies all operations or none"}))
            return
        except RecursionError:
            fail("the document is nested too deeply")
        print(json.dumps({"operation": "apply", "applied": True, "result": result, "operations": len(request["patch"])}))
        return
    if operation != "diff":
        fail("operation must be diff or apply")
    a, b = value_of(request, "a"), value_of(request, "b")
    arrays = request.get("arrays", "index")
    if arrays not in ("index", "key", "lcs"):
        fail("arrays must be index, key or lcs")
    key = request.get("key")
    if arrays == "key" and (not isinstance(key, str) or not key):
        fail("arrays key needs key: the field that identifies array elements")
    differ = Differ(arrays, key)
    try:
        differ.diff(a, b, [])
        verified = same(apply(a, differ.patch), b)
    except (PatchError, RecursionError) as error:
        fail(f"could not diff: {error}")
    summary = {kind: sum(1 for c in differ.changes if c["op"] == kind) for kind in ("added", "removed", "changed", "reordered")}
    print(json.dumps({"operation": "diff", "equal": not differ.changes, "summary": summary, "differences": differ.changes,
                      "patch": differ.patch, "patch_verified": verified, "arrays": arrays}))


if __name__ == "__main__":
    main()
