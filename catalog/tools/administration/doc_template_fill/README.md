# doc_template_fill

Fills `{{field}}` placeholders (spaces inside the braces allowed, `{{field|default}}` for a
fallback) with `values`, in plain `text` or in a Word document given as `docx_hex`.

## .docx without a package

A .docx is a zip of XML parts (ECMA-376). The tool reads it with `zipfile`, rewrites
`word/document.xml` and the headers, footers, footnotes, endnotes and comments, and writes every
member back with its original zip entry, so the rest of the file is untouched and the same input
gives the same bytes.

Word stores text in runs (`<w:r>`), each with its text element (`<w:t>`), and it splits a typed
`{{name}}` into several runs whenever formatting, spell-check marks or editing history fall
inside it. So the tool joins the text of all runs of each paragraph (the innermost one, for text
boxes nested in a paragraph), finds the placeholders there, puts each value into the run where
its placeholder starts (keeping that run's formatting) and removes the rest of the placeholder
from the following runs. Text is unescaped before matching and escaped again on writing; a
newline in a value becomes `<w:br/>` and a tab `<w:tab/>`.

The output has the filled document as hex, `paragraphs` (the text of each non-empty paragraph of
the body after filling, to check the result without opening Word), `parts_changed`, and, as in
text mode, `placeholders_found`, `missing` (left as they were with `missing: keep`, the default;
removed with `empty`; an error with `error`), `defaults_used` and `values_not_used`.

## Tests

A minimal .docx built by hand (content types, relationships and a body whose `{{name}}` is split
across two runs with a spell-check mark between them) filled three ways, plain text cases worked
out by hand, and the error cases (bad hex, not a zip, both inputs).

## Limits

Placeholders must sit inside one paragraph; fields, content controls, text in drawings outside
`<w:t>` and the .doc format are not handled. Values are plain text (no formatting, images or
tables). Files up to 20 MB.
