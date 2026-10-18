# sudoku_solve

Solves sudoku and says whether the puzzle is proper.

**Grids.** 4×4 (2×2 boxes), 6×6 (boxes of 2 rows × 3 columns), 9×9 (3×3) and 16×16 (4×4, symbols
1–9 then A–G, or numbers 1–16 in list form). The grid is a string of N² symbols read row by row,
with `.` or `0` for an empty cell (spaces, `|`, `-`, `+` and line breaks are ignored), or a list
of rows.

**Method.** Each cell holds a bit set of candidates. Placing a value removes it from the cell's
peers (same row, column and box); a cell left with one candidate is placed in turn (naked
single), and a value with only one place left in a row, column or box is placed there (hidden
single), until nothing changes. Then a depth-first search branches on the empty cell with the
fewest candidates, propagating after each guess. The search continues after the first solution
to look for a second: one solution means the puzzle is proper (`unique: true`); two are shown
with the cells where they differ (`status: "multiple_solutions"`). Duplicate givens give
`status: "invalid"` with the row, column or box; givens that leave some cell without a candidate
or that admit no solution give `"no_solution"`. `max_nodes` bounds the search (default 200 000);
if it runs out the answer says so instead of guessing.

**Difficulty hint** (proper puzzles only). The puzzle is solved again the way a person would,
trying the easiest technique first each time: naked singles, hidden singles, locked candidates
(pointing and claiming), naked pairs. The hint names the hardest technique needed (easy for
singles, medium for locked candidates, hard for naked pairs) or, when these are not enough,
"very hard" (it needs X-wings, chains or guessing). It is a rough guide, not a published grade.

The tests use the example puzzle of Wikipedia's "Sudoku" article with its published solution,
Arto Inkala's 2012 puzzle (which the technique ladder cannot crack), 6×6 and 16×16 grids cut from known
cyclic solutions, and the multiple-solution, invalid and unsolvable cases.

Limits: classic rules only (no diagonals, killer cages, jigsaw regions or other variants); the
technique ladder stops at naked pairs.
