#!/usr/bin/env python3
"""Lint a Content-Security-Policy header value: parse its directives and report the weaknesses
Google's CSP Evaluator checks for (unsafe-inline and unsafe-eval in scripts, wildcards and
plain schemes, data: in script-src, missing object-src, base-uri, script-src or default-src,
known allowlist bypass hosts, short nonces, deprecated and unknown directives, missing
reporting) plus missing frame-ancestors, with CSP Level 3 semantics: a nonce or hash cancels
'unsafe-inline', and 'strict-dynamic' cancels host and scheme sources in script-src.

Input (JSON on stdin):
  policy    the header value (several policies separated by commas are linted one by one)
  delivery  "header" (default) | "report-only" | "meta"
"""
import json
import re
import sys

FETCH = ["child-src", "connect-src", "default-src", "font-src", "frame-src", "img-src", "manifest-src", "media-src",
         "object-src", "prefetch-src", "script-src", "script-src-elem", "script-src-attr", "style-src", "style-src-elem",
         "style-src-attr", "worker-src", "fenced-frame-src"]
OTHER = ["base-uri", "sandbox", "form-action", "frame-ancestors", "navigate-to", "report-uri", "report-to",
         "require-trusted-types-for", "trusted-types", "upgrade-insecure-requests", "block-all-mixed-content",
         "require-sri-for", "webrtc", "plugin-types", "reflected-xss", "referrer", "disown-opener"]
DEPRECATED = {"report-uri": "deprecated in CSP Level 3 in favour of report-to; keep both for older browsers",
              "block-all-mixed-content": "obsolete: browsers block or upgrade mixed content anyway; use upgrade-insecure-requests",
              "plugin-types": "removed from CSP Level 3; plugins are gone, set object-src 'none'",
              "reflected-xss": "removed, never widely supported; it does nothing",
              "referrer": "removed; use the Referrer-Policy header",
              "prefetch-src": "removed from the specification",
              "navigate-to": "removed from the specification",
              "require-sri-for": "never shipped in browsers",
              "disown-opener": "never shipped; use Cross-Origin-Opener-Policy"}
KEYWORDS = {"'self'", "'none'", "'unsafe-inline'", "'unsafe-eval'", "'strict-dynamic'", "'unsafe-hashes'",
            "'report-sample'", "'unsafe-allow-redirects'", "'wasm-unsafe-eval'", "'inline-speculation-rules'"}
# Hosts that serve JSONP endpoints or old AngularJS/libraries, letting an attacker run script
# through an allowlisted origin (CSP Evaluator's allowlist bypass lists, abridged).
BYPASS_HOSTS = ["www.google.com", "google.com", "*.google.com", "www.googleapis.com", "ajax.googleapis.com",
                "*.googleapis.com", "accounts.google.com", "www.gstatic.com", "*.gstatic.com", "cdnjs.cloudflare.com",
                "cdn.jsdelivr.net", "unpkg.com", "*.yandex.ru", "api.twitter.com", "*.twitter.com", "www.youtube.com",
                "*.facebook.com", "connect.facebook.net", "*.vk.com", "cse.google.com", "translate.googleapis.com",
                "*.amazonaws.com", "*.cloudfront.net", "raw.githubusercontent.com", "*.github.io", "cdn.rawgit.com"]
SCRIPT = ["script-src", "script-src-elem", "script-src-attr"]


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def parse(policy_text):
    directives, order, findings = {}, [], []
    for raw in policy_text.split(";"):
        tokens = raw.split()
        if not tokens:
            continue
        name = tokens[0].lower()
        values = tokens[1:]
        if not re.fullmatch(r"[a-z0-9-]+", name):
            findings.append(finding("syntax", "invalid_directive_name", name, None, f"{tokens[0]!r} is not a valid directive name.", "Remove it or fix the typo."))
            continue
        if name in directives:
            findings.append(finding("syntax", "duplicate_directive", name, None, f"{name} appears twice; browsers ignore every occurrence after the first.", "Merge the values into one directive."))
            continue
        directives[name] = values
        order.append(name)
    return directives, order, findings


def finding(severity, check, directive, value, description, fix):
    return {"severity": severity, "check": check, "directive": directive, "value": value, "description": description, "fix": fix}


def is_nonce(source):
    return source.lower().startswith("'nonce-") and source.endswith("'")


def is_hash(source):
    return re.fullmatch(r"'(sha256|sha384|sha512)-[A-Za-z0-9+/_-]+={0,2}'", source, re.I) is not None


def host_of(source):
    match = re.fullmatch(r"(?:([a-z][a-z0-9+.-]*)://)?(\*|\*\.[^:/]+|[^:/*]+)(?::(\d+|\*))?(/.*)?", source, re.I)
    return (match.group(2).lower(), (match.group(1) or "").lower()) if match else (None, None)


def lint(policy_text, delivery):
    directives, order, findings = parse(policy_text)

    def add(*args):
        findings.append(finding(*args))

    for name in order:
        if name not in FETCH and name not in OTHER:
            add("syntax", "unknown_directive", name, None, f"{name} is not a CSP directive; browsers ignore it.", "Check the spelling against the CSP Level 3 directive list.")
        if name in DEPRECATED:
            add("info", "deprecated_directive", name, None, f"{name}: {DEPRECATED[name]}.", "Replace or remove it.")
        for value in directives[name]:
            low = value.lower()
            if low in {k.strip("'") for k in KEYWORDS} or low.startswith(("nonce-", "sha256-", "sha384-", "sha512-")):
                add("syntax", "keyword_without_quotes", name, value, f"{value} without single quotes is read as a host name, not a keyword.", f"Write '{value}'.")
            elif value.startswith("'") and low not in KEYWORDS and not is_nonce(value) and not is_hash(value) and name not in ("sandbox", "trusted-types", "require-trusted-types-for"):
                add("syntax", "invalid_keyword", name, value, f"{value} is not a CSP keyword.", "Check the spelling ('self', 'none', 'strict-dynamic', 'nonce-…').")
            if low.rstrip(":") in FETCH + OTHER and not low.endswith(":"):
                add("syntax", "missing_semicolon", name, value, f"{value} looks like a directive name inside {name}: a semicolon is probably missing.", "Separate directives with ';'.")
        if name in FETCH and "'none'" in [v.lower() for v in directives[name]] and len(directives[name]) > 1:
            add("syntax", "none_with_sources", name, "'none'", f"'none' together with other sources in {name}: 'none' is ignored.", "Use either 'none' alone or a list of sources.")

    default = directives.get("default-src")
    script_name = "script-src" if "script-src" in directives else "default-src" if default is not None else None
    script = directives.get(script_name, []) if script_name else []
    lower = [v.lower() for v in script]
    has_nonce = any(is_nonce(v) for v in script)
    has_hash = any(is_hash(v) for v in script)
    strict_dynamic = "'strict-dynamic'" in lower

    # Missing directives (CSP Evaluator checkMissingDirectives) and frame-ancestors.
    if script_name is None:
        add("high", "missing_script_src", "script-src", None, "Neither script-src nor default-src: any script from anywhere may run.", "Add script-src with nonces or hashes and 'strict-dynamic' (a strict CSP).")
    if "object-src" not in directives and not (default is not None and [v.lower() for v in default] == ["'none'"]):
        add("high", "missing_object_src", "object-src", None, "No object-src (and default-src is not 'none'): plugins such as Flash could load and run script.", "Add object-src 'none'.")
    if "base-uri" not in directives:
        severity = "high" if (has_nonce or has_hash or strict_dynamic) else "low"
        add(severity, "missing_base_uri", "base-uri", None, "No base-uri: an injected <base> tag can redirect relative script URLs to the attacker's host (it defeats nonces).", "Add base-uri 'none' (or 'self').")
    if default is None:
        add("medium", "missing_default_src", "default-src", None, "No default-src: every fetch directive not listed (connect-src, img-src, frame-src…) is unrestricted.", "Add default-src 'self' (or 'none') as the fallback.")
    if "frame-ancestors" not in directives:
        add("medium", "missing_frame_ancestors", "frame-ancestors", None, "No frame-ancestors: the page can be framed by any site (clickjacking). default-src does not cover it.", "Add frame-ancestors 'none' (or 'self').")
    if "report-uri" not in directives and "report-to" not in directives:
        add("info", "no_reporting", "report-to", None, "No report-uri or report-to: violations are not reported.", "Add report-to (and report-uri for older browsers).")
    if "require-trusted-types-for" not in directives:
        add("info", "no_trusted_types", "require-trusted-types-for", None, "Trusted Types are not required: DOM XSS sinks accept plain strings.", "Consider require-trusted-types-for 'script'.")

    # Script sources with CSP3 semantics.
    for name in [n for n in SCRIPT if n in directives] or ([script_name] if script_name else []):
        values = directives[name]
        low = [v.lower() for v in values]
        nonce = any(is_nonce(v) for v in values) or (name != script_name and has_nonce)
        hashed = any(is_hash(v) for v in values) or (name != script_name and has_hash)
        dynamic = "'strict-dynamic'" in low
        if "'unsafe-inline'" in low:
            if nonce or hashed:
                add("info", "unsafe_inline_ignored", name, "'unsafe-inline'", "'unsafe-inline' is ignored by CSP2+ browsers because a nonce or hash is present; it only serves old browsers.", "Keep it only as a fallback for CSP1 browsers.")
            else:
                add("high", "script_unsafe_inline", name, "'unsafe-inline'", "'unsafe-inline' allows inline <script> and event handlers: injected markup runs.", "Remove it and use nonces or hashes.")
        if "'unsafe-eval'" in low:
            add("medium", "script_unsafe_eval", name, "'unsafe-eval'", "'unsafe-eval' allows code built from strings (eval, new Function, setTimeout with a string), a common DOM XSS path.", "Remove it if the code does not need string evaluation ('wasm-unsafe-eval' is enough for WebAssembly).")
        if "'unsafe-hashes'" in low:
            add("medium", "script_unsafe_hashes", name, "'unsafe-hashes'", "'unsafe-hashes' allows hashed inline event handlers, which can be reused by injected markup.", "Move handlers to scripts with addEventListener.")
        if dynamic and not (nonce or hashed):
            add("info", "strict_dynamic_without_nonce", name, "'strict-dynamic'", "'strict-dynamic' without a nonce or hash: no script can load at all.", "Add a nonce or hashes.")
        for value in values:
            lv = value.lower()
            ignored = dynamic and (nonce or hashed) and (lv in ("'self'", "*") or lv.endswith(":") and lv != "data:" or host_of(lv)[0] is not None and not lv.startswith("'"))
            if lv == "data:":
                add("high", "script_data_scheme", name, value, "data: in a script directive lets an attacker supply the script as a data: URL.", "Remove data: from script sources.")
            elif ignored:
                continue
            elif lv == "*":
                add("high", "script_wildcard", name, value, "* allows scripts from any host.", "Use nonces or hashes with 'strict-dynamic' instead of host lists.")
            elif lv in ("https:", "http:"):
                add("high", "script_plain_scheme", name, value, f"{value} allows scripts from any host over that scheme.", "Use nonces or hashes with 'strict-dynamic'.")
            elif lv in ("blob:", "filesystem:"):
                add("medium", "script_blob_scheme", name, value, f"{value} allows scripts from locally created URLs.", "Remove it unless the application needs it.")
            elif not lv.startswith("'") and not lv.endswith(":"):
                host, scheme = host_of(lv)
                if host is None:
                    continue
                if host.startswith("*."):
                    add("medium", "script_wildcard_host", name, value, f"{value} allows every subdomain of {host[2:]}.", "List the exact hosts, or move to nonces.")
                if host in BYPASS_HOSTS or any(b.startswith("*.") and host.endswith(b[1:]) for b in BYPASS_HOSTS):
                    add("high", "script_allowlist_bypass", name, value, f"{value} hosts JSONP endpoints or libraries (AngularJS and others) known to bypass CSP allowlists.", "Remove it; load scripts with nonces and 'strict-dynamic'.")
                if scheme == "http":
                    add("medium", "script_http_source", name, value, f"{value} loads script over plain HTTP: a network attacker can replace it.", "Use https://.")
                if re.fullmatch(r"\d+\.\d+\.\d+\.\d+", host):
                    add("info", "ip_source", name, value, "An IP address as a source: it cannot be checked for allowlist bypasses and 127.0.0.1 is the attacker's own machine.", "Use host names.")
        for value in values:
            if is_nonce(value):
                nonce_value = value[7:-1]
                if len(nonce_value) < 8:
                    add("medium", "short_nonce", name, "'nonce-…'", f"The nonce has {len(nonce_value)} characters; it should be at least 128 bits of randomness (22+ base64 characters), new on every response.", "Generate a fresh random nonce of 16+ bytes per response.")
                elif not re.fullmatch(r"[A-Za-z0-9+/_-]+={0,2}", nonce_value):
                    add("info", "nonce_charset", name, "'nonce-…'", "The nonce is not base64: browsers match it as text, but the specification expects base64.", "Use base64 nonces.")
        if not (nonce or hashed) and any(not v.startswith("'") for v in values) and "'none'" not in low:
            add("info", "host_allowlist", name, None, "The script policy relies on a host allowlist; such lists are bypassable on most sites (Weichselbaum et al., CCS 2016).", "Prefer a strict CSP: nonces or hashes with 'strict-dynamic'.")
        if (nonce or hashed) and "'unsafe-inline'" not in low and name == script_name:
            add("info", "no_unsafe_inline_fallback", name, None, "Nonce or hash without 'unsafe-inline': CSP1 browsers will block all inline scripts.", "Add 'unsafe-inline' as a fallback (CSP2+ browsers ignore it).")
        if dynamic and not any(v in low for v in ("https:", "http:")) and name == script_name:
            add("info", "no_allowlist_fallback", name, None, "'strict-dynamic' without https: fallback: browsers without CSP3 will block scripts that are not nonced.", "Add https: as a fallback (ignored by CSP3 browsers).")

    # Wildcards and schemes in object-src and base-uri.
    for name in ("object-src", "base-uri"):
        values = directives.get(name, default if name == "object-src" else None) or []
        for value in values:
            lv = value.lower()
            if lv == "*":
                add("high", f"{name.split('-')[0]}_wildcard", name, value, f"* in {name} allows any origin.", f"Use {name} 'none'.")
            elif lv in ("https:", "http:", "data:"):
                add("high", f"{name.split('-')[0]}_plain_scheme", name, value, f"{value} in {name} allows any host with that scheme.", f"Use {name} 'none'.")
    for name in ("style-src",):
        values = [v.lower() for v in directives.get(name, default or [])]
        if "'unsafe-inline'" in values:
            add("low", "style_unsafe_inline", name, "'unsafe-inline'", "'unsafe-inline' in styles allows CSS injection (data exfiltration through selectors, UI redress).", "Use nonces or hashes for styles where possible.")
    for name, values in directives.items():
        for value in values:
            if value.lower().startswith("http://") and name not in SCRIPT:
                add("low", "http_source", name, value, f"{value} is fetched over plain HTTP.", "Use https://.")
    if "upgrade-insecure-requests" in directives and directives["upgrade-insecure-requests"]:
        add("syntax", "unexpected_values", "upgrade-insecure-requests", None, "upgrade-insecure-requests takes no values.", "Remove the values.")

    if delivery == "report-only":
        add("info", "report_only", None, None, "Content-Security-Policy-Report-Only: nothing is blocked, violations are only reported; sandbox is ignored in report-only policies.", "Enforce the policy with the Content-Security-Policy header once the reports are clean.")
        if "report-uri" not in directives and "report-to" not in directives:
            add("medium", "report_only_without_reporting", None, None, "A report-only policy without report-uri or report-to does nothing at all.", "Add report-to or report-uri.")
    elif delivery == "meta":
        for name in ("frame-ancestors", "report-uri", "sandbox"):
            if name in directives:
                add("medium", "ignored_in_meta", name, None, f"{name} is ignored in a <meta> policy (CSP3 section 3.3).", "Send it in the HTTP header.")
    rank = {"high": 0, "medium": 1, "syntax": 2, "low": 3, "info": 4}
    findings.sort(key=lambda f: rank[f["severity"]])
    strict = (has_nonce or has_hash) and "object-src" in directives and "base-uri" in directives and not any(f["severity"] == "high" for f in findings)
    return {"directives": {name: directives[name] for name in order}, "effective_script_directive": script_name,
            "strict_csp": strict, "nonce": has_nonce, "hashes": has_hash, "strict_dynamic": strict_dynamic, "findings": findings,
            "counts": {s: sum(f["severity"] == s for f in findings) for s in rank}}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"policy", "delivery"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    policy = request.get("policy")
    if not isinstance(policy, str) or not policy.strip():
        fail("policy is required: the Content-Security-Policy header value")
    if len(policy) > 100_000:
        fail("policy is longer than 100000 characters")
    policy = re.sub(r"^\s*content-security-policy(-report-only)?\s*:\s*", "", policy, flags=re.I)
    delivery = request.get("delivery", "header")
    if delivery not in ("header", "report-only", "meta"):
        fail("delivery must be header, report-only or meta")
    policies = [p for p in policy.split(",") if p.strip()]
    results = [lint(p, delivery) for p in policies]
    if len(results) == 1:
        output = dict(results[0])
    else:
        output = {"policies": results, "note": "Several policies: a resource must pass every one of them."}
    output["delivery"] = delivery
    output["severity_scale"] = "high, medium, syntax, low, info (after CSP Evaluator's HIGH, MEDIUM, SYNTAX and INFO)"
    print(json.dumps(output))


if __name__ == "__main__":
    main()
