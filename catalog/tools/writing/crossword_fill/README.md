# crossword_fill

Fills a crossword grid from a word list.

**Grid**: one string per row, `.` for an empty cell, `#` for a block, a letter for a cell that
is already filled (up to 25 × 25). A slot is a run of two or more open cells across or down;
slots are numbered the usual way, left to right and top to bottom, a cell getting a number
when it starts an across or a down slot.

**Words**: from the input; case is ignored and accents are folded (NFKD, combining marks
removed), so *Café* fills `CAFE`. Unless `allow_repeats` is true, a word fills at most one slot.

**Search**: backtracking. At every step the open slot with the fewest words that fit its
current letters is filled next (minimum remaining values); if any open slot has no fitting
word the step is undone at once (forward checking). Words that fit a slot are found by
intersecting an index of (length, position, letter). Candidates are tried in alphabetical
order, so the answer is deterministic. With `max_solutions` above 1 the search goes on to find
more fills and, if it finishes, says whether the fill is `unique`.

**Answers**: `status` is `filled` (with `grid` and `entries`: number, direction, row, column,
length, word), `no_fill` (the search was exhausted: no fill exists; lengths with no word at
all are listed) or `timeout` (`time_limit`, default 5 s, ran out; nothing is proved).

The tests use a 3 × 3 grid solved by hand (CAT/BOW across, CAB/TOW down; unique once a T is
fixed, two transposed fills without it) and the word square BAT/ARE/TEN, which needs every
word twice.

Limits: an exhaustive search; large grids with large lists may time out; no clue writing,
scoring of word quality, rebus squares or symmetry checks.
