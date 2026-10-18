#!/usr/bin/env python3
"""Write catalog/tools/research/chem_formula/elements.json: the element table the tool bundles,
fetched once from the IUPAC Commission on Isotopic Abundances and Atomic Weights (this
generator uses the network; the tool never does).

Source: CIAAW, "Abridged Standard Atomic Weights 2024"
  https://www.ciaaw.org/abridged-atomic-weights.htm  fetched 2026-10-18.
  The table is based on the "Atomic Weights 2021" report (T. Prohaska et al., Pure Appl. Chem.
  94 (2022) 573-600) with the 2024 CIAAW revisions of gadolinium, lutetium and zirconium.
  Abridged values are quoted to five significant figures where the variability of natural
  isotopic composition allows; the uncertainty is the abridged one printed by CIAAW.
  Licence: the official table of the IUPAC commission responsible for atomic weights; the
  page is (c) CIAAW, the values themselves are numerical facts of nature, reproduced with
  attribution.

Elements with no stable isotope and no characteristic terrestrial composition (Tc, Pm, Po-Ac,
Np and beyond) have no standard atomic weight; CIAAW prints a dash and the table stores null.
Footnotes g, m and r are kept as given.

Usage: python3 tools/catalog/data/chem_formula.py [--file page.html]
"""
import html
import json
import pathlib
import re
import sys
import urllib.request

URL = "https://www.ciaaw.org/abridged-atomic-weights.htm"
FETCHED = "2026-10-18"


def text_of(page):
    page = re.sub(r"<script.*?</script>|<style.*?</style>", "", page, flags=re.S)
    page = re.sub(r"</t[dh]>", " ", page)
    page = re.sub(r"<[^>]+>", " ", page)
    page = html.unescape(page).replace(" ", " ")
    page = re.sub(r"[ \t]+", " ", page)
    return [line.strip() for line in page.splitlines() if line.strip()]


def main():
    if len(sys.argv) == 3 and sys.argv[1] == "--file":
        page = pathlib.Path(sys.argv[2]).read_text()
    else:
        request = urllib.request.Request(URL, headers={"User-Agent": "AXLR-catalog data generator"})
        with urllib.request.urlopen(request, timeout=120) as response:
            page = response.read().decode("utf-8")
    if "Abridged Standard Atomic Weights 2024" not in page:
        raise SystemExit("the page is not the 2024 abridged table")
    lines = text_of(page)
    # the rows may be split over several lines; join the cells between two atomic numbers
    joined = " ".join(lines)
    cells = re.findall(r"(?<!\S)(\d{1,3}) ([A-Z][a-z]?) ([a-z]+) +((?:\d+(?:\.\d+)? ± \d+(?:\.\d+)?)|—)((?: [gmr])*)(?= \d{1,3} [A-Z]| Z )", joined)
    elements = []
    for z, symbol, name, weight, notes in cells:
        if weight == "—":
            value = uncertainty = None
        else:
            value, uncertainty = [part.strip() for part in weight.split("±")]
        elements.append({"z": int(z), "symbol": symbol, "name": name, "weight": value,
                         "uncertainty": uncertainty, "notes": notes.split()})
    if [e["z"] for e in elements] != list(range(1, 119)):
        raise SystemExit(f"parsed {len(elements)} rows, not Z = 1..118 in order")
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/research/chem_formula/elements.json"
    payload = {"title": "IUPAC/CIAAW Abridged Standard Atomic Weights 2024",
               "basis": "Atomic Weights 2021 report (Prohaska et al., Pure Appl. Chem. 94 (2022) 573-600) with the 2024 revisions of Gd, Lu and Zr",
               "source_url": URL,
               "licence": "official table of the IUPAC commission (page (c) CIAAW); numerical values reproduced as facts, with attribution",
               "fetched": FETCHED, "generator": "tools/catalog/data/chem_formula.py",
               "notes_legend": {"g": "geological materials with an isotopic composition outside normal limits are known",
                                "m": "modified isotopic compositions may be found in commercial material",
                                "r": "range in isotopic composition of normal terrestrial material limits precision"},
               "elements": elements}
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n")
    print(f"{out}: {len(elements)} elements, {sum(e['weight'] is None for e in elements)} without a standard atomic weight")


if __name__ == "__main__":
    main()
