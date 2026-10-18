#!/usr/bin/env python3
"""HOTP (RFC 4226) and TOTP (RFC 6238) one-time passwords from a secret given in the input,
for a time or counter given in the input (the clock is never read), and verification of a
code within a window of steps.

Input (JSON on stdin):
  secret           the shared key: base32 (default, as in otpauth:// URIs), hex or ascii
  secret_encoding  "base32" | "hex" | "ascii"
  mode             "totp" (default) | "hotp"
  time             TOTP: Unix seconds (integer) or "YYYY-MM-DDTHH:MM:SSZ"; required
  counter          HOTP: the moving factor, integer >= 0; required
  algorithm        "SHA1" (default) | "SHA256" | "SHA512"
  digits           6 (default), 7 or 8
  period           TOTP step in seconds, default 30
  t0               TOTP epoch in Unix seconds, default 0
  code             verify this code instead of only generating
  window           steps (or counters) accepted on each side when verifying, default 1
"""
import datetime as dt
import hashlib
import hmac
import json
import re
import sys

ALGORITHMS = {"SHA1": hashlib.sha1, "SHA256": hashlib.sha256, "SHA512": hashlib.sha512}
B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def base32_decode(text):
    """RFC 4648 base32, case-insensitive, spaces and hyphens ignored, padding optional."""
    clean = re.sub(r"[\s-]", "", text).upper().rstrip("=")
    if not clean:
        fail("secret is empty")
    bits = buffer = 0
    out = bytearray()
    for char in clean:
        value = B32.find(char)
        if value < 0:
            fail("secret is not valid base32 (letters A-Z and digits 2-7)")
        buffer = (buffer << 5) | value
        bits += 5
        if bits >= 8:
            bits -= 8
            out.append((buffer >> bits) & 0xFF)
    if len(clean) * 5 % 8 >= 5:
        fail("secret has an impossible base32 length")
    return bytes(out)


def decode_secret(request):
    secret = request.get("secret")
    if not isinstance(secret, str) or not secret.strip():
        fail("secret is required (a string)")
    encoding = request.get("secret_encoding", "base32")
    if encoding == "base32":
        key = base32_decode(secret)
    elif encoding == "hex":
        try:
            key = bytes.fromhex(re.sub(r"\s", "", secret))
        except ValueError:
            fail("secret is not valid hex")
    elif encoding == "ascii":
        key = secret.encode("utf-8")
    else:
        fail("secret_encoding must be base32, hex or ascii")
    if not key:
        fail("secret is empty")
    return key


def integer(request, field, default=None, low=None, high=None):
    value = request.get(field, default)
    if isinstance(value, bool) or not isinstance(value, int):
        fail(f"{field} must be an integer")
    if (low is not None and value < low) or (high is not None and value > high):
        fail(f"{field} must be between {low} and {high}" if high is not None else f"{field} must be at least {low}")
    return value


def parse_time(value):
    if isinstance(value, bool):
        fail("time must be Unix seconds or YYYY-MM-DDTHH:MM:SSZ")
    if isinstance(value, int):
        return value
    if isinstance(value, float) and value.is_integer():
        return int(value)
    if isinstance(value, str) and re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})", value):
        try:
            moment = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
        except ValueError:
            fail("time is not a valid date and time")
        return int((moment - dt.datetime(1970, 1, 1, tzinfo=dt.timezone.utc)).total_seconds())
    fail("time is required: Unix seconds (integer) or YYYY-MM-DDTHH:MM:SSZ; the clock is never read")


def hotp(key, counter, digest, digits):
    """RFC 4226 section 5.3: HMAC over the 8-byte counter, dynamic truncation, modulo 10^digits."""
    mac = hmac.new(key, counter.to_bytes(8, "big"), digest).digest()
    offset = mac[-1] & 0x0F
    binary = int.from_bytes(mac[offset:offset + 4], "big") & 0x7FFFFFFF
    return str(binary % 10 ** digits).zfill(digits)


def iso(seconds):
    try:
        return (dt.datetime(1970, 1, 1, tzinfo=dt.timezone.utc) + dt.timedelta(seconds=seconds)).strftime("%Y-%m-%dT%H:%M:%SZ")
    except OverflowError:
        return None


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    known = {"secret", "secret_encoding", "mode", "time", "counter", "algorithm", "digits", "period", "t0", "code", "window"}
    unknown = sorted(set(request) - known)
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    key = decode_secret(request)
    algorithm = str(request.get("algorithm", "SHA1")).upper().replace("-", "")
    if algorithm not in ALGORITHMS:
        fail("algorithm must be SHA1, SHA256 or SHA512")
    digits = integer(request, "digits", 6, 6, 8)
    mode = request.get("mode", "totp")
    result = {"mode": mode, "algorithm": algorithm, "digits": digits, "key_bytes": len(key)}
    if mode == "totp":
        if "time" not in request:
            fail("time is required: Unix seconds (integer) or YYYY-MM-DDTHH:MM:SSZ; the clock is never read")
        now = parse_time(request["time"])
        period = integer(request, "period", 30, 1, 86400)
        t0 = integer(request, "t0", 0)
        if now < t0:
            fail("time is before t0")
        counter = (now - t0) // period
        start = t0 + counter * period
        result.update({"time": now, "time_utc": iso(now), "period": period, "t0": t0, "counter": counter,
                       "counter_hex": format(counter, "016X"), "valid_from_utc": iso(start),
                       "valid_until_utc": iso(start + period), "seconds_remaining": start + period - now})
    elif mode == "hotp":
        counter = integer(request, "counter", None, 0, 2 ** 64 - 1)
        result["counter"] = counter
    else:
        fail("mode must be totp or hotp")
    digest = ALGORITHMS[algorithm]
    result["code"] = hotp(key, counter, digest, digits)
    if "code" in request:
        code = request["code"]
        if isinstance(code, int) and not isinstance(code, bool):
            code = str(code).zfill(digits)
        if not isinstance(code, str) or not re.fullmatch(r"\d{6,8}", code.strip()):
            fail("code must be a string of 6 to 8 digits")
        code = code.strip()
        window = integer(request, "window", 1, 0, 100)
        match = None
        for delta in sorted(range(-window, window + 1), key=abs):
            candidate = counter + delta
            if 0 <= candidate < 2 ** 64 and len(code) == digits and hmac.compare_digest(hotp(key, candidate, digest, digits), code):
                match = delta
                break
        result["verify"] = {"valid": match is not None, "matched_offset": match, "matched_counter": None if match is None else counter + match,
                            "window": window}
        if len(code) != digits:
            result["verify"]["note"] = f"the code has {len(code)} digits, the configuration {digits}"
    result["note"] = "The secret is not echoed. Codes depend only on the input; the clock was not read."
    print(json.dumps(result))


if __name__ == "__main__":
    main()
