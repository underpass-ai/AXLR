# csv_stats

A profile of each column of a CSV, after csvkit's `csvstat`.

The dialect is sniffed with Python's `csv.Sniffer` among comma, semicolon, tab and pipe (the
output says which and whether it was sniffed), or set with `delimiter` (`\t` for tab). A UTF-8
byte-order mark is dropped; blank lines are skipped; rows shorter than the header are padded
with empty cells and counted in `ragged_rows`. `header: false` names the columns `column_1`, ….

For each column: `count`, `nulls` (cells equal to one of `null_values`, by default empty, NA,
N/A, null, NULL, None, nan, NaN and -), `non_null`, `distinct`, `unique`, the `top` values with
their counts (ties in text order), and by inferred type:

| Type | Rule (every non-null value) | Extra statistics |
|:--|:--|:--|
| `integer` | `[+-]digits` | min, max, sum, mean, median, variance, stdev, zeros |
| `decimal` | a decimal number, exponent allowed | the same, and max_decimal_places |
| `boolean` | true/false, yes/no, t/f, y/n (any case) | true and false counts |
| `date` | ISO `YYYY-MM-DD` | min, max, median, span_days |
| `datetime` | ISO date and time, `T` or space, optional zone | min, max |
| `text` | anything else | min and max (text order), min and max length |

Numbers are read as exact fractions: sum, min, max and median are exact; mean and the sample
variance (n − 1) are exact fractions shown with `precision` significant digits (28 by default),
the standard deviation is the Decimal square root at that precision. `1` and `0` are integers,
not booleans.

## Tests

Anscombe's quartet (Anscombe, 1973, *The American Statistician* 27:17), data set I: mean of x 9,
sample variance of x 11, mean of y 7.50 (82.51 / 11 = 7.5009…), variance of y 4.127; a
semicolon file with every type, worked by hand (ages 34, 28 and 41: mean 103/3, median 34,
variance 127/3); tabs, quoting and no header; precision; and usage errors.

## Limits

Dates other than ISO (31/12/2024) are text; thousands separators and decimal commas are not
numbers; the whole file is held in memory (up to 20 MB).
