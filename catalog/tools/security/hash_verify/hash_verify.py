#!/usr/bin/env python3
"""Digests of a text, hex bytes or a file in the workspace: MD5, SHA-1, the SHA-2 and SHA-3
families and BLAKE2, optionally as HMAC with a key; and verification against an expected
digest (hex, any case, or a `sha256sum`-style line), compared in constant time.

Input (JSON on stdin), one source:
  text     "abc"              hashed as UTF-8
  hex      "616263"           raw bytes
  file     "dist/app.tar.gz"  a path relative to the working directory
and:
  algorithms  ["sha256", "md5", ...]   default ["sha256"]
  expected    "ba7816bf..."            verify; the algorithm is guessed from its length if one is not given
  hmac_key    "secret"                 HMAC instead of a plain digest (UTF-8 key)
"""
import hashlib
import hmac
import json
import pathlib
import re
import sys

ALGORITHMS = ("md5", "sha1", "sha224", "sha256", "sha384", "sha512", "sha3_224", "sha3_256", "sha3_384", "sha3_512", "blake2b", "blake2s")
BY_LENGTH = {32: "md5", 40: "sha1", 56: "sha224", 64: "sha256", 96: "sha384", 128: "sha512"}
WEAK = {"md5", "sha1"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_source(request):
    sources = [k for k in ("text", "hex", "file") if k in request]
    if len(sources) != 1:
        fail("give exactly one of text, hex or file")
    kind = sources[0]
    value = request[kind]
    if not isinstance(value, str):
        fail(f"{kind} must be a string")
    if kind == "text":
        return value.encode("utf-8"), {"text_bytes": len(value.encode("utf-8"))}
    if kind == "hex":
        cleaned = re.sub(r"[\s:]", "", value)
        try:
            return bytes.fromhex(cleaned), {"bytes": len(cleaned) // 2}
        except ValueError:
            fail("hex must be pairs of hexadecimal digits")
    path = pathlib.Path(value)
    if path.is_absolute() or ".." in path.parts:
        fail("file must be a relative path inside the working directory")
    try:
        data = path.read_bytes()
    except OSError as error:
        fail(f"cannot read {value}: {error.strerror}")
    return data, {"file": value, "bytes": len(data)}


def digest(name, data, key):
    if key is not None:
        return hmac.new(key, data, getattr(hashlib, name)).hexdigest()
    return hashlib.new(name, data).hexdigest()


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    data, about = read_source(request)
    key = request.get("hmac_key")
    if key is not None and not isinstance(key, str):
        fail("hmac_key must be a string")
    key = key.encode("utf-8") if key is not None else None
    expected = request.get("expected")
    algorithms = request.get("algorithms")
    if expected is not None:
        if not isinstance(expected, str):
            fail("expected must be a string")
        expected = expected.strip().split()[0].lower() if expected.strip() else ""
        if not re.fullmatch(r"[0-9a-f]+", expected):
            fail("expected must be a hexadecimal digest")
        if algorithms is None:
            guess = BY_LENGTH.get(len(expected))
            if guess is None:
                fail("cannot tell the algorithm from the digest length; give algorithms")
            algorithms = [guess]
    algorithms = algorithms or ["sha256"]
    if not isinstance(algorithms, list) or not all(a in ALGORITHMS for a in algorithms):
        fail(f"algorithms must be a list drawn from {', '.join(ALGORITHMS)}")
    digests = {a: digest(a, data, key) for a in algorithms}
    result = {**about, "hmac": key is not None, "digests": digests}
    if expected is not None:
        matches = [a for a, d in digests.items() if hmac.compare_digest(d, expected)]
        result["verified"] = bool(matches)
        result["matched_algorithm"] = matches[0] if matches else None
    weak = sorted(WEAK & set(algorithms))
    if weak and key is None:
        result["warning"] = f"{' and '.join(weak)} {'are' if len(weak) > 1 else 'is'} broken for collision resistance; use only to compare against published checksums"
    print(json.dumps(result))


if __name__ == "__main__":
    main()
