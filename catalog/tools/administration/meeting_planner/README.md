# meeting_planner

Finds when a group spread across time zones is at work at the same time.

Each participant has an IANA zone (`tz`), working hours (`start`, `end`, local wall-clock time;
an `end` at or before `start` runs past midnight) and working days (`workdays`, ISO weekdays,
default Monday to Friday). For each calendar day of `date`, or of `start_date` to `end_date`, in
`reference_tz` (default: the first participant's zone), the tool converts every participant's
working periods to UTC with `zoneinfo` (so daylight-saving changes apply on the right day),
intersects them, and clips the result to that day. A participant's working period may belong to
a different local date: Tokyo's Wednesday morning is Los Angeles's Tuesday afternoon, and each
window shows every participant's local date, time, weekday and UTC offset.

Slots of `duration_minutes` start on multiples of `step_minutes` on the UTC clock inside each
common window. Each slot's score is the smallest margin, over participants, between the slot
and the start or end of that person's working period: a slot in the middle of everyone's
overlap scores higher than one that starts at someone's 9:00 or ends at someone's 18:00. Slots
are listed best first, ties by time; `slot_count` says how many there were in all.

## Tests

The hand-checked Madrid–New York case on 1 July 2026 (CEST UTC+2, EDT UTC−4): overlap 13:00–16:00
UTC, 15:00–18:00 in Madrid, 09:00–12:00 in New York, with the slot scores worked out by hand;
the March 2026 weeks when the US has moved to daylight time and Europe has not; Tokyo and Los
Angeles meeting across the date line; a Saturday; bad zones and times.

## Limits

No holidays, lunch breaks or personal calendars: give them as narrower hours or separate runs.
Ranking is a simple fairness heuristic, not a preference model.
