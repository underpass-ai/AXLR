# moon_phase

The Moon's phase at an instant (`operation: "at"`, the default) or the list of a year's
principal phases (`operation: "year"`). Instants are UTC: a trailing `Z` or an offset is
accepted and converted, a time without one is taken as UTC.

For an instant it gives the phase name, the phase angle (Sun-Moon angle seen from the Moon,
0° at full, 180° at new), the illuminated fraction, the mean elongation, the age in days since
the previous new Moon, that new Moon, and the next new Moon, first quarter, full Moon and last
quarter in time order. Each phase carries `utc` (to the second), `utc_minute` (rounded, as
almanacs print it), `td` and `jde` (dynamical time, Meeus' own result) and the lunation `k`.

## Method

- **Phase times**: Meeus, *Astronomical Algorithms*, chapter 49. The mean phase
  `JDE = 2451550.09766 + 29.530588861k + …` for lunation `k` (integer for new Moon, +¼, +½, +¾
  for the others), then the periodic terms in the Sun's and Moon's anomalies `M`, `M′`, the
  argument of latitude `F` and the node `Ω` (25 terms for new and full Moon, 25 plus the `W`
  correction for the quarters, with `E` for the eccentricity), and the 14 planetary
  arguments `A1…A14`. The result is good to a few seconds of dynamical time.
- **TD to UTC**: `UT = TD − ΔT`, ΔT from the Espenak-Meeus polynomials (NASA's *Five Millennium
  Canon of Solar Eclipses*). For 2026 they give 75.4 s, while the measured ΔT is about 69 s,
  so modern UTC times are about 6 s early; the output reports the ΔT used.
- **Illuminated fraction**: Meeus' lower-accuracy formula 48.4, the phase angle `i` from the
  mean elongation `D` and the anomalies with six periodic terms, and `k = (1 + cos i)/2`. It is
  good to about 0.002-0.003 in `k`, not the full chapter-48 accuracy.
- **Phase name**: the principal phase when the instant is within 12 hours of it; otherwise
  waxing crescent (new to first quarter), waxing gibbous (to full), waning gibbous (to last
  quarter) or waning crescent (to the next new Moon), by the actual phase times. `waxing` is
  true from new to full.

## Tests

- Meeus Example 49.a: the new Moon of 1977 February is JDE 2443192.65118 = 1977 February 18,
  03:37:42 TD, reproduced to the second.
- Meeus Example 48.a, 1992 April 12 at 0h TD: the rigorous `k` is 0.6786; the lower-accuracy
  formula 48.4 used here gives 0.6802 (phase angle 68.88° against 69.08°), so the test asserts
  only the prefix 0.68.
- The USNO "Phases of the Moon" table for 2026 (https://aa.usno.navy.mil/api/moon/phases/year?year=2026,
  fetched 2026-10-18): 50 phases. Compared over the whole year the tool is within 41 s of
  USNO's minute values for all four phase types: 42 of the 50 round to the same minute, and
  the other 8 lie 31-41 s after USNO's rounded minute, just across the half-minute boundary
  (USNO rounds, and the ΔT difference moves the tool a few seconds). The tests assert the cases that round to the same minute: full
  Moon 10-26 04:12, last quarter 11-01 20:28, new Moon 11-09 07:02, first quarter 11-17
  11:48, the new Moon of 10-10 15:50, and January's full and new Moons.

## Limits

Geocentric: phase times and the fraction ignore the observer's position (topocentric
differences are under a minute for phases and small for the fraction). Years 1000-3000; ΔT,
and so UTC, is uncertain by many seconds in the past and by an unknown amount in the future.
No eclipses, perigee, moonrise or moonset.
