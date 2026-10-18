# jwt_decode

Shows what is inside a JSON Web Token. **It does not verify the signature**, and every output
says so (`signature_verified: false` and a `warning`): a decoded claim is only what the sender
wrote, and must not be trusted until a verifier holding the right key, with the algorithm
pinned, has accepted the token.

## What it does

The compact form is split on dots: three parts are a JWS (header, payload, signature), five a
JWE (only its protected header is readable). Each part is base64url without padding (RFC 7515
section 2), decoded by a small table-driven decoder in the program; `+` or `/` (plain base64)
is refused with a clear message. A leading `Bearer ` is removed.

The output has the header, `alg`, `typ`, `kid` and `cty`, the payload, the registered claims
(`iss`, `sub`, `aud`, `exp`, `nbf`, `iat`, `jti`) with `iat`, `nbf` and `exp` as UTC dates,
the token's lifetime (`exp − iat`) and the signature length. With `now` (Unix seconds or an ISO
date; the clock is never read), `validity` says whether the token is expired (RFC 7519 4.1.4:
the time must be *before* `exp`, so at `exp` it is expired), not yet valid (4.1.5: before
`nbf`), its age and the seconds left, with `leeway` seconds of skew allowed.

Findings, with a severity, after RFC 8725 (JWT Best Current Practices):

- `alg_none` (critical): an unsecured JWT that anyone can forge;
- `hmac_confusion`: HS256/384/512 tokens are what an attacker sends to a verifier that holds an
  RSA or EC public key and lets the token choose the algorithm (the public key then becomes the
  HMAC secret); `asymmetric` reminds the verifier to pin RS/PS/ES/EdDSA;
- `embedded_jwk`, `embedded_jku`, `embedded_x5u`, `embedded_x5c`: key material or key URLs
  inside the token (key injection if the verifier uses them);
- `kid_injection`: path or quote characters in `kid`;
- `empty_signature`, `signature_length` (HS and ES signatures have fixed lengths), `no_exp`,
  `exp_before_iat`, `iat_future`, non-numeric time claims, `crit`, unusual `typ`, nested JWTs.

## Tests

The RFC 7519 section 3.1 example token (header `typ` JWT, `alg` HS256; claims `iss` joe,
`exp` 1300819380, which is 2011-03-22T18:43:00Z, and `http://example.com/is_root` true; its
header contains CR LF whitespace, as in the RFC), checked one second before and at `exp`, with
a leeway, and with a `Bearer ` prefix; the RFC 7519 section 6.1 unsecured example (`alg` none);
the RFC 7516 JWE header `{"alg":"RSA-OAEP","enc":"A256GCM"}`; tokens built for the tests with
`jku`, `nbf` in the future and without `exp`; and the usage errors.

## Limits

No signature verification or decryption (by design: no keys are taken); JWS JSON serialisation
is not parsed; `aud` and `iss` are shown but not checked against expected values.
