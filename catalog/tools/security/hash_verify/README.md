# hash_verify

Checksums and HMACs with Python's `hashlib` and `hmac`.

Source: exactly one of `text` (UTF-8), `hex` (raw bytes; spaces and colons ignored) or `file`
(a relative path inside the working directory; absolute paths and `..` are refused).

`algorithms` picks any of MD5, SHA-1, SHA-224/256/384/512, SHA3-224/256/384/512, BLAKE2b and
BLAKE2s (SHA-256 by default). With `hmac_key` every digest is an HMAC (RFC 2104) under that
key. With `expected` the result says whether one of the digests equals it, compared in
constant time; the expected value may be upper case or a whole `sha256sum` line, and when no
algorithm is given it is guessed from the length (32 hex digits MD5, 40 SHA-1, 64 SHA-256…).

MD5 and SHA-1 come with a warning: they are fine for matching a published checksum and wrong
for anything that must resist a forger.

The tests are the standard vectors: FIPS 180-4 and 202 for "abc", RFC 1321 for the empty
string, and the HMAC-SHA256 example of "The quick brown fox…" with key "key".

Limits: files are read whole into memory; no password hashing (use a KDF such as scrypt or
Argon2 for that).
