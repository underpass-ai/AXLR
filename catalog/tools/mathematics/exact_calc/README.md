# exact_calc

A calculator that does not round. The expression is parsed with Python's `ast` and only
numbers, names, `+ - * / // % **` (`^` is read as `**`), unary signs and a fixed list of
functions are evaluated; anything else (attributes, strings, comprehensions, calls to other
names) is refused. Nothing is passed to `eval`.

Numbers are `Fraction`s, and decimal literals are read from their source text, so `0.1` is
exactly 1/10 and `0.1 + 0.2` is 3/10. Integer powers, `factorial`, `binomial`, `perm`,
`gcd`, `lcm`, `floor`, `ceil`, `round` stay exact, as do roots of perfect squares and cubes
(`sqrt(9/4)`, `8**(1/3)`). Irrational results (`sqrt(2)`, `pi`, `e`, `ln`, `log`, `exp`,
trigonometry) are computed in `Decimal` with `precision` significant digits (30 by default)
and reported with `"exact": false` and `"result": null`; `decimal` carries the value.

Limits: exponents up to ±10000 and factorials up to 5000, to keep answers bounded;
trigonometric functions go through binary floats (about 16 digits) whatever the precision.
