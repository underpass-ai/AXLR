# unit_lab_convert

Converts a laboratory result between mass concentration (mg/dL, mg/L, g/L) and amount
concentration (mmol/L, µmol/L) through the analyte's molar mass, in exact fractions.

The molar mass is computed from the formula with the IUPAC/CIAAW conventional atomic weights
(H 1.008, C 12.011, N 14.007, O 15.999, Ca 40.078), so every factor can be traced:

| analyte | counted as | formula | g/mol | usual factor |
|:--|:--|:--|--:|--:|
| glucose | glucose | C6H12O6 | 180.156 | mg/dL × 0.05551 = mmol/L |
| cholesterol_total, _hdl, _ldl | cholesterol | C27H46O | 386.664 | × 0.02586 |
| triglycerides | triolein (convention) | C57H104O6 | 885.453 | × 0.01129 |
| creatinine | creatinine | C4H7N3O | 113.120 | mg/dL × 88.40 = µmol/L |
| urea | urea | CH4N2O | 60.056 | × 0.1665 |
| bun | urea, from its nitrogen | N2 | 28.014 | × 0.3570 |
| uric_acid | uric acid | C5H4N4O3 | 168.112 | mg/dL × 59.48 = µmol/L |
| calcium | total calcium | Ca | 40.078 | × 0.2495 |

Two traps are handled explicitly. Triglycerides have no single molar mass; laboratories convert
as if all were triolein, which is where the usual 0.01129 comes from. BUN (blood urea nitrogen)
is the mass of the two nitrogen atoms of urea, so 1 mmol/L of urea is 2.8014 mg/dL of BUN but
6.0056 mg/dL of urea; they are separate analytes.

`result` is rounded half-up to `decimals` (default 2); `result_exact` is the exact fraction and
`factor` the multiplier for this pair of units. `list_analytes: true` prints the table.

The tests check the known values 100 mg/dL glucose = 5.55 mmol/L and 1 mg/dL creatinine = 88.4
µmol/L, the conventional factors for cholesterol (0.02586), triglycerides (0.01129), BUN
(14 mg/dL = 5.0 mmol/L urea) and uric acid (59.48), and the reverse direction.

Every output, errors included, carries the disclaimer: *Not medical advice: a unit conversion
only. Reference ranges and units differ between laboratories; verify with a qualified clinician
before any clinical use.*

Limits: no mEq/L, no ionised calcium or albumin correction, no enzyme activities (U/L), no
proteins (g/dL) or haemoglobin; the factors are for the substance named, so a laboratory using
a different convention (for example glucose as "180" g/mol) differs in the third digit.
