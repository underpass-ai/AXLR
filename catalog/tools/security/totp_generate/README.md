# totp_generate

One-time passwords as authenticator apps compute them. HOTP (RFC 4226): HMAC of the 8-byte
big-endian counter with the shared key, dynamic truncation (the low 4 bits of the last byte
pick an offset, 31 bits are read from there) and the result modulo 10^digits, left-padded with
zeros. TOTP (RFC 6238): the same with the counter `T = floor((time − t0) / period)`.

`time` (TOTP) and `counter` (HOTP) are required: the clock is never read, so a code can be
computed for any moment and the same input always gives the same output. The output carries
the code, the counter (decimal and as the 16-hex-digit value the RFC tables show), the start
and end of the step in UTC and the seconds left in it. The secret is not echoed.

The secret is base32 by default (RFC 4648; case, spaces, hyphens and missing `=` padding are
tolerated), as in `otpauth://` URIs and QR codes; `hex` and `ascii` are there for test vectors
and for systems that store raw keys. The base32 decoder is written out in the program.

With `code`, the output adds `verify`: the code is compared (in constant time) against the
counters `T − window … T + window`, nearest first, and the offset that matched is reported.
RFC 6238 section 5.2 recommends at most one step of drift on each side (the default).

## Tests

All eighteen codes of RFC 6238 Appendix B were checked (six times, each with SHA-1, SHA-256 and SHA-512;
note the RFC uses a 20-, 32- and 64-byte seed for the three hashes), and four of its rows are
in the tests, one given as an ISO date; RFC 4226 Appendix D counters 0 and 9 (755224, 520489),
the base32 form of its secret, verification with a window, and the usage errors.

## Limits

No `otpauth://` URI parsing or QR codes; no replay protection (a verifier must remember the
last counter it accepted); Steam Guard and other non-decimal variants are not supported.
