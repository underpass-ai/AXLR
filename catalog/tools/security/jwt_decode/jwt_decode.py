#!/usr/bin/env python3
"""Decode a JSON Web Token's header and payload WITHOUT verifying its signature: alg, kid and
typ, the registered time claims (iat, nbf, exp) as dates and, given a reference time, whether
the token is expired or not yet valid; flags alg "none", algorithm-confusion risks and keys
embedded in the header.

Input (JSON on stdin):
  token   the compact JWS (header.payload.signature) or JWE (five parts); "Bearer " is stripped
  now     reference time: Unix seconds or "YYYY-MM-DDTHH:MM:SSZ" (needed for expiry; the clock is never read)
  leeway  seconds of clock skew allowed for exp and nbf, default 0
"""
import datetime as dt
import json
import re
import sys

B64URL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
DECODE = {char: index for index, char in enumerate(B64URL)}
EPOCH = dt.datetime(1970, 1, 1, tzinfo=dt.timezone.utc)
REGISTERED = {"iss": "issuer", "sub": "subject", "aud": "audience", "exp": "expiration time", "nbf": "not before",
              "iat": "issued at", "jti": "JWT ID"}
WARNING = "The signature is NOT verified. Do not trust any claim in this token until a verifier with the right key and a pinned algorithm has accepted it."


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def b64url_decode(text, part):
    """RFC 4648 section 5 base64url without padding (RFC 7515 section 2), written out by hand."""
    if text.endswith("="):
        text = text.rstrip("=")
    if len(text) % 4 == 1:
        fail(f"{part} has an impossible base64url length")
    buffer = bits = 0
    out = bytearray()
    for char in text:
        value = DECODE.get(char)
        if value is None:
            if char in "+/":
                fail(f"{part} uses '+' or '/': that is base64, not the base64url a JWT requires")
            fail(f"{part} is not base64url (character {char!r})")
        buffer = ((buffer << 6) | value) & 0xFFFFFF
        bits += 6
        if bits >= 8:
            bits -= 8
            out.append((buffer >> bits) & 0xFF)
    return bytes(out)


def parse_json_part(raw, part):
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError:
        return None, None
    try:
        return json.loads(text), text
    except ValueError:
        return None, text


def parse_time(value, field):
    if isinstance(value, bool):
        fail(f"{field} must be Unix seconds or YYYY-MM-DDTHH:MM:SSZ")
    if isinstance(value, (int, float)):
        return value
    if isinstance(value, str) and re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})", value):
        try:
            return (dt.datetime.fromisoformat(value.replace("Z", "+00:00")) - EPOCH).total_seconds()
        except ValueError:
            pass
    fail(f"{field} must be Unix seconds or YYYY-MM-DDTHH:MM:SSZ")


def iso(seconds):
    try:
        moment = EPOCH + dt.timedelta(seconds=seconds)
    except (OverflowError, ValueError):
        return None
    return moment.strftime("%Y-%m-%dT%H:%M:%SZ") if moment.microsecond == 0 else moment.isoformat().replace("+00:00", "Z")


def number(value):
    return int(value) if isinstance(value, float) and value.is_integer() else value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"token", "now", "leeway"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    token = request.get("token")
    if not isinstance(token, str) or not token.strip():
        fail("token is required (a string)")
    token = re.sub(r"^\s*bearer\s+", "", token.strip(), flags=re.I).strip()
    if len(token) > 1_000_000:
        fail("token is longer than 1 MB")
    if re.search(r"\s", token):
        fail("token contains whitespace")
    parts = token.split(".")
    if len(parts) not in (3, 5):
        fail(f"a compact JWT has 3 parts (JWS) or 5 (JWE) separated by dots; this one has {len(parts)}")
    now = parse_time(request["now"], "now") if "now" in request else None
    leeway = request.get("leeway", 0)
    if isinstance(leeway, bool) or not isinstance(leeway, (int, float)) or leeway < 0:
        fail("leeway must be a number of seconds >= 0")

    header, header_text = parse_json_part(b64url_decode(parts[0], "header"), "header")
    if not isinstance(header, dict):
        fail("the header is not a JSON object")
    alg = header.get("alg")
    findings = []

    def finding(severity, code, message):
        findings.append({"severity": severity, "code": code, "message": message})

    result = {"signature_verified": False, "warning": WARNING, "format": "JWS" if len(parts) == 3 else "JWE",
              "header": header, "alg": alg, "typ": header.get("typ"), "kid": header.get("kid"), "cty": header.get("cty")}

    if len(parts) == 5:
        result["note"] = "JWE: the payload is encrypted; only the protected header can be read without the key."
        result["enc"] = header.get("enc")
        finding("info", "jwe", "Encrypted token: claims are not visible without decryption.")
        result["findings"] = findings
        print(json.dumps(result))
        return

    payload_raw = b64url_decode(parts[1], "payload")
    signature = b64url_decode(parts[2], "signature")
    if header.get("b64") is False:
        payload, payload_text = None, parts[1]
        finding("info", "b64_false", "Header b64: false (RFC 7797): the payload is not base64url-encoded.")
    else:
        payload, payload_text = parse_json_part(payload_raw, "payload")
    result["signature_bytes"] = len(signature)

    # Algorithm checks (RFC 8725 sections 2.1, 3.1 and 3.2).
    if not isinstance(alg, str):
        finding("high", "alg_missing", "The header has no alg string; RFC 7515 requires one.")
    elif alg.lower() == "none":
        finding("critical", "alg_none", "alg is \"none\": an unsecured JWT (RFC 7519 section 6). Anyone can forge it; verifiers must reject it unless explicitly expected.")
        if signature:
            finding("high", "none_with_signature", "alg none but a non-empty signature part: a malformed or tampered token.")
    elif alg.upper().startswith("HS"):
        finding("medium", "hmac_confusion", f"{alg} is a shared-secret MAC. If the verifier holds an RSA or EC public key, it must refuse HS* tokens: accepting them lets an attacker sign with the public key as the HMAC secret (algorithm confusion, RFC 8725 section 2.1). Pin the expected algorithm.")
    elif alg[:2].upper() in ("RS", "PS", "ES") or alg.upper() == "EDDSA":
        finding("info", "asymmetric", f"{alg} is a public-key signature. The verifier must pin it and never fall back to HS* with the same key (algorithm confusion).")
    else:
        finding("medium", "alg_unknown", f"alg {alg!r} is not a JWS algorithm registered in RFC 7518 or RFC 8037.")
    if isinstance(alg, str) and alg.lower() != "none" and not signature:
        finding("high", "empty_signature", "The signature part is empty although alg is not none.")
    expected = {"HS256": 32, "HS384": 48, "HS512": 64, "ES256": 64, "ES384": 96, "ES512": 132}
    if isinstance(alg, str) and alg in expected and signature and len(signature) != expected[alg]:
        finding("medium", "signature_length", f"{alg} signatures are {expected[alg]} bytes; this one is {len(signature)}.")
    for key in ("jwk", "jku", "x5u", "x5c"):
        if key in header:
            finding("high", f"embedded_{key}", f"Header carries {key}: a verifier that takes its key from the token itself can be fooled (key injection). Use only keys you already trust.")
    kid = header.get("kid")
    if isinstance(kid, str) and (".." in kid or "/" in kid or "\\" in kid or "'" in kid or "\x00" in kid):
        finding("medium", "kid_injection", "kid contains path or quote characters: if it is used to look a key up in files or SQL, sanitise it.")
    if "crit" in header:
        finding("info", "crit", f"crit lists extensions the verifier must understand: {header['crit']}.")
    if isinstance(header.get("typ"), str) and header["typ"].upper() not in ("JWT", "AT+JWT", "JOSE", "APPLICATION/JWT") and not header["typ"].upper().endswith("+JWT"):
        finding("info", "typ", f"typ is {header['typ']!r}; RFC 8725 section 3.11 recommends explicit types to keep token kinds apart.")

    if isinstance(payload, dict):
        result["payload"] = payload
        claims = {}
        for name, label in REGISTERED.items():
            if name in payload:
                claims[name] = {"name": label, "value": payload[name]}
        times = {}
        for name in ("iat", "nbf", "exp"):
            if name not in payload:
                continue
            value = payload[name]
            if isinstance(value, bool) or not isinstance(value, (int, float)):
                finding("medium", f"{name}_type", f"{name} is not a NumericDate (seconds since the epoch) as RFC 7519 section 2 requires.")
                continue
            times[name] = value
            claims[name]["utc"] = iso(value)
        result["claims"] = claims
        if "exp" not in payload:
            finding("medium", "no_exp", "No exp claim: the token never expires on its own.")
        if "exp" in times and "iat" in times:
            result["lifetime_seconds"] = number(times["exp"] - times["iat"])
            if times["exp"] < times["iat"]:
                finding("medium", "exp_before_iat", "exp is earlier than iat.")
        if now is None:
            result["validity"] = {"evaluated": False, "note": "Give now (Unix seconds or an ISO date) to check exp and nbf; the clock is never read."}
        else:
            validity = {"evaluated": True, "now": number(now), "now_utc": iso(now), "leeway": number(leeway)}
            # RFC 7519 4.1.4: now must be before exp; 4.1.5: now must be at or after nbf.
            expired = "exp" in times and now >= times["exp"] + leeway
            early = "nbf" in times and now < times["nbf"] - leeway
            validity["expired"] = expired if "exp" in times else None
            validity["not_yet_valid"] = early if "nbf" in times else None
            if "exp" in times:
                validity["seconds_to_expiry"] = number(times["exp"] - now)
            if "iat" in times:
                validity["age_seconds"] = number(now - times["iat"])
                if times["iat"] > now + leeway:
                    finding("medium", "iat_future", "iat is in the future relative to now.")
            validity["status"] = "expired" if expired else "not_yet_valid" if early else "within_time_claims"
            result["validity"] = validity
            if expired:
                finding("high", "expired", "The token is expired at the given time.")
            if early:
                finding("medium", "not_yet_valid", "The token is not valid yet (nbf) at the given time.")
    else:
        result["payload"] = None
        result["payload_text"] = payload_text
        if str(header.get("cty", "")).upper() == "JWT":
            finding("info", "nested", "cty JWT: the payload is itself a JWT (nested); decode it again.")
        elif payload_text is None:
            finding("info", "binary_payload", "The payload is not UTF-8 text.")
        else:
            finding("info", "non_json_payload", "The payload is not a JSON object, so it carries no JWT claims.")
    result["findings"] = findings
    print(json.dumps(result))


if __name__ == "__main__":
    main()
