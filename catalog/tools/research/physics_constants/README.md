# physics_constants

Looks up the CODATA 2022 recommended values of the fundamental physical constants: value,
standard uncertainty, relative uncertainty, unit, the concise notation physicists write
(`9.1093837139(28)e-31`) and whether the value is exact.

## Lookup

- `symbol`: an exact symbol from the bundled list (`c`, `h`, `hbar`, `e`, `k`, `N_A`, `G`,
  `m_e`, `m_p`, `alpha`, `R`, `F`, `mu_0`, `epsilon_0`, `sigma`, `a_0`, `R_inf`…); case is
  tried exactly first, then ignored.
- `name`: the quantity name as NIST prints it, case-insensitive (`"Bohr radius"`).
- `query`: tries an exact name, then a symbol, then keywords: every word of the query must
  begin a word of the name (`"proton mag mom"` finds "proton mag. mom."). Names that start
  with the query come first, then shorter names. `limit` caps the list (default 10).
- `list: true`: every name with its symbols.

## Data

`constants.json` holds all 355 rows of NIST's "Fundamental Physical Constants — Complete
Listing, 2022 CODATA adjustment" (`https://physics.nist.gov/cuu/Constants/Table/allascii.txt`),
fetched on 2026-10-18 by `tools/catalog/data/physics_constants.py`. NIST's data are a work of
the U.S. Government and in the public domain. Values are kept as the strings NIST prints, with
the digit-group spaces removed, so no digit is lost to floating point; `value_float` is a
convenience. "(exact)" becomes `exact: true`, and values NIST truncates with "..." (exact
constants with infinitely many digits, such as the molar gas constant `R = N_A k`) carry a note.

The symbols are not in the NIST file: they are a short list written in the generator for the
common constants, said so in the table. Every answer carries the table's source, licence and
fetch date. A new CODATA adjustment is a rerun of the generator and a new version of the tool.

## Tests

Against the NIST listing itself: `c = 299 792 458 m s^-1` exact, `h = 6.626 070 15 e-34 J Hz^-1`
exact, the electron mass `9.109 383 7139(28) e-31 kg`, `G = 6.674 30(15) e-11` and the
fine-structure constant `7.297 352 5643(11) e-3`; plus keyword ranking, the truncation flag,
the 355-row count and the input errors.

## Limits

No unit conversion and no arithmetic between constants (correlations between CODATA values
are not in the listing, so derived uncertainties would be wrong). Symbols exist only for the
common constants; the rest are found by name or keywords.
