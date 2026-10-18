# retention_schedule

Applies a retention schedule **that the input gives** to a list of records. Retention periods
differ by country, sector and record, and the same record often falls under several rules
(tax, commercial, labour, data protection), so the tool bundles none: a record type without
a rule comes back `unscheduled`.

Each rule: `record_type`, `period` (`{"years", "months", "days"}`, an ISO 8601 period such as
`P6Y`, or `permanent`), `trigger` (the name of the record date the period runs from),
`count_from` (`trigger_date` or `next_day`), `rounding` (`end_of_month`, `end_of_year`: many
rules run from the end of the year the record was made), `action` (`destroy`, `review`,
`archive`) and `legal_basis`. Each record: `id`, `record_type`, `dates` (a map of named dates),
`legal_hold`.

## Method

`retain_until` = trigger date, moved to the end of its month or year if the rule says so,
plus one day with `next_day`, plus the period (calendar months; a missing day falls to the
month's last day, and after rounding the result stays at month end). The action date is the
day after. With several rules for a type the latest `retain_until` governs (a `permanent`
rule always does). Status against `as_of`: `retain`, `past_retention`, `permanent`,
`on_hold` (a legal hold suspends any action whatever the dates), `missing_dates` (a rule's
trigger date is missing, so the record cannot be cleared; the known minimum is given) or
`unscheduled`.

## Tests

The known value is hand-checked with Spanish rules given in the input: invoices under the
four-year prescription of the Ley General Tributaria (arts. 66 and 67.1: counted from the day
after the end of the filing period) and the six years from the last book entry of the Código
de Comercio (art. 30.1). For an invoice of Q4 2020 (filing deadline 30 January 2021, last entry
31 December 2020): 31 January 2021 + 4 years = 31 January 2025; 31 December 2020 + 6 years =
31 December 2026, which governs; on 18 October 2026 there are 13 + 30 + 31 = 74 days left. The
same invoice from 2019 ended on 31 December 2025, 291 days before 18 October 2026. Other cases:
legal hold, a missing trigger date, end-of-year rounding, a month-end clamp, permanent records
and usage errors.

## Limits

No interruption or suspension of limitation periods other than an explicit legal hold, and no
business-day rules (use `deadline_calc` for those). Not legal advice; the output carries that
disclaimer.
