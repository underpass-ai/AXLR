# mail_merge

Renders one document per CSV row from a text template.

The CSV is given inline in `csv` (a leading byte-order mark is dropped; `delimiter` picks `;`
or tab instead of `,`; quoting follows RFC 4180 through Python's `csv` module). The first row is
the header; blank lines are skipped; an empty or repeated column name is an error, because a
repeated name would make one column silently hide the other.

Placeholders are `{{field}}` or `{{ field }}`, and `{{field|default}}` gives text to use when the
field is missing. A field is missing in a row when the column does not exist or the cell is
empty. Missing fields without a default are reported per row (`missing`, with the reason, and
the summary `rows_with_missing`) and, by `missing`, left as the placeholder (`keep`, the
default, so they stand out), replaced by nothing (`empty`), or turned into an error (`error`,
exit 2). Substitution is a single pass: a value that itself contains `{{...}}` is not expanded.

`filename_template` names each document the same way; `/ \ : * ? " < > |` and control
characters become `_`, and repeated names are listed in `warnings`. The output also lists the
placeholders that have no column and the columns the template does not use. `rows` renders only
some rows (1-based, in the order given).

## Tests

Hand-checked merges: a quoted cell with a comma, a default used for an empty cell, an empty cell
and a missing column reported, unsafe and duplicate file names, row selection, and the errors.

## Limits

Plain text only (for .docx use `doc_template_fill`); no conditionals or loops; at most 10,000
rows. The documents are returned, not written to disk.
