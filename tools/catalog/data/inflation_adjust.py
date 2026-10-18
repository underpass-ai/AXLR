#!/usr/bin/env python3
"""Write catalog/tools/finance/inflation_adjust/cpi.json: the price-index series the tool
bundles, fetched once from the agencies' public APIs (this generator uses the network; the
tool never does). Each series records its source URL, series id, base and fetch date.

  us_cpi_u  BLS CPI-U, U.S. city average, all items, not seasonally adjusted (CUUR0000SA0),
            1982-84=100. Public API v1, POST, at most 10 years per request, annual averages
            (period M13) with "annualaverage": true. The flat file at download.bls.gov answers
            403 to scripts, so it is not used.
  ea_hicp   Eurostat HICP, all items (CP00), euro area (EA, changing composition), 2015=100:
            annual average index prc_hicp_aind (unit INX_A_AVG) and monthly index
            prc_hicp_midx (unit I15).
  es_ipc    INE Spain, IPC, national general index, base 2025 (series IPC290751), monthly;
            the annual average is the mean of the twelve monthly indexes, rounded to 3
            decimals, for complete years only.

Usage: python3 tools/catalog/data/inflation_adjust.py [--cache DIR]
  --cache DIR  keep the raw responses in DIR and reuse them on a rerun (BLS v1 has a daily quota)
"""
import datetime as dt
import json
import pathlib
import sys
import urllib.request
from decimal import ROUND_HALF_UP, Decimal

BLS_URL = "https://api.bls.gov/publicAPI/v1/timeseries/data/"
EUROSTAT = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/"
EA_ANNUAL_URL = EUROSTAT + "prc_hicp_aind?format=JSON&unit=INX_A_AVG&coicop=CP00&geo=EA"
EA_MONTHLY_URL = EUROSTAT + "prc_hicp_midx?format=JSON&unit=I15&coicop=CP00&geo=EA"
INE_URL = "https://servicios.ine.es/wstempus/js/ES/DATOS_SERIE/IPC290751?nult=5000"
HEADERS = {"User-Agent": "AXLR-catalog data generator", "Accept": "application/json"}


def fetch(url, cache, key, body=None):
    if cache:
        path = cache / f"{key}.json"
        if path.exists():
            return json.loads(path.read_text())
    data = json.dumps(body).encode() if body is not None else None
    headers = dict(HEADERS, **({"Content-Type": "application/json"} if body is not None else {}))
    request = urllib.request.Request(url, data=data, headers=headers)
    with urllib.request.urlopen(request, timeout=120) as response:
        payload = json.loads(response.read().decode("utf-8"))
    if cache:
        (cache / f"{key}.json").write_text(json.dumps(payload))
    return payload


def bls(cache, today):
    annual, monthly = {}, {}
    for start in range(1913, today.year + 1, 10):
        body = {"seriesid": ["CUUR0000SA0"], "startyear": str(start), "endyear": str(min(start + 9, today.year)), "annualaverage": True}
        payload = fetch(BLS_URL, cache, f"bls_{start}", body)
        if payload.get("status") != "REQUEST_SUCCEEDED":
            raise RuntimeError(f"BLS {start}: {payload.get('status')} {payload.get('message')}")
        for row in payload["Results"]["series"][0]["data"]:
            try:
                value = Decimal(row["value"])
            except ArithmeticError:
                continue  # "-": not published (e.g. October 2025)
            if row["period"] == "M13":
                annual[row["year"]] = format(value, "f")
            elif row["period"].startswith("M"):
                monthly[f"{row['year']}-{row['period'][1:]}"] = format(value, "f")
    return {
        "title": "United States, CPI-U, U.S. city average, all items, not seasonally adjusted",
        "agency": "U.S. Bureau of Labor Statistics", "series_id": "CUUR0000SA0", "base": "1982-84=100", "currency": "USD",
        "source_url": BLS_URL + "CUUR0000SA0",
        "annual_method": "BLS annual average (period M13)",
        "fetched": today.isoformat(),
        "annual": dict(sorted(annual.items())), "monthly": dict(sorted(monthly.items())),
    }


def eurostat_series(payload):
    times = payload["dimension"]["time"]["category"]["index"]
    values = payload["value"]
    out = {}
    for label, position in times.items():
        value = values.get(str(position))
        if value is not None:
            out[label] = format(Decimal(str(value)).quantize(Decimal("0.01")), "f")
    return dict(sorted(out.items()))


def eurostat(cache, today):
    annual = fetch(EA_ANNUAL_URL, cache, "ea_annual")
    monthly = fetch(EA_MONTHLY_URL, cache, "ea_monthly")
    return {
        "title": "Euro area (changing composition), HICP, all items",
        "agency": "Eurostat", "series_id": "prc_hicp_aind INX_A_AVG CP00 EA; prc_hicp_midx I15 CP00 EA", "base": "2015=100", "currency": "EUR",
        "source_url": EA_ANNUAL_URL, "source_url_monthly": EA_MONTHLY_URL,
        "annual_method": "Eurostat annual average index (INX_A_AVG)",
        "dataset_updated": annual.get("updated"),
        "fetched": today.isoformat(),
        "annual": eurostat_series(annual), "monthly": eurostat_series(monthly),
    }


def ine(cache, today):
    payload = fetch(INE_URL, cache, "ine")
    monthly = {}
    for row in payload["Data"]:
        if row.get("Valor") is None or row.get("Secreto"):
            continue
        monthly[f"{row['Anyo']}-{int(row['FK_Periodo']):02d}"] = Decimal(str(row["Valor"]))
    annual = {}
    for year in sorted({key[:4] for key in monthly}):
        months = [monthly.get(f"{year}-{m:02d}") for m in range(1, 13)]
        if all(v is not None for v in months):
            annual[year] = format((sum(months) / 12).quantize(Decimal("0.001"), ROUND_HALF_UP), "f")
    return {
        "title": "Spain, IPC (índice de precios de consumo), national general index",
        "agency": "Instituto Nacional de Estadística (INE)", "series_id": "IPC290751", "base": "2025=100", "currency": "EUR",
        "source_url": INE_URL,
        "annual_method": "mean of the twelve monthly indexes, rounded to 3 decimals (complete years only)",
        "fetched": today.isoformat(),
        "annual": annual, "monthly": {k: format(v, "f") for k, v in sorted(monthly.items())},
    }


def main():
    cache = None
    if len(sys.argv) == 3 and sys.argv[1] == "--cache":
        cache = pathlib.Path(sys.argv[2])
        cache.mkdir(parents=True, exist_ok=True)
    today = dt.date.today()
    series, missing = {}, {}
    for name, build in (("us_cpi_u", bls), ("ea_hicp", eurostat), ("es_ipc", ine)):
        try:
            series[name] = build(cache, today)
        except Exception as error:  # ship the others; the README and the file say which failed
            missing[name] = f"{type(error).__name__}: {error}"
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/finance/inflation_adjust/cpi.json"
    payload = {"generated": today.isoformat(), "generator": "tools/catalog/data/inflation_adjust.py", "series": series, "not_fetched": missing}
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n")
    for name, s in series.items():
        years = list(s["annual"])
        print(f"{name}: annual {years[0]}-{years[-1]}, {len(s['monthly'])} months")
    for name, why in missing.items():
        print(f"{name}: NOT FETCHED ({why})")
    print(out)


if __name__ == "__main__":
    main()
