#!/usr/bin/env python3
"""Write catalog/tools/programming/license_detect/fingerprints.json: hashed word-shingle
fingerprints of common licence texts and of their standard notices (headers), built from the
SPDX License List data. This generator uses the network once; the tool never does.

Source: SPDX license-list-data, release v3.29.0 (tag published 2026-09-16), files
  https://raw.githubusercontent.com/spdx/license-list-data/v3.29.0/json/details/<id>.json
(fields licenseText, standardLicenseHeader, name, isOsiApproved, isFsfLibre). The SPDX License
List data is published by the SPDX project under CC0-1.0. Fetched on 2026-10-18.

Only fingerprints are bundled, never the texts: each text is normalised with the SPDX matching
guidelines (case, whitespace, punctuation, quotes and dashes, copyright lines, http/https,
the equivalent words of license-list-XML/equivalentwords.txt), cut into overlapping shingles
of SHINGLE consecutive words, and each shingle is hashed (first 6 hex digits of its SHA-1).
Long texts keep only the hashes whose value is divisible by a per-licence modulus (the tool
applies the same rule), so the file stays under 256 KiB. normalise() and shingles() are
copied verbatim into the tool; keep them identical.

Usage: python3 tools/catalog/data/license_detect.py [--cache DIR]
"""
import datetime as dt
import hashlib
import json
import pathlib
import re
import sys
import urllib.request

RELEASE = "v3.29.0"
BASE = f"https://raw.githubusercontent.com/spdx/license-list-data/{RELEASE}/json/details/"
FETCHED = "2026-10-18"
LICENSES = [
    "MIT", "MIT-0", "BSD-2-Clause", "BSD-3-Clause", "0BSD", "ISC", "Apache-2.0", "GPL-2.0-only", "GPL-3.0-only",
    "LGPL-2.1-only", "LGPL-3.0-only", "AGPL-3.0-only", "MPL-2.0", "EPL-1.0", "EPL-2.0", "EUPL-1.2", "Unlicense",
    "CC0-1.0", "BSL-1.0", "Zlib", "CC-BY-4.0", "CC-BY-SA-4.0", "Artistic-2.0", "WTFPL", "OFL-1.1", "PostgreSQL",
    "BlueOak-1.0.0", "UPL-1.0",
]
OR_LATER = {"GPL-2.0-only": "GPL-2.0-or-later", "GPL-3.0-only": "GPL-3.0-or-later",
            "LGPL-2.1-only": "LGPL-2.1-or-later", "LGPL-3.0-only": "LGPL-3.0-or-later",
            "AGPL-3.0-only": "AGPL-3.0-or-later"}
MAX_KEPT = 2000         # hashes kept per licence text at most (lowered until the file fits)
TARGET_BYTES = 240_000  # stay below the 256 KiB bundle limit
OUT = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/programming/license_detect/fingerprints.json"

# ---- copied verbatim into catalog/tools/programming/license_detect/license_detect.py ----
SHINGLE = 5
EQUIVALENT = {
    "acknowledgment": "acknowledgement", "analogue": "analog", "analyse": "analyze", "artefact": "artifact",
    "authorisation": "authorization", "authorised": "authorized", "calibre": "caliber", "cancelled": "canceled",
    "capitalisations": "capitalizations", "catalogue": "catalog", "categorise": "categorize", "centre": "center",
    "emphasised": "emphasized", "favour": "favor", "favourite": "favorite", "fulfil": "fulfill",
    "fulfilment": "fulfillment", "initialise": "initialize", "judgment": "judgement", "labelling": "labeling",
    "labour": "labor", "licence": "license", "maximise": "maximize", "merchantibility": "merchantability",
    "modelled": "modeled", "modelling": "modeling", "offence": "offense", "optimise": "optimize",
    "organisation": "organization", "organise": "organize", "practise": "practice", "programme": "program",
    "realise": "realize", "recognise": "recognize", "signalling": "signaling", "utilisation": "utilization",
    "whilst": "while", "wilful": "wilfull", "licences": "licenses",
}
PAIRS = {("copyright", "owner"): ["copyright", "holder"], ("per", "cent"): ["percent"],
         ("sub", "license"): ["sublicense"], ("non", "commercial"): ["noncommercial"]}


def normalise(text):
    """SPDX matching guidelines, simplified: a list of lowercase words."""
    kept = []
    for line in text.replace("\r\n", "\n").replace("\r", "\n").split("\n"):
        s = line.strip().lower().lstrip("#*/;!-= \t")
        if re.match(r"^(copyright\b|\(c\)|©|all rights reserved\.?$)", s):
            continue
        kept.append(line)
    t = "\n".join(kept).lower()
    t = t.replace("&", " and ").replace("©", " ")
    t = re.sub(r"https?://", " ", t)
    t = re.sub(r"[‘’“”\"'`]", "", t)
    words = re.findall(r"[a-z0-9]+", t)
    out = []
    i = 0
    while i < len(words):
        pair = tuple(words[i:i + 2])
        if pair in PAIRS:
            out.extend(PAIRS[pair])
            i += 2
            continue
        out.append(EQUIVALENT.get(words[i], words[i]))
        i += 1
    return out


def shingles(words):
    """Hashes (6 hex digits) of every run of SHINGLE consecutive words."""
    if len(words) < SHINGLE:
        return {hashlib.sha1(" ".join(words).encode()).hexdigest()[:6]} if words else set()
    return {hashlib.sha1(" ".join(words[i:i + SHINGLE]).encode()).hexdigest()[:6]
            for i in range(len(words) - SHINGLE + 1)}
# ---- end of the copied part ----


def fetch(spdx_id, cache):
    if cache:
        path = cache / f"{spdx_id}.json"
        if path.exists():
            return json.loads(path.read_text())
    request = urllib.request.Request(BASE + spdx_id + ".json", headers={"User-Agent": "AXLR-catalog data generator"})
    with urllib.request.urlopen(request, timeout=60) as response:
        data = json.loads(response.read().decode("utf-8"))
    if cache:
        (cache / f"{spdx_id}.json").write_text(json.dumps(data))
    return data


def lines_of(hashes, per_line=16):
    hashes = sorted(hashes)
    return [" ".join(hashes[i:i + per_line]) for i in range(0, len(hashes), per_line)]


def sample(hashes, modulus):
    return {h for h in hashes if int(h, 16) % modulus == 0}


def build(cache, max_kept):
    licenses, headers = [], []
    for spdx_id in LICENSES:
        data = fetch(spdx_id, cache)
        words = normalise(data["licenseText"])
        full = shingles(words)
        modulus = 1
        while len(sample(full, modulus)) > max_kept:
            modulus += 1
        kept = sample(full, modulus)
        licenses.append({"id": spdx_id, "name": data["name"], "osi_approved": bool(data.get("isOsiApproved")),
                         "fsf_libre": bool(data.get("isFsfLibre")), "or_later_id": OR_LATER.get(spdx_id),
                         "words": len(words), "shingles": len(full), "modulus": modulus, "kept": len(kept),
                         "hashes": lines_of(kept)})
        ids = [spdx_id] + ([OR_LATER[spdx_id]] if spdx_id in OR_LATER else [])
        for hid in ids:
            hdata = data if hid == spdx_id else fetch(hid, cache)
            header = hdata.get("standardLicenseHeader")
            if header:
                hwords = normalise(header)
                headers.append({"id": hid, "license": spdx_id, "words": len(hwords), "hashes": lines_of(shingles(hwords))})
    return licenses, headers


def main():
    cache = None
    if len(sys.argv) == 3 and sys.argv[1] == "--cache":
        cache = pathlib.Path(sys.argv[2])
        cache.mkdir(parents=True, exist_ok=True)
    max_kept = MAX_KEPT
    while True:
        licenses, headers = build(cache, max_kept)
        doc = {
            "source": "SPDX License List data, json/details/<id>.json (licenseText, standardLicenseHeader)",
            "source_url": BASE,
            "release": RELEASE,
            "data_license": "CC0-1.0",
            "fetched": FETCHED,
            "generated": dt.date.today().isoformat(),
            "method": f"SPDX-guideline normalisation, {SHINGLE}-word shingles, SHA-1 first 6 hex digits; "
                      "a text keeps the hashes divisible by its modulus",
            "licenses": licenses,
            "headers": headers,
        }
        text = json.dumps(doc, indent=1) + "\n"
        if len(text.encode()) <= TARGET_BYTES:
            break
        max_kept -= 50
    OUT.write_text(text)
    print(f"wrote {OUT} ({len(text.encode())} bytes, {len(licenses)} licences, {len(headers)} headers, "
          f"at most {max_kept} hashes per text)")


if __name__ == "__main__":
    main()
