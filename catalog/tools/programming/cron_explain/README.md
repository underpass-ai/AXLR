# cron_explain

Explains a cron expression in English and Spanish and lists its next runs after a date-time
you give (`from`, required whenever `count` is above 0; there is no "now", so the answer is
reproducible) in an IANA time zone.

## Syntax read

The five crontab(5) fields, minute (0-59), hour (0-23), day of month (1-31), month (1-12 or
`jan`-`dec`) and day of week (0-7, 0 and 7 both Sunday, or `sun`-`sat`), each a list of
`*`, `n`, `a-b`, `*/s` and `a-b/s` elements; leading zeros are fine. With `seconds: true` a
leading seconds field (0-59) is expected, as in Spring and Quartz; the day-of-week numbering
stays crontab's. The macros `@yearly`/`@annually`, `@monthly`, `@weekly`, `@daily`/`@midnight`,
`@hourly` expand to their crontab(5) equivalents; `@reboot` is explained and has no runs. A
whole crontab line is accepted: what follows the fields is returned as `command`, never run.

Non-standard extensions are accepted and listed under `nonstandard`: `?` in a day field
(read as `*`), `a/s` without a range (`a-max/s`), and Quartz's `L` (last day of the month),
`L-n`, `nW` (weekday nearest day n, never leaving the month), `LW`, `dL` (last weekday d of the
month) and `d#k` (k-th weekday d), with crontab's day numbers (Sunday 0) as cronie does, not
Quartz's (Sunday 1).

## Day rule

Vixie cron's rule, as crontab(5) documents it: when both day-of-month and day-of-week are
restricted, a day matches if **either** matches (`30 4 1,15 * 5` runs on the 1st, the 15th and
every Friday). A field counts as unrestricted when its first character is `*`, so
`0 0 */2 * sun` runs only on Sundays with an odd date (an AND), as the man page's LIMITATIONS
section says, and not on every Sunday plus every odd date as POSIX would. `day_rule` in the
output says which case applied.

## Time zones and DST

Runs are wall-clock times in `timezone` (default `UTC`; the zone database comes from Python's
`zoneinfo`, so a system without tz data only offers UTC and says so). A `from` without an
offset is local to the zone. Each run is given with its offset and in UTC.

- A local time removed by a spring-forward gap (02:30 on 29 March 2026 in Europe/Madrid) is
  skipped with `dst: "skip"` (the default); with `dst: "shift"` it runs once at the first
  instant after the gap (03:00 CEST), close to what Vixie cron and cronie do for fixed-time
  jobs. Several times inside one gap collapse into that one run.
- A local time that happens twice at the end of DST runs once, at its first occurrence.

Each such case is listed in `dst_notes`.

The search walks day by day for up to 30 years (so `0 0 29 2 *` finds the leap days and
`0 0 30 2 *` ends with a note instead of looping).

## Tests

Known values from crontab(5): `30 4 1,15 * 5` (the OR rule), `0 0 */2 * sun` (Sundays that are
odd dates), `23 0-23/2 * * *`, `5 4 * * sun`, `0 */4 1 * mon` and `@weekly = 0 0 * * 0`; the
next runs were checked by hand on the 2026 calendar (18 October 2026 is a Sunday). Also the
Madrid 2026 DST transitions (29 March, 25 October), a New York case, leap days, a Quartz
`#` case and the seconds field.

Limits: no Quartz year field or Quartz day numbering; `H` (Jenkins hash) and other
vendor-specific syntax are rejected; the wording follows the fields mechanically (it says
"every 2nd hour from 0 through 23", not "every other hour").
