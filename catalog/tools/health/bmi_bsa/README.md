# bmi_bsa

Body-mass index and body-surface area from weight and height, in metric (kg, cm) or imperial
(lb, in; converted with the exact 1959 factors 0.45359237 kg/lb and 2.54 cm/in).

**BMI** is `kg / m²`, kept as an exact fraction (`bmi_exact`) and shown to two and one
decimals. The category is the WHO adult classification applied to the exact value: under 16
severe thinness, 16 to under 17 moderate, 17 to under 18.5 mild thinness (all
"Underweight"), 18.5 to under 25 normal range, 25 to under 30 pre-obese, 30, 35 and 40 the
lower bounds of obese classes I, II and III. Because the cut-off applies to the exact value, a
BMI of 24.96 is "Normal range" even though it rounds to 25.0. With `age` under 18 the adult
categories are withheld (`null`) and a note says children and adolescents need BMI-for-age
percentiles.

**BSA** in m², height H in cm and weight W in kg:

- Mosteller (1987): `√(H·W/3600)`, computed in 60-digit decimals;
- DuBois & DuBois (1916): `0.007184 · W^0.425 · H^0.725`;
- Haycock (1978): `0.024265 · W^0.5378 · H^0.3964`;
- Gehan & George (1970): `0.0235 · W^0.51456 · H^0.42246`;

each rounded half-up to four decimals.

The tests check Mosteller's 180 cm and 80 kg = 2.00 m² (exact: √4), the DuBois formula for the
same person (1.9964, worked by hand with logarithms), the two examples of the CDC's adult BMI
page (68 kg and 165 cm is 24.98; 150 lb and 65 in is 24.96) and the WHO boundaries.

Every output, errors included, carries the disclaimer: *Not medical advice: arithmetic from
published formulas only. Verify every result with a qualified clinician before any clinical
use.*

Limits: BMI says nothing about body composition, and the WHO cut-offs are for adults (some
populations use lower action points); no percentile tables for children; the BSA formulas
were fitted on small samples and differ by a few per cent from each other.
