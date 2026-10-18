# timeline_builder

Builds a timeline from dated events: a sorted Markdown list, a Markdown table and an ASCII chart
scaled to `width` columns, plus checks for duplicates and overlaps.

**Dates.** Each event has a `label` and either `date` (or `year`) or a `start` and `end`. A date
is an integer year, negative for BCE, or a string: `YYYY`, `YYYY-MM`, `YYYY-MM-DD` (a leading
`-` for BCE), `44 BC`, `500 BCE`, `AD 79`, `79 CE`. Years follow the historical convention:
there is no year 0, and 1 BCE is followed by 1 CE. Internally the tool uses astronomical
numbering (1 BCE = 0, 44 BCE = −43), so durations across the era change are right: 27 BC to
AD 14 is 40 years and 1 BC to AD 1 is one. Days are checked against the month, with the
proleptic Gregorian leap rule.

**Sorting** is by start, then end, then label. A partial date sorts at the beginning of its
period and, as a range end, covers the whole period (a range ending in 1453 runs to the end of
1453). Labels use BCE/CE or, with `era: "BC"`, BC/AD; the era is added to CE years only when the
timeline has BCE dates.

**Outputs.** The list (`- **1337 CE – 1453 CE** — Hundred Years' War`), a table with start, end
and the length of ranges in years (difference of start positions: 1337–1453 is 116), and an
ASCII chart: a scale line with the first and last dates, then one row per event with `*` for a
point and `[===]` for a range (`#` when it fits one column). Columns are computed exactly with
fractions, `round((t − first) / (last − first) × (width − 1))` half up, where t counts months as
twelfths and days as 1/372 of a year.

**Checks.** `duplicates`: same dates and the same label ignoring case. `same_date`: different
point events on the same date. `overlaps`: ranges that intersect, `contains` when one is inside
the other. `during`: point events within a range.

The tests check the no-year-0 arithmetic, the order, the checks and two ASCII renderings worked
by hand (79 CE at (79 + 752)/2206 × 49 = 18.46, column 18).

Limits: no "circa" or uncertain dates, no calendars other than the proleptic Gregorian
(Julian dates are taken as given), no time of day; the ASCII chart puts one event per row.
