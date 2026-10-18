# base_convert

Number representations, exactly.

- `convert`: `value` is read in `from_base` (2–36, digits 0–9 then A–Z, case-insensitive; a
  sign, one point and `_` separators allowed; `p/q` fractions too). Without `from_base` a `0x`,
  `0o` or `0b` prefix sets the base, else 10. The value becomes an exact `Fraction` and is
  written in each `to_base` (default 2, 8, 10 and 16). Fractional digits come from long
  division with the remainders remembered, so a repeating block is detected exactly and shown
  in parentheses (0.1 in base 2 is `0.0(0011)`, 1/7 in base 10 is `0.(142857)`); if no period
  appears within `fraction_digits` (32 by default) the digits end in `...` with
  `truncated: true`.
- `to_roman` / `from_roman`: 1 to 3999 in the standard form. Input is validated against the
  pattern `M{0,3}(CM|CD|D?C{0,3})(XC|XL|L?X{0,3})(IX|IV|V?I{0,3})`, so `IIII`, `IC`, `VX`
  and `MMMM` are refused rather than guessed at. Lower case is accepted.
- `to_twos_complement`: an integer in a `bits`-wide signed range (−2^(bits−1) to
  2^(bits−1) − 1; anything else is refused) as a bit pattern, hex and the unsigned reading;
  `from_twos_complement` reads a bit string, its length being the width.
- `to_bcd` / `from_bcd`: packed 8421 BCD, four bits per decimal digit; nibbles 1010–1111 are
  refused.

The tests check textbook values: 255 = 0xFF = 0o377, MCMXCIV = 1994, −1 in 8-bit two's
complement = 11111111, 10000000 = −128, and the Python tutorial's statement that decimal 0.1 is
the repeating binary fraction 0.000110011….

Limits: values up to 2000 characters, two's complement up to 4096 bits, BCD up to 1000 digits; no
balanced or negative bases, no Roman numerals above 3999 (vinculum) or with zero.
