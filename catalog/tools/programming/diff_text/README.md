# diff_text

Compares two texts given inline (`a`, the old one, and `b`, the new one) with Python's
`difflib`:

- `unified`: `difflib.unified_diff` over the lines (line ends kept) with `context` lines
  around each hunk (default 3) and `from_name`/`to_name` in the `---`/`+++` headers. A last
  line without a newline is followed by `\ No newline at end of file`, as GNU diff and git
  print it, so the result can be applied with `patch`. `hunks` counts the `@@` headers.
- `lines`: line counts of both texts and the lines added and removed (a changed line counts
  once in each).
- `line_ratio` and `char_ratio`: `SequenceMatcher.ratio()`, which is 2·M/T for M matched
  elements out of T in both sequences, given as an exact fraction, a rounded value and M and
  T. Lines for `line_ratio`, characters for `char_ratio` (only up to 20,000 characters per
  text, because the matcher is quadratic). `autojunk` is off, so long texts are not
  "simplified" by the popular-element heuristic. The ratio depends on the argument order,
  as difflib documents.
- `side_by_side` (optional): one row per line with the line numbers on each side and an
  operator (` ` equal, `|` changed, `<` only on the left, `>` only on the right), and the same
  rows rendered as a fixed-width table (`width` per column; longer lines are cut with `…`).
- `words` (optional): the texts split into words, runs of whitespace and single punctuation
  marks, compared token by token; the inline result marks deletions `[-…-]` and insertions
  `{+…+}` as `git diff --word-diff=plain` does, with the list of changes and the token ratio.

The tests reproduce the `unified_diff` example of the difflib documentation byte for byte
(`before.py`/`after.py`, `@@ -1,4 +1,4 @@`), its `ratio()` examples (`tide`/`diet` gives
0.25 and the reverse 0.5; the "private Thread currentThread;" pair gives 0.866, exactly
58/67), and check the newline marker, zero context, the word diff and the side-by-side rows.

Limits: texts up to 200,000 characters each; no file input; difflib's matcher finds a
longest-matching-block diff, which can differ from GNU diff's minimal (Myers) diff on some
inputs; no ignore-whitespace or ignore-case modes.
