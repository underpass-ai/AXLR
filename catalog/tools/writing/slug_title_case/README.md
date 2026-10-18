# slug_title_case

Four text transformations.

**slug**: the extra letters NFKD does not decompose are mapped first (ß → ss, æ → ae, œ → oe,
ø → o, đ → d, ł → l, þ → th, & → and), then the text is decomposed (NFKD), combining marks
dropped, anything not ASCII dropped, lowercased, and every run of other characters becomes
one `separator`. The slug is cut at the last separator that keeps it within `max_length`
(a single word longer than that is cut in the word); `truncated` says whether words were lost.

**title_case**: the first and last words and the first word after a colon, question mark or
em dash are capitalized; minor words are lowercased:

| `style` | Lowercase |
|:--|:--|
| `chicago` (18th ed., 2024) | articles; *and, but, for, or, nor*; *to*, *as*; prepositions of four letters or fewer (*Through*, *According*, *About* are capitalized) |
| `chicago17` | the same but every preposition, whatever its length (*through*, *according*) |
| `ap` | articles, prepositions and conjunctions of three letters or fewer |
| `apa` (7th ed.) | articles, and conjunctions and prepositions of three letters or fewer (*and, as, but, for, if, nor, or, so, yet*; *at, by, in, of, off, on, per, to, up, via*) |

Hyphenated compounds: each element follows the same rules (*Under-the-Counter*,
*Self-Sustaining*); in `chicago17` an element after a prefix that is not a word by itself
(*anti, non, pre, post, re, semi, sub…*) stays lowercase unless it is capitalized in the input
as a proper noun (*Anti-intellectuals*, *Non-English-Speaking*, *Pre-Columbian*), while the
18th edition capitalizes it (*Anti-Intellectuals*). Words in capitals (*NASA*) or with an inner
capital (*iPhone*, *McDonald*) are kept; a title typed all in capitals is lowercased first.

**sentence_case**: everything lowercase except the first word, the first word after `:`,
`.`, `?` or `!`, *I*, acronyms, words with an inner capital, and the words listed in `keep`.

**smart_quotes**: `---` → em dash, `--` → en dash, `...` → ellipsis; a straight quote after
the start, a space, a bracket or a dash opens, any other closes; an apostrophe between letters
(*it’s*) or before two digits (*’90s*) is ’. With `language: "es"` double quotes nest as
« … “ … ” … », the order the RAE recommends.

The tests use the Chicago Manual of Style Q&A's own examples ("The Gospel According to
Matthew" in the 18th edition and "according" in the 17th, "Apples as Well as Oranges", "Are
Anti-intellectuals Overrated?" and "Non-English-Speaking Representatives"), the Chicago/AP
contrast "A River Runs through It", and Django's documented slugify example ("Joel is a slug").

Limits: parts of speech come from word lists, so a preposition used as an adverb or particle
(*Look Up*, *Turn Off*), which the guides capitalize, stays lowercase; proper nouns must be
capitalized in the input to survive sentence case or the 17th-edition prefix rule.
