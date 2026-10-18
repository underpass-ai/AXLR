# anagram_wordplay

Five word games on text and word lists. Letters are compared after folding: lowercase, Unicode
NFKD with the combining marks removed (so `á` counts as `a` and `ñ` as `n`).

| `operation` | What it returns |
|:--|:--|
| `anagrams` | Words whose sorted letters equal those of `text` (the text itself excluded); with `multi_word: true`, phrases of 2 to `max_words` words, each word at least `min_word_length` letters, listed once in alphabetical word order (no permutations) |
| `palindrome` | Whether the letters and digits read the same both ways (punctuation, spaces, case and accents ignored), and whether the words do |
| `lipogram` | Whether the text avoids `letters`, with each offending character and its offset (a code-point index from 0) |
| `word_ladder` | A shortest chain from `start` to `end` changing one letter at a time, every step a word of the list (breadth-first search) |
| `letter_frequency` | Counts and percentages per letter, most frequent first, missing a–z letters and whether the text is a pangram |

Word lists: `words` in the input, and/or `bundled: true` for `words_en.txt`, 29,483
lowercase English words of 1 to 9 letters (no proper nouns, abbreviations or possessives). It
was written by `tools/catalog/data/anagram_wordplay.py` from SCOWL / the English Speller
Database (ESDB) size 35 ("small"), US spelling, fetched from `app.aspell.net/create` on
2026-10-18. Licence of that list, as its notice requires to be reproduced:

> Copyright 2000-2026 by Kevin Atkinson. Permission to use, copy, modify, distribute, and sell
> any part of the English Speller Database (ESDB, previously known as SCOWLv2), or word lists
> created from it, is hereby granted without fee, provided that the above copyright notice
> appears in all copies and that both the above copyright notice and this notice appear in
> supporting documentation. Kevin Atkinson makes no representations about the suitability of
> this database for any purpose. It is provided "as is" without express or implied warranty.

The full notice, including the credit to 12dicts and ENABLE2K (public domain), heads the data file.

The tests check textbook cases: listen/silent (with enlist, inlets, tinsel), "dormitory = dirty
room", Leigh Mercer's "A man, a plan, a canal: Panama" and the Spanish "Dábale arroz a la zorra
el abad", Lewis Carroll's doublet HEAD → heal → teal → tell → tall → TAIL (five steps,
Vanity Fair 1879), the pangram "The quick brown fox jumps over the lazy dog" (35 letters) and a
hand-counted lipogram.

Limits: the multi-word search stops at `max_results` or `time_limit` (`search_complete` says
which); texts of at most 40 letters for anagrams; the bundled list is small and American, so
rare words and longer words (10 letters or more) are missing — pass your own list for those.
