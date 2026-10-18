# dna_tools

Everyday sequence operations on DNA or RNA, with no package. The sequence may be pasted from a
FASTA file or a GenBank listing: header lines (`>`, `;`), whitespace and digits are dropped and
letters are upper-cased. Allowed letters are the IUPAC nucleotide codes `A C G T U R Y S W K M
B D H V N` and the gap `-`; a sequence with `U` and no `T` is RNA, and mixing both is an error.

## Operations

- `summary` (default): reverse complement, GC content, base counts, the frame +1 translation and
  the molecular weight.
- `reverse_complement`, `complement`: IUPAC pairs A/T (A/U in RNA), C/G, R/Y, K/M, B/V, D/H;
  S, W, N and `-` map to themselves.
- `gc_content`: `(G + C + S) / (G + C + S + A + T + U + W)`; other ambiguity codes and gaps
  are left out of both counts, which is Biopython's `gc_fraction` default ("remove").
- `transcribe` (DNA coding strand to mRNA, T→U) and `back_transcribe`.
- `translate`: NCBI translation table 1, the standard code, codon by codon from the frame's
  first base; `frames` 1 (+1), 3 (+1, +2, +3) or 6 (also −1, −2, −3 on the reverse complement).
  A codon with ambiguity codes is expanded: if every reading gives one amino acid it is used
  (`CTN` → L, `TAR` → stop), otherwise `X`. Stops are `*`; `to_stop` ends at the first one.
  Leftover bases at the end are not translated.
- `orfs`: in all six frames, an ORF runs from the first start codon after a stop (or after the
  start of the frame) to the next in-frame stop; nested starts inside it are not reported separately, as
  NCBI ORFfinder does by default. `starts` is `ATG` (default) or `table` (ATG, CTG and TTG, the
  initiation codons of table 1, all read as M). `min_aa` counts amino acids without the stop
  (default 30). `require_stop: false` also reports an ORF that runs off the end. `start` and
  `end` are 1-based positions on the input sequence for both strands (start < end), sorted by
  length.
- `molecular_weight`: Thermo Fisher Scientific's anhydrous formulas, single-stranded DNA
  `313.21 A + 304.2 T + 289.18 C + 329.21 G − 61.96` g/mol (5′-OH, no phosphate) and RNA
  `329.21 A + 306.17 U + 305.18 C + 345.21 G + 159.0` (5′ triphosphate, as from in vitro
  transcription); `strands: double` adds the complementary DNA strand. Ambiguity codes count
  as the mean of the bases they stand for.

## Tests

The Biopython Tutorial's own examples (the sequence `ATGGCCATTGTAATGGGCCGCTGAAAGGGTGCCCGATAG`:
reverse complement, transcription, translation `MAIVMGR*KGAR*` and `MAIVMGR` to the stop;
`gc_fraction` 0.46875 of its GC example), codons of NCBI table 1 including ambiguous ones,
the six frames, ORFs on both strands, molecular weights worked out by hand from the formula, and
the input errors.

## Limits

Only translation table 1 (no mitochondrial or other codes); no alignment, restriction sites or
melting temperature; molecular weights are approximations that ignore counter-ions,
modifications and terminal chemistry other than the stated one. Up to 1 000 000 bases.
