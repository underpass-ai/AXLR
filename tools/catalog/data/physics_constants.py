#!/usr/bin/env python3
"""Write catalog/tools/research/physics_constants/constants.json: the CODATA 2022 recommended
values of the fundamental physical constants, fetched once from NIST (this generator uses the
network; the tool never does).

Source: NIST, "Fundamental Physical Constants --- Complete Listing, 2022 CODATA adjustment",
  https://physics.nist.gov/cuu/Constants/Table/allascii.txt
  fetched 2026-10-18. Works of the U.S. Government are in the public domain (17 U.S.C. 105);
  NIST asks that the source be cited: E. Tiesinga, P. J. Mohr, D. B. Newell and B. N. Taylor,
  "The 2022 CODATA Recommended Values of the Fundamental Physical Constants", NIST.

The file is fixed width: quantity (columns 0-59), value (60-84), uncertainty (85-109), unit
(110-). Digits are grouped with spaces, which are removed; "(exact)" marks an exact value and
"..." a value NIST truncated (exact but with more digits than printed).

SYMBOLS below is this generator's own short list of the usual symbols of the common constants
(written for this catalog, not part of the NIST file); every other constant is found by name.

Usage: python3 tools/catalog/data/physics_constants.py [--file allascii.txt]
"""
import json
import pathlib
import re
import sys
import urllib.request

URL = "https://physics.nist.gov/cuu/Constants/Table/allascii.txt"
FETCHED = "2026-10-18"

SYMBOLS = {
    "speed of light in vacuum": ["c", "c0"],
    "Planck constant": ["h"],
    "reduced Planck constant": ["hbar", "ħ"],
    "elementary charge": ["e"],
    "Boltzmann constant": ["k", "k_B", "kB"],
    "Avogadro constant": ["N_A", "NA", "L"],
    "Newtonian constant of gravitation": ["G"],
    "standard acceleration of gravity": ["g_n", "gn"],
    "standard atmosphere": ["atm"],
    "electron mass": ["m_e", "me"],
    "proton mass": ["m_p", "mp"],
    "neutron mass": ["m_n", "mn"],
    "atomic mass constant": ["m_u", "mu", "u"],
    "fine-structure constant": ["alpha", "α"],
    "inverse fine-structure constant": ["1/alpha"],
    "molar gas constant": ["R"],
    "Faraday constant": ["F"],
    "vacuum electric permittivity": ["epsilon_0", "eps0", "ε0"],
    "vacuum mag. permeability": ["mu_0", "mu0", "μ0"],
    "characteristic impedance of vacuum": ["Z_0", "Z0"],
    "Stefan-Boltzmann constant": ["sigma", "σ"],
    "Wien wavelength displacement law constant": ["b"],
    "Rydberg constant": ["R_inf", "Rinf", "R∞"],
    "Bohr radius": ["a_0", "a0"],
    "Bohr magneton": ["mu_B", "muB", "μB"],
    "nuclear magneton": ["mu_N", "muN", "μN"],
    "classical electron radius": ["r_e", "re"],
    "Compton wavelength": ["lambda_C", "λC"],
    "Hartree energy": ["E_h", "Eh"],
    "electron volt": ["eV"],
    "mag. flux quantum": ["Phi_0", "Φ0"],
    "conductance quantum": ["G_0", "G0"],
    "von Klitzing constant": ["R_K", "RK"],
    "Josephson constant": ["K_J", "KJ"],
    "molar volume of ideal gas (273.15 K, 101.325 kPa)": ["V_m", "Vm"],
    "Loschmidt constant (273.15 K, 101.325 kPa)": ["n_0", "n0"],
    "first radiation constant": ["c_1", "c1"],
    "second radiation constant": ["c_2", "c2"],
    "Thomson cross section": ["sigma_e", "σe"],
    "electron g factor": ["g_e", "ge"],
    "proton-electron mass ratio": ["m_p/m_e", "mp/me"],
    "electron charge to mass quotient": ["-e/m_e"],
    "Planck length": ["l_P", "lP"],
    "Planck mass": ["m_P", "mP"],
    "Planck time": ["t_P", "tP"],
    "Planck temperature": ["T_P", "TP"],
}


def number(text):
    return re.sub(r"\s+", "", text.replace("...", ""))


def parse(raw):
    lines = raw.splitlines()
    start = next(i for i, line in enumerate(lines) if line.startswith("-----")) + 1
    constants = []
    for line in lines[start:]:
        if not line.strip():
            continue
        name, value, uncertainty, unit = line[:60].strip(), line[60:85].strip(), line[85:110].strip(), line[110:].strip()
        exact = uncertainty == "(exact)"
        constants.append({
            "name": name,
            "value": number(value),
            "uncertainty": None if exact else number(uncertainty),
            "unit": unit,
            "exact": exact,
            "truncated": "..." in value,
            "symbols": SYMBOLS.get(name, []),
        })
    return constants


def main():
    if len(sys.argv) == 3 and sys.argv[1] == "--file":
        raw = pathlib.Path(sys.argv[2]).read_text()
    else:
        request = urllib.request.Request(URL, headers={"User-Agent": "AXLR-catalog data generator"})
        with urllib.request.urlopen(request, timeout=120) as response:
            raw = response.read().decode("utf-8")
    if "2022 CODATA adjustment" not in raw:
        raise SystemExit("the file is not the 2022 CODATA listing")
    constants = parse(raw)
    if len(constants) < 350:
        raise SystemExit(f"only {len(constants)} constants parsed: has the layout changed?")
    for c in constants:
        float(c["value"])
        if c["uncertainty"] is not None:
            float(c["uncertainty"])
    names = {c["name"] for c in constants}
    unknown = sorted(set(SYMBOLS) - names)
    if unknown:
        raise SystemExit(f"symbols for names not in the file: {unknown}")
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/research/physics_constants/constants.json"
    payload = {"title": "CODATA 2022 recommended values of the fundamental physical constants",
               "source": "NIST Fundamental Physical Constants, Complete Listing, 2022 CODATA adjustment",
               "source_url": URL, "licence": "public domain (U.S. Government work, 17 U.S.C. 105)",
               "fetched": FETCHED, "generator": "tools/catalog/data/physics_constants.py",
               "symbols_note": "symbols are the generator's own list of the usual symbols, not part of the NIST file",
               "constants": constants}
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n")
    print(f"{out}: {len(constants)} constants")


if __name__ == "__main__":
    main()
