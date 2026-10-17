# business_days

Working-day arithmetic for deadlines and administrative terms: count the working days in an
inclusive range, add or subtract working days to a date, or list them. Weekends default to
Saturday and Sunday (`weekend` takes other ISO weekdays, such as `[5, 6]` for Friday–Saturday)
and `holidays` is an explicit list, because public holidays differ by country, region and
year and a bundled table would silently go stale.

Limits: no half days; "add 0" returns the start date even when it is not a working day.
