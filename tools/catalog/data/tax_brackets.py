#!/usr/bin/env python3
"""Write catalog/tools/finance/tax_bracket_estimate/brackets.json: the bracket tables the tool
bundles, transcribed from the legal texts named in each table's `source` (read 18 Oct 2026).
A new year is a new table and a new version of the tool; tables are never edited in place.

Usage: python3 tools/catalog/data/tax_brackets.py
"""
import json
import pathlib

TABLES = {
    "es_irpf_2025_state_general": {
        "title": "Spain, IRPF, state scale of the general base (escala estatal, base liquidable general)",
        "year": 2025, "currency": "EUR",
        "source": "Ley 35/2006, art. 63.1, as amended by Ley 11/2020 (in force since 2021, unchanged for 2025): https://www.boe.es/buscar/act.php?id=BOE-A-2006-20764",
        "note": "Only the state half: the regional (autonómica) scale of the taxpayer's community is added on top; give it as brackets.",
        "brackets": [[12450, "9.5"], [20200, "12"], [35200, "15"], [60000, "18.5"], [300000, "22.5"], [None, "24.5"]],
    },
    "es_irpf_2025_state_savings": {
        "title": "Spain, IRPF, state scale of the savings base (base liquidable del ahorro)",
        "year": 2025, "currency": "EUR",
        "source": "Ley 35/2006, art. 66.1, as amended by Ley 7/2024 (from 1 January 2025): https://www.boe.es/buscar/act.php?id=BOE-A-2006-20764",
        "note": "Only the state half; the regional half of the savings scale has the same brackets and rates.",
        "brackets": [[6000, "9.5"], [50000, "10.5"], [200000, "11.5"], [300000, "13.5"], [None, "15"]],
    },
    "us_federal_2025_single": {
        "title": "United States, federal income tax, single filers",
        "year": 2025, "currency": "USD",
        "source": "IRS Revenue Procedure 2024-40, section 3.01, Table 3: https://www.irs.gov/pub/irs-drop/rp-24-40.pdf",
        "note": "Applies to taxable income, after the standard or itemised deduction.",
        "brackets": [[11925, "10"], [48475, "12"], [103350, "22"], [197300, "24"], [250525, "32"], [626350, "35"], [None, "37"]],
    },
    "us_federal_2025_married_joint": {
        "title": "United States, federal income tax, married filing jointly",
        "year": 2025, "currency": "USD",
        "source": "IRS Revenue Procedure 2024-40, section 3.01, Table 1: https://www.irs.gov/pub/irs-drop/rp-24-40.pdf",
        "note": "Applies to taxable income, after the standard or itemised deduction.",
        "brackets": [[23850, "10"], [96950, "12"], [206700, "22"], [394600, "24"], [501050, "32"], [751600, "35"], [None, "37"]],
    },
}


def main():
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/finance/tax_bracket_estimate/brackets.json"
    payload = {"generated": "2026-10-18", "generator": "tools/catalog/data/tax_brackets.py", "tables": TABLES}
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n")
    print(f"{out}: {len(TABLES)} tables")


if __name__ == "__main__":
    main()
