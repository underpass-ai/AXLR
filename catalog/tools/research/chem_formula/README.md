# chem_formula

Parses a chemical formula and returns the element counts, the Hill formula, the molar mass and
the mass percent of each element.

## Syntax

- Elements are a capital letter and an optional lower-case letter (`Co` is cobalt, `CO` carbon
  monoxide), each with an optional count. Unicode subscripts (`H₂O`) are read as digits.
- Groups in `( )`, `[ ]` or `{ }` with a count, nested to any depth: `Ca3(PO4)2`, `K4[Fe(CN)6]`.
- Hydrates and adducts: parts joined by `·`, `•`, `∙`, `⋅`, `.` or `*`, each with an optional
  leading coefficient: `CuSO4·5H2O`, `CuSO4*5H2O`, `Na2CO3.10H2O`. A leading coefficient on the
  whole formula multiplies it too (`2H2O`). Because `.` is the hydrate dot, fractional
  subscripts (`Fe0.95O`) are not supported.
- Charges at the end: `SO4^2-`, `SO4^{2-}`, `SO4^-2`, `SO4 2-`, `SO₄²⁻`, or repeated signs
  (`NH4+`, `OH-`, `Fe+++`). A charge of more than 1 needs `^`, a space or superscripts:
  `SO42-` reads as `S O42` with charge 1-.

## Arithmetic

`molar_mass = Σ count × atomic weight`, summed in `Decimal`, so the full sum (`molar_mass`) is
exact for the tabulated values, and `molar_mass_rounded` and each `mass_percent` are rounded
half up to `decimals` (default 2). `abridged_uncertainty_sum` adds count × the abridged
uncertainty of each element: a conservative bound, not a standard uncertainty. The molar mass
of an ion ignores the electrons, as is usual. The Hill formula puts C then H first when carbon
is present, otherwise every element alphabetically.

## Data

`elements.json` is IUPAC/CIAAW's table "Abridged Standard Atomic Weights 2024"
(`https://www.ciaaw.org/abridged-atomic-weights.htm`, fetched 2026-10-18 by
`tools/catalog/data/chem_formula.py`): the values of the "Atomic Weights 2021" report
(Prohaska et al., Pure Appl. Chem. 94 (2022) 573–600) with CIAAW's 2024 revisions of Gd, Lu and
Zr, abridged to five significant figures where natural variation allows (H 1.0080, C 12.011,
O 15.999, S 32.06…). These are the abridged standard values, not the "conventional" single
values CIAAW gives for interval elements. The 34 elements without a standard atomic weight (Tc,
Pm, Po to Ac, Np and beyond) are null; supply a value with `masses` (for example the mass
number of the isotope you mean). Every answer names the table and its fetch date.

## Tests

Textbook molar masses: water 18.015 g/mol (11.19 % H), calcium hydroxide 74.09, copper(II)
sulfate pentahydrate 249.68, glucose 180.16 (40.00 % C); plus nested brackets, the Hill order,
the charge notations, the `masses` override and the parse errors.

## Limits

No isotopes (`D`, `¹³C`), no structural formulas, no fractional or variable subscripts, no
balancing of equations. Abridged weights are for normal terrestrial materials; samples of
unusual origin (the `g`, `m`, `r` notes, kept in the table) may differ.
