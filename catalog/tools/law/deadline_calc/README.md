# deadline_calc

Computes the last day of a time limit from a counting rule **that the input states**. Rules
differ by country, by branch of law and by procedure, and a wrong rule gives a wrong deadline
that looks right, so the tool has no defaults for them and refuses to run without `rule`.

| Rule field | Values |
|:--|:--|
| `count_from` | `next_day` (the usual rule: the day of notification does not count) or `same_day` |
| `non_working_weekdays` | ISO weekdays, `[6, 7]` for Saturday and Sunday; required for `working_days` |
| `holidays` | dates, from the official calendar of the place |
| `non_working_periods` | `{"from": "08-01", "to": "08-31"}` every year, or full dates |
| `month_end` | `same_day`: a month from 15 March ends on 15 April; `day_before`: on 14 April. A missing day (31 February) is the month's last day |
| `if_last_day_non_working` | `next_working_day` or `keep` |

Working days skip every non-working day. Calendar days, weeks, months and years count every
day; non-working days matter only for the last one. The output gives the nominal and the
final end, the weekday, how many days of each kind were skipped, and a disclaimer.

Examples of rules (check them against the law in force):

- Spain, administrative procedure (Ley 39/2015, art. 30): `{"count_from": "next_day",
  "non_working_weekdays": [6, 7], "holidays": [<national, regional and local holidays>],
  "month_end": "same_day", "if_last_day_non_working": "next_working_day"}`.
- Spain, civil procedure (LEC arts. 130–133): the same, plus `"non_working_periods": [{"from":
  "08-01", "to": "08-31"}]` and 24 and 31 December as holidays.

Not legal advice: a tool cannot know which rule applies, whether a deadline was suspended, or
which local holidays a court observes.
