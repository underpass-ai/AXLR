# quiz_mcq_scaffold

Drafts multiple-choice questions from a passage and the terms a teacher wants tested. It is a
scaffold: the stems and distractors come from the text itself and are meant to be reviewed.

**Stem.** The first sentence containing the term (case-insensitive, whole words; a phrase may
span any whitespace) with every occurrence of the term replaced by `blank`. When the term opens
the sentence, the answer is written as the term was given, since its capital may only be
positional.

**Distractors** are candidate words and phrases (one to four words, not starting or ending
with a stopword) from the text, filtered and ranked by form, the usual heuristics for
homogeneous options:

1. the same number of words as the answer, digits only if the answer has digits;
2. not in the stem's own sentence, not a stopword, and not a variant of the answer (one contains
   the other, or they share all but their last two letters, as *chloroplast* and
   *chloroplasts*);
3. the same capitalisation pattern (lower, Title, ALL CAPS, mixed); a word seen only at the start
   of sentences is left out because its real case is unknown (*Darwin* or *Plants*?);
4. length within max(2, a third of the answer's length);
5. ranked by length difference plus a penalty of 2 for a different word ending (-tion, -ing,
   -ed, -ly, -s…, a rough part-of-speech signal).

From the best `2 × needed` candidates, sorted alphabetically, `random.Random(seed)` shuffles and
the first ones are taken; the choices (answer included, alphabetically sorted) are shuffled by
the same generator and labelled A to F. When too few candidates pass, the length rule and then
the capitalisation rule are relaxed and `notes` says so; if still short, `short_of_distractors`
counts the missing ones (nothing is invented).

The tests check a five-question set from a biology passage whose distractors were verified by
hand against the rules (for *chlorophyll*, 11 letters: *mitochondria*, *chloroplast*,
*heredity*, all lower case and from other sentences), capitalised terms, variants, a missing
term and a too-short text. The same input and seed always give the same questions.

Limits: no part-of-speech tagger or semantics, so a distractor can be of the wrong kind
(*carbon* for *Mendel* after relaxing capitalisation); English and Spanish stopwords only;
one stem per term.
