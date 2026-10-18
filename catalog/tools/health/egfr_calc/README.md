# egfr_calc

Estimated glomerular filtration rate in adults by the race-free CKD-EPI 2021 equations (Inker
et al., *N Engl J Med* 2021;385:1737-49), with the KDIGO 2012 G stage.

Creatinine (`Scr`) is in mg/dL, or µmol/L divided by 88.4. With κ = 0.7 (female) or 0.9 (male):

- **creatinine** (`egfr_cr`): `142 × min(Scr/κ, 1)^α × max(Scr/κ, 1)^−1.200 × 0.9938^age ×
  1.012 [female]`, α = −0.241 female, −0.302 male;
- **creatinine-cystatin C** (`egfr_cr_cys`, when `cystatin_c` in mg/L is given): `135 ×
  min(Scr/κ, 1)^β × max(Scr/κ, 1)^−0.544 × min(Scys/0.8, 1)^−0.323 × max(Scys/0.8, 1)^−0.778 ×
  0.9961^age × 0.963 [female]`, β = −0.219 female, −0.144 male.

Each result is given unrounded (two decimals) and, as the National Kidney Foundation's
implementation guidance asks, as a whole number (`egfr`) reported between 5 and 150 (`reported`
is `<5` or `>150` outside). The stage comes from the whole number: G1 ≥ 90, G2 60–89, G3a 45–59,
G3b 30–44, G4 15–29, G5 < 15. Under 18 the tool refuses ("do not calculate"), as the NKF test
table does.

The tests are the eight age, sex and creatinine combinations in the NKF's example IT ticket for
implementing the 2021 equation (age 18 and 90, male and female, at and around κ; e.g. 90-year-old
female, 1.50 mg/dL → 33), the "do not calculate" row for under 18, and two cases worked by
hand: a 50-year-old woman with 1.0 mg/dL is 142 × 1.4286^−1.2 × 0.9938^50 × 1.012 = 142 × 0.6518
× 0.7328 × 1.012 = 68.6, reported 69 (some summaries quote 68 by truncating); with cystatin C
1.0 mg/L the combined equation gives 74.0.

Every output, errors included, carries the disclaimer: *Not medical advice: an arithmetic
estimate from the published CKD-EPI 2021 equations. Verify every result with a qualified
clinician before any clinical use.*

Limits: needs an IDMS-standardised creatinine and a steady state; not valid in acute kidney
injury, pregnancy, extremes of muscle mass or diet, or children (use a paediatric equation);
it does not check the laboratory's measuring range; staging CKD also needs chronicity and
albuminuria (the A categories), which the tool does not take.
