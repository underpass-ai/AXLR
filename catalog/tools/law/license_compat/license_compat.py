#!/usr/bin/env python3
"""Open-source licence compatibility: can code under each component licence be combined into a
work distributed under a target licence? Each verdict carries its reason and its source (FSF
licence list and GPL FAQ, Apache's third-party licence policy, the MPL 2.0 FAQ, licence texts).
Linking mode matters for the LGPL (and is stated for the GPL, where the FSF treats static and
dynamic linking alike). Not legal advice.

Input (JSON on stdin):
  target      SPDX id of the combined work's licence, or "proprietary"
  components  [ "MIT", {"license": "LGPL-2.1-only", "linking": "dynamic"}, ... ]
              linking: "source" (code copied into the work's files) | "static" | "dynamic"
              MPL-2.0 option "incompatible_with_secondary": true (Exhibit B notice present)
              EPL-2.0 option "secondary_license": true (Exhibit A names GPL-2.0-or-later)
  linking     default linking for components that do not state one (default "static")
  list_licenses  true to list the known ids
"""
import json
import sys

REVIEWED = "2026-10-18"
DISCLAIMER = ("General information on licence compatibility, not legal advice: verdicts follow the cited sources' "
              "interpretation (notably the FSF's view that linking creates a combined work); check the actual licence "
              "texts, notices and any exceptions of each component.")

SRC = {
    "fsf": "FSF, Various Licenses and Comments about Them (gnu.org/licenses/license-list.html)",
    "fsf_matrix": "FSF, GPL FAQ, license compatibility matrix (gnu.org/licenses/gpl-faq.html#AllCompatibility)",
    "fsf_linking": "FSF, GPL FAQ: static vs dynamic linking (gnu.org/licenses/gpl-faq.html#GPLStaticVsDynamic)",
    "apache": "Apache Software Foundation, ASF 3rd Party License Policy, categories A/B/X (apache.org/legal/resolved.html)",
    "mpl_faq": "Mozilla, MPL 2.0 FAQ (mozilla.org/MPL/2.0/FAQ/) and MPL-2.0 sections 1.12, 3.3",
    "agpl13": "GPL-3.0 section 13 and AGPL-3.0 section 13 (combination with the other licence)",
    "lgpl21": "LGPL-2.1 sections 3 (conversion to GPL), 5 and 6 (works that use the library)",
    "lgpl3": "LGPL-3.0 section 4 (combined works) and GPL-3.0 section 7 (removing additional permissions)",
    "epl2": "EPL-2.0 sections 1 (Secondary License), 3 and Exhibit A",
    "cddl": "CDDL-1.0 sections 3.1 and 3.6 (larger works)",
}

# kind: permissive | weak_file (MPL, CDDL) | weak_module (EPL) | lgpl | gpl | agpl
# gnu: the GNU licences the code can be conveyed under inside a combined work
LICENSES = {
    "MIT": {"kind": "permissive", "apache": "A", "notice": "keep the copyright and permission notice"},
    "MIT-0": {"kind": "permissive", "apache": "A", "notice": "no condition (no attribution required)"},
    "BSD-2-Clause": {"kind": "permissive", "apache": "A", "notice": "keep the copyright notice, conditions and disclaimer in source and binary forms"},
    "BSD-3-Clause": {"kind": "permissive", "apache": "A", "notice": "keep the notices; do not use the contributors' names to endorse the product"},
    "ISC": {"kind": "permissive", "apache": "A", "notice": "keep the copyright and permission notice"},
    "Zlib": {"kind": "permissive", "apache": "A", "notice": "do not misrepresent the origin; mark altered source versions; keep the notice in source"},
    "BSL-1.0": {"kind": "permissive", "apache": "A", "notice": "keep the licence text in source copies (not required in machine-executable object code)"},
    "Unlicense": {"kind": "permissive", "apache": "A", "notice": "no condition (public domain dedication)"},
    "CC0-1.0": {"kind": "permissive", "apache": "A (handled as public-domain-like, not on the A list itself)",
                "notice": "no condition; CC0 section 4(a) grants no patent rights"},
    "Apache-2.0": {"kind": "permissive", "apache": "A", "notice": "keep the licence, the NOTICE file and attributions; state changes in modified files; the patent grant and its termination apply"},
    "MPL-2.0": {"kind": "weak_file", "apache": "B"},
    "CDDL-1.0": {"kind": "weak_file", "apache": "B"},
    "EPL-2.0": {"kind": "weak_module", "apache": "B"},
    "LGPL-2.1-only": {"kind": "lgpl", "apache": "X", "gnu": {"LGPL21", "GPL2", "GPL3", "GPL3+"}},
    "LGPL-2.1-or-later": {"kind": "lgpl", "apache": "X", "gnu": {"LGPL21", "LGPL3", "LGPL3+", "GPL2", "GPL3", "GPL3+"}},
    "LGPL-3.0-only": {"kind": "lgpl", "apache": "X", "gnu": {"LGPL3", "GPL3"}},
    "LGPL-3.0-or-later": {"kind": "lgpl", "apache": "X", "gnu": {"LGPL3", "LGPL3+", "GPL3", "GPL3+"}},
    "GPL-2.0-only": {"kind": "gpl", "apache": "X", "gnu": {"GPL2"}},
    "GPL-2.0-or-later": {"kind": "gpl", "apache": "X", "gnu": {"GPL2", "GPL3", "GPL3+"}},
    "GPL-3.0-only": {"kind": "gpl", "apache": "X", "gnu": {"GPL3"}},
    "GPL-3.0-or-later": {"kind": "gpl", "apache": "X", "gnu": {"GPL3", "GPL3+"}},
    "AGPL-3.0-only": {"kind": "agpl", "apache": "X", "gnu": {"AGPL3"}},
    "AGPL-3.0-or-later": {"kind": "agpl", "apache": "X", "gnu": {"AGPL3", "AGPL3+"}},
}
# What a GNU target licence offers for the work as a whole.
TARGET_GNU = {
    "LGPL-2.1-only": {"LGPL21"}, "LGPL-2.1-or-later": {"LGPL21", "LGPL3", "LGPL3+"},
    "LGPL-3.0-only": {"LGPL3"}, "LGPL-3.0-or-later": {"LGPL3", "LGPL3+"},
    "GPL-2.0-only": {"GPL2"}, "GPL-2.0-or-later": {"GPL2", "GPL3", "GPL3+"},
    "GPL-3.0-only": {"GPL3"}, "GPL-3.0-or-later": {"GPL3", "GPL3+"},
    "AGPL-3.0-only": {"AGPL3"}, "AGPL-3.0-or-later": {"AGPL3", "AGPL3+"},
}
TOKEN_NAME = {"GPL2": "GPL-2.0", "GPL3": "GPL-3.0", "GPL3+": "a later GPL", "LGPL21": "LGPL-2.1", "LGPL3": "LGPL-3.0",
              "LGPL3+": "a later LGPL", "AGPL3": "AGPL-3.0", "AGPL3+": "a later AGPL"}
ALIASES = {"GPL-2.0": "GPL-2.0-only", "GPL-2.0+": "GPL-2.0-or-later", "GPL-3.0": "GPL-3.0-only", "GPL-3.0+": "GPL-3.0-or-later",
           "LGPL-2.1": "LGPL-2.1-only", "LGPL-2.1+": "LGPL-2.1-or-later", "LGPL-3.0": "LGPL-3.0-only", "LGPL-3.0+": "LGPL-3.0-or-later",
           "AGPL-3.0": "AGPL-3.0-only", "AGPL-3.0+": "AGPL-3.0-or-later"}
LINKING = ("source", "static", "dynamic")
RANK = {"compatible": 0, "conditional": 1, "incompatible": 2}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def spdx(value, field, allow_proprietary=False):
    if not isinstance(value, str) or not value.strip():
        fail(f"{field} must be an SPDX licence id")
    text = value.strip()
    if allow_proprietary and text.lower() == "proprietary":
        return "proprietary", None
    by_lower = {k.lower(): k for k in list(LICENSES) + list(ALIASES)}
    key = by_lower.get(text.lower())
    if key is None:
        fail(f"{field}: unknown licence {text!r}; known: {', '.join(LICENSES)}" + (", proprietary" if allow_proprietary else ""))
    if key in ALIASES:
        return ALIASES[key], f"{key} is a deprecated SPDX id, read as {ALIASES[key]}"
    return key, None


def gnu_combination(inbound, target):
    """FSF view: the combined work must be conveyed under one GNU licence both sides allow."""
    offered = LICENSES[inbound]["gnu"]
    wanted = TARGET_GNU[target]
    common = offered & wanted
    if common == wanted:
        return "compatible", f"{inbound} code can be conveyed under {target}", [SRC["fsf_matrix"]], None
    if common:
        names = sorted(TOKEN_NAME[t] for t in common if not t.endswith("+"))
        later = any(t.endswith("+") for t in common)
        only = " or ".join(names) + (" or later" if later else " only")
        return ("conditional", f"the combination can be conveyed only under {only}, not under every version {target} offers",
                [SRC["fsf_matrix"]], f"distribute the combined work under {only}")
    if target.startswith("AGPL") and "GPL3" in offered:
        return ("compatible", f"{inbound} code can be combined with an AGPL-3.0 work; the GPL part keeps GPL-3.0 terms and "
                "the AGPL network clause applies to the combination", [SRC["agpl13"], SRC["fsf_matrix"]], None)
    if target.startswith("GPL") and "AGPL3" in offered and ("GPL3" in wanted):
        return ("conditional", "GPL-3.0 section 13 allows combining with AGPL-3.0 code, but that part stays under AGPL-3.0 and "
                "its section 13 (source offer to network users) applies to the combination", [SRC["agpl13"]],
                "offer the corresponding source to users interacting over a network (AGPL-3.0 s. 13)")
    if target.startswith("LGPL") and any(t.startswith("GPL") for t in offered):
        return ("incompatible", f"{inbound} code cannot be relicensed under {target}; the combined library could only be "
                "conveyed under the GPL", [SRC["fsf_matrix"]], None)
    return "incompatible", f"no GNU licence version is acceptable to both {inbound} and {target}", [SRC["fsf_matrix"]], None


def verdict(inbound, target, linking, options):
    info = LICENSES[inbound]
    kind = info["kind"]
    tkind = "proprietary" if target == "proprietary" else LICENSES[target]["kind"]
    strong_gnu_target = tkind in ("gpl", "agpl")
    if inbound == target:
        return "compatible", "same licence", [], None
    if kind == "permissive":
        notice = info["notice"]
        if inbound == "Apache-2.0" and target in ("GPL-2.0-only", "LGPL-2.1-only"):
            alt = " (LGPL-2.1 section 3 would allow converting the library to GPL-3.0)" if target.startswith("LGPL") else ""
            return ("incompatible", "the FSF lists Apache-2.0 as compatible with GPL version 3 but not version 2: its patent "
                    "termination and indemnification provisions are restrictions GPL-2.0 does not allow" + alt, [SRC["fsf"]], None)
        if inbound == "Apache-2.0" and target in ("GPL-2.0-or-later", "LGPL-2.1-or-later"):
            v3 = "GPL-3.0" if target.startswith("GPL") else "LGPL-3.0"
            return ("conditional", f"Apache-2.0 is compatible with version 3 only, so the combined work must be conveyed under {v3} or later",
                    [SRC["fsf"]], f"distribute the combined work under {v3}-or-later; {notice}")
        if tkind in ("gpl", "agpl", "lgpl"):
            return "compatible", f"{inbound} is a permissive licence the FSF lists as GPL-compatible", [SRC["fsf"]], notice
        if target == "Apache-2.0":
            return "compatible", f"{inbound} is in Apache category {info['apache']}: may be included in Apache-licensed works", [SRC["apache"]], notice
        return "compatible", f"{inbound} is permissive: it can be included in a work under any licence if its conditions are kept", [], notice
    if kind in ("weak_file", "weak_module"):
        if strong_gnu_target or tkind == "lgpl":
            if inbound == "MPL-2.0":
                if options.get("incompatible_with_secondary"):
                    return ("incompatible", "the MPL-2.0 code carries the Exhibit B 'Incompatible With Secondary Licenses' notice, "
                            "so section 3.3 does not apply", [SRC["mpl_faq"]], None)
                return ("compatible", "MPL-2.0 section 3.3 lets a Larger Work combine the code under a Secondary License "
                        "(GPL-2.0, LGPL-2.1, AGPL-3.0 or later versions); the MPL files stay available under MPL-2.0 too",
                        [SRC["mpl_faq"], SRC["fsf"]], "keep the MPL-2.0 notice; the MPL files remain dual-licensed")
            if inbound == "EPL-2.0":
                if options.get("secondary_license") and tkind == "gpl":
                    return ("compatible", "the initial contributor designated GPL-2.0-or-later as a Secondary License (Exhibit A), "
                            "so the code may be conveyed under the GPL", [SRC["epl2"], SRC["fsf"]], None)
                if options.get("secondary_license") and tkind == "agpl":
                    return ("conditional", "via the GPL-2.0-or-later Secondary License the code can be conveyed under GPL-3.0 and "
                            "combined with the AGPL-3.0 work (section 13)", [SRC["epl2"], SRC["agpl13"]], "the EPL part is conveyed under GPL-3.0")
                return ("incompatible", "the FSF lists EPL-2.0 as GPL-incompatible unless a Secondary License (GPL) is designated "
                        "in Exhibit A" + ("; that designation covers only the GPL, not the LGPL" if tkind == "lgpl" else ""),
                        [SRC["fsf"], SRC["epl2"]], None)
            return ("incompatible", "the FSF lists CDDL-1.0 as incompatible with the GNU GPL (its copyleft terms conflict)",
                    [SRC["fsf"]], None)
        scope = "module" if kind == "weak_module" else "file"
        where = {"MPL-2.0": "MPL-2.0 sections 3.2 and 3.3 (Larger Work)", "CDDL-1.0": SRC["cddl"], "EPL-2.0": SRC["epl2"]}[inbound]
        cond = (f"{scope}-level copyleft: the {inbound} {scope}s (and any {scope} you copy its code into, or modify) stay under "
                f"{inbound} and their source must be made available when distributed; the rest of the work can be under {target}")
        srcs = [where]
        if target == "Apache-2.0":
            cond += "; in Apache projects category B: binary form only, labelled"
            srcs.append(SRC["apache"])
        return "conditional", f"{inbound} allows combination in a larger work under another licence", srcs, cond
    if kind == "lgpl":
        if strong_gnu_target or (tkind == "lgpl" and linking == "source"):
            return gnu_combination(inbound, target)
        if linking == "source":
            return ("incompatible", f"code copied from an {inbound} library makes those files a modified library (or a GPL work "
                    "after LGPL conversion); they cannot be licensed under " + target, [SRC["lgpl21"] if "2.1" in inbound else SRC["lgpl3"]], None)
        legal = SRC["lgpl21"] if "2.1" in inbound else SRC["lgpl3"]
        if linking == "dynamic":
            cond = ("keep the library a separate shared library the user can replace; ship or offer its source and licence; "
                    "allow modification of the work for the user's own use and reverse engineering to debug it")
        else:
            cond = ("provide the work's object files (or source) so the user can relink it with a modified library; ship or offer "
                    "the library's source and licence; allow reverse engineering to debug modifications")
        return "conditional", f"a work that only links to an {inbound} library may be under another licence if the LGPL conditions are met", [legal], cond
    # gpl / agpl inbound
    if strong_gnu_target or tkind == "lgpl":
        return gnu_combination(inbound, target)
    link_note = "" if linking == "source" else " The FSF treats static and dynamic linking alike: both create a combined work."
    return ("incompatible", f"{inbound} requires the whole combined work to be conveyed under its terms; {target} cannot be used "
            "for it." + link_note, [SRC["fsf"], SRC["fsf_linking"]] if linking != "source" else [SRC["fsf"]], None)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if request.get("list_licenses"):
        print(json.dumps({"licenses": [{"id": k, "kind": v["kind"], "apache_category": v["apache"]} for k, v in LICENSES.items()],
                          "targets_also": ["proprietary"], "aliases": ALIASES, "reviewed": REVIEWED}))
        return
    target, target_note = spdx(request.get("target"), "target", allow_proprietary=True)
    default_linking = request.get("linking", "static")
    if default_linking not in LINKING:
        fail("linking must be source, static or dynamic")
    components = request.get("components")
    if not isinstance(components, list) or not components:
        fail("components must be a non-empty list of SPDX ids or objects")
    results, notes = [], [target_note] if target_note else []
    for i, comp in enumerate(components):
        if isinstance(comp, str):
            comp = {"license": comp}
        if not isinstance(comp, dict):
            fail(f"components[{i}] must be an SPDX id or an object with license")
        unknown = set(comp) - {"license", "linking", "name", "incompatible_with_secondary", "secondary_license"}
        if unknown:
            fail(f"components[{i}] has unknown keys: {', '.join(sorted(unknown))}")
        lic, note = spdx(comp.get("license"), f"components[{i}].license")
        if note:
            notes.append(note)
        linking = comp.get("linking", default_linking)
        if linking not in LINKING:
            fail(f"components[{i}].linking must be source, static or dynamic")
        for flag in ("incompatible_with_secondary", "secondary_license"):
            if flag in comp and not isinstance(comp[flag], bool):
                fail(f"components[{i}].{flag} must be true or false")
        status, reason, sources, conditions = verdict(lic, target, linking, comp)
        row = {"license": lic, "linking": linking, "verdict": status, "reason": reason, "sources": sources,
               "apache_category": LICENSES[lic]["apache"]}
        if conditions:
            row["conditions"] = conditions
        if "name" in comp:
            row["name"] = str(comp["name"])
        results.append(row)
    overall = max((r["verdict"] for r in results), key=RANK.get)
    out = {"target": target, "overall": overall, "components": results,
           "notes": notes + ["Running a program as a separate process and communicating at arm's length (pipes, sockets, "
                             "command line) is generally not a combination (FSF GPL FAQ, 'mere aggregation')."],
           "reviewed": REVIEWED, "disclaimer": DISCLAIMER}
    print(json.dumps(out))


if __name__ == "__main__":
    main()
