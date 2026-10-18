# xlsx_read

Turns the sheets of an Excel `.xlsx` workbook into data without openpyxl or pandas. Give the
file as `hex` or as a relative `path`.

## How it reads the file

An .xlsx is a zip of SpreadsheetML parts (ECMA-376 Part 1). The tool reads
`xl/workbook.xml` (sheet names, visibility and the `date1904` flag), its relationships (which
part holds each sheet), `xl/sharedStrings.xml` (each `si` is the text of its `t`, or of its
rich-text runs joined; phonetic guides are skipped) and `xl/styles.xml` (the number format of
each cell style), then each worksheet's `sheetData`:

| Cell `t` | Value |
|:--|:--|
| `s` | the shared string |
| `inlineStr` | the text inside `is` |
| `str` | the cached text result of a formula |
| `b` | `true` / `false` |
| `e` | the error text (`#DIV/0!`, `#N/A`…) |
| `n` (default) | a number (integers without a decimal point as integers), or a date if its style has a date format |

A number is a date when its style's format is a built-in date or time format (ids 14–22,
27, 30, 36, 45, 47, 50, 57) or a custom format code containing day, month or year (or hour
and second) parts outside quoted text and brackets. Elapsed-time formats (`[h]:mm`) stay
numbers. Serial day numbers follow ECMA-376 Part 1 18.17.4: in the 1900 system, serial 1 is
1900-01-01, serials from 61 are counted from 1899-12-30 because serial 60 is the
non-existent 1900-02-29 that Excel keeps for Lotus 1-2-3 compatibility (it is left as a number
with a note); in the 1904 system serial 0 is 1904-01-01. A fraction is the time of day.
`dates: "serial"` keeps the numbers.

## Output

`date_system`, `sheet_names`, and per sheet: `values` (rows as arrays from A1 to the last used
cell, `null` for empty cells), `records` (rows keyed by the first row, with `header`), `csv`,
`dimension`, `merged_cells` (only the top-left cell of a merged range holds the value),
`truncated` (when `max_rows` stopped reading) and `notes`.

## Tests

A workbook built by hand for the tests (two sheets, shared strings with a rich-text run and an
escaped `&`, an inline string, a boolean, an error, a formula's cached string, a merged range,
the built-in date format 14, a custom `dd/mm/yyyy hh:mm` format and a two-decimal format) and
a second one in the 1904 system. Known values: serial 45000 is 2023-03-15 in the 1900 system
and 2027-03-16 in the 1904 system (1462 days apart), serial 59 is 1900-02-28, 61 is
1900-03-01 and 60 is the phantom leap day; plus usage errors (unknown sheet, an old .xls,
bad hex).

## Limits

Formulas are not recalculated (the cached value is read; a workbook saved by a generator that
writes no cached values shows them empty); number formats are not applied to plain numbers
(2.5 stays 2.5 even when shown as 2.50); hidden rows and columns are read; no .xls, .xlsb or
.ods; password-protected workbooks are encrypted and cannot be read; files up to 20 MB.
