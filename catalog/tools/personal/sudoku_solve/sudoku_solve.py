#!/usr/bin/env python3
"""Sudoku solver for 4x4, 6x6, 9x9 and 16x16 grids: constraint propagation (naked and hidden
singles) with depth-first search on the cell with the fewest candidates, counting solutions up to
two to tell a proper puzzle from one with several solutions, and a difficulty hint from the
logical techniques a human solver would need.

Input (JSON on stdin):
  grid    a string of N*N symbols read row by row ("." or "0" for an empty cell; spaces, "|", "-",
          "+" and line breaks are ignored), or a list of N rows (strings, or lists of numbers with
          0 or null for empty)
          symbols: 1-4, 1-6, 1-9; for 16x16 1-9 then A-G (or numbers 1-16 in list form)
  size    4, 6, 9 or 16 (default: from the number of cells)
  max_nodes  search limit (default 200000)
Boxes: 2x2 for 4x4, 2 rows x 3 columns for 6x6, 3x3 for 9x9, 4x4 for 16x16.
"""
import json
import sys

BOXES = {4: (2, 2), 6: (2, 3), 9: (3, 3), 16: (4, 4)}
SYMBOLS16 = "123456789ABCDEFG"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def symbol(v, n):
    return SYMBOLS16[v - 1] if n == 16 else str(v)


def parse_grid(request):
    raw = request.get("grid")
    size = request.get("size")
    if size is not None and (isinstance(size, bool) or size not in BOXES):
        fail("size must be 4, 6, 9 or 16")
    cells = []
    if isinstance(raw, str):
        text = "".join(ch for ch in raw if ch not in " \t\r\n|-+")
        cells = list(text)
    elif isinstance(raw, list):
        for r, row in enumerate(raw):
            if isinstance(row, str):
                cells.extend(ch for ch in row if ch not in " \t|")
            elif isinstance(row, list):
                for c, v in enumerate(row):
                    if v is None:
                        cells.append(0)
                    elif isinstance(v, bool) or not isinstance(v, (int, str)):
                        fail(f"grid[{r}][{c}] must be a number, a symbol or null")
                    else:
                        cells.append(v)
            else:
                fail(f"grid[{r}] must be a string or a list")
    else:
        fail("grid must be a string or a list of rows")
    n = size or {16: 4, 36: 6, 81: 9, 256: 16}.get(len(cells))
    if n is None or len(cells) != n * n:
        fail(f"grid has {len(cells)} cells; expected 16, 36, 81 or 256 (or size*size)")
    values = []
    for i, cell in enumerate(cells):
        where = f"row {i // n + 1}, column {i % n + 1}"
        if isinstance(cell, int):
            if not 0 <= cell <= n:
                fail(f"{where}: {cell} is out of range 0 to {n}")
            values.append(cell)
            continue
        ch = str(cell).strip().upper()
        if ch in (".", "0", "_", "*", "X", ""):
            values.append(0)
        elif n == 16 and len(ch) == 1 and ch in SYMBOLS16:
            values.append(SYMBOLS16.index(ch) + 1)
        elif ch.isdigit() and 1 <= int(ch) <= n and (n != 16 or len(ch) <= 2):
            values.append(int(ch))
        else:
            fail(f"{where}: {cell!r} is not a symbol of a {n}x{n} sudoku")
    return n, values


class Sudoku:
    def __init__(self, n):
        self.n = n
        br, bc = BOXES[n]
        self.all = (1 << n) - 1
        self.units = []
        for r in range(n):
            self.units.append([r * n + c for c in range(n)])
        for c in range(n):
            self.units.append([r * n + c for r in range(n)])
        for r0 in range(0, n, br):
            for c0 in range(0, n, bc):
                self.units.append([(r0 + dr) * n + c0 + dc for dr in range(br) for dc in range(bc)])
        self.rows, self.cols, self.boxes = self.units[:n], self.units[n:2 * n], self.units[2 * n:]
        self.cell_units = [[u for u in self.units if i in u] for i in range(n * n)]
        self.peers = [sorted({j for u in self.cell_units[i] for j in u} - {i}) for i in range(n * n)]

    def conflicts(self, values):
        found = []
        names = ["row"] * self.n + ["column"] * self.n + ["box"] * self.n
        for k, unit in enumerate(self.units):
            seen = {}
            for i in unit:
                v = values[i]
                if v:
                    if v in seen:
                        found.append(f"{names[k]} {k % self.n + 1} has {symbol(v, self.n)} twice")
                    seen[v] = i
        return found

    def initial(self, values):
        cand = [self.all] * (self.n * self.n)
        for i, v in enumerate(values):
            if v:
                cand = self.assign(cand, i, v)
                if cand is None:
                    return None
        return cand

    def assign(self, cand, i, v):
        """Set cell i to v and propagate naked and hidden singles; None on contradiction."""
        cand = self.assign_simple(cand, i, v)
        return None if cand is None else self.hidden(cand)

    def hidden(self, cand):
        """Hidden singles to a fixed point."""
        changed = True
        while changed:
            changed = False
            for unit in self.units:
                for b in range(self.n):
                    bit = 1 << b
                    places = [i for i in unit if cand[i] & bit]
                    if not places:
                        return None
                    if len(places) == 1 and cand[places[0]] != bit:
                        cand = self.assign_simple(cand, places[0], b + 1)
                        if cand is None:
                            return None
                        changed = True
        return cand

    def assign_simple(self, cand, i, v):
        cand = list(cand)
        stack = [(i, v)]
        while stack:
            i, v = stack.pop()
            bit = 1 << (v - 1)
            if not cand[i] & bit:
                return None
            cand[i] = bit
            for p in self.peers[i]:
                if cand[p] & bit:
                    cand[p] &= ~bit
                    if cand[p] == 0:
                        return None
                    if cand[p] & (cand[p] - 1) == 0:
                        stack.append((p, cand[p].bit_length()))
        return cand

    def search(self, cand, limit_solutions, budget):
        """Depth-first search; returns (solutions, nodes used, exhausted)."""
        solutions = []
        nodes = 0
        stack = [cand]
        while stack:
            cand = stack.pop()
            nodes += 1
            if nodes > budget:
                return solutions, nodes, False
            best, best_count = -1, 99
            for i, c in enumerate(cand):
                if c & (c - 1):
                    k = bin(c).count("1")
                    if k < best_count:
                        best, best_count = i, k
                        if k == 2:
                            break
            if best < 0:
                solutions.append([c.bit_length() for c in cand])
                if len(solutions) >= limit_solutions:
                    return solutions, nodes, True
                continue
            c = cand[best]
            options = []
            while c:
                bit = c & -c
                c ^= bit
                options.append(bit.bit_length())
            for v in reversed(options):
                nxt = self.assign_simple(cand, best, v)
                if nxt is not None:
                    nxt = self.hidden(nxt)
                if nxt is not None:
                    stack.append(nxt)
        return solutions, nodes, True

    # --- the human-technique ladder for the difficulty hint ---
    def rate(self, values):
        """Apply techniques in order of difficulty; return (highest technique used, solved?)."""
        cand = [self.all] * (self.n * self.n)
        for i, v in enumerate(values):
            if v:
                cand = self.eliminate_set(cand, i, v)
                if cand is None:
                    return None, False
        used = "givens only"
        ladder = [("naked singles", self.step_naked), ("hidden singles", self.step_hidden),
                  ("locked candidates (pointing and claiming)", self.step_locked), ("naked pairs", self.step_pairs)]
        level = {name: k for k, (name, _) in enumerate(ladder)}
        guard = 0
        while not all(c & (c - 1) == 0 for c in cand):
            guard += 1
            if guard > 10000:
                break
            progressed = False
            for name, step in ladder:
                nxt = step(cand)
                if nxt is None:
                    return None, False
                if nxt != cand:
                    cand = nxt
                    if used == "givens only" or level[name] > level.get(used, -1):
                        used = name
                    progressed = True
                    break
            if not progressed:
                return used, False
        return used, True

    def eliminate_set(self, cand, i, v):
        cand = list(cand)
        bit = 1 << (v - 1)
        if not cand[i] & bit:
            return None
        cand[i] = bit
        for p in self.peers[i]:
            cand[p] &= ~bit
            if cand[p] == 0:
                return None
        return cand

    def step_naked(self, cand):
        cand = list(cand)
        for i, c in enumerate(cand):
            if c and c & (c - 1) == 0:
                for p in self.peers[i]:
                    if cand[p] & c:
                        if cand[p] == c:
                            return None
                        cand[p] &= ~c
        return cand

    def step_hidden(self, cand):
        for unit in self.units:
            for b in range(self.n):
                bit = 1 << b
                places = [i for i in unit if cand[i] & bit]
                if not places:
                    return None
                if len(places) == 1 and cand[places[0]] != bit:
                    return self.eliminate_set(cand, places[0], b + 1)
        return cand

    def step_locked(self, cand):
        cand = list(cand)
        for box in self.boxes:
            boxset = set(box)
            for line in self.rows + self.cols:
                inter = boxset.intersection(line)
                if not inter:
                    continue
                for b in range(self.n):
                    bit = 1 << b
                    in_box = [i for i in box if cand[i] & bit]
                    in_line = [i for i in line if cand[i] & bit]
                    if len(in_box) > 1 and all(i in inter for i in in_box):  # pointing
                        hits = [i for i in in_line if i not in boxset]
                        if hits:
                            for i in hits:
                                cand[i] &= ~bit
                            return cand
                    if len(in_line) > 1 and all(i in inter for i in in_line):  # claiming
                        hits = [i for i in in_box if i not in inter]
                        if hits:
                            for i in hits:
                                cand[i] &= ~bit
                            return cand
        return cand

    def step_pairs(self, cand):
        cand = list(cand)
        for unit in self.units:
            pairs = {}
            for i in unit:
                c = cand[i]
                if bin(c).count("1") == 2:
                    pairs.setdefault(c, []).append(i)
            for c, cells in pairs.items():
                if len(cells) == 2:
                    hits = [i for i in unit if i not in cells and cand[i] & c]
                    if hits:
                        for i in hits:
                            cand[i] &= ~c
                            if cand[i] == 0:
                                return None
                        return cand
        return cand


def rows_of(values, n):
    return ["".join(symbol(v, n) if v else "." for v in values[r * n:(r + 1) * n]) for r in range(n)]


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"grid", "size", "max_nodes"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    budget = request.get("max_nodes", 200000)
    if isinstance(budget, bool) or not isinstance(budget, int) or not 1 <= budget <= 2000000:
        fail("max_nodes must be an integer from 1 to 2000000")
    n, values = parse_grid(request)
    s = Sudoku(n)
    br, bc = BOXES[n]
    result = {"size": n, "box": f"{br}x{bc}", "givens": sum(1 for v in values if v), "puzzle": rows_of(values, n)}
    problems = s.conflicts(values)
    if problems:
        result.update({"status": "invalid", "valid": False, "problems": problems, "solutions_found": 0})
        print(json.dumps(result))
        return
    cand = s.initial(values)
    if cand is None:
        result.update({"status": "no_solution", "valid": False, "solutions_found": 0,
                       "problems": ["the givens contradict each other through propagation"]})
        print(json.dumps(result))
        return
    solutions, nodes, finished = s.search(cand, 2, budget)
    result["search_nodes"] = nodes
    if not finished and not solutions:
        result.update({"status": "search_limit", "valid": None, "solutions_found": 0,
                       "problems": [f"no solution found within {budget} search nodes"]})
        print(json.dumps(result))
        return
    if not solutions:
        result.update({"status": "no_solution", "valid": False, "solutions_found": 0})
    elif len(solutions) == 1 and finished:
        result.update({"status": "solved", "valid": True, "unique": True, "solutions_found": 1,
                       "solution": rows_of(solutions[0], n)})
    elif len(solutions) == 1:
        result.update({"status": "solved", "valid": True, "unique": None, "solutions_found": 1,
                       "solution": rows_of(solutions[0], n),
                       "problems": [f"uniqueness not established within {budget} search nodes"]})
    else:
        result.update({"status": "multiple_solutions", "valid": False, "unique": False, "solutions_found": 2,
                       "solution": rows_of(solutions[0], n), "another_solution": rows_of(solutions[1], n),
                       "differs_at": [f"r{i // n + 1}c{i % n + 1}" for i in range(n * n) if solutions[0][i] != solutions[1][i]]})
    if len(solutions) == 1:
        technique, logical = s.rate(values)
        if logical:
            hint = {"givens only": "complete", "naked singles": "easy", "hidden singles": "easy",
                    "locked candidates (pointing and claiming)": "medium", "naked pairs": "hard"}[technique]
        else:
            hint = "very hard (needs techniques beyond naked pairs, or guessing)"
            technique = f"{technique}, then search"
        result["difficulty"] = {"hint": hint, "hardest_technique": technique, "solved_by_logic": logical,
                                "ladder": "naked singles, hidden singles, locked candidates, naked pairs; anything harder is reported as search"}
    print(json.dumps(result))


if __name__ == "__main__":
    main()
