# sleep_cycles

A bedtime and wake-up planner in whole sleep cycles.

- `mode: "wake_at"`: `time` is when you must wake; for each cycle count from `max_cycles` down
  to `min_cycles`, fall asleep at `wake − n × cycle_minutes` and go to bed `latency_minutes`
  earlier.
- `mode: "bed_at"`: `time` is when you go to bed; you fall asleep `latency_minutes` later and
  wake at `asleep + n × cycle_minutes`.

Defaults: 90-minute cycles, 15 minutes to fall asleep, 3 to 6 cycles. Times wrap around
midnight; `day_offset` (−1, 0, +1) and `day` say whether a time falls on the previous, same or
next day relative to the given time. An option is `recommended` when its total sleep falls in
`recommended_hours`, by default 7 to 9 hours, the National Sleep Foundation's range for adults
aged 18 to 64 (Hirshkowitz et al., *Sleep Health* 2015); other ages need their own range
(for example 8 to 10 h for teenagers).

The tests are worked by hand (wake at 07:00: bed at 21:45, 23:15, 00:45 or 02:15) and check the
NSF adult range against 6, 7.5 and 9 hours.

Every output, errors included, carries the disclaimer: *Not medical advice: the 90-minute cycle
is an average and real cycles vary between people and through the night. Persistent sleep
problems need a qualified clinician.*

Limits: cycles are treated as equal in length, which they are not (early cycles are richer in
deep sleep, later ones in REM); no time zones or daylight-saving changes.
