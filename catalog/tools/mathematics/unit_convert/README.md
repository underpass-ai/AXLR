# unit_convert

Unit conversion that checks dimensions and keeps the factor exact.

Every unit in the bundled `units.json` is stored as an exact fraction of SI base units and a
dimension vector over (m, kg, s, A, K, mol, cd). A unit expression is parsed into a product of
units with integer powers: `*`, `·`, `.` or a space multiply, `/` divides the next factor
(`J/kg/K` is J/(kg·K)), parentheses group, and powers are written `^2`, `**2`, `m2`, `s-1` or
`m²`. A name is looked up as a symbol or alias first and only then as prefix + unit, so `min`
is the minute, `mm` the millimetre, `Pa` the pascal, `pt` the pint and `ct` the carat; SI
prefixes from quecto to quetta (µ, μ or u for micro) attach to the SI and SI-accepted units
(`kWh`, `mAh`, `keV`, `mL`, `kt` for kilotonne) but not to customary ones. The conversion is
refused (exit 2) unless both sides have the same dimension vector; without `to` the result is
given in SI base units.

The answer has the exact `factor` and `result_exact` as fractions and decimal renderings, exact
when the expansion terminates (`1609.344`) and otherwise at `precision` significant digits with
`result_is_rounded: true` (100 km/h = 250/9 m/s = 27.7777777777778).

Temperatures: when `from` and `to` are each a single temperature unit (`K`, `degC` or `°C`,
`degF` or `°F`, `degR`), the offsets are applied, T/K = (t + offset) × scale (98.6 °F = 37 °C
exactly). Inside a compound unit, or with `temperature: "difference"`, degrees are intervals
(1 °F = 5/9 K), and the answer says so.

Degrees, arcminutes and arcseconds are π/180 rad and so irrational: they carry a 50-digit value
and make the result `result_exact: null`.

The table (`list_units: true` lists it) is written by `tools/catalog/data/unit_convert.py` from
the SI Brochure (9th ed., Tables 2, 4, 7, 8), NIST SP 811 Appendix B and NIST Handbook 44: the
inch is 0.0254 m and the pound 0.45359237 kg exactly (1959 agreement), the US gallon 231 in³,
the imperial gallon 4.54609 L, g_n 9.80665 m/s², the IT calorie 4.1868 J, the Btu_IT
1055.05585262 J, the conventional mmHg 133.322387415 Pa, the Julian year 365.25 d. Each answer
names the table's date and sources.

The tests check NIST SP 811 Appendix B factors (mile, kW·h, Btu/(lb·°F), US gallon,
horsepower, psi), 0 °C = 273.15 K from the SI Brochure, and hand-checked cases (100 km/h,
98.6 °F, −40 °C = −40 °F).

Limits: no currency, data sizes, logarithmic units (dB, pH), US survey units, or fractional
powers; ambiguous juxtapositions such as `Tm` are read as prefix + unit (terametre), so write
`T*m` for tesla metres.
