# wordle_helper

Helps with Wordle-style games: works out the feedback of a guess, narrows a word list with the
feedback you got, and suggests the next guess.

**Feedback rule** (the game's, also for repeated letters): first every letter in the right place
is green; then, left to right, each other letter of the guess is yellow if the answer still has
an unmatched copy of it, which that yellow uses up, and grey otherwise. So SPEED against ABIDE
gives one yellow E and one grey E, and ROBOT against FLOOR marks the O in place green and the
other O yellow. `score: {guess, answer}` returns just this.

**Filtering.** A word stays a candidate exactly when guessing each earlier word against it would
have produced the feedback you saw. This is the whole rule, repeated letters included, without
hand-written "contains"/"excludes" shortcuts that get duplicates wrong. Feedback is written per
letter as `g`, `y`, `.` (or `2`, `1`, `0`, or the coloured squares 🟩 🟨 ⬛ ⬜).

**Ranking.**
- `entropy` (default): for a possible guess, the candidates are split by the feedback pattern
  they would give; the expected information is H = −Σ p·log₂ p over the patterns, with the
  expected number of remaining candidates Σ nᵢ²/N beside it. Guesses come from the whole word
  list (or, with `hard_mode`, only from the candidates); ties go to words that could be the
  answer. To stay fast, when the list is large only the guesses with the best letter-frequency
  score are evaluated (about 400 000 pattern computations in all), and the answer says so.
- `frequency`: candidates by how many candidates contain each of their distinct letters, with
  letters in their common positions as a tie-breaker.

**Word list.** `words` in the input (any length, all equal), or the bundled `words5.txt`: 3 438
five-letter lowercase words from SCOWL / the English Speller Database (ESDB) by Kevin Atkinson,
size 35 ("small"), US spelling, written by `tools/catalog/data/wordle_helper.py` on 2026-10-18.
It is a general word list, not the game's own answer or guess list (which belongs to The New
York Times and is not bundled), so it includes plurals and lacks some words the game accepts;
pass the list you want to play with in `words`. ESDB notice, as its licence requires: "Copyright
2000-2026 by Kevin Atkinson. Permission to use, copy, modify, distribute, and sell any part of
the English Speller Database (ESDB, previously known as SCOWLv2), or word lists created from it,
is hereby granted without fee, provided that the above copyright notice appears in all copies
and that both the above copyright notice and this notice appear in supporting documentation.
Kevin Atkinson makes no representations about the suitability of this database for any purpose.
It is provided "as is" without express or implied warranty." (The full header is in the data
file.)

The tests check feedback cases with repeated letters worked by hand from the game's rules (SPEED
vs ABIDE, ROBOT vs FLOOR from Wikipedia's description, EERIE vs THERE, ALLOY vs LLAMA), filtering
and entropy on a small list, emoji feedback and the bundled list.

Limits: one game at a time; no lookahead beyond one guess (not an optimal solver).
