#!/usr/bin/env python3
"""Monthly cash-flow projection from recurring items: each month's inflows, outflows, net flow
and closing balance from an opening balance, the first month the balance falls below zero and
the minimum balance. Items recur monthly, quarterly, yearly or once between a start and an end
month and may grow by a percentage a year. Decimal arithmetic, each occurrence in cents.

Input (JSON on stdin):
  start            first month of the forecast, "YYYY-MM"
  months           number of months, 1-600
  opening_balance  default 0
  items            [{"name": "rent", "amount": -1200, "frequency": "monthly",
                     "start": "2026-01", "end": "2027-12", "growth_pct": 3, "day": 5}]
                   amount: positive inflow, negative outflow; frequency monthly | quarterly |
                   yearly | once; start defaults to the forecast start; end (inclusive) is
                   optional; growth_pct compounds once per full year since the item's start;
                   day is accepted and ignored (the forecast is monthly)
  detail           true: add each month's amount per item
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

getcontext().prec = 40
CENT = Decimal("0.01")
STEP = {"monthly": 1, "quarterly": 3, "yearly": 12}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    text = str(value).strip()
    try:
        number = Decimal(text[:-1]) if text.endswith("%") else Decimal(text)
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    return number


def month_index(value, field):
    if not isinstance(value, str) or not re.fullmatch(r"\d{4}-\d{2}", value.strip()):
        fail(f"{field} must be a month YYYY-MM")
    value = value.strip()
    year, month = int(value[:4]), int(value[5:7])
    if not 1 <= month <= 12:
        fail(f"{field} must be a month YYYY-MM")
    return year * 12 + month - 1


def label(index):
    return f"{index // 12:04d}-{index % 12 + 1:02d}"


def money(x):
    return format(x.quantize(CENT, ROUND_HALF_UP), "f")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"start", "months", "opening_balance", "items", "detail"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field: {extra[0]}")
    first = month_index(request.get("start"), "start")
    count = request.get("months")
    if isinstance(count, bool) or not isinstance(count, int) or not 1 <= count <= 600:
        fail("months must be an integer from 1 to 600")
    opening = dec(request.get("opening_balance", 0), "opening_balance").quantize(CENT, ROUND_HALF_UP)
    detail = request.get("detail", False)
    if not isinstance(detail, bool):
        fail("detail must be true or false")
    raw = request.get("items")
    if not isinstance(raw, list) or not raw:
        fail("items must be a non-empty list")
    items = []
    names = set()
    for i, item in enumerate(raw):
        where = f"items[{i}]"
        if not isinstance(item, dict):
            fail(f"{where} must be an object")
        unknown = sorted(set(item) - {"name", "amount", "frequency", "start", "end", "growth_pct", "day"})
        if unknown:
            fail(f"{where} has an unknown field: {unknown[0]}")
        if "amount" not in item:
            fail(f"{where}.amount is required")
        name = item.get("name", f"item {i + 1}")
        if not isinstance(name, str) or not name:
            fail(f"{where}.name must be a non-empty string")
        if name in names:
            fail(f"{where}.name {name!r} is repeated")
        names.add(name)
        frequency = item.get("frequency", "monthly")
        if frequency not in ("monthly", "quarterly", "yearly", "once"):
            fail(f"{where}.frequency must be monthly, quarterly, yearly or once")
        begin = month_index(item["start"], f"{where}.start") if "start" in item else first
        end = month_index(item["end"], f"{where}.end") if "end" in item else None
        if end is not None and end < begin:
            fail(f"{where}.end is before its start")
        growth = dec(item.get("growth_pct", 0), f"{where}.growth_pct")
        if growth <= -100:
            fail(f"{where}.growth_pct must be above -100")
        day = item.get("day")
        if day is not None and (isinstance(day, bool) or not isinstance(day, int) or not 1 <= day <= 31):
            fail(f"{where}.day must be an integer from 1 to 31 (it is ignored)")
        items.append({"name": name, "amount": dec(item["amount"], f"{where}.amount"), "frequency": frequency,
                      "begin": begin, "end": end, "growth": 1 + growth / 100})
    balance = opening
    rows, total_in, total_out = [], Decimal(0), Decimal(0)
    minimum, minimum_month, first_negative, negative_months = None, None, None, 0
    for m in range(first, first + count):
        inflow = outflow = Decimal(0)
        parts = {}
        for item in items:
            offset = m - item["begin"]
            if offset < 0 or (item["end"] is not None and m > item["end"]):
                continue
            if item["frequency"] == "once":
                if offset != 0:
                    continue
            elif offset % STEP[item["frequency"]]:
                continue
            value = (item["amount"] * item["growth"] ** (offset // 12)).quantize(CENT, ROUND_HALF_UP)
            if value >= 0:
                inflow += value
            else:
                outflow -= value
            parts[item["name"]] = money(value)
        net = inflow - outflow
        opening_month = balance
        balance += net
        total_in += inflow
        total_out += outflow
        if minimum is None or balance < minimum:
            minimum, minimum_month = balance, label(m)
        if balance < 0:
            negative_months += 1
            if first_negative is None:
                first_negative = label(m)
        row = {"month": label(m), "opening_balance": money(opening_month), "inflows": money(inflow),
               "outflows": money(outflow), "net": money(net), "closing_balance": money(balance)}
        if detail:
            row["items"] = parts
        rows.append(row)
    result = {
        "start": label(first), "end": label(first + count - 1), "months": count, "opening_balance": money(opening),
        "closing_balance": money(balance),
        "totals": {"inflows": money(total_in), "outflows": money(total_out), "net": money(total_in - total_out)},
        "minimum_balance": {"amount": money(minimum), "month": minimum_month},
        "first_month_below_zero": first_negative, "months_below_zero": negative_months,
        "schedule": rows,
    }
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except ArithmeticError:
        fail("the numbers are out of the range this tool can compute")
