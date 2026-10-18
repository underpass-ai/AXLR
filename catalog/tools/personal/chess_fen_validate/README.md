# chess_fen_validate

Checks that a FEN string describes a position that could occur in a game of chess, and
describes it.

**Syntax** (PGN standard, section 16.1): six space-separated fields. The board has eight ranks
from the 8th to the 1st separated by `/`, each made of piece letters (`PNBRQK` white,
`pnbrqk` black) and digits 1–8 for runs of empty squares adding up to eight squares (two digits
in a row are flagged); side to move `w` or `b`; castling `-` or the letters `KQkq` in that
order; en passant `-` or a square; the halfmove clock (a non-negative integer) and the fullmove
number (from 1).

**Position rules**:
- exactly one king of each colour; at most 8 pawns and 16 pieces a side; pieces beyond the
  initial set (a second queen, a third rook…) only as many as missing pawns could have promoted;
- no pawn on the 1st or 8th rank;
- each castling right needs its king on e1/e8 and its rook on h1/a1/h8/a8;
- an en-passant square must be on rank 6 with white to move (rank 3 with black), the pawn that
  just moved two squares must stand in front of it, the square and the one it passed from must
  be empty, and the halfmove clock must be 0. The square is recorded after every two-square pawn
  move, as in the PGN standard; `strict_en_passant: true` applies the newer convention and also
  requires that a pawn can actually capture. Either way the answer lists the pawns that could;
- the side not to move must not be in check (attacks by pawns, knights, kings and sliding pieces
  are traced on the board); the side to move may be, by at most two pieces, and the checkers are
  listed;
- a halfmove clock larger than the plies played since move 1 is a warning (possible from a set-up
  position), and 100 or more notes the fifty-move rule.

The answer has `valid`, every `error` and `warning`, a text diagram (letters, or chess symbols
with `unicode: true`) and the material count in the usual points (pawn 1, knight and bishop 3,
rook 5, queen 9) with the balance.

The tests use the starting position and the three positions after 1.e4, 1...c5 and 2.Nf3 given
in Wikipedia's "Forsyth–Edwards Notation" article, and positions built to break one rule each.

Limits: it does not prove that a position is reachable (retrograde analysis), check pawn
structures against captures, or detect checkmate and stalemate; Chess960 (Shredder or X-FEN)
castling letters are not accepted.
