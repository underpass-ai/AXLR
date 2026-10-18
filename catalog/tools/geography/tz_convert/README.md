# tz_convert

Time-zone arithmetic over the IANA tz database through Python's `zoneinfo`.

- `convert`: a date-time in `from_zone` (or with its own offset or `Z`) to each of `to_zones`,
  with offset, abbreviation, DST flag and weekday.
- `offset`: the same description for one `zone` at one moment.
- `transitions`: every change of UTC offset in a `year`, with the UTC instant and the local
  clock just before and after (02:00 → 03:00 in spring, 03:00 → 02:00 in autumn in Madrid).

Local times are checked: one inside a spring-forward gap gets a `warning` (`nonexistent`),
one inside the repeated autumn hour gets `ambiguous`, and `fold` chooses which of the two is
meant (0 the first, the default; 1 the second).

The answer depends on the tz database installed with the interpreter; the output's `tzdata`
names its version (from `tzdata.zi`), or `null` when it cannot be read. Future dates use the
rules known today; governments change them.

Limits: zone names only (no "EST" or "+02:00" as a zone; use `Etc/GMT-2` style names for fixed
offsets); transitions are found to the second by searching each hour of the year.
