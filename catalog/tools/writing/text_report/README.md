# text_report

Counts and frequencies for a text given inline.

**Tokens.** A word is a run of letters, possibly joined by an inner apostrophe or hyphen
(*didn't*, *well-known* are one word each), or a number with inner `.` or `,` (*3.5*, *1,000*).
Words are case-folded before counting. Characters are code points: `characters` counts all of
them, `characters_no_spaces` all but whitespace, `letters` the alphabetic ones.

**Sentences** end at `.`, `!`, `?` or `…` followed by whitespace or the end of the text (closing
quotes and brackets stay with the sentence), and at a paragraph break. **Paragraphs** are
separated by blank lines. Abbreviations such as "Dr." therefore end a sentence.

**Top words** are sorted by count, then alphabetically; `stopwords: "en"` or `"es"` removes a
short list of function words (written for this tool: 126 English and 115 Spanish words).
**N-grams** (2 and 3) are counted inside sentences; with stopwords on, an n-gram that starts or
ends with a stopword is skipped.

**Time.** reading = words ÷ `reading_wpm` (default 238, Brysbaert's 2019 meta-analysis average
for silent reading of English non-fiction), speaking = words ÷ `speaking_wpm` (default 150, a
common presentation pace). Seconds are rounded half up; the wpm used is in the output.

**Lexical diversity.** Type-token ratio = distinct words ÷ words, also as an exact fraction.
With `mtld: true`, MTLD (McCarthy & Jarvis, 2010): walking through the words, a factor closes
each time the running TTR falls to 0.72 or below; the leftover segment adds
(1 − TTR) ÷ (1 − 0.72) of a factor; MTLD = words ÷ factors, averaged over a forward and a
backward pass. It is `null` when no factor forms (every word distinct).

The tests check a paragraph counted by hand (81 characters, 63 without spaces, 57 letters, 18
words, 15 distinct, 4 sentences, 2 paragraphs, *the* 4 times; 18 words take 4.5 s at 238 wpm),
the two "to be" bigrams of Hamlet's line, and MTLD worked by hand for "a a a a" (2.00).

Limits: no language detection, stemming or lemmatisation; sentence splitting is punctuation
based; MTLD is unstable for texts under about 100 words.
