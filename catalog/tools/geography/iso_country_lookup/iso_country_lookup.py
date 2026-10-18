#!/usr/bin/env python3
"""Look up a country by ISO 3166-1 code (alpha-2, alpha-3 or numeric) or English name, or find
the countries that use a time zone, a currency or a calling code, from the bundled
countries.json (tz database iso3166.tab, zone.tab and zone1970.tab; Unicode CLDR codeMappings,
currencyData and English names; CLDR 33.1 telephoneCodeData), written by
tools/catalog/data/iso_country_lookup.py. Every answer names the table's sources and date.

Input (JSON on stdin), exactly one of:
  code          "ES", "ESP", "724" or 724
  name          "spain", "Türkiye", "turkey", "cote d'ivoire" (case and accents ignored;
                an inexact name returns the candidates that contain it)
  zone          an IANA time zone, "Europe/Madrid"
  currency      an ISO 4217 code, "EUR"
  calling_code  "+34", "34" or 34
  list          true: every country with its codes and English name
"""
import json
import pathlib
import re
import sys
import unicodedata

KEYS = ("code", "name", "zone", "currency", "calling_code", "list")


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def fold(text):
    text = unicodedata.normalize("NFKD", text.replace("\u2019", "'").replace("\u2018", "'"))
    text = "".join(c for c in text if not unicodedata.combining(c)).casefold()
    return re.sub(r"\s+", " ", text).strip()


def short(entry):
    return {"alpha2": entry["alpha2"], "alpha3": entry["alpha3"], "numeric": entry["numeric"], "name": entry["name"]}


def full(entry, currencies):
    out = dict(entry)
    out["currencies"] = [dict({"code": c}, **currencies.get(c, {})) for c in entry["currencies"]]
    return out


def text_field(request, key):
    value = request[key]
    if not isinstance(value, str) or not value.strip():
        fail(f"{key} must be a non-empty string")
    return value.strip()


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - set(KEYS))
    if extra:
        fail(f"unknown field: {extra[0]}")
    given = [k for k in KEYS if k in request and not (k == "list" and request[k] is False)]
    if len(given) != 1:
        fail("give exactly one of code, name, zone, currency, calling_code or list")
    data = json.loads((pathlib.Path(__file__).parent / "countries.json").read_text(encoding="utf-8"))
    countries, currencies = data["countries"], data["currencies"]
    source = {"tz_version": data["tz_version"], "generated": data["generated"], "generator": data["generator"], "datasets": data["sources"],
              "not_included": data["not_included"]}
    key = given[0]
    result = {"query": {key: request[key]}}

    if key == "list":
        if request["list"] is not True:
            fail("list must be true")
        result["count"] = len(countries)
        result["countries"] = [short(e) for e in countries.values()]
    elif key == "code":
        value = request["code"]
        if isinstance(value, int) and not isinstance(value, bool):
            value = str(value)
        if not isinstance(value, str) or not value.strip():
            fail("code must be an alpha-2, alpha-3 or numeric ISO 3166-1 code")
        value = value.strip().upper()
        if re.fullmatch(r"\d{1,3}", value):
            field, value = "numeric", value.zfill(3)
        elif re.fullmatch(r"[A-Z]{2}", value):
            field = "alpha2"
        elif re.fullmatch(r"[A-Z]{3}", value):
            field = "alpha3"
        else:
            fail("code must be an alpha-2 (ES), alpha-3 (ESP) or numeric (724) ISO 3166-1 code")
        found = [e for e in countries.values() if e[field] == value]
        if not found:
            fail(f"no country with {field} code {value} in ISO 3166-1 as listed by the tz database {data['tz_version']}")
        result["match"] = field
        result["country"] = full(found[0], currencies)
    elif key == "name":
        wanted = fold(text_field(request, "name"))
        exact, partial = [], []
        for entry in countries.values():
            names = [entry["name"], entry["tz_name"], *entry["other_names"]]
            folded = [fold(n) for n in names]
            if wanted in folded:
                exact.append(entry)
            elif any(wanted in n for n in folded):
                partial.append(entry)
        if exact:
            result["match"] = "exact"
            result["country"] = full(exact[0], currencies)
        elif len(partial) == 1:
            result["match"] = "partial"
            result["country"] = full(partial[0], currencies)
        elif partial:
            result["match"] = "ambiguous"
            result["candidates"] = [short(e) for e in partial]
        else:
            fail(f"no country name contains {request['name']!r}")
    elif key == "zone":
        wanted = text_field(request, "zone")
        found = []
        for entry in countries.values():
            in_tab = [z for z in entry["zones"] if z.casefold() == wanted.casefold()]
            in_1970 = [z for z in entry["zones_1970"] if z.casefold() == wanted.casefold()]
            if in_tab or in_1970:
                item = short(entry)
                item.update({"in_zone_tab": bool(in_tab), "in_zone1970_tab": bool(in_1970)})
                comment = entry.get("zone_comments", {}).get((in_tab or in_1970)[0])
                if comment:
                    item["zone_comment"] = comment
                found.append(item)
        if not found:
            fail(f"no country lists the time zone {wanted!r} in zone.tab or zone1970.tab (tz {data['tz_version']}); "
                 "names are case-sensitive Area/Location, e.g. Europe/Madrid")
        result["count"] = len(found)
        result["countries"] = found
    elif key == "currency":
        wanted = text_field(request, "currency").upper()
        if not re.fullmatch(r"[A-Z]{3}", wanted):
            fail("currency must be a three-letter ISO 4217 code such as EUR")
        found = [short(e) for e in countries.values() if wanted in e["currencies"]]
        if not found:
            fail(f"no country uses {wanted} as a current tender currency in CLDR currencyData")
        result["currency"] = dict({"code": wanted}, **currencies[wanted])
        result["count"] = len(found)
        result["countries"] = found
    elif key == "calling_code":
        value = request["calling_code"]
        if isinstance(value, int) and not isinstance(value, bool):
            value = str(value)
        if not isinstance(value, str) or not re.fullmatch(r"\s*\+?\s*\d{1,4}\s*", value):
            fail("calling_code must be a country calling code such as \"+34\", \"34\" or 34")
        wanted = "+" + re.sub(r"\D", "", value).lstrip("0")
        found = [short(e) for e in countries.values() if wanted in e["calling_codes"]]
        if not found:
            fail(f"no country has the calling code {wanted} in CLDR 33.1 telephoneCodeData")
        result["calling_code"] = wanted
        result["count"] = len(found)
        result["countries"] = found
    result["source"] = source
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
