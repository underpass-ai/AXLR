# cert_inspect

Reads a PEM X.509 certificate and reports what `openssl x509` sees in it: subject and issuer
(RFC 2253 order, most specific first), serial in hex, version, `notBefore` and `notAfter` as
ISO 8601 UTC, the SHA-256 fingerprint of the DER encoding, the subject alternative names,
the public key type, size and curve, the signature algorithm, whether it is self-issued
(subject equals issuer) and whether basicConstraints says `CA:TRUE`.

`now` is required: the days left are counted from it, never from the clock, so the same input
always gives the same answer. `days_to_expiry` is `(notAfter − now)` in whole days rounded down
(−1 means it expired within the last 24 hours). `status` is `valid`,
`expired` (now after notAfter) or `not_yet_valid` (now before notBefore). Warnings: expired,
not yet valid, under 30 days left, RSA under 2048 bits, an MD5 or SHA-1 signature, no SAN
(browsers match host names against the SAN only, per RFC 6125 and the CA/Browser Forum rules).

## How it works

The program is bash with coreutils and `openssl` (OpenSSL 1.1.1 or 3.x; the field labels that
differ between versions are not matched literally). It reads the JSON object from stdin with
`sed` (no jq or python): the `pem` string is taken as written, and its JSON escapes `\n`, `\r`,
`\/` and `\\` are undone; the first `BEGIN CERTIFICATE` block is kept and piped to
`openssl x509 -noout` (no temporary file). Dates are converted with GNU `date -u -d`. The
output is one JSON object whose strings are escaped by the script.

## Tests

A self-signed RSA-2048 certificate generated once for the tests with
`openssl req -x509 -newkey rsa:2048 -days 365 -set_serial 0x1a2b3c -subj "/C=ES/O=AXLR Test/CN=test.example" -addext "subjectAltName=DNS:test.example,DNS:www.test.example,IP:127.0.0.1"`
on 2026-10-18: the tests assert the subject, serial, SANs and the 365-day validity it was
made with, the fingerprint checked separately with `openssl x509 -outform DER | sha256sum`,
and days to expiry counted by hand. A second, EC P-256 certificate with `-days 30` and no SAN
checks the key fields, an expired status and the missing-SAN warning; plus the usage errors
(no `pem`, no `now`, a bad date, text that is not PEM, corrupted base64).

## Limits

No chain building, signature verification, revocation (CRL/OCSP) or host-name matching; only
the first certificate of a bundle is read; DER input is not accepted (convert it to PEM); JSON
`\uXXXX` escapes inside `pem` are not decoded (a PEM never needs them). Requires GNU `date`.
