#!/usr/bin/env python3
"""Fill a crossword grid from a word list by backtracking search: the slot with the fewest
fitting words is filled first (minimum remaining values), and a choice that leaves a crossing
slot with no fitting word is undone at once (forward checking). Stops at a time limit.

Input (JSON on stdin):
  grid           ["..T", ".#.", "..."]  '.' empty, '#' block, a letter is fixed
  words          ["CAT", ...]  the word list (case and accents ignored)
  allow_repeats  a word may fill two slots (default false)
  max_solutions  stop after this many fills (default 1, at most 100); more than 1 tells
                 whether the fill is unique
  time_limit     seconds (default 5, at most 25)
"""
import json
import sys
import time
import unicodedata


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fold(word):
    decomposed = unicodedata.normalize("NFKD", word.upper())
    return "".join(c for c in decomposed if not unicodedata.combining(c) and c.isalpha())


def slots_of(grid):
    rows, cols = len(grid), len(grid[0])
    open_cell = lambda r, c: 0 <= r < rows and 0 <= c < cols and grid[r][c] != "#"
    slots, numbers, n = [], {}, 0
    for r in range(rows):
        for c in range(cols):
            if not open_cell(r, c):
                continue
            starts = []
            if not open_cell(r, c - 1) and open_cell(r, c + 1):
                starts.append("across")
            if not open_cell(r - 1, c) and open_cell(r + 1, c):
                starts.append("down")
            if starts:
                n += 1
                numbers[(r, c)] = n
            for direction in starts:
                cells, rr, cc = [], r, c
                while open_cell(rr, cc):
                    cells.append((rr, cc))
                    rr, cc = (rr, cc + 1) if direction == "across" else (rr + 1, cc)
                slots.append({"number": n, "direction": direction, "row": r + 1, "col": c + 1, "cells": cells})
    return slots


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    grid = request.get("grid")
    if not isinstance(grid, list) or not grid or not all(isinstance(r, str) for r in grid):
        fail("grid must be a list of strings, one per row")
    if len(grid) > 25 or any(len(r) != len(grid[0]) for r in grid) or not 1 <= len(grid[0]) <= 25:
        fail("grid rows must all have the same length, at most 25 rows and 25 columns")
    cells = [[ch.upper() for ch in row] for row in grid]
    for row in cells:
        for ch in row:
            if ch not in (".", "#") and not ("A" <= ch <= "Z"):
                fail("grid cells must be '.', '#' or a letter A-Z")
    words = request.get("words")
    if not isinstance(words, list) or not words or not all(isinstance(w, str) for w in words):
        fail("words must be a non-empty list of strings")
    if len(words) > 200000:
        fail("words has more than 200000 entries")
    allow_repeats = request.get("allow_repeats", False)
    if not isinstance(allow_repeats, bool):
        fail("allow_repeats must be true or false")
    max_solutions = request.get("max_solutions", 1)
    if not isinstance(max_solutions, int) or isinstance(max_solutions, bool) or not 1 <= max_solutions <= 100:
        fail("max_solutions must be an integer from 1 to 100")
    limit = request.get("time_limit", 5)
    if isinstance(limit, bool) or not isinstance(limit, (int, float)) or not 0 < limit <= 25:
        fail("time_limit must be a number of seconds from 0 to 25")

    vocab = sorted({fold(w) for w in words if len(fold(w)) >= 2})
    slots = slots_of(cells)
    if not slots:
        fail("the grid has no slot of two or more open cells")
    by_length, index = {}, {}
    for w in vocab:
        by_length.setdefault(len(w), []).append(w)
    for length, ws in by_length.items():
        for k, w in enumerate(ws):
            for p, ch in enumerate(w):
                index.setdefault((length, p, ch), set()).add(k)
    missing = sorted({len(s["cells"]) for s in slots} - set(by_length))

    deadline = time.monotonic() + limit
    used, assigned, solutions, nodes = {}, {}, [], 0
    state = {"timeout": False}

    def candidates(slot):
        length = len(slot["cells"])
        pool = by_length.get(length, [])
        fixed = [(p, cells[r][c]) for p, (r, c) in enumerate(slot["cells"]) if cells[r][c] != "."]
        if fixed:
            sets = sorted((index.get((length, p, ch), set()) for p, ch in fixed), key=len)
            ids = set(sets[0])
            for s in sets[1:]:
                ids &= s
            found = [pool[k] for k in sorted(ids)]
        else:
            found = pool
        if not allow_repeats:
            found = [w for w in found if w not in used]
        return found

    def search():
        nonlocal nodes
        nodes += 1
        if time.monotonic() > deadline:
            state["timeout"] = True
            return True
        open_slots = [i for i in range(len(slots)) if i not in assigned]
        if not open_slots:
            solutions.append(["".join(row) for row in cells])
            return len(solutions) >= max_solutions
        best, best_c = None, None
        for i in open_slots:
            cand = candidates(slots[i])
            if not cand:
                return False  # forward check: a slot no word fits
            if best is None or len(cand) < len(best_c):
                best, best_c = i, cand
                if len(cand) == 1:
                    break
        slot = slots[best]
        for word in best_c:
            changed = []
            for (r, c), ch in zip(slot["cells"], word):
                if cells[r][c] == ".":
                    cells[r][c] = ch
                    changed.append((r, c))
            assigned[best] = word
            used[word] = used.get(word, 0) + 1
            if search():
                return True
            used[word] -= 1
            if not used[word]:
                del used[word]
            del assigned[best]
            for r, c in changed:
                cells[r][c] = "."
        return False

    search()
    result = {"slots": len(slots), "words_given": len(vocab), "nodes": nodes, "solutions_found": len(solutions)}
    if solutions:
        first = solutions[0]
        entries = []
        for s in slots:
            word = "".join(first[r][c] for r, c in s["cells"])
            entries.append({"number": s["number"], "direction": s["direction"], "row": s["row"], "col": s["col"],
                            "length": len(s["cells"]), "word": word})
        entries.sort(key=lambda e: (e["direction"] != "across", e["number"]))
        result.update({"status": "filled", "grid": first, "entries": entries})
        if max_solutions > 1:
            result["other_solutions"] = solutions[1:]
            result["search_complete"] = not state["timeout"] and len(solutions) < max_solutions
            if result["search_complete"]:
                result["unique"] = len(solutions) == 1
    elif state["timeout"]:
        result.update({"status": "timeout", "note": f"no fill found within {limit} s; the search was not finished"})
    else:
        result.update({"status": "no_fill", "note": "the search was exhausted: no fill exists with this word list"
                       + ("" if allow_repeats else " without repeating a word")})
        if missing:
            result["no_words_of_length"] = missing
    print(json.dumps(result))


if __name__ == "__main__":
    main()
