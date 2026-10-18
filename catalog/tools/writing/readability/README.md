# readability

The classic readability formulas, with the counts they rest on.

| Language | Score | Formula |
|:--|:--|:--|
| en | Flesch reading ease | 206.835 − 1.015·(words/sentences) − 84.6·(syllables/words), with Flesch's bands |
| en | Flesch-Kincaid grade | 0.39·(w/s) + 11.8·(syl/w) − 15.59 |
| en | Gunning fog | 0.4·(w/s + 100·complex/w), complex = 3+ syllables, not capitalised, not -es/-ed/-ing |
| en | SMOG | 1.043·√(polysyllables·30/sentences) + 3.1291 |
| en | Coleman-Liau | 0.0588·L − 0.296·S − 15.8 (letters and sentences per 100 words) |
| en | ARI | 4.71·(letters/words) + 0.5·(w/s) − 21.43 |
| es | Fernández-Huerta | 206.84 − 60·(syl/w) − 1.02·(w/s) |
| es | Szigriszt-Pazos | 206.835 − 62.3·(syl/w) − (w/s), with the INFLESZ bands of Barrio-Cantalejo et al. (2008) |

Spanish syllables follow the spelling rules: a, e, o and accented í, ú are strong; two strong
vowels split (le-er, pa-ís), a weak one joins its neighbour (ciu-dad), the *u* of *que, qui,
gue, gui* is silent, final *y* is a vowel. English syllables are a heuristic (vowel groups,
silent final *e*, *-ed*, *-es*, common two-vowel splits), right for most words and off by one
for some (*idea*, *created*); Coleman-Liau and ARI use letters only and are unaffected.

Sentences end at `.`, `!`, `?` or `…` before a space or the end, or at a blank line; a period
after a single capital (*U.S.*) or a common abbreviation (*Dr.*, *etc.*, *Sra.*) does not. The
first test is Coleman and Liau's own passage, which their paper scores at grade 14.5.

Limits: formulas built for continuous prose; lists, tables and code give meaningless scores.
