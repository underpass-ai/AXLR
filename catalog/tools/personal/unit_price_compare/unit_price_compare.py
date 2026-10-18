#!/usr/bin/env python3
"""Unit-price comparison: the price per unit of several offers with different package sizes and
units (g, kg, oz, lb; ml, l, US and imperial fl oz; items), multipacks and deals ("3 for 2",
"2 for 5.00", "buy 2 get 1 free", percent or amount off), the cheapest offer per unit and what
each other offer costs extra. Decimal arithmetic; unit factors from the bundled units.json.

Input (JSON on stdin):
  offers     [{"name", "price", "size", "unit", "count"?, "deal"?, "discount_pct"?, "discount_amount"?}]
             price   price of ONE pack as sold (number or decimal string)
             size    quantity in one item of the pack; unit its unit
             count   items in the pack (multipack), default 1
             deal    "N for M" (pay for M of N packs when M < N is a whole number; otherwise N packs
                     for a total price M), "buy N get K free", "bogo", or {"buy": N, "pay_for": M}
                     or {"buy": N, "total_price": P}
             discount_pct / discount_amount   off the price of each pack, applied before the deal
  per        unit to express prices in, with an optional number: "kg", "100 g", "l", "fl_oz"
             (default kg, l or item, by the family of the offers)
  places     decimal places of the unit prices (default 4)
  currency   label copied to the output
  list_units true: list the bundled units
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

getcontext().prec = 50


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load_table():
    try:
        with open("units.json", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, ValueError) as error:
        fail(f"cannot read the bundled units.json: {error}")


def dec(value, field, positive=False, allow_zero=True):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    text = str(value).strip().replace(",", "") if isinstance(value, str) else str(value)
    text = text.lstrip("$€£¥")
    try:
        number = Decimal(text)
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    if number < 0 or (positive and number == 0) or (not allow_zero and number == 0):
        fail(f"{field} must be {'positive' if positive else 'zero or more'}")
    return number


def lookup(units, name, field):
    if not isinstance(name, str) or not name.strip():
        fail(f"{field} must be a unit name")
    key = name.strip()
    if key in units:
        return key
    for symbol, info in units.items():
        if key in info["aliases"] or key.lower() == symbol.lower() or key.lower() in [a.lower() for a in info["aliases"]]:
            return symbol
    fail(f"{field}: unknown unit {name!r}; known: {', '.join(units)}")


def parse_deal(deal, field):
    """Return (packs bought, packs paid for or None, total price or None, label)."""
    if deal is None:
        return 1, 1, None, None
    if isinstance(deal, dict):
        extra = set(deal) - {"buy", "pay_for", "total_price"}
        if extra:
            fail(f"{field}: unknown keys {sorted(extra)}")
        buy = deal.get("buy")
        if isinstance(buy, bool) or not isinstance(buy, int) or buy < 1:
            fail(f"{field}.buy must be a positive integer")
        if "pay_for" in deal and "total_price" not in deal:
            pay = deal["pay_for"]
            if isinstance(pay, bool) or not isinstance(pay, int) or not 0 < pay <= buy:
                fail(f"{field}.pay_for must be an integer from 1 to buy")
            return buy, pay, None, f"{buy} for the price of {pay}"
        if "total_price" in deal and "pay_for" not in deal:
            total = dec(deal["total_price"], f"{field}.total_price", positive=True)
            return buy, None, total, f"{buy} for {total}"
        fail(f"{field} needs pay_for or total_price (one of them)")
    if not isinstance(deal, str):
        fail(f"{field} must be a string or an object")
    text = deal.strip().lower()
    if text in ("bogo", "bogof", "buy one get one free", "buy 1 get 1 free", "2 for 1"):
        return 2, 1, None, "2 for the price of 1"
    match = re.fullmatch(r"buy\s+(\d+)\s*,?\s*get\s+(\d+)\s+free", text)
    if match:
        buy, free = int(match.group(1)), int(match.group(2))
        if buy < 1 or free < 1:
            fail(f"{field}: the numbers must be positive")
        return buy + free, buy, None, f"{buy + free} for the price of {buy}"
    match = re.fullmatch(r"(\d+)\s*(?:for|x)\s*([$€£¥]?)\s*(\d+(?:\.\d+)?)", text)
    if match:
        buy, symbol, amount = int(match.group(1)), match.group(2), match.group(3)
        if buy < 1:
            fail(f"{field}: the number of packs must be positive")
        if not symbol and "." not in amount and 0 < int(amount) < buy:
            return buy, int(amount), None, f"{buy} for the price of {int(amount)}"
        total = dec(amount, field, positive=True)
        return buy, None, total, f"{buy} for {total}"
    fail(f"{field}: cannot read deal {deal!r} (use \"3 for 2\", \"2 for 5.00\", \"buy 2 get 1 free\", \"bogo\" or an object)")


def parse_per(text, units, family_unit):
    if text is None:
        return Decimal(1), family_unit
    if not isinstance(text, str):
        fail("per must be a string such as \"kg\" or \"100 g\"")
    match = re.fullmatch(r"\s*(\d+(?:\.\d+)?)?\s*(.+?)\s*", text)
    amount = Decimal(match.group(1)) if match.group(1) else Decimal(1)
    if amount <= 0:
        fail("per: the amount must be positive")
    return amount, lookup(units, match.group(2), "per")


def fmt(x, places):
    q = Decimal(1).scaleb(-places)
    return format(x.quantize(q, ROUND_HALF_UP), "f")


def plain(x):
    s = format(x.normalize(), "f")
    return s


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"offers", "per", "places", "currency", "list_units"}
    extra = set(request) - allowed
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    table = load_table()
    units = table["units"]
    if request.get("list_units"):
        print(json.dumps({"units": {k: {"family": v["family"], "factor_to_base": v["factor"], "base": table["base_units"][v["family"]],
                                        "aliases": v["aliases"], "definition": v["definition"], "source": v["source"]}
                                    for k, v in units.items()},
                          "table": {"generated": table["generated"], "source": table["source"]}}))
        return
    offers = request.get("offers")
    if not isinstance(offers, list) or len(offers) < 1:
        fail("offers must be a non-empty list")
    if len(offers) > 200:
        fail("at most 200 offers")
    places = request.get("places", 4)
    if isinstance(places, bool) or not isinstance(places, int) or not 0 <= places <= 12:
        fail("places must be an integer from 0 to 12")
    currency = request.get("currency")
    if currency is not None and not isinstance(currency, str):
        fail("currency must be a string")

    rows, families = [], set()
    for i, offer in enumerate(offers):
        field = f"offers[{i}]"
        if not isinstance(offer, dict):
            fail(f"{field} must be an object")
        extra = set(offer) - {"name", "price", "size", "unit", "count", "deal", "discount_pct", "discount_amount"}
        if extra:
            fail(f"{field}: unknown keys {sorted(extra)}")
        name = offer.get("name", f"offer {i + 1}")
        if not isinstance(name, str):
            fail(f"{field}.name must be a string")
        if "price" not in offer or "size" not in offer or "unit" not in offer:
            fail(f"{field} needs price, size and unit")
        price = dec(offer["price"], f"{field}.price")
        size = dec(offer["size"], f"{field}.size", positive=True)
        unit = lookup(units, offer["unit"], f"{field}.unit")
        count = offer.get("count", 1)
        if isinstance(count, bool) or not isinstance(count, int) or count < 1:
            fail(f"{field}.count must be a positive integer")
        pack_price = price
        notes = []
        if "discount_pct" in offer:
            pct = dec(offer["discount_pct"], f"{field}.discount_pct")
            if pct > 100:
                fail(f"{field}.discount_pct must be at most 100")
            pack_price = pack_price * (100 - pct) / 100
            notes.append(f"{plain(pct)} % off")
        if "discount_amount" in offer:
            amount = dec(offer["discount_amount"], f"{field}.discount_amount")
            if amount > pack_price:
                fail(f"{field}.discount_amount is more than the price")
            pack_price -= amount
            notes.append(f"{plain(amount)} off")
        buy, pay_for, total_price, label = parse_deal(offer.get("deal"), f"{field}.deal")
        if total_price is not None:
            spend = total_price
        else:
            spend = pack_price * pay_for
        if label:
            notes.append(label)
        info = units[unit]
        families.add(info["family"])
        quantity_base = size * Decimal(info["factor"]) * count * buy
        rows.append({"name": name, "unit": unit, "family": info["family"], "spend": spend, "packs": buy,
                     "quantity_base": quantity_base, "notes": notes, "pack_price": pack_price, "count": count, "size": size})
    if len(families) > 1:
        fail(f"offers mix {' and '.join(sorted(families))}; only offers measured the same way can be compared")
    family = families.pop()
    default_per = {"mass": "kg", "volume": "l", "count": "count"}[family]
    per_amount, per_unit = parse_per(request.get("per"), units, default_per)
    if units[per_unit]["family"] != family:
        fail(f"per is a {units[per_unit]['family']} unit but the offers are {family}")
    per_base = per_amount * Decimal(units[per_unit]["factor"])
    per_label = (plain(per_amount) + " " if per_amount != 1 else "") + per_unit

    for row in rows:
        row["unit_price"] = row["spend"] / row["quantity_base"] * per_base
    best = min(rows, key=lambda r: r["unit_price"])
    best_price = best["unit_price"]
    out_rows = []
    for rank, row in enumerate(sorted(rows, key=lambda r: r["unit_price"]), 1):
        extra_pct = (row["unit_price"] - best_price) / best_price * 100 if best_price else Decimal(0)
        entry = {
            "rank": rank, "name": row["name"],
            "packs_to_buy": row["packs"], "pay": fmt(row["spend"], 2),
            "quantity": plain(row["quantity_base"] / per_base * per_amount) + " " + per_unit,
            "quantity_base": plain(row["quantity_base"]) + " " + table["base_units"][family],
            "unit_price": fmt(row["unit_price"], places),
            "extra_vs_best_pct": fmt(extra_pct, 2),
            "extra_per_unit_vs_best": fmt(row["unit_price"] - best_price, places),
        }
        if row["notes"]:
            entry["deal"] = "; ".join(row["notes"])
        if row is not best:
            # cost of the best offer's quantity bought at this offer's unit price, minus the best's spend
            entry["savings_choosing_best_for_same_quantity"] = fmt(row["unit_price"] / per_base * best["quantity_base"] - best["spend"], 2)
        out_rows.append(entry)
    result = {"family": family, "per": per_label, "best": best["name"], "best_unit_price": fmt(best_price, places),
              "ranking": out_rows,
              "table": {"generated": table["generated"], "source": table["source"]}}
    ties = [r["name"] for r in rows if r["unit_price"] == best_price and r is not best]
    if ties:
        result["tied_with_best"] = ties
    if currency:
        result["currency"] = currency
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
