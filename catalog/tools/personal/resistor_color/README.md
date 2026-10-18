# resistor_color

Reads and writes the colour bands of fixed resistors and finds standard values.

**decode** takes the bands from the head end. With 3 or 4 bands the first two are digits, then
the multiplier, then the tolerance (none means ±20 %); with 5 or 6 bands the first three are
digits, then multiplier and tolerance, and a sixth band is the temperature coefficient. Colours
follow IEC 60062:2016: digits black 0 to white 9; multipliers pink ×10⁻³, silver ×10⁻², gold
×10⁻¹, black ×1 to white ×10⁹; tolerances silver 10 %, gold 5 %, brown 1 %, red 2 %, green
0.5 %, blue 0.25 %, violet 0.1 %, orange 0.05 %, yellow 0.02 %, grey 0.01 %; temperature
coefficients black 250, brown 100, red 50, orange 15, yellow 25, green 20, blue 10, violet 5,
grey 1 ppm/K. The answer gives ohms exactly, with an SI prefix, the tolerance limits and the E
series that contain the value. A black first band is accepted with a warning (it often means
the resistor is read backwards).

**encode** turns a value into the 4-band code (two digits, default gold 5 %) and the 5-band code
(three digits, default brown 1 %), or says why one is impossible (too many digits, multiplier
out of range); `tcr` adds the sixth band. Values may be plain numbers, SI text (`4.7k`, `10 kΩ`)
or RKM code (`4k7`, `R47`, `2M2`).

**nearest** finds the standard values just below and above in an E series (default E24) and
picks the nearer one on a logarithmic scale, the scale the series are built on, with the error
in percent.

The table (`codes.json`) is written by `tools/catalog/data/resistor_color.py` from IEC 60062:2016
and IEC 60063:2015 as reproduced in Wikipedia's "Electronic color code" and "E series of
preferred numbers" (retrieved 2026-10-18). The series are the official lists, not the formula
round(10^(n/m)): the generator checks them against the formula and finds exactly the documented
exceptions (eight in E24, such as 2.7 for 2.6 and 4.7 for 4.6; 9.20 for 9.19 in E192; none in
E48 and E96).

The tests check the standard example brown-black-red-gold = 1 kΩ ±5 %, the worked examples of
the Wikipedia article (red-violet-green-gold = 2.7 MΩ; 56 kΩ 2 % in five bands; the 2.26 kΩ 1 %
resistor red-red-blue-brown-brown), the E12 decade and the E24 and E192 exceptions.

Limits: no capacitor or inductor codes, no SMD numeric codes, no military or manufacturer
variants (old colour meanings for grey and white, gratuitous fifth bands); the direction of
reading is taken as given.
