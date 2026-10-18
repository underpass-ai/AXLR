#!/usr/bin/env python3
"""Portfolio statistics from a price (or return) table given inline as CSV: per asset and for a
weighted portfolio rebalanced every period, the total and annualised return, mean and
volatility (annualised), Sharpe ratio over a risk-free rate, maximum drawdown with its peak,
trough and recovery dates; the correlation matrix and each series' beta to a benchmark column.

Input (JSON on stdin):
  csv               "date,AAA,BBB\n2026-01-02,100,50\n..."  first column the date (or date_column)
  kind              "prices" (default) | "returns" (simple returns per period, as fractions)
  return_type       "simple" (default) | "log": the returns the mean, volatility, Sharpe,
                    correlation and beta are computed on
  periods_per_year  default 252 (12 monthly, 52 weekly)
  risk_free_rate    annual, default 0; per period it is rate / periods_per_year
  weights           {"AAA": 0.6, "BBB": 0.4}   optional; normalised to sum 1
  benchmark         a column name to compute betas against (it stays out of the portfolio
                    unless weighted)
  date_column       name of the date column (default the first)
  delimiter         default ","
Rows are taken in the order given (oldest first).
"""
import csv
import io
import json
import math
import sys
from decimal import Decimal, InvalidOperation

PLACES = 10


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    text = str(value).strip()
    scale = 1.0
    if text.endswith("%"):
        text, scale = text[:-1], 0.01
    try:
        number = float(Decimal(text)) * scale
    except (InvalidOperation, ValueError):
        fail(f"{field} must be a number or a decimal string")
    if not math.isfinite(number):
        fail(f"{field} must be finite")
    return number


def r(x):
    return None if x is None else round(x, PLACES)


def mean(xs):
    return sum(xs) / len(xs)


def sample_cov(xs, ys):
    if len(xs) < 2:
        return None
    mx, my = mean(xs), mean(ys)
    return sum((a - mx) * (b - my) for a, b in zip(xs, ys)) / (len(xs) - 1)


def drawdown(wealth, dates):
    """wealth[0] is the starting value; dates[k] labels wealth[k]."""
    peak_i, worst, worst_peak, worst_trough = 0, 0.0, None, None
    for i, value in enumerate(wealth):
        if value > wealth[peak_i]:
            peak_i = i
        dd = value / wealth[peak_i] - 1
        if dd < worst:
            worst, worst_peak, worst_trough = dd, peak_i, i
    if worst_peak is None:
        return {"max_drawdown": 0.0, "peak": None, "trough": None, "recovery": None}
    recovery = next((dates[i] for i in range(worst_trough + 1, len(wealth)) if wealth[i] >= wealth[worst_peak]), None)
    return {"max_drawdown": r(worst), "peak": dates[worst_peak], "trough": dates[worst_trough], "recovery": recovery}


def stats(simple, wealth, dates, log, ppy, rf):
    returns = [math.log1p(x) for x in simple] if log else simple
    n = len(simple)
    total = wealth[-1] / wealth[0] - 1
    out = {"periods": n, "total_return": r(total)}
    out["annualised_return"] = r((1 + total) ** (ppy / n) - 1) if total > -1 else None
    m = mean(returns)
    out["mean_return"] = r(m)
    out["annualised_mean_return"] = r(m * ppy)
    variance = sample_cov(returns, returns)
    sd = math.sqrt(variance) if variance is not None else None
    out["volatility"] = r(sd)
    out["annualised_volatility"] = r(sd * math.sqrt(ppy)) if sd is not None else None
    rf_period = rf / ppy
    out["sharpe_ratio"] = r((m - rf_period) / sd * math.sqrt(ppy)) if sd else None
    out.update(drawdown(wealth, dates))
    return out, returns


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"csv", "kind", "return_type", "periods_per_year", "risk_free_rate", "weights", "benchmark", "date_column", "delimiter"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field: {extra[0]}")
    text = request.get("csv")
    if not isinstance(text, str) or not text.strip():
        fail("csv must be the table as text: a date column and one column per asset")
    kind = request.get("kind", "prices")
    if kind not in ("prices", "returns"):
        fail("kind must be prices or returns")
    return_type = request.get("return_type", "simple")
    if return_type not in ("simple", "log"):
        fail("return_type must be simple or log")
    ppy = request.get("periods_per_year", 252)
    if isinstance(ppy, bool) or not isinstance(ppy, (int, float)) or not 0 < ppy <= 100000:
        fail("periods_per_year must be a positive number")
    rf = num(request.get("risk_free_rate", 0), "risk_free_rate")
    delimiter = request.get("delimiter", ",")
    if not isinstance(delimiter, str) or len(delimiter) != 1:
        fail("delimiter must be one character")
    rows = [row for row in csv.reader(io.StringIO(text.strip()), delimiter=delimiter) if any(c.strip() for c in row)]
    if len(rows) < 2:
        fail("csv needs a header row and data rows")
    header = [h.strip() for h in rows[0]]
    if len(set(header)) != len(header):
        fail("csv column names must be unique")
    date_column = request.get("date_column", header[0])
    if date_column not in header:
        fail(f"date_column {date_column!r} is not a column")
    d = header.index(date_column)
    assets = [h for i, h in enumerate(header) if i != d]
    if not assets:
        fail("csv needs at least one asset column besides the date")
    dates, columns = [], {a: [] for a in assets}
    for line, row in enumerate(rows[1:], 2):
        if len(row) != len(header):
            fail(f"csv line {line} has {len(row)} cells, the header has {len(header)}")
        dates.append(row[d].strip())
        for i, h in enumerate(header):
            if i == d:
                continue
            cell = row[i].strip()
            try:
                value = float(cell)
            except ValueError:
                fail(f"csv line {line}, column {h}: {cell!r} is not a number")
            if not math.isfinite(value):
                fail(f"csv line {line}, column {h}: not a finite number")
            columns[h].append(value)
    simple, wealth = {}, {}
    if kind == "prices":
        if len(dates) < 2:
            fail("prices need at least two rows")
        for a in assets:
            p = columns[a]
            if any(x <= 0 for x in p):
                fail(f"column {a}: prices must be positive")
            simple[a] = [p[k] / p[k - 1] - 1 for k in range(1, len(p))]
            wealth[a] = [x / p[0] for x in p]
        wealth_dates = dates
        return_dates = dates[1:]
    else:
        for a in assets:
            if any(x <= -1 for x in columns[a]):
                fail(f"column {a}: a simple return of -100% or less")
            simple[a] = columns[a]
            w = [1.0]
            for x in simple[a]:
                w.append(w[-1] * (1 + x))
            wealth[a] = w
        wealth_dates = ["start"] + dates
        return_dates = dates
    log = return_type == "log"
    result = {"kind": kind, "return_type": return_type, "periods_per_year": ppy, "risk_free_rate": rf,
              "first_date": wealth_dates[0], "last_date": dates[-1], "assets": {}}
    returns = {}
    for a in assets:
        result["assets"][a], returns[a] = stats(simple[a], wealth[a], wealth_dates, log, ppy, rf)
    if "weights" in request:
        raw = request["weights"]
        if not isinstance(raw, dict) or not raw:
            fail("weights must be an object of column: weight")
        for a in raw:
            if a not in assets:
                fail(f"weights: {a!r} is not an asset column")
        values = {a: num(v, f"weights[{a!r}]") for a, v in raw.items()}
        total_weight = sum(values.values())
        if total_weight <= 0:
            fail("weights must add up to a positive number")
        weights = {a: v / total_weight for a, v in values.items()}
        port = [sum(weights[a] * simple[a][k] for a in weights) for k in range(len(return_dates))]
        if any(x <= -1 for x in port):
            fail("the portfolio loses 100% or more in a period")
        w = [1.0]
        for x in port:
            w.append(w[-1] * (1 + x))
        port_stats, returns["portfolio"] = stats(port, w, wealth_dates, log, ppy, rf)
        port_stats["weights"] = {a: r(v) for a, v in weights.items()}
        port_stats["rebalancing"] = "every period"
        result["portfolio"] = port_stats
    names = list(returns)
    if len(return_dates) >= 2:
        corr = {}
        for a in names:
            corr[a] = {}
            for b in names:
                cov = sample_cov(returns[a], returns[b])
                va, vb = sample_cov(returns[a], returns[a]), sample_cov(returns[b], returns[b])
                corr[a][b] = r(cov / math.sqrt(va * vb)) if va and vb else None
        result["correlation"] = corr
    if "benchmark" in request:
        bench = request["benchmark"]
        if bench not in assets:
            fail(f"benchmark {bench!r} is not an asset column")
        if len(return_dates) < 2:
            fail("beta needs at least two returns")
        vb = sample_cov(returns[bench], returns[bench])
        result["benchmark"] = bench
        result["beta"] = {a: (r(sample_cov(returns[a], returns[bench]) / vb) if vb else None) for a in names}
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except ArithmeticError:
        fail("the numbers are out of the range this tool can compute")
