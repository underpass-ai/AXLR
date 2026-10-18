# sun_times

Sunrise, sunset, solar noon, day length and the three twilights for a local date at a place,
plus the Sun's declination and the equation of time. Times come back as ISO 8601 local
date-times with their offset (an event can fall on another UTC date than the local one), to the
second, or rounded to the nearest minute with `rounding: "minute"` (as almanacs print them).

The zone is an IANA name (`timezone: "Europe/Madrid"`, which needs the system's tz database) or
a fixed `utc_offset` (`"+02:00"`, `5.5`); `UTC` is the default and needs no database. An unknown
zone, or a system without tzdata, is a usage error (exit 2) that suggests `utc_offset`.

## Method

The NOAA solar calculator's formulas (the spreadsheet of NOAA's Global Monitoring Laboratory,
also used by the astral library), which are Meeus' low-accuracy Sun (*Astronomical
Algorithms*, chapter 25) and Smart's equation of time (Meeus 28.3):

- From the Julian century `T`: mean longitude `L0`, mean anomaly `M`, eccentricity `e`, the
  equation of centre, true and apparent longitude (`− 0.00569° − 0.00478° sin Ω`), mean
  obliquity corrected by `0.00256° cos Ω`, declination `δ = asin(sin ε sin λ)`.
- Equation of time `E = 4·deg(y sin 2L0 − 2e sin M + 4ey sin M cos 2L0 − ½y² sin 4L0 −
  1.25e² sin 2M)` minutes, `y = tan²(ε/2)`.
- Solar noon (UTC minutes) `720 − 4·longitude − E`; an event at solar altitude `h0` has hour
  angle `H = acos((sin h0 − sin φ sin δ)/(cos φ cos δ))` and happens `4H` minutes before or after
  noon. `h0` is −0.833° for sunrise and sunset (refraction plus the Sun's semi-diameter, sea
  level) and −6°, −12°, −18° for civil, nautical and astronomical twilight. Each event is
  iterated with the Sun's position at the event itself, as NOAA's web calculator does, and
  the solar noon chosen is the one that falls on the requested local date.
- When the Sun never reaches `h0` the event is `null`: sunrise/sunset give
  `day_status: "polar_day"` or `"polar_night"` (or `sun_rises_only`/`sun_sets_only` on the day a
  polar period begins or ends), and a twilight gives `status: "sun_always_above_-6_deg"` and so
  on. In a polar night the twilights usually still exist.
- `datetime` adds the Sun's position at that instant: declination, apparent longitude,
  obliquity, equation of time, and NOAA's geometric (unrefracted) elevation and azimuth. The
  formulas take the instant as given and ignore ΔT, as NOAA does.

## Tests

- The USNO one-day table for Madrid (40.4168 N, 3.7038 W) on 2026-06-21 in UT
  (https://aa.usno.navy.mil/api/rstt/oneday?date=2026-06-21&coords=40.4168,-3.7038&tz=0, fetched
  2026-10-18): civil twilight begins 04:12, rise 04:45, transit 12:17, set 19:48, civil twilight
  ends 20:22. The minute-rounded output equals USNO's for rise, transit and both civil
  twilights; NOAA's sunset is 19:48:31, which rounds to 19:49, so that one is asserted as "within
  19:48" from the second-precision output. USNO and NOAA use different solar theories and
  agree to about a minute.
- Meeus, Example 25.a, 1992 October 13 at 0h: apparent declination −7.78507° (matched to five
  decimals) and apparent longitude 199.90895° (NOAA's coefficients give 199.90894°).
- Meeus, chapter 28, the same instant: the book's equation of time is 13m 42.6s; the tool's
  Smart-formula value is 13.71089 min = 13m 42.7s; the test asserts the shared 13.71 min.
- Polar day at Tromsø and the North Pole in June, polar night at Tromsø in December with civil
  twilight still present, and the usage errors.

## Limits

The low-accuracy Sun is good to about 0.01°, so times are good to about a minute between
latitudes ±72° and get worse near the polar circles, where the Sun skims the horizon. No
observer elevation, no local horizon, standard refraction only. Years 2 to 9998, but the
formulas are meant for a few centuries around 2000.
