# iban_validate

Checks IBANs the way a bank does before accepting a transfer order:

1. only letters and digits after removing spaces, dashes and an `IBAN` prefix;
2. a known country code and that country's exact length (SWIFT IBAN Registry, lengths as
   published in 2025; the output names the table);
3. the ISO 7064 mod-97 check: the first four characters moved to the end, letters turned into
   numbers (A = 10 … Z = 35), the whole number modulo 97 must be 1;
4. for ES, DE, FR, IT, PT, GB, IE, NL, BE, AT, CH, PL and SE, the BBAN split into its fields;
   for Spain, the two national check digits of the old CCC (weights 1, 2, 4, 8, 5, 10, 9, 7,
   3, 6) as well, which catch an IBAN computed from a mistyped account number: its mod-97
   check passes, its CCC check does not.

With `country` and `bban` it computes the two check digits (98 − mod 97) and returns the
IBAN.

Limits: it says whether an IBAN is well formed, never whether the account exists or is open.
National checks beyond Spain's are not verified.
