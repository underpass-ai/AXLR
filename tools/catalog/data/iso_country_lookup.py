#!/usr/bin/env python3
"""Write catalog/tools/geography/iso_country_lookup/countries.json: the country table the tool
bundles, built once from public-domain and Unicode-licensed files (this generator uses the
network; the tool never does). Fetched on 2026-10-18 from:

  tz database (IANA), release 2026e, public domain ("This file is in the public domain"):
    https://data.iana.org/time-zones/tzdb/iso3166.tab   master list of alpha-2 codes, tz names
    https://data.iana.org/time-zones/tzdb/zone.tab      one row per (country, zone)
    https://data.iana.org/time-zones/tzdb/zone1970.tab  zones since 1970, several countries per
                                                         row; inverted here per country
    https://data.iana.org/time-zones/tzdb/version
  Unicode CLDR 48.2 (tag release-48-2), Unicode License v3:
    common/supplemental/supplementalData.xml  codeMappings/territoryCodes (alpha-3, numeric),
                                              currencyData (ISO 4217 per region, CLDR fraction digits:
                                              these are formatting digits and can differ
                                              from the ISO 4217 minor unit, e.g. IQD),
                                              currencyCodes (ISO 4217 numeric)
    common/main/en.xml                        English territory and currency names
    LICENSE                                   the Unicode License v3 notice, copied into the
                                              table as the licence requires
  Unicode CLDR 33.1 (tag release-33-1), Unicode License (data files):
    common/supplemental/telephoneCodeData.xml  ITU-T E.164 country codes per territory. CLDR
                                               deprecated this file in CLDR 34 (2018); its own
                                               references are the ITU list of 15 Sep 2007 and
                                               wtng.info, last edited in 2017 (Kosovo 383).

The ISO 4217 lists published by SIX (the ISO 4217 maintenance agency) were not used: SIX's
terms of use (https://www.six-group.com/en/services/legal/terms-and-conditions/terms-of-use.html,
read on 2026-10-18) say the site's content is for personal use and "may not be reproduced or
reused in any way or used for commercial purposes". CLDR's currencyData is used instead.
No permissively licensed source of capitals was found, so capitals are not included.

Usage: python3 tools/catalog/data/iso_country_lookup.py [--cache DIR]
  --cache DIR  keep the downloaded files in DIR and reuse them on a rerun
"""
import datetime as dt
import json
import pathlib
import sys
import urllib.request
import xml.etree.ElementTree as ET

TZDB = "https://data.iana.org/time-zones/tzdb/"
CLDR = "https://raw.githubusercontent.com/unicode-org/cldr/release-48-2/"
CLDR33 = "https://raw.githubusercontent.com/unicode-org/cldr/release-33-1/"
FILES = {
    "iso3166.tab": TZDB + "iso3166.tab",
    "zone.tab": TZDB + "zone.tab",
    "zone1970.tab": TZDB + "zone1970.tab",
    "version": TZDB + "version",
    "supplementalData.xml": CLDR + "common/supplemental/supplementalData.xml",
    "en.xml": CLDR + "common/main/en.xml",
    "telephoneCodeData.xml": CLDR33 + "common/supplemental/telephoneCodeData.xml",
    "LICENSE": CLDR + "LICENSE",
}


def fetch(name, cache):
    if cache and (cache / name).exists():
        return (cache / name).read_text(encoding="utf-8")
    request = urllib.request.Request(FILES[name], headers={"User-Agent": "AXLR-catalog data generator"})
    with urllib.request.urlopen(request, timeout=120) as response:
        text = response.read().decode("utf-8")
    if cache:
        (cache / name).write_text(text, encoding="utf-8")
    return text


def rows(text):
    for line in text.splitlines():
        if line and not line.startswith("#"):
            yield line.split("\t")


def main():
    cache = None
    if len(sys.argv) == 3 and sys.argv[1] == "--cache":
        cache = pathlib.Path(sys.argv[2])
        cache.mkdir(parents=True, exist_ok=True)
    today = dt.date.today().isoformat()
    tz_version = fetch("version", cache).strip()

    countries = {}
    for cols in rows(fetch("iso3166.tab", cache)):
        countries[cols[0]] = {"alpha2": cols[0], "tz_name": cols[1]}

    for cols in rows(fetch("zone.tab", cache)):
        if cols[0] in countries:
            entry = countries[cols[0]].setdefault("zones", [])
            entry.append(cols[2])
            if len(cols) > 3 and cols[3]:
                countries[cols[0]].setdefault("zone_comments", {})[cols[2]] = cols[3]
    for cols in rows(fetch("zone1970.tab", cache)):
        for code in cols[0].split(","):
            if code in countries:
                countries[code].setdefault("zones_1970", []).append(cols[2])

    supplemental = ET.fromstring(fetch("supplementalData.xml", cache).encode("utf-8"))
    for item in supplemental.iter("territoryCodes"):
        code = item.get("type")
        if code in countries:
            countries[code]["alpha3"] = item.get("alpha3")
            numeric = item.get("numeric")
            countries[code]["numeric"] = numeric.zfill(3) if numeric else None
    currency_numeric = {c.get("type"): c.get("numeric").zfill(3) for c in supplemental.iter("currencyCodes")}
    digits = {}
    for info in supplemental.iter("info"):
        if info.get("iso4217"):
            digits[info.get("iso4217")] = int(info.get("digits", "2"))
    used = set()
    for region in supplemental.iter("region"):
        code = region.get("iso3166")
        if code not in countries:
            continue
        current = [c.get("iso4217") for c in region.findall("currency")
                   if c.get("to") is None and c.get("tender", "true") != "false"]
        countries[code]["currencies"] = current
        used.update(current)

    english = ET.fromstring(fetch("en.xml", cache).encode("utf-8"))
    names, variants = {}, {}
    for item in english.iter("territory"):
        code, alt = item.get("type"), item.get("alt")
        if code in countries:
            if alt is None:
                names[code] = item.text
            elif alt in ("short", "variant"):
                variants.setdefault(code, []).append(item.text)
    currency_names = {}
    for item in english.iter("currency"):
        label = item.find("displayName")
        if label is not None and item.get("type") in used:
            currency_names[item.get("type")] = label.text

    telephone = ET.fromstring(fetch("telephoneCodeData.xml", cache).encode("utf-8"))
    for item in telephone.iter("codesByTerritory"):
        code = item.get("territory")
        if code in countries:
            countries[code]["calling_codes"] = ["+" + c.get("code") for c in item.findall("telephoneCountryCode")]

    for code, entry in countries.items():
        entry["name"] = names.get(code, entry["tz_name"])
        others = [n for n in variants.get(code, []) + [entry["tz_name"]] if n != entry["name"]]
        entry["other_names"] = list(dict.fromkeys(others))
        entry.setdefault("alpha3", None)
        entry.setdefault("numeric", None)
        entry.setdefault("currencies", [])
        entry.setdefault("calling_codes", [])
        entry.setdefault("zones", [])
        entry.setdefault("zones_1970", [])
    order = ["alpha2", "alpha3", "numeric", "name", "other_names", "tz_name", "currencies", "calling_codes", "zones", "zones_1970", "zone_comments"]
    countries = {code: {k: e[k] for k in order if k in e} for code, e in sorted(countries.items())}
    currencies = {c: {"name": currency_names.get(c), "numeric": currency_numeric.get(c), "cldr_fraction_digits": digits.get(c, digits.get("DEFAULT", 2))}
                  for c in sorted(used)}

    payload = {
        "generated": today,
        "generator": "tools/catalog/data/iso_country_lookup.py",
        "sources": {
            "codes_and_tz_names": {"what": "ISO 3166-1 alpha-2 list, tz names, time zones", "source": f"IANA tz database {tz_version}: iso3166.tab, zone.tab, zone1970.tab", "url": TZDB, "licence": "public domain", "fetched": today},
            "alpha3_numeric_currency_names": {"what": "alpha-3 and numeric codes, ISO 4217 currencies in use per country, English names", "source": "Unicode CLDR 48.2: supplementalData.xml (codeMappings, currencyData), main/en.xml", "url": CLDR, "licence": "Unicode License v3", "fetched": today},
            "calling_codes": {"what": "ITU-T E.164 country calling codes", "source": "Unicode CLDR 33.1 telephoneCodeData.xml (deprecated in CLDR 34, 2018; references the ITU list of 15 Sep 2007, last edited 2017)", "url": CLDR33 + "common/supplemental/telephoneCodeData.xml", "licence": "Unicode License (data files)", "fetched": today},
        },
        "not_included": {
            "capital": "no permissively licensed source found",
            "six_iso4217_list": "SIX's terms of use say its site content \"may not be reproduced or reused in any way or used for commercial purposes\"; CLDR currencyData is used instead",
        },
        "unicode_license_notice": fetch("LICENSE", cache).strip().splitlines(),
        "tz_version": tz_version,
        "countries": countries,
        "currencies": currencies,
    }
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/geography/iso_country_lookup/countries.json"
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n", encoding="utf-8")
    missing = {k: [c for c, e in countries.items() if not e[k]] for k in ("alpha3", "currencies", "calling_codes", "zones")}
    print(f"{len(countries)} countries, {len(currencies)} currencies, tzdb {tz_version}")
    for key, codes in missing.items():
        print(f"without {key}: {' '.join(codes) or '-'}")
    print(out)


if __name__ == "__main__":
    main()
