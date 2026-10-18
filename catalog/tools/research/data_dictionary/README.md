# data_dictionary

Profiles the columns of a CSV dataset and drafts a data dictionary to finish by hand. The CSV
comes inline (`csv`) or as a relative `path` inside the working directory. The delimiter is
detected (the one of `,` `;` tab `|` that splits the first 30 lines into the most consistent
number of fields) or given; `header: false` numbers the columns `column_1`, `column_2`…

## Per column

- `type`: the narrowest type every non-null value fits: `boolean` (true/false, yes/no, t/f,
  y/n), `integer`, `number`, `date` (ISO `YYYY-MM-DD`, or `DD/MM/YYYY` / `MM/DD/YYYY` with `/`
  or `.`, real calendar dates only), `datetime` (ISO 8601), `time` (`HH:MM[:SS]`), otherwise
  `string`. Integers mixed with decimals are `number`, dates mixed with date-times are
  `datetime`, anything else mixed is `string` with `type_counts` showing the mix.
- `nulls` and `null_percent`: values in `null_values` (default: empty, `NA`, `N/A`, `NULL`,
  `null`, `None`, `NaN`, `nan`, `-`), compared after trimming spaces.
- `distinct`, `unique`, the first `max_examples` distinct values (default 3).
- numbers: `min`, `max` (as written in the file, compared exactly in `Decimal`) and `mean`
  (6 decimals); dates: `format`, `min`, `max` (as ISO dates; omitted when day/month order is
  ambiguous); strings: `min_length`, `max_length`; booleans: the spellings used.
- `hints`: `e-mail address`, `URL` or `UUID` when every value matches; `zero-padded digits`
  (the column becomes `string`, since a number would drop the zeros); `binary 0/1 flag`;
  `identifier: unique and never null` for columns named like ids (`id`, `*_id`, `userId`,
  `*_key`, `*_code`) or unique integer/UUID columns of at least 3 rows; `categorical` with
  `value_counts` when there are at most 20 distinct values and at most half as many as values;
  `constant`; untrimmed whitespace. Day-first or month-first is decided by a day above 12.

## Output

`columns` (the profile), `dictionary` (a JSON skeleton: name, type, format, nullable, unique,
example, allowed values, range, and empty `description`, `unit`, `source` and `notes` to fill
in), `markdown` (the same as a table with an empty Description column) and `warnings` (renamed
duplicate headers, rows with the wrong number of fields, whose missing cells count as null).

## Tests

The UCI Iris data set (150 rows) against the summary statistics published in its `iris.names`
file: minima and maxima 4.3–7.9, 2.0–4.4, 1.0–6.9 and 0.1–2.5, means 5.84, 3.05, 3.76 and 1.20,
three classes of 50; a small members CSV checked by hand column by column (types, nulls,
ranges, e-mail, zero-padded codes, categories, skeleton and Markdown); a semicolon file with
day-first dates, a repeated header and a short row; and the input errors.

## Limits

Types are guessed from text, so a column of codes such as `1, 2, 3` looks like an integer;
locales with decimal commas give `string`; no statistics beyond min, max and mean; the whole
file is held in memory (up to 20 MB).
