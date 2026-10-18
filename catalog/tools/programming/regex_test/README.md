# regex_test

Runs a Python `re` pattern over a text and shows what it does:

- `matches`: every non-overlapping match (`anchor: "search"`, `re.finditer`), or the single
  match at the start (`match`) or of the whole text (`fullmatch`), each with the matched text,
  its `span` `[start, end)`, the numbered `groups`, their `group_spans` (`null` for a group
  that did not take part) and, when the pattern has named groups, `named`. Spans count Unicode
  code points, as Python string indexes do. At most `max_matches` are listed (default 1000).
- `substitution` (with `replacement`): the result of `re.subn` and the number of replacements.
  The template uses Python's syntax: `\1`, `\g<1>`, `\g<name>`, and escapes such as `\n`.
- `split` (with `split: true`): `re.split`, with captured separators included as Python does.

Flags are `IGNORECASE`, `MULTILINE`, `DOTALL`, `VERBOSE` and `ASCII` (or `i`, `m`, `s`, `x`,
`a`); inline flags such as `(?i)` in the pattern work too.

Python's regex engine backtracks, so a pattern such as `(a+)+$` on a long run of `a` followed
by another character takes exponential time. The whole run (compile, match, substitute, split)
happens under a `signal.setitimer` timer (`timeout`, default 2 s, at most 10 s); when it fires,
the tool prints an error naming the stage and exits 2, instead of hanging. The pattern is
limited to 10,000 characters and the text to 200,000.

The tests use the examples of the Python `re` documentation: `findall` of `\bf[a-z]*`, the
`split` examples over `'Words, words, words.'` (with and without a capturing group and with
`maxsplit=1`) and over `'0a3B9'` with `IGNORECASE`, `Match.groupdict` on "Malcolm Reynolds",
`Match.start`/`end` on `tony@tiremove_thisger.net`, the `sub` examples (`def myfunc():`, and
`x*` over `abxd` giving `-a-b--d-`) and `subn` with `\sAND\s`; plus a catastrophic
backtracking case that must time out.

Limits: Python `re` only (no PCRE, JavaScript or POSIX dialects, no possessive quantifiers or
atomic groups before Python 3.11); no `regex` module features; the timer needs a POSIX system
(it is what the catalog's runtime is).
