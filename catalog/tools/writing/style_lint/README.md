# style_lint

Flags common style problems in English or Spanish prose and says where they are. It is a
**heuristic**: it matches word patterns and short lists, without parsing, so it misses some
cases and flags some correct sentences. Read each finding as a question, not a verdict.

| Check | Rule |
|:--|:--|
| `passive` | English: a form of *be* (am, is, are, was, were, be, been, being), at most two words in between that are *not, also, always, never, often, still, already, just, then, being* or end in *-ly*, then a past participle (*-ed*, or one of about 100 irregular forms). Spanish: a form of *ser* (voz pasiva) or *estar* (pasiva de estado, resultative) + a participle in *-ado/-ada/-ido/-ida* (and plurals) or an irregular one (*hecho, dicho, escrito, visto, puesto, abierto, roto…*), with the same kind of short gap (*no, ya, muy, también…*, *-mente*) |
| `filler` | Words and phrases that often add nothing: *very, really, just, basically, kind of, in order to…*; *realmente, básicamente, bastante, o sea, en realidad…* |
| `adverb` | Words in *-ly* longer than four letters (minus a list of non-adverbs such as *only, family, reply, friendly*) or in *-mente* longer than six (minus *mente, demente, clemente, vehemente*); fillers are reported once, as filler |
| `repetition` | The same word (at least `min_repeat_length` letters, not a short function word) again within `repeat_window` words; the finding is the later use, with its `distance` |
| `long_sentence` | A sentence of more than `long_sentence_words` words (default 30) |
| `sentence_variety` | Sentence-length statistics (min, max, mean, population standard deviation, coefficient of variation); with five or more sentences and a coefficient below 0.25, a finding over the whole text |

Every finding has `check`, `start`, `end` (code-point indices, end exclusive, so
`text[start:end]` is the flagged span), the flagged `text` and a `message`. Findings are sorted
by position; `summary` counts them per check. Sentences end at `.`, `!`, `?` or `…` before a
space, or at a blank line.

The tests check two samples annotated by hand, one per language (two passives, the fillers,
an adverb and a repeat each, with offsets counted by hand), a sentence counted by hand against
a threshold, and five equal sentences for the variety check.

Limits: *is tired* and *está cansado* (adjectival participles) are flagged as passives; English
passives with *get*, Spanish *se* passives (*se venden casas*) and participles not ending in
the listed forms are missed; *-ed* adjectives and past tenses after *be* are not told apart.
The thresholds (30 words, 0.25) are conventions, not research results.
