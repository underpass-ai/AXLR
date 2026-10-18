# legislative_calendar

Works out procedural deadlines for a bill (amendment periods, committee reports, notice
periods, objection windows) from three things the caller supplies: the bill's known dates
(`events`), the chamber's session calendar, and the counting rules. It has no built-in
knowledge of any parliament: every chamber counts differently, so the rules must come from its
standing orders.

**Calendar.** Either an explicit list of `sitting_days`, or a `start`/`end` range with
`sitting_weekdays` (ISO, Monday = 1). Both forms accept `recesses` (named periods with no
sittings), `non_sitting_days` (cancelled sittings) and `extra_sitting_days` (extraordinary
sittings, which count even inside a recess).

**Rules**, each counted from an event or from another rule's deadline (chains are resolved in
dependency order; a cycle is an error):

- `sitting_days`: the deadline is the `count`-th sitting day strictly after (or `before`) the
  start date; with `include_start`, a start date that is itself a sitting day counts as day one.
- `calendar_days`: the start date plus (or minus) `count` days.
- `calendar_days_excluding_recess`: the same, but days inside a recess do not count (for rules
  where time stops running during recess); `recess_days_skipped` shows how many were passed.
- `roll` (calendar units): a deadline on a non-sitting day moves to the next or previous
  sitting day; `unrolled` keeps the date before the move.

A deadline that would fall beyond the calendar given is `null` with a `note`, never guessed.
Every result says its weekday, whether it is a sitting day and which recess it falls in.

Test: a calendar built by hand on the real October–November 2026 calendar (sittings Tuesday to
Thursday, a recess from 12 to 16 October, 28 October cancelled), with six rules worked out by
hand day by day: six sitting days after Thursday 1 October is Thursday 22 October; four more is
Wednesday 4 November; 15 calendar days lands on Friday 16 October in the recess and rolls to
Tuesday 20 October; 15 days not counting the five recess days is 21 October; two sitting days
before 5 November is 3 November; and 30 sitting days runs past the 21 in the calendar.

The output carries a `disclaimer`: deadlines are only as good as the calendar and rules given.

Limits: no time-of-day cut-offs, no "working days" distinct from sitting days (give them as the
sitting calendar if that is what the rule counts), no automatic public holidays.
