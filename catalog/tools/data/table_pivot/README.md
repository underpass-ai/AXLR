# table_pivot

Summarises a CSV the way a spreadsheet pivot table does.

## Group-by

`group_by` lists the key columns; each distinct combination becomes an output row (none: a
single row over the whole table). `values` lists the aggregates, each `{column, agg}` with an
optional output `name`:

| `agg` | Result |
|:--|:--|
| `sum` | exact decimal sum of the non-empty cells (every one must be a number) |
| `mean` | the sum divided by the count of non-empty cells |
| `count` | without `column`, rows; with `column`, non-empty cells |
| `count_distinct` | distinct non-empty values |
| `min`, `max` | numeric when every non-empty cell is a number, otherwise text order |

Numbers are read as `Decimal`, so 0.1 + 0.2 is 0.3, and printed as decimal strings; a mean
that does not terminate carries 34 significant digits unless `decimals` rounds every result
half-up to that many places. Empty cells are skipped (they are not zeros).

## Pivot

`columns` names one column whose distinct values become output columns: each cell is the
aggregate of the rows with that key and that value. With several aggregates the columns are
named `value|agg(column)`. Combinations with no rows get `fill` (null by default; "0" is
common). `totals: true` adds a `Total` column per aggregate (row totals) and a `totals` row
(column totals and the grand total), all computed again from the underlying rows, so a total
mean is the mean of all values and a total distinct count is distinct over the whole column.
Group keys and pivot values are sorted, numbers numerically (2 before 10); `sort: "none"` keeps
first-seen order.

## Tests

A six-row sales table worked out by hand: sums, means, counts, distinct counts and maxima per
region with totals (South's empty cell is skipped: mean 3.625 over two numbers, 3 rows); a
region × quarter pivot with every total; a fill value; 0.1 + 0.2 = 0.3; half-up rounding of
10/3 and 2/3; two aggregates in one pivot; numeric key order; a table with no group; errors.

## Limits

One pivot column; no median, percentile or standard deviation (see `csv_stats`); the CSV is held
in memory (20 MB).
