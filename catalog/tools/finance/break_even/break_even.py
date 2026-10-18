#!/usr/bin/env python3
"""Cost-volume-profit analysis in Decimal: break-even units and revenue, contribution margin
and ratio, margin of safety, operating income and degree of operating leverage at a sales
level, the units for a target profit (before or after tax), a sales-mix break-even for several
products, and a sensitivity table of the break-even over price and variable-cost changes.

Input (JSON on stdin):
  fixed_costs     total fixed costs of the period
  price           unit selling price                   one product
  variable_cost   unit variable cost                   one product
  products        [{"name", "price", "variable_cost", "mix"}]   several products instead;
                  mix is a share (any scale) of units or of revenue, see mix_basis
  mix_basis       "units" (default) | "revenue"
  sales_units     expected sales in units (one product)       for margin of safety and leverage
  sales_revenue   expected sales revenue (either case)
  target_profit   operating profit wanted
  tax_rate        with target_profit as an after-tax figure: 0.25 or "25%"
  sensitivity     {"price_changes_pct": [-10, 0, 10], "variable_cost_changes_pct": [0, 5],
                   "fixed_cost_changes_pct": [0]}   one product
"""
import json
import sys
from decimal import ROUND_CEILING, ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

getcontext().prec = 40


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field, percent=False):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    text = str(value).strip()
    scale = Decimal(1)
    if percent and text.endswith("%"):
        text, scale = text[:-1], Decimal("0.01")
    try:
        number = Decimal(text) * scale
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    return number


def q(x, places=2):
    return format(x.quantize(Decimal(1).scaleb(-places), ROUND_HALF_UP), "f")


def units_out(x):
    return {"exact": q(x, 4), "rounded_up": format(x.to_integral_value(ROUND_CEILING), "f")}


def check_product(price, variable, label):
    if price <= 0:
        fail(f"{label}price must be positive")
    if variable < 0:
        fail(f"{label}variable_cost must not be negative")
    if price <= variable:
        fail(f"{label}price must exceed variable_cost: no contribution margin, no break-even")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"fixed_costs", "price", "variable_cost", "products", "mix_basis", "sales_units", "sales_revenue",
             "target_profit", "tax_rate", "sensitivity"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field: {extra[0]}")
    if "fixed_costs" not in request:
        fail("fixed_costs is required")
    fixed = dec(request["fixed_costs"], "fixed_costs")
    if fixed < 0:
        fail("fixed_costs must not be negative")
    single = "products" not in request
    if single and ("price" not in request or "variable_cost" not in request):
        fail("give price and variable_cost, or products")
    if not single and ("price" in request or "variable_cost" in request):
        fail("give either price and variable_cost or products, not both")
    result = {"fixed_costs": q(fixed)}

    if single:
        price = dec(request["price"], "price")
        variable = dec(request["variable_cost"], "variable_cost")
        check_product(price, variable, "")
        cm = price - variable
        cm_ratio = cm / price
        be_units = fixed / cm
        be_revenue = be_units * price
        result.update({"contribution_margin_per_unit": q(cm), "contribution_margin_ratio": q(cm_ratio, 6),
                       "break_even_units": units_out(be_units), "break_even_revenue": q(be_revenue)})
    else:
        basis = request.get("mix_basis", "units")
        if basis not in ("units", "revenue"):
            fail("mix_basis must be units or revenue")
        raw = request["products"]
        if not isinstance(raw, list) or not raw:
            fail("products must be a non-empty list")
        products = []
        for i, item in enumerate(raw):
            if not isinstance(item, dict) or set(item) - {"name", "price", "variable_cost", "mix"}:
                fail(f"products[{i}] must be an object with name, price, variable_cost and mix")
            for key in ("price", "variable_cost", "mix"):
                if key not in item:
                    fail(f"products[{i}].{key} is required")
            p = dec(item["price"], f"products[{i}].price")
            v = dec(item["variable_cost"], f"products[{i}].variable_cost")
            check_product(p, v, f"products[{i}].")
            mix = dec(item["mix"], f"products[{i}].mix")
            if mix < 0:
                fail(f"products[{i}].mix must not be negative")
            products.append({"name": str(item.get("name", f"product {i + 1}")), "price": p, "variable": v, "mix": mix})
        total_mix = sum(p["mix"] for p in products)
        if total_mix <= 0:
            fail("the mix shares must not all be zero")
        if basis == "units":
            weighted_cm = sum(p["mix"] * (p["price"] - p["variable"]) for p in products) / total_mix
            weighted_price = sum(p["mix"] * p["price"] for p in products) / total_mix
            cm_ratio = weighted_cm / weighted_price
            be_units_total = fixed / weighted_cm
            be_revenue = be_units_total * weighted_price
            rows = [{"name": p["name"], "mix_share": q(p["mix"] / total_mix, 6), "units": units_out(be_units_total * p["mix"] / total_mix),
                     "revenue": q(be_units_total * p["mix"] / total_mix * p["price"])} for p in products]
            result["weighted_contribution_margin_per_unit"] = q(weighted_cm, 4)
            result["break_even_units"] = units_out(be_units_total)
        else:
            cm_ratio = sum(p["mix"] * (p["price"] - p["variable"]) / p["price"] for p in products) / total_mix
            be_revenue = fixed / cm_ratio
            rows = []
            for p in products:
                revenue = be_revenue * p["mix"] / total_mix
                rows.append({"name": p["name"], "mix_share": q(p["mix"] / total_mix, 6), "units": units_out(revenue / p["price"]), "revenue": q(revenue)})
        result.update({"mix_basis": basis, "contribution_margin_ratio": q(cm_ratio, 6), "break_even_revenue": q(be_revenue), "products": rows})

    if "sales_units" in request or "sales_revenue" in request:
        if "sales_units" in request and "sales_revenue" in request:
            fail("give sales_units or sales_revenue, not both")
        if "sales_units" in request:
            if not single:
                fail("with several products give sales_revenue")
            units = dec(request["sales_units"], "sales_units")
            if units < 0:
                fail("sales_units must not be negative")
            revenue = units * price
        else:
            revenue = dec(request["sales_revenue"], "sales_revenue")
            if revenue < 0:
                fail("sales_revenue must not be negative")
        contribution = revenue * cm_ratio
        income = contribution - fixed
        safety = {"revenue": q(revenue), "contribution_margin": q(contribution), "operating_income": q(income),
                  "margin_of_safety": q(revenue - be_revenue),
                  "margin_of_safety_pct": q((revenue - be_revenue) / revenue * 100) if revenue else None}
        if single:
            safety["margin_of_safety_units"] = q((revenue - be_revenue) / price, 4)
        safety["degree_of_operating_leverage"] = q(contribution / income, 4) if income != 0 else None
        if income <= 0:
            safety["note"] = "at or below break-even: operating leverage is not meaningful"
        result["at_sales"] = safety

    if "target_profit" in request:
        target = dec(request["target_profit"], "target_profit")
        before_tax = target
        out = {"target_profit": q(target)}
        if "tax_rate" in request:
            tax = dec(request["tax_rate"], "tax_rate", percent=True)
            if not 0 <= tax < 1:
                fail("tax_rate must be from 0 to below 100%")
            before_tax = target / (1 - tax)
            out["tax_rate"] = format(tax, "f")
            out["profit_before_tax"] = q(before_tax)
        needed_revenue = (fixed + before_tax) / cm_ratio
        out["revenue"] = q(needed_revenue)
        if single:
            out["units"] = units_out((fixed + before_tax) / cm)
        result["target"] = out
    elif "tax_rate" in request:
        fail("tax_rate needs target_profit")

    if "sensitivity" in request:
        if not single:
            fail("sensitivity works on one product (price and variable_cost)")
        grid = request["sensitivity"]
        allowed = {"price_changes_pct", "variable_cost_changes_pct", "fixed_cost_changes_pct"}
        if not isinstance(grid, dict) or set(grid) - allowed:
            fail("sensitivity must be an object with price_changes_pct, variable_cost_changes_pct, fixed_cost_changes_pct")
        axes = {}
        for key in sorted(allowed):
            values = grid.get(key, [0])
            if not isinstance(values, list) or not values or len(values) > 25:
                fail(f"sensitivity.{key} must be a list of 1 to 25 percent changes")
            axes[key] = [dec(v, f"sensitivity.{key}[]") for v in values]
        table = []
        for fc in axes["fixed_cost_changes_pct"]:
            for pc in axes["price_changes_pct"]:
                for vc in axes["variable_cost_changes_pct"]:
                    p = price * (1 + pc / 100)
                    v = variable * (1 + vc / 100)
                    f = fixed * (1 + fc / 100)
                    row = {"price_change_pct": format(pc.normalize(), "f"), "variable_cost_change_pct": format(vc.normalize(), "f"),
                           "fixed_cost_change_pct": format(fc.normalize(), "f"), "price": q(p), "variable_cost": q(v)}
                    if p <= v or p <= 0:
                        row.update({"break_even_units": None, "break_even_revenue": None, "note": "no contribution margin"})
                    else:
                        row.update({"break_even_units": units_out(f / (p - v)), "break_even_revenue": q(f / (p - v) * p)})
                    table.append(row)
        result["sensitivity"] = table
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except ArithmeticError:
        fail("the numbers are out of the range this tool can compute")
