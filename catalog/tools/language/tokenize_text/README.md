# tokenize_text

Splits a text into tokens and sentences and counts them. Every token and sentence carries its
offsets as code-point indexes into the input (end exclusive), so `text[start:end]` gives it back.

## Tokens

At each position the first rule that matches wins:

| Type | Rule |
|:--|:--|
| `url` | `http://`, `https://`, `ftp://` or `www.` up to whitespace; trailing `.,;:!?…` and unbalanced closing brackets are left out |
| `email` | `local@domain.tld` with Unicode word characters |
| `number` | digits with internal `.` `,` `'` or narrow spaces between digit groups (`3.50`, `1.000,5`), an exponent, a sign at the start of a word (`+34`, `-2`), a trailing `%` or `‰` |
| `word` | letters, combining marks and digits (Unicode categories L, M, N), with internal apostrophes (`'` `’` `ʼ`) and hyphens (`-` `‐` `‑`): `l'homme`, `rock’n’roll`, `well-known`, `COVID-19`; digits running into letters make a word (`1st`, `3D`, `mp3`) |
| `cjk` | one Han ideograph, hiragana or katakana character (with any combining mark); Hangul is written with spaces and is tokenised as words |
| `punctuation`, `symbol` | any other character; runs of `.`, `-`, `!`/`?` are one token (`...`, `?!`); emoji modifiers, variation selectors and ZWJ sequences stay with their emoji |

## Sentences

A sentence ends at `.`, `!`, `?`, `…`, `‼` (with any closing quotes or brackets that follow),
when whitespace follows and the next word does not start in lowercase (opening `¿ ¡ « " (`
are skipped when looking at the next word). `。！？．` end a sentence even without a space. A
blank line ends a sentence too. A period does not end a sentence after:

- an abbreviation of the chosen `language` (`Mr.`, `Dr.`, `e.g.`; `Sr.`, `Dña.`, `pág.`;
  `M.`, `Mme`, `p.`; `Hr.`, `Nr.`, `z.B.`, `usw.`), except that `etc.`, `usw.`, `Inc.` and a
  few others may end one when the next word is capitalised;
- a dotted acronym of short parts (`U.N.`, `i.e.`) before a lowercase word, or a spaced one
  (`z. B.`, `d. h.`);
- a single capital letter (initials: `J. R. R. Tolkien`);
- in German, a number (ordinals: `am 3. Oktober`).

## Counts

Characters, characters without whitespace, tokens by type, unique words (case-folded unless
`lowercase` is false), sentences, paragraphs (blocks separated by blank lines), mean word length
and the `top` most frequent words.

## Tests

The expected values are hand-checked: the offsets in the first case were counted character by
character, and the sentence splits follow the rules above for English, Spanish (`Sr.`, `etc.`,
`¿…?`), German (`3.`, `z. B.`), Japanese (`。`) and French apostrophes.

## Limits

Rule-based, not statistical (Punkt, spaCy and ICU do better on unseen abbreviations). An
abbreviation at the very end of a sentence followed by a capitalised word is kept inside the
sentence unless it is one of the few that can end one, and a dotted acronym before a
capitalised word is taken as a sentence end (`the U.S. Army` is split). Chinese and Japanese are not segmented into words,
only into characters. Thai, Lao, Khmer and Burmese, written without spaces, come out as one
word per run of letters. Elisions such as French `l'` are not split from their word.
