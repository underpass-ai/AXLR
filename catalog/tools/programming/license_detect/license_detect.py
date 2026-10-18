#!/usr/bin/env python3
"""Identify the SPDX licence of a LICENSE text by similarity against bundled fingerprints of
common licences (fingerprints.json, built from the SPDX License List data), with a score and
the closest alternatives. A short standard notice (such as the GPL "This program is free
software..." header) is reported as a notice, not as the full licence.

Input (JSON on stdin):
  text   the LICENSE / COPYING text, or a source-file header
  top    how many alternatives to list (default 5)
"""
import hashlib
import json
import pathlib
import re
import sys
from fractions import Fraction

MAX_TEXT = 1_000_000
DATA = pathlib.Path(__file__).resolve().parent / "fingerprints.json"

# ---- copied verbatim from tools/catalog/data/license_detect.py ----
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


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(f):
    return f"{float(f):.3f}"


def load():
    try:
        data = json.loads(DATA.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        fail(f"cannot read the bundled fingerprints.json: {error}")
    for item in data["licenses"] + data["headers"]:
        item["set"] = {h for line in item["hashes"] for h in line.split()}
    return data


def score(fp_set, modulus, query):
    q = {h for h in query if int(h, 16) % modulus == 0} if modulus > 1 else query
    inter = len(fp_set & query)
    recall = Fraction(inter, len(fp_set)) if fp_set else Fraction(0)
    precision = Fraction(inter, len(q)) if q else Fraction(0)
    dice = Fraction(2 * inter, len(fp_set) + len(q)) if fp_set or q else Fraction(0)
    return dice, recall, precision


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - {"text", "top"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be the licence text (a non-empty string)")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    top = request.get("top", 5)
    if not isinstance(top, int) or isinstance(top, bool) or not 1 <= top <= 28:
        fail("top must be an integer from 1 to 28")

    data = load()
    words = normalise(text)
    query = shingles(words)
    declared = sorted(set(re.findall(r"SPDX-License-Identifier:\s*([A-Za-z0-9.+\-() ]+?)\s*(?:\*/|-->|$)", text, re.M)))

    ranked = []
    for lic in data["licenses"]:
        dice, recall, precision = score(lic["set"], lic["modulus"], query)
        ranked.append((dice, recall, precision, lic))
    ranked.sort(key=lambda r: (r[0], r[1]), reverse=True)
    best_dice, best_recall, best_precision, best = ranked[0]

    header_hits = []
    for h in data["headers"]:
        _, recall, _ = score(h["set"], 1, query)
        header_hits.append((recall, h))
    header_hits.sort(key=lambda r: (r[0], r[1]["words"]), reverse=True)
    later = bool(re.search(r"\bany\s+later\s+version\b", text, re.I))

    result = {"words": len(words)}
    if declared:
        result["declared"] = declared
    if best_dice >= Fraction(85, 100):
        kind, confidence = "license", "high"
    elif best_dice >= Fraction(60, 100):
        kind, confidence = "license", "medium"
    elif header_hits and header_hits[0][0] >= Fraction(60, 100) and best_recall < Fraction(30, 100):
        kind, confidence = "notice", "high" if header_hits[0][0] >= Fraction(85, 100) else "medium"
    elif best_recall >= Fraction(85, 100):
        kind, confidence = "license_within_larger_text", "medium"
    else:
        kind, confidence = "unknown", "low"

    result["kind"] = kind
    result["confidence"] = confidence
    if kind == "notice":
        h = header_hits[0][1]
        family = [x for r, x in header_hits if x["license"] == h["license"]]
        ids = {x["id"] for x in family}
        notice_id = h["id"]
        or_later = next((x["or_later_id"] for x in data["licenses"] if x["id"] == h["license"]), None)
        if or_later and or_later in ids:
            notice_id = or_later if later else h["license"]
        result["spdx_id"] = notice_id
        name = next(x["name"] for x in data["licenses"] if x["id"] == h["license"])
        result["name"] = name.replace(" only", " or later") if notice_id.endswith("-or-later") else name
        result["score"] = dec(header_hits[0][0])
        result["note"] = ("this is the licence's standard notice (header), not the licence text itself; "
                          "ship the full text in a LICENSE or COPYING file")
    elif kind in ("license", "license_within_larger_text"):
        result["spdx_id"] = best["id"]
        result["name"] = best["name"]
        result["score"] = dec(best_dice)
        result["osi_approved"] = best["osi_approved"]
        result["fsf_libre"] = best["fsf_libre"]
        if best["or_later_id"]:
            result["possible_ids"] = [best["id"], best["or_later_id"]]
            result["note"] = (f"the licence text is the same for {best['id']} and {best['or_later_id']}; the notice "
                              "in the source files (\"version N\" or \"version N or any later version\") decides")
        if kind == "license_within_larger_text":
            result["note"] = (result.get("note", "") + "; " if result.get("note") else "") + \
                "the licence text is contained in a longer document (several licences, or added terms)"
    elif declared:
        result["kind"], result["confidence"] = "identifier", "high"
        result["spdx_id"] = declared[0] if len(declared) == 1 else " AND ".join(f"({d})" for d in declared)
        result["note"] = "taken from the SPDX-License-Identifier line; no licence text to compare"
    else:
        result["spdx_id"] = None
        result["note"] = "no bundled licence is close enough; it may be a custom or less common licence"
    result["alternatives"] = [{"spdx_id": lic["id"], "score": dec(d), "recall": dec(r), "precision": dec(p)}
                              for d, r, p, lic in ranked[:top]]
    if header_hits and header_hits[0][0] > 0:
        result["notice_matches"] = [{"spdx_id": h["id"], "recall": dec(r)} for r, h in header_hits[:3] if r > 0]
    result["data"] = {"source": data["source_url"], "release": data["release"], "fetched": data["fetched"],
                      "data_license": data["data_license"]}
    result["disclaimer"] = "Automated similarity match, not legal advice; read the licence text before relying on it."
    print(json.dumps(result))


if __name__ == "__main__":
    main()
