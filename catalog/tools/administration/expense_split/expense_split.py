#!/usr/bin/env python3
"""Split shared expenses and settle up: each expense is shared equally, by weights or by exact
amounts among some participants; balances are what each paid minus their share; the settlement
lists who pays whom, with the fewest transfers. Exact Decimal arithmetic in the currency's minor
unit; when a share does not divide evenly, the leftover cents go, one each, to the largest
fractional remainders, ties to the earlier person in `among` (largest-remainder method).

Input (JSON on stdin):
  expenses      [{"description"?, "paid_by", "amount", "among"?: [names], "weights"?: {name: w},
                  "shares"?: {name: amount}}]   amount as number or decimal string
  participants  optional list of names (order used for ties; default order of appearance)
  payments      optional [{"from", "to", "amount"}] already paid back
  decimals      optional minor-unit digits, default 2;  currency optional label
"""
import json
import sys
from decimal import Decimal, InvalidOperation
from fractions import Fraction

MAX_EXACT = 15


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def amount(value, field, unit, allow_zero=False):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        number = Decimal(str(value).strip())
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    if abs(number) > Decimal("1e15"):
        fail(f"{field} is too large")
    if number < 0 or (number == 0 and not allow_zero):
        fail(f"{field} must be {'zero or ' if allow_zero else ''}positive")
    units = number / unit
    if units != units.to_integral_value():
        fail(f"{field} has more decimals than the currency's minor unit")
    return int(units)


def name_of(value, field, people):
    if not isinstance(value, str) or not value.strip():
        fail(f"{field} must be a participant name")
    if value not in people:
        if fixed_people:
            fail(f"{field}: {value!r} is not in participants")
        people.append(value)
    return value


fixed_people = False


def split(total, weights):
    """Largest remainder: floor shares, then one unit each to the largest remainders."""
    whole = sum(weights.values(), Fraction(0))
    raw = {n: Fraction(total) * w / whole for n, w in weights.items()}
    base = {n: int(r.numerator // r.denominator) for n, r in raw.items()}
    left = total - sum(base.values())
    order = sorted(weights, key=lambda n: (-(raw[n] - base[n]), list(weights).index(n)))
    for n in order[:left]:
        base[n] += 1
    return base


def zero_sum_groups(names, balances):
    """Partition the non-zero balances into the largest number of zero-sum groups (exact,
    by dynamic programming over subsets); n - groups is the minimum number of transfers."""
    n = len(names)
    full = (1 << n) - 1
    sums = [0] * (1 << n)
    for mask in range(1, 1 << n):
        low = mask & -mask
        sums[mask] = sums[mask ^ low] + balances[low.bit_length() - 1]
    best = [0] * (1 << n)
    for mask in range(1, 1 << n):
        top = 0
        m = mask
        while m:
            low = m & -m
            value = best[mask ^ low]
            if value > top:
                top = value
            m ^= low
        best[mask] = top + (1 if sums[mask] == 0 else 0)
    groups, mask, current = [], full, []
    while mask:
        need = best[mask] - (1 if sums[mask] == 0 else 0)
        if sums[mask] == 0 and current:
            groups.append(current)
            current = []
        m = mask
        while m:
            low = m & -m
            if best[mask ^ low] == need:
                break
            m ^= low
        current.append(names[low.bit_length() - 1])
        mask ^= low
    if current:
        groups.append(current)
    return groups


def greedy(group, balance, rank):
    """Largest creditor receives from largest debtor until all are even."""
    bal = {n: balance[n] for n in group}
    transfers = []
    while True:
        creditors = sorted((n for n in bal if bal[n] > 0), key=lambda n: (-bal[n], rank[n]))
        debtors = sorted((n for n in bal if bal[n] < 0), key=lambda n: (bal[n], rank[n]))
        if not creditors or not debtors:
            break
        c, d = creditors[0], debtors[0]
        x = min(bal[c], -bal[d])
        transfers.append((d, c, x))
        bal[c] -= x
        bal[d] += x
    return transfers


def main():
    global fixed_people
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"expenses", "participants", "payments", "decimals", "currency"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    decimals = request.get("decimals", 2)
    if not isinstance(decimals, int) or isinstance(decimals, bool) or not 0 <= decimals <= 6:
        fail("decimals must be an integer from 0 to 6")
    unit = Decimal(1).scaleb(-decimals)
    show = lambda units: str((Decimal(units) * unit).quantize(unit))
    people = []
    if request.get("participants") is not None:
        given = request["participants"]
        if not isinstance(given, list) or not given or not all(isinstance(p, str) and p.strip() for p in given):
            fail("participants must be a non-empty list of names")
        if len(set(given)) != len(given):
            fail("participants has a repeated name")
        people = list(given)
        fixed_people = True
    expenses = request.get("expenses")
    if not isinstance(expenses, list) or not expenses:
        fail("expenses must be a non-empty list")
    if len(expenses) > 5000:
        fail("at most 5000 expenses")
    # first pass: collect names in order of appearance
    for i, e in enumerate(expenses):
        if not isinstance(e, dict):
            fail(f"expenses[{i}] must be an object")
        bad = sorted(set(e) - {"description", "paid_by", "amount", "among", "weights", "shares"})
        if bad:
            fail(f"expenses[{i}] has unknown field(s): {', '.join(bad)}")
        name_of(e.get("paid_by"), f"expenses[{i}].paid_by", people)
        for key in ("among",):
            if e.get(key) is not None:
                if not isinstance(e[key], list) or not e[key]:
                    fail(f"expenses[{i}].among must be a non-empty list of names")
                for j, n in enumerate(e[key]):
                    name_of(n, f"expenses[{i}].among[{j}]", people)
        for key in ("weights", "shares"):
            if e.get(key) is not None:
                if not isinstance(e[key], dict) or not e[key]:
                    fail(f"expenses[{i}].{key} must be a non-empty object of name: value")
                for n in e[key]:
                    name_of(n, f"expenses[{i}].{key}", people)
    if len(people) > 200:
        fail("at most 200 participants")
    rank = {n: i for i, n in enumerate(people)}
    paid = {n: 0 for n in people}
    owed = {n: 0 for n in people}
    lines = []
    for i, e in enumerate(expenses):
        field = f"expenses[{i}]"
        total = amount(e.get("amount"), f"{field}.amount", unit)
        payer = e["paid_by"]
        if e.get("weights") is not None and e.get("shares") is not None:
            fail(f"{field}: give weights or shares, not both")
        if e.get("shares") is not None:
            shares = {n: amount(v, f"{field}.shares.{n}", unit, allow_zero=True) for n, v in e["shares"].items()}
            if e.get("among") is not None and set(e["among"]) != set(shares):
                fail(f"{field}: among and shares name different people")
            if sum(shares.values()) != total:
                fail(f"{field}: shares add up to {show(sum(shares.values()))}, not {show(total)}")
            method = "exact shares"
        else:
            among = e.get("among")
            if among is None:
                among = list(e["weights"]) if e.get("weights") is not None else list(people)
            if len(set(among)) != len(among):
                fail(f"{field}.among repeats a name")
            if e.get("weights") is not None:
                w = e["weights"]
                missing = [n for n in among if n not in w]
                if missing or set(w) - set(among):
                    fail(f"{field}: weights must name exactly the people in among")
                weights = {}
                for n in among:
                    value = w[n]
                    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
                        fail(f"{field}.weights.{n} must be a number")
                    try:
                        weights[n] = Fraction(Decimal(str(value)))
                    except (InvalidOperation, ValueError):
                        fail(f"{field}.weights.{n} must be a number")
                    if weights[n] < 0:
                        fail(f"{field}.weights.{n} must not be negative")
                if sum(weights.values()) == 0:
                    fail(f"{field}: weights add up to zero")
                method = "weights"
            else:
                weights = {n: Fraction(1) for n in among}
                method = "equal"
            shares = split(total, weights)
        paid[payer] += total
        for n, s in shares.items():
            owed[n] += s
        lines.append({"description": e.get("description", ""), "paid_by": payer, "amount": show(total), "method": method,
                      "shares": {n: show(s) for n, s in shares.items()}})

    settled = []
    payments = request.get("payments") or []
    if not isinstance(payments, list):
        fail("payments must be a list")
    for i, p in enumerate(payments):
        if not isinstance(p, dict) or set(p) - {"from", "to", "amount"}:
            fail(f"payments[{i}] must be an object with from, to and amount")
        a = name_of(p.get("from"), f"payments[{i}].from", people)
        b = name_of(p.get("to"), f"payments[{i}].to", people)
        if a == b:
            fail(f"payments[{i}]: from and to are the same person")
        x = amount(p.get("amount"), f"payments[{i}].amount", unit)
        paid.setdefault(a, 0)
        owed.setdefault(a, 0)
        paid.setdefault(b, 0)
        owed.setdefault(b, 0)
        rank.setdefault(a, len(rank))
        rank.setdefault(b, len(rank))
        settled.append((a, b, x))
    balance = {n: paid[n] - owed[n] for n in people}
    for a, b, x in settled:
        balance[a] += x
        balance[b] -= x
    assert sum(balance.values()) == 0

    nonzero = [n for n in people if balance[n] != 0]
    if len(nonzero) <= MAX_EXACT:
        groups = zero_sum_groups(nonzero, [balance[n] for n in nonzero]) if nonzero else []
        method = "minimum transfers: the balances split into the most zero-sum groups (exact search), each settled largest creditor with largest debtor"
    else:
        groups = [nonzero]
        method = f"greedy largest creditor with largest debtor (more than {MAX_EXACT} open balances: at most n-1 transfers, not always the minimum)"
    transfers = []
    for group in groups:
        for d, c, x in greedy(sorted(group, key=lambda n: rank[n]), balance, rank):
            transfers.append({"from": d, "to": c, "amount": show(x)})
    result = {
        "currency": request.get("currency", ""),
        "total_spent": show(sum(paid[n] for n in people)),
        "people": [{"name": n, "paid": show(paid[n]), "share": show(owed[n]), "balance": show(balance[n])} for n in people],
        "balances": {n: show(balance[n]) for n in people},
        "transfers": transfers,
        "transfer_count": len(transfers),
        "method": method,
        "expenses": lines,
        "rounding": "shares in whole minor units; leftover units to the largest remainders, ties to the earlier name in among",
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
