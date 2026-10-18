# habit_streaks

Streaks and completion rates from a plain list of the days a habit was done.

Every count is relative to `as_of`, given in the input (the tool never reads the clock), from
the habit's `start` (default: its first logged day). Logs after `as_of` or before `start` are
ignored and counted; duplicates collapse.

**Day schedules** (`"daily"`, `"weekdays"` = Monday to Friday, `{"days": [1, 3, 5]}` with ISO
weekdays, 1 = Monday): the scheduled days from `start` to `as_of` are the due days.

- current streak: consecutive due days done, counting back from `as_of`;
- longest streak: the longest run of consecutive due days done, and the day it ended;
- a logged day that is not scheduled (a Saturday for a weekday habit) neither extends nor breaks a
  streak and is listed in `logged_on_unscheduled_days`;
- completion: done due days / due days, overall and over each window of the last `windows` days
  (default 7, 30 and 90), as a percentage with one decimal, rounded half up;
- missed days: due days not done (`missed_count`, and the most recent `missed_limit` of them).

**Weekly targets** (`{"per_week": N}`): weeks are ISO weeks, Monday to Sunday, and a week is met
when the habit was done on at least N distinct days of it. Streaks count consecutive met weeks;
completion is weeks met / weeks; each window counts the weeks that overlap it and also the
sessions done against the pro-rata target N × days / 7. `this_week` shows the count so far, what
remains and the days left. A first week cut short by `start` is judged like any other, so give
`start` on a Monday for a fair first week.

**Today.** With `today_pending` (default true) a due `as_of` day that has no log yet (or a current
week whose target is not met yet) is left out: it neither breaks the streak nor counts as missed
or in the rates. With `today_pending: false` it counts as missed.

The tests check hand-worked October 2026 calendars: a daily habit with two gaps (current 6,
longest 6, 15 of 17 = 88.2 %), a weekday habit with a missed Wednesday and a Saturday extra
(longest 9 ending Tuesday 13), a 3-per-week habit over three ISO weeks, a pending current week,
the effect of `today_pending: false` and a Monday-Wednesday-Friday habit.

Limits: one log entry per day (no counts or durations), no time zones (dates are calendar days
as given), no pauses or rest days other than the schedule.
