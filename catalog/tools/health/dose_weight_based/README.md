# dose_weight_based

Dose arithmetic for weight- or surface-based orders. It **never suggests a dose**: the amount per
kg or per m², the hard maximum single dose (`max_dose`, required) and everything else come from
the prescription or formulary in the input. The tool multiplies, divides, rounds and caps in
exact fractions and lists what it did in `flags`.

1. `calculated_dose = dose × weight_kg` (or `× bsa_m2` with `basis: "m2"`); with
   `dose_period: "per_day"` it is then divided by `frequency_per_day`.
2. With `rounding_increment`, rounded to a multiple of it (`nearest`, half up, or `down`).
3. If above `max_dose`, replaced by `max_dose` (rounded down to the increment) and flagged
   `CAPPED`. If `max_daily_dose` is given and `dose × frequency` exceeds it, each dose becomes
   `max_daily_dose / frequency` (rounded down), also flagged.
4. With `concentration` (a number per mL, or `{amount, volume_ml}` such as 120 mg in 5 mL), the
   volume is `dose / concentration` (the D/H × Q formula); with `volume_increment` the measured
   volume is rounded to the nearest step, or down when the nearest step would deliver more than
   the dose, and the dose in that volume is shown.

`effective_dose_per_basis` shows what the final dose is per kg or m² after capping and rounding.

The tests are worked by hand: 15 mg/kg for 20 kg is 300 mg and, at 120 mg/5 mL, 12.5 mL; 15
mg/kg for 80 kg (1,200 mg) capped at a given 1,000 mg; a per-day order split in three and capped;
a daily maximum; 375 mg/m² × 1.73 m² = 648.75 mg rounded to 650; and a volume that rounds down
rather than over-deliver.

Every output, errors included, carries the disclaimer: *Not medical advice and not a dosing
recommendation: arithmetic on the numbers given only. Every dose must be checked against the
prescription and a current formulary by a qualified clinician or pharmacist.*

Limits: no drug database, no renal or hepatic adjustment, no ideal or adjusted body weight, no
infusion rates; the unit label is not checked against the concentration's unit.
