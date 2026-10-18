# date_parse

Turns a written date, time, date-time or duration into ISO 8601 and names the rule that matched.
Relative expressions are computed from `reference`, which the input must give: the tool never
reads the clock, so the same input always gives the same answer.

## Forms

- ISO 8601: `2026-10-18`, `2026-10-18T10:30:00+02:00`, basic `20261018T103000Z`, week dates
  `2026-W42-7`, ordinal dates `2026-291`, times `10:30`, and durations
  `PnYnMnWnDTnHnMnS` (a fraction only on the smallest component, a leading `-` for negative).
  A duration reports its components; when it has no years or months, its length in seconds
  (days as 24 hours); with a `reference`, its end, adding years and months by calendar.
- Numeric dates `18/10/2026`, `18.10.2026`, `2026/10/18`. `date_order` (`DMY` or `MDY`) says how
  to read them; without it a date is accepted only when one number is above 12, and the rule
  says `inferred`. Two-digit years are refused.
- Month names in English and Spanish: `18 October 2026`, `October 18, 2026`, `18 de octubre de
  2026`, optionally preceded by a weekday, which must agree. Without a year, the reference's year.
- Relative, English and Spanish (accents optional): `today`/`hoy`, `tomorrow`/`mañana`,
  `yesterday`/`ayer`, `the day after tomorrow`/`pasado mañana`, `anteayer`; `in 3 days`,
  `3 days from now`, `dentro de 3 días`, `en 3 días`; `3 weeks ago`, `hace 3 semanas`; `next
  week`, `la semana que viene`, `el mes pasado`; numbers as digits or words up to twelve.
- Weekdays: `next Monday` / `el próximo lunes` / `el lunes que viene` is the first Monday strictly
  after the reference (a week later when the reference is a Monday); `last Monday` / `el lunes
  pasado` the last one strictly before; `this Monday`, `el lunes` or a bare `Monday` the first one
  on or after the reference.
- A time of day can follow any date: `tomorrow at 3pm`, `mañana a las 10:30`, `next Friday 09:00`.

Months and years are added by calendar with the day clamped to the month's end (31 January plus
one month is 28 February); weeks, days, hours and minutes by exact length.

## Tests

The ISO 8601 duration example `P3Y6M4DT12H30M5S` and hand-checked values: `P1DT2H` = 93600 s,
`P4W` = 28 days, relative dates from Sunday 18 October 2026, and the error cases (ambiguous
numeric date, missing reference, weekday that does not match, unparseable text).

## Limits

A small, explicit subset, not a general parser: no other languages, no "the first Monday of
next month", no time zones by name, no two-digit years. Anything else exits with a clear error.
