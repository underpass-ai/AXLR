# quiz_cloze

Gap-fill questions from a passage, with the answer key and an Anki version.

**Choosing the gaps** (`mode`):

- `auto` (default): content words, meaning words of at least `min_length` letters (default 4)
  that are not on the English or Spanish stopword list, ranked by frequency in the text and then
  length (`rank_by: "length"` reverses the order of the two); the top `count` distinct words are
  blanked, at their first occurrence or, with `all_occurrences`, everywhere. Ties are broken
  alphabetically or, with a `seed`, by `random.Random(seed).shuffle` of the alphabetically
  sorted tie group, so a seed always gives the same quiz.
- `terms`: the given words or phrases, matched case-insensitively as whole words (a phrase
  may span any whitespace); terms that are not found are listed in `not_found`.
- `nth`: the classic fixed-ratio cloze (Taylor 1953): every `every_n`-th word starting at
  `offset` (default n, or a seeded random start from 1 to n); `content_only` counts content
  words only.

**Output.** `cloze_text` replaces each gap with `_____ (n)` (`blank` sets the gap text);
`answer_key` gives each answer with its character offset and word number; `anki` is the passage
in Anki cloze syntax, `{{c1::…}}`, `{{c2::…}}`, one card per gap, or every gap `c1` (one card
hiding all of them) with `anki_single_card`.

The tests check a selection worked by hand (in the mitochondria sentence, *mitochondria* and
*cell* are the only content words that occur twice), the Anki cloze syntax as the manual shows
it, the every-5th-word cloze, a seeded tie-break against Python's own `random.Random(42)`, and
Spanish stopwords.

Limits: words are runs of letters (with internal apostrophes and hyphens); no lemmatisation, so
*cell* and *cells* count apart; no distractors (see `quiz_mcq_scaffold`); `{{` and `::` inside an
answer are spaced out to keep the Anki syntax valid.
