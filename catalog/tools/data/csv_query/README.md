# csv_query

SQL over CSV files without a database server: each CSV is loaded into an in-memory SQLite
table and one query runs against all of them (as `csvsql --query` does in csvkit).

## Loading

Tables come from `tables` (`{name: csv}`), `csv` (one table named `data`, or `table`) and
`paths` (`{name: relative path}`). Names are letters, digits and `_`. The first row is the
header (blank names become `column_N`, repeated names get `_2`, `_3`); the delimiter is sniffed
among comma, semicolon, tab and pipe unless `delimiter` is given. With `infer_types` (the
default) a column whose non-empty cells are all integers is `INTEGER`, all numbers `REAL`,
otherwise `TEXT`; empty cells are `NULL`. With `infer_types: false` every column is `TEXT`.
`tables` in the output gives each table's columns, types, row count and delimiter.

## Read-only

Only one statement is accepted, and it must start with `SELECT`, `WITH` or `VALUES`. Then, as
defence in depth: `PRAGMA query_only = ON`; an authorizer (`sqlite3.set_authorizer`) that allows
only reading tables, selecting, recursive CTEs and functions, and refuses everything else
(ATTACH, DETACH, PRAGMA and the `pragma_*` table-valued functions, writes, schema changes) and
the `load_extension` function; extension loading stays disabled; SQLite itself refuses a
second statement. A progress handler stops a query after 10 seconds, and at most `limit` rows
(1000 by default) are returned, with `truncated` saying whether there were more.

## Output

`columns`, `rows` as JSON objects (arrays when two result columns share a name), `row_count`,
`truncated`, `csv` (the same result as CSV text, NULL as an empty cell) and the SQLite version.

## Tests

A GROUP BY and a JOIN on two small tables worked out by hand (North 10 + 5.5 = 15.5; red apples
10 + 7 + 3 = 20; an empty cell giving a NULL sum; kiwi dropping out of the inner join and kept by
the LEFT JOIN), type inference on and off, a bounded recursive CTE (1 + … + 5 = 15), the row
limit, and the refusals: two statements, ATTACH, load_extension, `pragma_table_info`.

## Limits

REAL columns are binary floats (sums like 0.1 + 0.2 show float rounding); dates stay TEXT (ISO
dates still sort and compare correctly); the CSVs are held in memory (20 MB in total).
