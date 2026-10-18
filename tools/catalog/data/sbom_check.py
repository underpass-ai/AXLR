#!/usr/bin/env python3
"""Write catalog/tools/security/sbom_check/spdx_licenses.json: the SPDX License List ids
(licences and exceptions, with their deprecated and OSI-approved flags) that the tool uses to
check licence identifiers in SBOMs.

Source: SPDX License List data, https://github.com/spdx/license-list-data (CC0-1.0 for the
data files), JSON files
  https://raw.githubusercontent.com/spdx/license-list-data/main/json/licenses.json
  https://raw.githubusercontent.com/spdx/license-list-data/main/json/exceptions.json
fetched once by this generator (it uses the network; the tool never does). Fetched
2026-10-18: License List version 3.29.0, released 2026-09-16. A newer list is a new version of
the tool.

Usage: python3 tools/catalog/data/sbom_check.py [--from DIR]
  --from DIR  read licenses.json and exceptions.json from DIR instead of fetching them
"""
import datetime as dt
import json
import pathlib
import sys
import urllib.request

BASE = "https://raw.githubusercontent.com/spdx/license-list-data/main/json/"
OUT = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/security/sbom_check/spdx_licenses.json"


def load(name, local):
    if local:
        return json.loads((local / name).read_text(encoding="utf-8"))
    request = urllib.request.Request(BASE + name, headers={"User-Agent": "AXLR-catalog data generator"})
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.loads(response.read().decode("utf-8"))


def main():
    local = pathlib.Path(sys.argv[2]) if len(sys.argv) == 3 and sys.argv[1] == "--from" else None
    licenses = load("licenses.json", local)
    exceptions = load("exceptions.json", local)
    head = {
        "title": "SPDX License List identifiers",
        "license_list_version": licenses["licenseListVersion"],
        "release_date": licenses["releaseDate"][:10],
        "source_url": "https://github.com/spdx/license-list-data",
        "fetched": dt.date.today().isoformat(),
    }
    lines = ["{"]
    for key, value in head.items():
        lines.append(f"  {json.dumps(key)}: {json.dumps(value)},")
    lines.append('  "licenses": {')
    rows = sorted(licenses["licenses"], key=lambda item: item["licenseId"].lower())
    for index, item in enumerate(rows):
        flags = {"deprecated": bool(item.get("isDeprecatedLicenseId")), "osi": bool(item.get("isOsiApproved"))}
        comma = "," if index < len(rows) - 1 else ""
        lines.append(f"    {json.dumps(item['licenseId'])}: {json.dumps(flags)}{comma}")
    lines.append("  },")
    lines.append('  "exceptions": {')
    rows = sorted(exceptions["exceptions"], key=lambda item: item["licenseExceptionId"].lower())
    for index, item in enumerate(rows):
        comma = "," if index < len(rows) - 1 else ""
        lines.append(f"    {json.dumps(item['licenseExceptionId'])}: {json.dumps({'deprecated': bool(item.get('isDeprecatedLicenseId'))})}{comma}")
    lines.append("  }")
    lines.append("}")
    text = "\n".join(lines) + "\n"
    json.loads(text)
    OUT.write_text(text, encoding="utf-8")
    print(f"wrote {OUT}")


if __name__ == "__main__":
    main()
