#!/usr/bin/env python3
"""DNA/RNA sequence tools: reverse complement with IUPAC ambiguity codes, GC content,
transcription, translation with the standard genetic code (NCBI translation table 1) in 1, 3
or 6 frames, ORF search, and approximate molecular weight.

Input (JSON on stdin):
  sequence     DNA or RNA letters (IUPAC codes), FASTA header lines, whitespace and digits allowed
  operation    "summary" (default) | "reverse_complement" | "complement" | "gc_content" |
               "transcribe" | "back_transcribe" | "translate" | "orfs" | "molecular_weight"
  frames       translate: 1 (frame +1, default), 3 (+1 +2 +3) or 6 (also -1 -2 -3)
  to_stop      translate: stop at the first stop codon (default false)
  min_aa       orfs: minimum length in amino acids, stop not counted (default 30)
  starts       orfs: "ATG" (default) or "table" (ATG, CTG and TTG, the initiation codons of table 1)
  require_stop orfs: an ORF must end in a stop codon (default true); false also reports ORFs
               running off the end
  strands      molecular_weight: "single" (default) or "double"
"""
import json
import re
import sys

MAX_LENGTH = 1_000_000
# NCBI translation table 1, The Standard Code (https://www.ncbi.nlm.nih.gov/Taxonomy/Utils/wprintgc.cgi)
AAS = "FFLLSSSSYY**CC*WLLLLPPPPHHQQRRRRIIIMTTTTNNKKSSRRVVVVAAAADDEEGGGG"
STARTS = "---M------**--*----M---------------M----------------------------"
BASE1 = "TTTTTTTTTTTTTTTTCCCCCCCCCCCCCCCCAAAAAAAAAAAAAAAAGGGGGGGGGGGGGGGG"
BASE2 = "TTTTCCCCAAAAGGGGTTTTCCCCAAAAGGGGTTTTCCCCAAAAGGGGTTTTCCCCAAAAGGGG"
BASE3 = "TCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAGTCAG"
CODE = {a + b + c: aa for a, b, c, aa in zip(BASE1, BASE2, BASE3, AAS)}
TABLE_STARTS = {a + b + c for a, b, c, s in zip(BASE1, BASE2, BASE3, STARTS) if s == "M"}
IUPAC = {"A": "A", "C": "C", "G": "G", "T": "T", "R": "AG", "Y": "CT", "S": "CG", "W": "AT", "K": "GT",
         "M": "AC", "B": "CGT", "D": "AGT", "H": "ACT", "V": "ACG", "N": "ACGT"}
COMPLEMENT = {"A": "T", "C": "G", "G": "C", "T": "A", "R": "Y", "Y": "R", "S": "S", "W": "W", "K": "M",
              "M": "K", "B": "V", "V": "B", "D": "H", "H": "D", "N": "N", "-": "-"}
# anhydrous weights per nucleotide, g/mol (Thermo Fisher Scientific, "DNA and RNA molecular
# weights and conversions"): ssDNA sum - 61.96; ssRNA (5' triphosphate) sum + 159.0
DNA_MW = {"A": 313.21, "C": 289.18, "G": 329.21, "T": 304.2}
RNA_MW = {"A": 329.21, "C": 305.18, "G": 345.21, "U": 306.17}
OPERATIONS = ("summary", "reverse_complement", "complement", "gc_content", "transcribe", "back_transcribe",
              "translate", "orfs", "molecular_weight")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def clean(text):
    lines = [line for line in text.splitlines() if not line.lstrip().startswith((">", ";"))]
    seq = re.sub(r"[\s\d]", "", "".join(lines)).upper()
    if not seq:
        fail("sequence is empty")
    if len(seq) > MAX_LENGTH:
        fail(f"sequence may be at most {MAX_LENGTH} letters")
    bad = sorted(set(seq) - set("ACGTURYSWKMBDHVN-"))
    if bad:
        fail(f"sequence has letters that are not IUPAC nucleotide codes: {''.join(bad)}")
    if "T" in seq and "U" in seq:
        fail("sequence mixes T and U")
    return seq, ("RNA" if "U" in seq else "DNA")


def complement(dna):
    return "".join(COMPLEMENT[ch] for ch in dna)


def reverse_complement(dna):
    return complement(dna)[::-1]


def translate_codon(codon):
    options = {CODE[a + b + c] for a in IUPAC[codon[0]] for b in IUPAC[codon[1]] for c in IUPAC[codon[2]]}
    return options.pop() if len(options) == 1 else "X"


def translate(dna, offset, to_stop=False):
    protein = []
    for i in range(offset, len(dna) - 2, 3):
        aa = translate_codon(dna[i:i + 3])
        if aa == "*" and to_stop:
            break
        protein.append(aa)
    return "".join(protein)


def gc(seq):
    g = sum(seq.count(x) for x in "GCS")
    at = sum(seq.count(x) for x in "ATUW")
    return g, at


def frames_of(dna, count):
    out = [("+%d" % (k + 1), dna, k) for k in range(3 if count > 1 else 1)]
    if count == 6:
        rc = reverse_complement(dna)
        out += [("-%d" % (k + 1), rc, k) for k in range(3)]
    return out


def orfs(dna, min_aa, starts, require_stop):
    found, n = [], len(dna)
    for label, strand_seq, offset in frames_of(dna, 6):
        start = None
        protein = []
        for i in range(offset, n - 2, 3):
            codon = strand_seq[i:i + 3]
            aa = translate_codon(codon)
            if start is None:
                if codon in starts:
                    start, protein = i, ["M"]
                continue
            if aa == "*":
                if len(protein) >= min_aa:
                    found.append((label, start, i + 3, "".join(protein), True))
                start = None
                continue
            protein.append(aa)
        if start is not None and not require_stop and len(protein) >= min_aa:
            end = start + 3 * len(protein)
            found.append((label, start, end, "".join(protein), False))
    results = []
    for label, s, e, protein, stop in found:
        if label.startswith("+"):
            first, last = s + 1, e
        else:
            first, last = n - e + 1, n - s
        results.append({"frame": label, "strand": label[0], "start": first, "end": last, "length_nt": e - s,
                        "length_aa": len(protein), "has_stop": stop, "protein": protein})
    results.sort(key=lambda r: (-r["length_aa"], r["start"]))
    return results


def molecular_weight(seq, kind, strands):
    table = RNA_MW if kind == "RNA" else DNA_MW

    def single(s):
        total, ambiguous = 0.0, 0
        for ch in s:
            if ch == "-":
                continue
            options = [b if kind == "DNA" else b.replace("T", "U") for b in IUPAC[ch.replace("U", "T")]]
            if len(options) > 1:
                ambiguous += 1
            total += sum(table[b] for b in options) / len(options)
        return total + (159.0 if kind == "RNA" else -61.96), ambiguous

    mw, ambiguous = single(seq)
    out = {"type": kind, "strands": strands, "ambiguous_bases_averaged": ambiguous}
    if strands == "double":
        if kind == "RNA":
            fail("strands double is for DNA")
        other, _ = single(reverse_complement(seq))
        out["molecular_weight"] = round(mw + other, 2)
    else:
        out["molecular_weight"] = round(mw, 2)
    out["unit"] = "g/mol"
    out["formula"] = ("ssRNA: A 329.21 + U 306.17 + C 305.18 + G 345.21 per base, + 159.0 (5' triphosphate)"
                      if kind == "RNA" else "ssDNA: A 313.21 + T 304.2 + C 289.18 + G 329.21 per base, - 61.96 (5'-OH)")
    return out


def flag(request, name, default):
    value = request.get(name, default)
    if not isinstance(value, bool):
        fail(f"{name} must be true or false")
    return value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"sequence", "operation", "frames", "to_stop", "min_aa", "starts", "require_stop", "strands"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    text = request.get("sequence")
    if not isinstance(text, str):
        fail("sequence must be a string")
    operation = request.get("operation", "summary")
    if operation not in OPERATIONS:
        fail("operation must be one of " + ", ".join(OPERATIONS))
    seq, kind = clean(text)
    dna = seq.replace("U", "T")
    out = {"operation": operation, "type": kind, "length": len(seq)}

    def as_kind(s):
        return s.replace("T", "U") if kind == "RNA" else s

    needs_nogap = operation in ("translate", "orfs", "summary")
    if needs_nogap and "-" in seq:
        fail("remove gaps ('-') before translating")
    if operation in ("reverse_complement", "summary"):
        out["reverse_complement"] = as_kind(reverse_complement(dna))
    if operation == "complement":
        out["complement"] = as_kind(complement(dna))
    if operation in ("gc_content", "summary"):
        g, at = gc(seq)
        out["gc"] = {"gc_bases": g, "at_bases": at, "gc_fraction": round(g / (g + at), 6) if g + at else None,
                     "gc_percent": round(100 * g / (g + at), 2) if g + at else None,
                     "rule": "(G+C+S) / (G+C+S+A+T+U+W); other ambiguity codes and gaps are left out"}
        out["counts"] = {base: seq.count(base) for base in sorted(set(seq))}
    if operation == "transcribe":
        if kind == "RNA":
            fail("the sequence is already RNA")
        out["rna"] = dna.replace("T", "U")
    if operation == "back_transcribe":
        if kind == "DNA":
            fail("the sequence is already DNA")
        out["dna"] = dna
    if operation in ("translate", "summary"):
        frames = request.get("frames", 1)
        if frames not in (1, 3, 6) or isinstance(frames, bool):
            fail("frames must be 1, 3 or 6")
        to_stop = flag(request, "to_stop", False)
        out["table"] = "NCBI translation table 1 (standard code)"
        out["translations"] = [{"frame": label, "protein": translate(s, k, to_stop)} for label, s, k in frames_of(dna, frames)]
        if len(dna) % 3 and frames == 1:
            out["note"] = f"length is not a multiple of 3; the last {len(dna) % 3} base(s) are not translated"
    if operation == "orfs":
        min_aa = request.get("min_aa", 30)
        if isinstance(min_aa, bool) or not isinstance(min_aa, int) or not 1 <= min_aa <= 100000:
            fail("min_aa must be an integer from 1 to 100000")
        starts_name = request.get("starts", "ATG")
        if starts_name not in ("ATG", "table"):
            fail("starts must be ATG or table")
        starts = {"ATG"} if starts_name == "ATG" else TABLE_STARTS
        found = orfs(dna, min_aa, starts, flag(request, "require_stop", True))
        out.update(table="NCBI translation table 1 (standard code)", starts=sorted(starts), min_aa=min_aa,
                   count=len(found), orfs=found,
                   rule="from the first start codon after a stop to the next in-frame stop, in all six frames; start and end are 1-based positions on the input sequence (start < end on both strands)")
    if operation in ("molecular_weight", "summary"):
        strands = request.get("strands", "single")
        if strands not in ("single", "double"):
            fail("strands must be single or double")
        if "-" in seq:
            fail("remove gaps ('-') before computing a molecular weight")
        out["molecular_weight"] = molecular_weight(seq, kind, strands)
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
