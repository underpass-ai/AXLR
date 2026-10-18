#!/usr/bin/env python3
"""Public holidays of a country and year from the bundled table holidays.json (computed from
each holiday's legal rule by tools/catalog/data/holiday_table.py, with its source and date).

Input (JSON on stdin):
  country          "ES" | "PT" | "FR" | "DE" | "IT" | "GB-ENG" | "US"
  year             integer within the table's years
  region           optional ISO 3166-2 code (e.g. "FR-67"): keeps regional rows of that region
                   and the regional rows whose regions vary by year
  include_regional optional, default true; false keeps national rows only
  list_countries   true: only list the bundled countries and years

A row is listed for a year when its legal date or its observed (substitute) date falls in it,
so a US New Year's Day observed on 31 December appears in both years.
"""
import datetime as dt
import json
import pathlib
import sys

WEEKDAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]
NOTE = ("Regional and local holidays, and one-off changes made after the table's date (a new law, a royal "
        "proclamation, a moved holiday), are not all included and must be added from the official calendar.")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load():
    return json.loads((pathlib.Path(__file__).parent / "holidays.json").read_text(encoding="utf-8"))


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"country", "year", "region", "include_regional", "list_countries"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    table = load()
    first, last = table["years"]
    countries = table["countries"]
    if request.get("list_countries") is True:
        print(json.dumps({"countries": {code: c["name"] for code, c in countries.items()}, "years": [first, last],
                          "generated": table["generated"], "generator": table["generator"]}, ensure_ascii=False))
        return
    country = request.get("country")
    if not isinstance(country, str) or country.upper() not in countries:
        fail(f"country must be one of {', '.join(countries)}")
    country = country.upper()
    year = request.get("year")
    if not isinstance(year, int) or isinstance(year, bool):
        fail("year must be an integer")
    if not first <= year <= last:
        fail(f"year must be between {first} and {last} (the table's range)")
    region = request.get("region")
    if region is not None:
        if not isinstance(region, str) or not region.strip():
            fail("region must be an ISO 3166-2 code such as FR-67")
        region = region.strip().upper()
        if not region.startswith(country.split("-")[0] + "-"):
            fail(f"region {region} is not a subdivision of {country}")
    include_regional = request.get("include_regional", True)
    if not isinstance(include_regional, bool):
        fail("include_regional must be true or false")

    data = countries[country]
    rows = []
    for y in (year - 1, year, year + 1):
        for date, observed, rule_id in data["years"].get(str(y), []):
            legal = dt.date.fromisoformat(date)
            moved = dt.date.fromisoformat(observed) if observed else None
            if legal.year != year and (moved is None or moved.year != year):
                continue
            rule = data["rules"][rule_id]
            if rule["scope"] == "regional":
                if not include_regional:
                    continue
                if region is not None and rule["regions"] is not None and region not in rule["regions"]:
                    continue
            effective = moved or legal
            row = {"date": date, "weekday": WEEKDAYS[legal.weekday()], "observed": observed,
                   "day_off": effective.isoformat(), "day_off_weekday": WEEKDAYS[effective.weekday()],
                   "name": rule["name"], "name_en": rule["name_en"], "scope": rule["scope"], "rule": rule_id}
            if rule["scope"] == "regional":
                row["regions"] = rule["regions"]
                if rule["regions"] is None:
                    row["regions_note"] = "regions vary by year: check the official regional calendar"
            row["source"] = rule["source"]
            rows.append(row)
    rows.sort(key=lambda r: (r["day_off"], r["date"]))
    result = {
        "country": country, "country_name": data["name"], "year": year, "region": region,
        "count": len(rows),
        "holidays": rows,
        "days_off": [r["day_off"] for r in rows if r["day_off"][:4] == str(year)],
        "by_date": {r["day_off"]: r["name_en"] for r in rows},
        "observed_rule": data["observed_rule"],
        "country_note": data["note"],
        "note": NOTE,
        "table": {"generated": table["generated"], "generator": table["generator"], "method": table["method"]},
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
