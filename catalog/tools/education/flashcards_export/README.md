# flashcards_export

Turns flashcards kept as CSV into a text file Anki imports directly (File > Import), using the
file headers documented in the Anki manual (Importing > Text Files > File Headers, Anki 2.1.54
and later).

**Input.** `csv` is the text; `delimiter` defaults to a comma. The first row is a header when one of
its cells is `front` (any case) and all are short names, or when `header` is true; otherwise `columns` names the fields (default front, back, tags, deck). Any other column
name is an extra note field, written between back and tags. Rows with an empty front are
skipped and listed.

**Output.** `tsv` starts with the header lines, then one note per line, tab-separated:

```
#separator:Tab
#html:false
#notetype:Basic          (with notetype)
#deck:Vocab              (with deck)
#tags:lesson1 unit2      (with tags: added to every note)
#columns:Front	Back	Tags
#tags column:3           (with a tags column)
#deck column:4           (with a deck column)
```

Escaping follows the manual: a field holding a tab, a newline or a double quote is put in double
quotes, and each double quote inside is doubled (`"thank you, ""thanks"""`). With `html: true`
newlines become `<br>` by default (`newlines: "quote"` keeps them in a quoted field instead; the
manual warns that multi-line quoted fields break cloze deletions spanning lines). A tag column is
normalised to space-separated tags (commas and semicolons also separate), since Anki tags cannot
contain spaces; `::` hierarchies are kept.

The tests check the header syntax and the quoting rule against the manual's text, the HTML
`<br>` mode, the deck column and the global headers.

Limits: no media files, no GUID column, no cloze generation (see `quiz_cloze` for `{{c1::…}}`
text); with `html: true` the fields are passed through as HTML, so `<` and `&` in plain text must
already be escaped.
