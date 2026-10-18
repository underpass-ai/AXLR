#!/usr/bin/env python3
"""Convert an amount between currencies with the ECB euro foreign exchange reference rates.

Input (JSON on stdin):
  amount           number or decimal string
  from, to         ISO 4217 codes (EUR or one of the ECB's ~30 quoted currencies)
  date             optional "YYYY-MM-DD" in the last 90 days (uses eurofxref-hist-90d.xml);
                   without it the latest daily file (eurofxref-daily.xml) is used
  use_previous     when date has no rates (weekend, TARGET holiday), use the latest earlier day
  decimals         decimals of the converted amount (default 2, 0..10), half up
  list_currencies  only list the currencies and the rate date
  response         optional: the exact XML the ECB returned (offline or cached use);
                   when given, no request is made
Cross rates go through the euro: amount * (to per EUR) / (from per EUR), in Decimal.
"""
import datetime as dt
import json
import re
import sys
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, localcontext

DAILY = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
HIST_90D = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml"
UA = "AXLR-catalog-tool/1.0 (+https://axlr.underpassai.com)"
TIMEOUT = 15
NS = "{http://www.ecb.int/vocabulary/2002-08-01/eurofxref}"
NOTE = ("ECB euro reference rates are published for information purposes only; using them for "
        "transaction purposes is strongly discouraged. They are set around 16:00 CET on TARGET business days.")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def decimal_of(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        number = Decimal(str(value).strip())
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    return number


def currency(value, field):
    if not isinstance(value, str) or not re.match(r"^[A-Za-z]{3}$", value.strip()):
        fail(f"{field} must be a three-letter ISO 4217 code")
    return value.strip().upper()


def fetch(url):
    if not url.startswith("https://"):
        fail("only HTTPS is allowed")
    request = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "application/xml"})
    try:
        with urllib.request.urlopen(request, timeout=TIMEOUT) as reply:
            return reply.read().decode("utf-8")
    except urllib.error.HTTPError as error:
        fail(f"ECB answered HTTP {error.code}")
    except (urllib.error.URLError, OSError, ValueError) as error:
        fail(f"request to the ECB failed: {error}")


def parse(text):
    """Every <Cube time="..."> of a daily or historical file: {date: {currency: Decimal}}."""
    try:
        root = ET.fromstring(text.encode("utf-8"))
    except ET.ParseError as error:
        fail(f"response is not ECB eurofxref XML: {error}")
    days = {}
    for cube in root.iter(NS + "Cube"):
        time = cube.get("time")
        if time is None:
            continue
        try:
            day = dt.date.fromisoformat(time)
        except ValueError:
            fail(f"response has a bad date: {time!r}")
        rates = {}
        for child in cube.findall(NS + "Cube"):
            code, rate = child.get("currency"), child.get("rate")
            if code and rate:
                try:
                    rates[code] = Decimal(rate)
                except InvalidOperation:
                    fail(f"response has a bad rate for {code}: {rate!r}")
        days[day] = rates
    if not days:
        fail("response has no ECB reference rates (no <Cube time=...>)")
    return days


def significant(value, digits=10):
    if value == 0:
        return "0"
    rounded = value.quantize(Decimal(1).scaleb(value.adjusted() - digits + 1), rounding=ROUND_HALF_UP)
    return format(rounded.normalize(), "f")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    try:
        json.dumps(request, ensure_ascii=False).encode("utf-8")
    except UnicodeEncodeError:
        fail("input contains an unpaired surrogate (not valid Unicode text)")
    known = {"amount", "from", "to", "date", "use_previous", "decimals", "list_currencies", "response"}
    unknown = set(request) - known
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    listing = request.get("list_currencies", False)
    if not isinstance(listing, bool):
        fail("list_currencies must be true or false")
    date = request.get("date")
    if date is not None:
        try:
            date = dt.date.fromisoformat(date)
        except (TypeError, ValueError):
            fail("date must be YYYY-MM-DD")
    use_previous = request.get("use_previous", False)
    if not isinstance(use_previous, bool):
        fail("use_previous must be true or false")
    decimals = request.get("decimals", 2)
    if isinstance(decimals, bool) or not isinstance(decimals, int) or not 0 <= decimals <= 10:
        fail("decimals must be an integer from 0 to 10")
    if not listing:
        amount = decimal_of(request.get("amount"), "amount")
        source_code = currency(request.get("from"), "from")
        target_code = currency(request.get("to"), "to")
    url = HIST_90D if date else DAILY
    response = request.get("response")
    if response is not None:
        if not isinstance(response, str) or not response.strip():
            fail("response must be the non-empty XML text the ECB returned")
        text, fetched = response, False
    else:
        text, fetched = fetch(url), True
    days = parse(text)
    available = sorted(days)
    if date is None:
        rate_date = available[-1]
    elif date in days:
        rate_date = date
    else:
        earlier = [d for d in available if d < date]
        if date < available[0] or date > available[-1] or not use_previous or not earlier:
            fail(f"no ECB rates for {date.isoformat()}; the data covers {available[0].isoformat()} to "
                 f"{available[-1].isoformat()} on TARGET business days (use_previous takes the latest earlier day; "
                 "older dates need the ECB's full history, which this tool does not read)")
        rate_date = earlier[-1]
    rates = dict(days[rate_date], EUR=Decimal(1))
    source = {"publisher": "European Central Bank", "url": url, "fetched": fetched}
    if listing:
        print(json.dumps({"rate_date": rate_date.isoformat(), "currencies": sorted(rates),
                          "per_eur": {k: str(v) for k, v in sorted(rates.items())}, "source": source, "note": NOTE}))
        return
    for code in (source_code, target_code):
        if code not in rates:
            fail(f"the ECB publishes no reference rate for {code} on {rate_date.isoformat()}; "
                 f"available: {', '.join(sorted(rates))}")
    with localcontext() as context:
        context.prec = 40
        exact = amount * rates[target_code] / rates[source_code]
        rate = rates[target_code] / rates[source_code]
        inverse = rates[source_code] / rates[target_code]
        converted = exact.quantize(Decimal(1).scaleb(-decimals), rounding=ROUND_HALF_UP)
    result = {
        "amount": str(amount), "from": source_code, "to": target_code,
        "converted": str(converted),
        "rate": significant(rate), "inverse_rate": significant(inverse),
        "rate_date": rate_date.isoformat(),
        "per_eur": {source_code: str(rates[source_code]), target_code: str(rates[target_code])},
        "method": "direct ECB rate" if "EUR" in (source_code, target_code) else "cross rate via EUR",
        "source": source, "note": NOTE,
    }
    if date is not None:
        result["requested_date"] = date.isoformat()
    print(json.dumps(result))


if __name__ == "__main__":
    main()
