# budget_breakdown

Turns a budget table into the figures a reader of public accounts usually wants. The budget
comes as CSV text in the input (`csv`), with a header row; `columns` names the headers for the
programme, the amount and, optionally, the year and the population (defaults `programme`,
`amount`, `year`, `population`). Rows with the same programme and year are added together.

For each year:

- **total** and, with a population, **per capita** (`total / population`);
- each programme's **amount**, **share** of the year's total in percent, and per-capita amount;
- the **top** programmes by amount (`top`, default 5).

For each pair of consecutive years: the change in the total (absolute and percent), in the
per-capita total, and per programme, sorted from largest increase to largest cut. A programme
missing in the earlier year is `new` (its percent change is `null`); one missing in the later
year is `dropped`.

Amounts are `Decimal` and accept `1234.56` or `1,234.56`; with `decimal_comma` they are read as
`1.234,56` (Spanish, French, German style). Anything else is rejected rather than guessed, and a
row with more fields than the header (a decimal comma in a comma-separated file) is an error.
The delimiter is guessed from the header unless given. Population comes from the column (one
value per year; a conflict is an error) or from `population`, a number or `{year: number}`.
Printed figures are rounded half up to 0.01; every sum and ratio is computed before rounding,
so shares may add up to 99.99 or 100.01.

Test: a two-year municipal budget checked by hand (2 600 000 in 2024 for 50 000 people, 52.00
per head, Education 46.15 %; 2 800 000 in 2025 for 52 000, 53.85 per head; total +7.69 %, per
head +3.55 %, Housing new, Culture dropped), a Spanish-format file with semicolons and decimal
commas, and the error cases.

Limits: one level of programmes (no chapter/article hierarchy); no inflation adjustment (use
`inflation_adjust` first); file paths are not read: pass the CSV text.
