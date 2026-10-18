# calendar_convert

Converts one date to every calendar it knows. The input names the source calendar (`from`)
and the date in it; the output gives the same day as an R.D. fixed day number, the Julian day
number (`jdn`), the Julian date at the given universal time (`jd`, and `mjd`), the weekday,
and the date in the proleptic Gregorian, Julian, ISO week, Hebrew and Islamic calendars.

## Method

Everything goes through R.D. fixed day numbers (R.D. 1 = Monday 1 January 1 CE, Gregorian),
with the integer algorithms of Dershowitz and Reingold, *Calendrical Calculations*:

- **Gregorian** and **Julian**: D&R's `fixed-from-gregorian`, `gregorian-from-fixed` and their
  Julian counterparts, proleptic in both directions. Years are **astronomical**: year 0 is
  1 BCE and −43 is 44 BCE; each date also carries an `era` text ("44 BCE"). D&R's Julian
  calendar has no year 0; their `y′ = y + 1` for BCE years is exactly the astronomical year.
- **Julian day**: `JD = R.D. + 1721424.5` at 0h UT; `JDN = R.D. + 1721425` is the JD at noon;
  `MJD = JD − 2400000.5`. A `time` (UT) adds its fraction of a day; `from: "jd"` takes the
  fraction from the JD itself and reports `time_ut`. JD arithmetic is exact (fractions).
- **ISO week date** (ISO 8601): week 1 is the week with the year's first Thursday; D&R
  `iso-from-fixed`, Monday = 1.
- **Hebrew** (the fixed arithmetic calendar, D&R chapter 8): leap years are those with
  `(7y + 1) mod 19 < 7`; the new year comes from the molad of Tishrei with the four
  postponements (*dehiyyot*) folded into `hebrew-calendar-elapsed-days` and the year-length
  correction; Marheshvan and Kislev vary to give years of 353-355 or 383-385 days. Months are
  numbered as in D&R, **Nisan = 1 … Tishrei = 7 … Adar = 12 (Adar I in a leap year), Adar II =
  13**; names are accepted on input (Tishrei/Tishri, Heshvan/Marheshvan, Adar I, Adar II…).
  A bare "Adar" in a leap year is refused as ambiguous. `molad: true` adds the molad (mean
  conjunction) of the month: weekday, hours and parts (1080 to the hour) in the traditional
  reckoning, the day starting at 6 pm, Jerusalem mean time; and the same moment as a civil
  Gregorian date and time.
- **Islamic**: the tabular (arithmetic) calendar of D&R 7.1, a 30-year cycle whose years
  2, 5, 7, 10, 13, 16, 18, 21, 24, 26 and 29 are leap (355 days, Dhu al-Hijja of 30), i.e.
  `(14 + 11y) mod 30 < 11`; odd months have 30 days, even months 29; epoch 1 Muharram 1 AH =
  Friday 16 July 622 Julian (R.D. 227015). The output names the scheme.

## Tests

- 2000 January 1 at 0h UT is JD 2451544.5 and JDN 2451545 (the J2000.0 epoch is JD 2451545.0 at
  noon), a Saturday.
- The sample date R.D. 710347 of D&R's Appendix C: 1945 November 12 Gregorian, 30 October
  1945 Julian, 7 Kislev 5706, 6 Dhu al-Hijja 1364 (arithmetic), a Monday, both ways; checked
  also against a second fact, Hanukkah (25 Kislev 5706) on 1945-11-30.
- Rosh Hashana 5787 (1 Tishrei) is Saturday 12 September 2026.
- The ISO 8601 examples quoted by Wikipedia's "ISO week date" article: 2008-12-29 is
  2009-W01-1 and 2009-W53-7 is 2010-01-03.
- The tabular Islamic epoch falls on Friday 16 July 622 Julian; the molad of Tishrei AM 1 is
  the traditional *BaHaRaD*, Monday 5 hours 204 parts.
- During development the Gregorian and ISO code was checked against Python's
  `date.toordinal()` and `isocalendar()` over random dates, and every Hebrew, Islamic and
  Julian date of thousands of years was round-tripped.

## Limits

- The Islamic calendar used in practice depends on sighting the crescent or on a national
  rule (Umm al-Qura in Saudi Arabia); the tabular date can differ by one or two days. Other
  tabular leap-year schemes (15 instead of 16, the "Kuwaiti" algorithm) are not offered.
- The Julian calendar was applied irregularly before 8 CE (the triennial leap years); dates
  then are proleptic, as are all Gregorian dates before 1582 and all Hebrew dates before the
  calendar was fixed. Hebrew dates before AM 1 are not given.
- The Hebrew day and the Islamic day begin at sunset; the tool converts civil days (the
  daytime part of the evening-to-evening day).
- Years are limited to ±1,000,000.
