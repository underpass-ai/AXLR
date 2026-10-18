#!/usr/bin/env python3
"""PRISMA 2020 flow-diagram numbers for a new systematic review (databases and registers, and
optionally other methods), derived from the counts at each stage or from per-record decisions,
with consistency checks, the template's box texts and a Mermaid flowchart.

Input (JSON on stdin), counts:
  databases               {"MEDLINE": 120, ...} or a total
  registers               {"ClinicalTrials.gov": 15} or a total (default 0)
  duplicates_removed, automation_removed, other_removed       records removed before screening
  records_excluded        records excluded at title/abstract screening
  reports_not_retrieved   reports sought but not retrieved
  reports_excluded        {"reason": n, ...} reports excluded after full-text assessment
  studies_included        studies in the review (default: the number of included reports)
  records_screened, reports_sought, reports_assessed, reports_included   optional, checked
  other_methods           {"sources": {"Citation searching": 8, ...}, "reports_not_retrieved": n,
                           "reports_excluded": {"reason": n}}
or decisions:
  records  [{"source": "MEDLINE", "via": "database" | "register" | "other",
             "removed": "duplicate" | "automation" | "other",      (before screening)
             "screening": "include" | "exclude",
             "retrieved": true | false,
             "excluded_reason": "...",                              (after full-text assessment)
             "study": "id"}]                                        (groups reports into studies)
"""
import json
import sys

COUNT_FIELDS = ("databases", "registers", "duplicates_removed", "automation_removed", "other_removed",
                "records_excluded", "reports_not_retrieved", "reports_excluded", "studies_included",
                "records_screened", "reports_sought", "reports_assessed", "reports_included", "other_methods")
MAX_RECORDS = 200_000


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def count(value, field, default=None):
    if value is None:
        if default is None:
            fail(f"{field} is required")
        return default
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        fail(f"{field} must be a non-negative integer")
    return value


def breakdown(value, field, default_empty=False):
    """A per-source object or a plain total -> (dict or None, total)."""
    if value is None:
        if default_empty:
            return {}, 0
        fail(f"{field} is required")
    if isinstance(value, dict):
        for name, n in value.items():
            if not name.strip():
                fail(f"{field}: names must not be empty")
            count(n, f"{field}.{name}")
        return dict(value), sum(value.values())
    return None, count(value, field)


def from_records(records):
    if not isinstance(records, list) or not records:
        fail("records must be a non-empty list")
    if len(records) > MAX_RECORDS:
        fail(f"at most {MAX_RECORDS} records")
    db, reg, oth = {}, {}, {}
    removed = {"duplicate": 0, "automation": 0, "other": 0}
    excluded = not_retrieved = o_not_retrieved = 0
    reasons, o_reasons = {}, {}
    studies, reports = set(), 0
    for i, r in enumerate(records):
        if not isinstance(r, dict):
            fail(f"records[{i}] must be an object")
        extra = set(r) - {"source", "via", "removed", "screening", "retrieved", "excluded_reason", "study", "id"}
        if extra:
            fail(f"records[{i}]: unknown field(s) {', '.join(sorted(extra))}")
        source, via = r.get("source"), r.get("via", "database")
        if not isinstance(source, str) or not source.strip():
            fail(f"records[{i}].source must be a non-empty string")
        if via not in ("database", "register", "other"):
            fail(f"records[{i}].via must be database, register or other")
        bucket = {"database": db, "register": reg, "other": oth}[via]
        bucket[source] = bucket.get(source, 0) + 1
        if via != "other":
            rem = r.get("removed")
            if rem is not None:
                if rem not in removed:
                    fail(f"records[{i}].removed must be duplicate, automation or other")
                removed[rem] += 1
                continue
            screening = r.get("screening")
            if screening not in ("include", "exclude"):
                fail(f"records[{i}].screening must be include or exclude (or give removed)")
            if screening == "exclude":
                excluded += 1
                continue
        elif r.get("removed") is not None or r.get("screening") is not None:
            fail(f"records[{i}]: other methods go straight to retrieval (no removed or screening)")
        retrieved = r.get("retrieved", True)
        if not isinstance(retrieved, bool):
            fail(f"records[{i}].retrieved must be true or false")
        if not retrieved:
            if via == "other":
                o_not_retrieved += 1
            else:
                not_retrieved += 1
            continue
        reason = r.get("excluded_reason")
        if reason is not None:
            if not isinstance(reason, str) or not reason.strip():
                fail(f"records[{i}].excluded_reason must be a non-empty string")
            target = o_reasons if via == "other" else reasons
            target[reason] = target.get(reason, 0) + 1
            continue
        reports += 1
        study = r.get("study", r.get("id", f"record {i}"))
        studies.add(str(study))
    request = {"databases": db, "registers": reg, "duplicates_removed": removed["duplicate"],
               "automation_removed": removed["automation"], "other_removed": removed["other"],
               "records_excluded": excluded, "reports_not_retrieved": not_retrieved, "reports_excluded": reasons,
               "studies_included": len(studies)}
    if oth:
        request["other_methods"] = {"sources": oth, "reports_not_retrieved": o_not_retrieved, "reports_excluded": o_reasons}
    return request


def compute(request):
    db, n_db = breakdown(request.get("databases"), "databases")
    reg, n_reg = breakdown(request.get("registers"), "registers", default_empty=True)
    dup = count(request.get("duplicates_removed"), "duplicates_removed", 0)
    auto = count(request.get("automation_removed"), "automation_removed", 0)
    other_rm = count(request.get("other_removed"), "other_removed", 0)
    rec_excl = count(request.get("records_excluded"), "records_excluded")
    not_ret = count(request.get("reports_not_retrieved"), "reports_not_retrieved", 0)
    reasons, n_reasons = breakdown(request.get("reports_excluded"), "reports_excluded", default_empty=True)
    checks = []

    def check(name, ok, detail):
        checks.append({"check": name, "ok": bool(ok), "detail": detail})

    identified = n_db + n_reg
    removed = dup + auto + other_rm
    screened = identified - removed
    sought = screened - rec_excl
    assessed = sought - not_ret
    included_main = assessed - n_reasons
    check("removed before screening <= identified", screened >= 0, f"{removed} removed of {identified} identified")
    check("records excluded <= records screened", sought >= 0, f"{rec_excl} excluded of {screened} screened")
    check("reports not retrieved <= reports sought", assessed >= 0, f"{not_ret} not retrieved of {sought} sought")
    check("reports excluded <= reports assessed", included_main >= 0, f"{n_reasons} excluded of {assessed} assessed")
    other = None
    if request.get("other_methods") is not None:
        om = request["other_methods"]
        if not isinstance(om, dict):
            fail("other_methods must be an object")
        extra = set(om) - {"sources", "reports_not_retrieved", "reports_excluded"}
        if extra:
            fail(f"other_methods: unknown field(s) {', '.join(sorted(extra))}")
        sources, n_src = breakdown(om.get("sources"), "other_methods.sources")
        o_not = count(om.get("reports_not_retrieved"), "other_methods.reports_not_retrieved", 0)
        o_reasons, o_n_reasons = breakdown(om.get("reports_excluded"), "other_methods.reports_excluded", default_empty=True)
        o_assessed = n_src - o_not
        o_included = o_assessed - o_n_reasons
        check("other methods: not retrieved <= sought", o_assessed >= 0, f"{o_not} not retrieved of {n_src} sought")
        check("other methods: excluded <= assessed", o_included >= 0, f"{o_n_reasons} excluded of {o_assessed} assessed")
        other = {"records_identified": n_src, "sources": sources, "reports_sought": n_src, "reports_not_retrieved": o_not,
                 "reports_assessed": o_assessed, "reports_excluded": o_n_reasons, "reasons": o_reasons,
                 "reports_included": o_included}
    reports_included = included_main + (other["reports_included"] if other else 0)
    studies = request.get("studies_included")
    notes = []
    if studies is None:
        studies = reports_included
        notes.append("studies_included not given: one report per study assumed")
    else:
        studies = count(studies, "studies_included")
        check("studies included <= reports of included studies", studies <= reports_included,
              f"{studies} studies, {reports_included} reports")
        if reports_included > 0:
            check("at least one study when reports are included", studies >= 1, f"{studies} studies")
    for field, value in (("records_screened", screened), ("reports_sought", sought), ("reports_assessed", assessed),
                         ("reports_included", reports_included)):
        if request.get(field) is not None:
            given = count(request[field], field)
            check(f"{field} given matches the flow", given == value, f"given {given}, computed {value}")
    if db is None and n_db:
        notes.append("PRISMA 2020 suggests reporting the records from each database separately, if feasible")
    counts = {"identification": {"databases": n_db, "databases_by_source": db, "registers": n_reg, "registers_by_source": reg,
                                 "records_identified": identified, "duplicates_removed": dup,
                                 "automation_removed": auto, "other_removed": other_rm, "removed_before_screening": removed},
              "screening": {"records_screened": screened, "records_excluded": rec_excl, "reports_sought": sought,
                            "reports_not_retrieved": not_ret, "reports_assessed": assessed,
                            "reports_excluded": n_reasons, "reasons": reasons},
              "included": {"studies_included": studies, "reports_included": reports_included,
                           "reports_included_databases_registers": included_main}}
    if other:
        counts["other_methods"] = other
    return counts, checks, notes


def n(x):
    return f"(n = {x})"


def boxes(c):
    ident, scr, inc, oth = c["identification"], c["screening"], c["included"], c.get("other_methods")
    db_lines = [f"Databases {n(ident['databases'])}"] + ["  " + f"{k} {n(v)}" for k, v in (ident["databases_by_source"] or {}).items()]
    reg_lines = [f"Registers {n(ident['registers'])}"] + ["  " + f"{k} {n(v)}" for k, v in (ident["registers_by_source"] or {}).items()]

    def excluded_box(box_id, stage, reasons, total):
        if reasons:
            return {"id": box_id, "stage": stage, "lines": ["Reports excluded:"] + [f"{k} {n(v)}" for k, v in reasons.items()]}
        return {"id": box_id, "stage": stage, "lines": [f"Reports excluded {n(total)}"]}

    out = [
        {"id": "identified", "stage": "Identification", "lines": ["Records identified from:"] + db_lines + reg_lines},
        {"id": "removed", "stage": "Identification", "lines": ["Records removed before screening:",
                                                              f"Duplicate records removed {n(ident['duplicates_removed'])}",
                                                              f"Records marked as ineligible by automation tools {n(ident['automation_removed'])}",
                                                              f"Records removed for other reasons {n(ident['other_removed'])}"]},
        {"id": "screened", "stage": "Screening", "lines": [f"Records screened {n(scr['records_screened'])}"]},
        {"id": "excluded", "stage": "Screening", "lines": [f"Records excluded {n(scr['records_excluded'])}"]},
        {"id": "sought", "stage": "Screening", "lines": [f"Reports sought for retrieval {n(scr['reports_sought'])}"]},
        {"id": "not_retrieved", "stage": "Screening", "lines": [f"Reports not retrieved {n(scr['reports_not_retrieved'])}"]},
        {"id": "assessed", "stage": "Screening", "lines": [f"Reports assessed for eligibility {n(scr['reports_assessed'])}"]},
        excluded_box("reports_excluded", "Screening", scr["reasons"], scr["reports_excluded"]),
    ]
    if oth:
        out += [
            {"id": "other_identified", "stage": "Identification (other methods)", "lines": ["Records identified from:"]
                + ([f"{k} {n(v)}" for k, v in oth["sources"].items()] if oth["sources"] else [f"Other methods {n(oth['records_identified'])}"])},
            {"id": "other_sought", "stage": "Screening (other methods)", "lines": [f"Reports sought for retrieval {n(oth['reports_sought'])}"]},
            {"id": "other_not_retrieved", "stage": "Screening (other methods)", "lines": [f"Reports not retrieved {n(oth['reports_not_retrieved'])}"]},
            {"id": "other_assessed", "stage": "Screening (other methods)", "lines": [f"Reports assessed for eligibility {n(oth['reports_assessed'])}"]},
            excluded_box("other_excluded", "Screening (other methods)", oth["reasons"], oth["reports_excluded"]),
        ]
    out.append({"id": "included", "stage": "Included", "lines": [f"Studies included in review {n(inc['studies_included'])}",
                                                              f"Reports of included studies {n(inc['reports_included'])}"]})
    return out


def mermaid(box_list, has_other):
    esc = lambda s: s.replace("&", "#amp;").replace('"', "#quot;").replace("<", "#lt;").replace(">", "#gt;")
    lines = ["flowchart TD"]
    for b in box_list:
        lines.append(f'  {b["id"]}["{"<br/>".join(esc(line.strip()) for line in b["lines"])}"]')
    edges = [("identified", "removed"), ("identified", "screened"), ("screened", "excluded"), ("screened", "sought"),
             ("sought", "not_retrieved"), ("sought", "assessed"), ("assessed", "reports_excluded"), ("assessed", "included")]
    if has_other:
        edges += [("other_identified", "other_sought"), ("other_sought", "other_not_retrieved"),
                  ("other_sought", "other_assessed"), ("other_assessed", "other_excluded"), ("other_assessed", "included")]
    lines += [f"  {a} --> {b}" for a, b in edges]
    return "\n".join(lines) + "\n"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if "records" in request:
        if set(request) - {"records"}:
            fail("records cannot be combined with count fields")
        request = from_records(request["records"])
        mode = "records"
    else:
        extra = set(request) - set(COUNT_FIELDS)
        if extra:
            fail(f"unknown field(s): {', '.join(sorted(extra))}")
        mode = "counts"
    counts, checks, notes = compute(request)
    box_list = boxes(counts)
    text = []
    for b in box_list:
        text.append(f"[{b['stage']}]")
        text += ["  " + line for line in b["lines"]]
    out = {"template": "PRISMA 2020 flow diagram for new systematic reviews which included searches of databases and registers"
                       + (" and other sources" if "other_methods" in counts else " only"),
           "input_mode": mode, "counts": counts, "consistent": all(c["ok"] for c in checks), "checks": checks,
           "notes": notes, "boxes": box_list, "text": "\n".join(text) + "\n",
           "mermaid": mermaid(box_list, "other_methods" in counts),
           "reference": "Page MJ, McKenzie JE, Bossuyt PM, et al. The PRISMA 2020 statement: an updated guideline for reporting systematic reviews. BMJ 2021;372:n71"}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
