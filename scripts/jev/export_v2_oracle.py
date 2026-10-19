#!/usr/bin/env python3
"""Export the JEV lab's AXLR v2 questions as the oracle for AXLR's Go port.

Development tool only: AXLR never runs Python. It imports the pinned lab
candidate (jev_judge.axlr_v2, revision axlr-accuracy-r4) from a checkout
of underpass-ai/jev-judge-lab and writes one JSON line per dossier with the
question, options and ID mapping make_request builds, or the error it
raises. With --receipts it also attaches the lab's recorded v2 answers
(request hash, typed answer and the prediction evaluate derived), so the
Go replay can compare wire bytes and decisions.

The lab corpus is private: write its export outside this repository.
Only the synthetic dossiers under tui/application/testdata/jev_v2 are
public.

    python3 scripts/jev/synthetic_v2_dossiers.py ../jev-judge-lab > /tmp/dossiers.jsonl
    python3 scripts/jev/export_v2_oracle.py --lab ../jev-judge-lab \
        --dossiers /tmp/dossiers.jsonl --output tui/application/testdata/jev_v2/synthetic.jsonl
    python3 scripts/jev/export_v2_oracle.py --lab ../jev-judge-lab --lab-corpus \
        --receipts --output /private/path/oracle.jsonl
"""
import argparse
import hashlib
import json
import subprocess
import sys
from pathlib import Path

ORACLE_COMMIT = "64fe6d992ab4b2097c9eecf869dcec24c45b3d88"
CORPUS = ["data/axlr-v2/cases.jsonl", "data/axlr-v2/regression.jsonl"]
RECEIPTS = ["results/axlr-v2/dev-r4-typesafe.jsonl", "results/axlr-v2/test-r4-typesafe.jsonl",
            "results/axlr-v2/regression-r4-typesafe.jsonl"]


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--lab", required=True, type=Path)
    parser.add_argument("--dossiers", type=Path, help="JSONL of {id, family, dossier}")
    parser.add_argument("--lab-corpus", action="store_true", help="export the lab's v2 corpus")
    parser.add_argument("--receipts", action="store_true", help="attach the lab's v2 TypeSafe receipts")
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    lab = args.lab.resolve()
    head = subprocess.run(["git", "-C", str(lab), "rev-parse", "HEAD"], capture_output=True, text=True, check=True).stdout.strip()
    if head != ORACLE_COMMIT:
        sys.exit(f"lab checkout is at {head}, the oracle is {ORACLE_COMMIT}")
    sys.path.insert(0, str(lab / "src"))
    from jev_judge.axlr_v2 import VERSION, make_request

    rows = []
    if args.dossiers:
        rows += [json.loads(line) for line in args.dossiers.read_text().split("\n") if line.strip()]
    if args.lab_corpus:
        for name in CORPUS:
            rows += [json.loads(line) for line in (lab / name).read_text().split("\n") if line.strip()]
    receipts = {}
    if args.receipts:
        for name in RECEIPTS:
            for line in filter(str.strip, (lab / name).read_text().split("\n")):
                result = json.loads(line)
                arm = result["arms"]["v2"]
                receipt = arm["receipt"]
                receipts.setdefault(result["id"], []).append({
                    "source": name, "repeat": result["repeat"], "model": receipt.get("model"),
                    "request_sha256": receipt.get("request_sha256"), "answers": receipt.get("answers"),
                    "error": receipt.get("error"), "prediction": arm.get("prediction")})
    with args.output.open("w") as out:
        for row in rows:
            record = {"id": row["id"], "family": row["family"], "dossier": row["dossier"],
                      "version": VERSION, "oracle": ORACLE_COMMIT}
            try:
                question, mapping = make_request(row["family"], row["dossier"])
                record["question"] = question
                record["mapping"] = {k: v for k, v in mapping.items() if k != "none"}
            except Exception as exc:  # the oracle's refusal is part of the contract
                record["error"] = f"{type(exc).__name__}: {exc}"
            for key in ("go_only_error", "go_divergence"):
                if row.get(key):
                    record[key] = row[key]
            if row["id"] in receipts:
                record["receipts"] = receipts[row["id"]]
            out.write(json.dumps(record, ensure_ascii=False, sort_keys=True) + "\n")
    digest = hashlib.sha256(args.output.read_bytes()).hexdigest()
    print(json.dumps({"output": str(args.output), "rows": len(rows), "sha256": digest,
                      "with_receipts": sum(1 for r in rows if r["id"] in receipts)}))


if __name__ == "__main__":
    main()
