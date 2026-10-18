# csp_lint

Reads a `Content-Security-Policy` header value and lists its weaknesses, each with a severity
(`high`, `medium`, `syntax`, `low`, `info`, after Google's CSP Evaluator), the directive and
source it concerns, a description and a fix. It also returns the parsed directives, which
directive governs scripts (`script-src`, else `default-src`) and whether the policy is a
"strict CSP" (nonces or hashes, `object-src` and `base-uri` set, no high finding).

## Semantics (CSP Level 3)

- Directives are split on `;`, names are case-insensitive, and a repeated directive is ignored
  after its first occurrence (reported). Commas separate whole policies, which are linted one by
  one (a resource must pass all of them).
- A nonce or hash in a script directive makes `'unsafe-inline'` ignored by CSP2+ browsers, so it
  is reported as an `info` fallback rather than `high`.
- `'strict-dynamic'` with a nonce or hash makes host sources, `'self'`, `*` and scheme sources
  (`https:`, `http:`) ignored, so they are not reported; `data:` still is.
- `default-src` is the fallback for fetch directives, never for `base-uri`, `form-action` or
  `frame-ancestors`.

## Checks

| Severity | Check |
|:--|:--|
| high | `'unsafe-inline'` in scripts without nonce or hash; `*`, `https:`, `http:` or `data:` in scripts; known JSONP/library hosts (Google APIs, cdnjs, jsDelivr, unpkg… CSP Evaluator's bypass lists, abridged); no `script-src` nor `default-src`; no `object-src` (unless `default-src 'none'`); `*` or schemes in `object-src` and `base-uri`; no `base-uri` when nonces, hashes or `'strict-dynamic'` are used |
| medium | `'unsafe-eval'`, `'unsafe-hashes'`; `*.host` wildcards and `http://` script hosts; nonces shorter than 8 characters; no `default-src`; no `frame-ancestors` (clickjacking; `default-src` does not cover it); report-only with no reporting endpoint; directives that a `<meta>` policy ignores |
| syntax | unknown directive, keyword without quotes (`self`), invalid keyword, a directive name inside another's values (missing `;`), `'none'` mixed with sources, duplicate directive |
| low | `'unsafe-inline'` in styles; plain `http://` sources; no `base-uri` without nonces |
| info | `'unsafe-inline'` ignored next to a nonce; host allowlist instead of nonces; nonce or hash without the `'unsafe-inline'` fallback; `'strict-dynamic'` without the `https:` fallback or without a nonce; IP sources; deprecated directives (`report-uri`, `block-all-mixed-content`, `plugin-types`…); no reporting; no Trusted Types; report-only delivery |

## Tests

Google's recommended strict policy from csp.withgoogle.com (`object-src 'none'; script-src
'nonce-…' 'unsafe-inline' 'unsafe-eval' 'strict-dynamic' https: http:; base-uri 'none';
report-uri …`), for which CSP Evaluator reports no high finding: the full list of findings is
asserted; `script-src 'unsafe-inline'` (missing `object-src` and inline scripts, high); a policy
with a bypassable host, `data:`, `object-src *`, a subdomain wildcard and an `http://` host;
syntax slips; a tight hash-based policy with no high or medium finding; short nonce, duplicate
directive, report-only and meta delivery, two comma-separated policies, and usage errors.

## Limits

Static analysis of the text: it cannot tell whether nonces are really random per response or
whether an allowlisted host is safe for your site; the bypass host list is short. Only CSP
semantics as of Level 3 (2024 drafts); `trusted-types` policy names and `sandbox` flags are not
validated.
