#!/usr/bin/env python3
"""Validate a chess position in Forsyth-Edwards Notation (FEN): syntax of the six fields, piece
counts, kings, pawns on the back ranks, castling rights against the king and rook squares, the
en-passant square, the move counters and that the side not to move is not in check. Returns a
board diagram, the material count and every problem found.

Input (JSON on stdin):
  fen                     the FEN string
  strict_en_passant       true: also require that an en-passant capture is actually possible (the
                          newer convention); default false (the PGN standard's: record the square
                          after every two-square pawn move)
  unicode                 true: diagram with chess symbols instead of letters
"""
import json
import sys

FILES = "abcdefgh"
VALUES = {"p": 1, "n": 3, "b": 3, "r": 5, "q": 9, "k": 0}
NAMES = {"p": "pawns", "n": "knights", "b": "bishops", "r": "rooks", "q": "queens", "k": "kings"}
UNICODE = {"K": "♔", "Q": "♕", "R": "♖", "B": "♗", "N": "♘", "P": "♙",
           "k": "♚", "q": "♛", "r": "♜", "b": "♝", "n": "♞", "p": "♟"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def sq(name):
    """'e4' -> (file 0-7, rank 0-7)."""
    return FILES.index(name[0]), int(name[1]) - 1


def sqname(f, r):
    return f"{FILES[f]}{r + 1}"


def parse_board(text, errors):
    ranks = text.split("/")
    if len(ranks) != 8:
        errors.append(f"the board has {len(ranks)} ranks; it needs 8 separated by '/'")
        return None
    board = {}
    for i, rank_text in enumerate(ranks):
        rank = 7 - i
        f = 0
        previous_digit = False
        for ch in rank_text:
            if ch.isdigit():
                if ch in "09":
                    errors.append(f"rank {rank + 1}: '{ch}' is not a valid empty-square count (1-8)")
                    return None
                if previous_digit:
                    errors.append(f"rank {rank + 1}: two digits in a row ('{rank_text}')")
                f += int(ch)
                previous_digit = True
            elif ch in "pnbrqkPNBRQK":
                if f > 7:
                    errors.append(f"rank {rank + 1} has more than 8 squares ('{rank_text}')")
                    return None
                board[(f, rank)] = ch
                f += 1
                previous_digit = False
            else:
                errors.append(f"rank {rank + 1}: '{ch}' is not a piece letter (pnbrqk / PNBRQK) or digit")
                return None
        if f != 8:
            errors.append(f"rank {rank + 1} has {f} squares, not 8 ('{rank_text}')")
            return None
    return board


def attackers(board, target, by_white):
    """Squares of pieces of the given colour that attack target."""
    tf, tr = target
    found = []
    own = (lambda p: p.isupper()) if by_white else (lambda p: p.islower())
    # pawns
    dr = -1 if by_white else 1
    for df in (-1, 1):
        s = (tf + df, tr + dr)
        p = board.get(s)
        if p and own(p) and p.lower() == "p":
            found.append(s)
    # knights
    for df, d in ((1, 2), (2, 1), (-1, 2), (-2, 1), (1, -2), (2, -1), (-1, -2), (-2, -1)):
        s = (tf + df, tr + d)
        p = board.get(s)
        if p and own(p) and p.lower() == "n":
            found.append(s)
    # king
    for df in (-1, 0, 1):
        for d in (-1, 0, 1):
            if df or d:
                s = (tf + df, tr + d)
                p = board.get(s)
                if p and own(p) and p.lower() == "k":
                    found.append(s)
    # sliders
    for (df, d), kinds in (((1, 0), "rq"), ((-1, 0), "rq"), ((0, 1), "rq"), ((0, -1), "rq"),
                           ((1, 1), "bq"), ((1, -1), "bq"), ((-1, 1), "bq"), ((-1, -1), "bq")):
        f, r = tf + df, tr + d
        while 0 <= f < 8 and 0 <= r < 8:
            p = board.get((f, r))
            if p:
                if own(p) and p.lower() in kinds:
                    found.append((f, r))
                break
            f, r = f + df, r + d
    return found


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"fen", "strict_en_passant", "unicode"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    fen = request.get("fen")
    if not isinstance(fen, str) or not fen.strip() or len(fen) > 200:
        fail("fen must be a non-empty string of at most 200 characters")
    strict_ep = request.get("strict_en_passant", False)
    use_unicode = request.get("unicode", False)
    if not isinstance(strict_ep, bool) or not isinstance(use_unicode, bool):
        fail("strict_en_passant and unicode must be booleans")
    errors, warnings = [], []
    fields = fen.split()
    result = {"fen": fen.strip()}
    if len(fields) != 6:
        errors.append(f"FEN has {len(fields)} fields; it needs 6 (board, side, castling, en passant, halfmove clock, fullmove number)")
    fields += [None] * (6 - len(fields))
    board = parse_board(fields[0], errors)
    side, castling, ep, half, full = fields[1:6]
    if side not in ("w", "b"):
        errors.append(f"side to move must be 'w' or 'b', not {side!r}")
    if castling is not None and castling != "-":
        if any(c not in "KQkq" for c in castling) or len(set(castling)) != len(castling) or \
                "".join(sorted(castling, key="KQkq".index)) != castling:
            errors.append(f"castling field {castling!r} must be '-' or KQkq letters in that order without repeats")
    if ep is not None and ep != "-":
        if len(ep) != 2 or ep[0] not in FILES or ep[1] not in "36":
            errors.append(f"en passant field {ep!r} must be '-' or a square on rank 3 or 6")
    halfmove = fullmove = None
    if half is not None:
        if half.isdigit():
            halfmove = int(half)
        else:
            errors.append(f"halfmove clock {half!r} must be a non-negative integer")
    if full is not None:
        if full.isdigit() and int(full) >= 1:
            fullmove = int(full)
        else:
            errors.append(f"fullmove number {full!r} must be a positive integer")

    if board is not None:
        counts = {p: 0 for p in "PNBRQKpnbrqk"}
        for p in board.values():
            counts[p] += 1
        for colour, upper in (("white", True), ("black", False)):
            c = {k: counts[k.upper() if upper else k] for k in "pnbrqk"}
            if c["k"] != 1:
                errors.append(f"{colour} has {c['k']} kings; there must be exactly one")
            if c["p"] > 8:
                errors.append(f"{colour} has {c['p']} pawns; at most 8")
            total = sum(c.values())
            if total > 16:
                errors.append(f"{colour} has {total} pieces; at most 16")
            extra_pieces = max(0, c["q"] - 1) + max(0, c["r"] - 2) + max(0, c["b"] - 2) + max(0, c["n"] - 2)
            if extra_pieces > 8 - c["p"]:
                errors.append(f"{colour} has {extra_pieces} pieces beyond the initial set but only {8 - c['p']} pawns could have promoted")
        for (f, r), p in board.items():
            if p.lower() == "p" and r in (0, 7):
                errors.append(f"pawn on {sqname(f, r)}: pawns cannot stand on the first or eighth rank")
        # castling rights against the pieces
        if castling and castling != "-" and all(c in "KQkq" for c in castling):
            need = {"K": ("e1", "h1", "K", "R"), "Q": ("e1", "a1", "K", "R"), "k": ("e8", "h8", "k", "r"), "q": ("e8", "a8", "k", "r")}
            for right in castling:
                ksq, rsq, king, rook = need[right]
                if board.get(sq(ksq)) != king:
                    errors.append(f"castling right {right} but the {'white' if right.isupper() else 'black'} king is not on {ksq}")
                elif board.get(sq(rsq)) != rook:
                    errors.append(f"castling right {right} but there is no {'white' if right.isupper() else 'black'} rook on {rsq}")
        # en passant square
        ep_info = None
        if ep and ep != "-" and len(ep) == 2 and ep[0] in FILES and ep[1] in "36" and side in ("w", "b"):
            f, r = sq(ep)
            if (side == "w") != (r == 5):
                errors.append(f"en passant square {ep} is on the wrong rank for {('white' if side == 'w' else 'black')} to move "
                              f"(rank {'6' if side == 'w' else '3'} expected)")
            else:
                mover_pawn = "p" if side == "w" else "P"
                pawn_sq = (f, r - 1) if side == "w" else (f, r + 1)
                origin = (f, r + 1) if side == "w" else (f, r - 1)
                if board.get(pawn_sq) != mover_pawn:
                    errors.append(f"en passant square {ep} but no {'black' if side == 'w' else 'white'} pawn on {sqname(*pawn_sq)} that just moved two squares")
                if (f, r) in board or origin in board:
                    errors.append(f"en passant square {ep}: {ep} and {sqname(*origin)} must be empty after a two-square pawn move")
                capturer = "P" if side == "w" else "p"
                capturers = [sqname(f + d, pawn_sq[1]) for d in (-1, 1) if 0 <= f + d < 8 and board.get((f + d, pawn_sq[1])) == capturer]
                ep_info = {"square": ep, "captured_pawn": sqname(*pawn_sq), "capture_possible_from": capturers}
                if strict_ep and not capturers:
                    errors.append(f"en passant square {ep} is recorded but no pawn can capture en passant (strict convention)")
            if halfmove not in (None, 0):
                errors.append(f"en passant square {ep} means a pawn just moved, so the halfmove clock must be 0, not {halfmove}")
        if ep_info:
            result["en_passant"] = ep_info
        # checks
        kings = {p: s for s, p in board.items() if p in "Kk"}
        if counts["K"] == 1 and counts["k"] == 1 and side in ("w", "b"):
            white_checkers = attackers(board, kings["k"], True)    # white pieces attacking the black king
            black_checkers = attackers(board, kings["K"], False)   # black pieces attacking the white king
            to_move_checkers = black_checkers if side == "w" else white_checkers
            other_checkers = white_checkers if side == "w" else black_checkers
            if other_checkers:
                errors.append(f"the side not to move ({'black' if side == 'w' else 'white'}) is in check from "
                              + ", ".join(sqname(*s) for s in other_checkers) + "; that position cannot arise")
            if len(to_move_checkers) > 2:
                errors.append(f"the side to move is in check from {len(to_move_checkers)} pieces; at most 2 is possible")
            result["in_check"] = bool(to_move_checkers)
            if to_move_checkers:
                result["checkers"] = [f"{board[s]}{sqname(*s)}" for s in to_move_checkers]
        # counters plausibility
        if halfmove is not None and fullmove is not None and side in ("w", "b"):
            plies = 2 * (fullmove - 1) + (1 if side == "b" else 0)
            if halfmove > plies:
                warnings.append(f"halfmove clock {halfmove} exceeds the {plies} plies played since move 1 (possible only from a set-up position)")
        if halfmove is not None and halfmove >= 100:
            warnings.append("halfmove clock is 100 or more: either player may claim a draw under the fifty-move rule")
        # diagram and material
        diagram = []
        for r in range(7, -1, -1):
            row = []
            for f in range(8):
                p = board.get((f, r))
                row.append((UNICODE[p] if use_unicode else p) if p else ".")
            diagram.append(f"{r + 1} " + " ".join(row))
        diagram.append("  " + " ".join(FILES))
        result["diagram"] = diagram
        material = {}
        for colour, upper in (("white", True), ("black", False)):
            c = {NAMES[k]: counts[k.upper() if upper else k] for k in "pnbrqk"}
            c["points"] = sum(VALUES[k] * counts[k.upper() if upper else k] for k in "pnbrqk")
            material[colour] = c
        material["balance"] = material["white"]["points"] - material["black"]["points"]
        material["scale"] = "pawn 1, knight 3, bishop 3, rook 5, queen 9"
        result["material"] = material
    result["side_to_move"] = {"w": "white", "b": "black"}.get(side)
    result["castling"] = castling
    result["halfmove_clock"] = halfmove
    result["fullmove_number"] = fullmove
    result["valid"] = not errors
    result["errors"] = errors
    result["warnings"] = warnings
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
