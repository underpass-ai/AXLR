# qr_generate

A QR Code encoder written from scratch on the Python standard library, following ISO/IEC
18004:2015 (model 2, versions 1 to 40). It replaces the tier-2 `segno` (pure Python, BSD) for
plain QR Codes; what segno does and this tool does not is listed under Limits.

## Method

1. **Mode.** `auto` picks numeric (digits only: groups of 3 digits in 10 bits), alphanumeric
   (0–9, A–Z, space, `$%*+-./:`: pairs in 11 bits) or byte (the UTF-8 bytes, 8 bits each). One
   segment per symbol. With `eci` a byte segment is preceded by ECI 000026 (UTF-8).
2. **Version.** The smallest version whose data capacity at the chosen level holds mode
   indicator + character count (10/12/14, 9/11/13 or 8/16/16 bits by version range) + data.
   `boost_error` then raises the level as far as the same version allows (segno's default).
3. **Codewords.** Terminator of up to four 0 bits, 0 bits to the byte boundary (only if the
   stream is not on one, ISO 7.4.10), then pad codewords 236 and 17 alternately.
4. **Error correction.** Reed-Solomon over GF(256) with the primitive polynomial
   x⁸ + x⁴ + x³ + x² + 1 (0x11D); the generator for n codewords is ∏ (x − αⁱ), i = 0…n−1. Data is
   split into the blocks of ISO Table 9 (short blocks first), each block gets its EC
   codewords, and data then EC codewords are interleaved column by column.
5. **Placement.** Finder patterns with separators, timing patterns, alignment patterns (centres
   from the ISO Annex E rule), the dark module at (4V + 9, 8), reserved format and version
   areas; codeword bits go in two-module columns upward and downward from the bottom right,
   skipping the vertical timing column; remainder bits are 0.
6. **Masking.** All eight mask patterns are applied to the data modules and scored with the four
   penalty rules of ISO 7.8.3: N1 = 3 + (run − 5) for each run of five or more same-colour
   modules in a row or column, N2 = 3 for each 2×2 block of one colour, N3 = 40 for each
   1:1:3:1:1 finder-like pattern with four light modules on one side (outside the symbol counts
   as light; overlapping hits are counted once), N4 = 10 × ⌊|dark % − 50| / 5⌋. As in segno,
   the masks are scored before format and version information are written (ISO 7.1, steps 6
   and 7), with those areas light; the lowest total wins, the lower mask number on a tie.
7. **Format and version information.** Level bits (L 01, M 00, Q 11, H 10) and the mask in a
   BCH(15,5) code (generator 0x537) XORed with 101010000010010; for version 7 and up, the
   version in a BCH(18,6) code (generator 0x1F25).

Outputs: `svg` (one path, `crispEdges`, quiet zone `border`, colours `dark`/`light`), `text`
(two characters `██` per dark module) and `matrix` (rows of 0/1 without quiet zone, 1 = dark).
`details` adds the data and EC codewords per block, the interleaved sequence, the generator's α
exponents, the format and version bit strings, the alignment centres and all eight penalties.

## Tests and checks

The tests pin published values: the Thonky QR Code Tutorial's HELLO WORLD example (data and EC
codewords at 1-M, data codewords at 1-Q, the 10-codeword generator polynomial, the L/mask 4
format string, format table entries, the version 7 and 40 information strings), the ISO/IEC
18004 Annex I example `01234567` at 1-M, ISO Table 7's 7089 digits at 40-L, and segno 1.6.6's
own output for a few inputs (version, mask, and the whole 21×21 matrix of HELLO WORLD).

Checked once while writing it, outside the test suite: for all 40 versions × 4 levels × 3
modes (480 symbols) the whole matrix and the chosen mask equal segno 1.6.6's; the capacity
boundaries (largest input per version, level and mode, and one more) give the same versions
as segno in all 960 cases; and 13 symbols up to 40-H, UTF-8 with and without ECI, decode to
their input with the zxing-cpp reader.

One difference from segno 1.6.6: when the bit stream ends on a byte boundary after the
terminator, which is every byte-mode symbol with room for the full terminator, segno 1.6.6
adds a further 0x00 codeword before the pad codewords (its `write_padding_bits` adds
`8 − length % 8` bits even when that is 8). ISO 7.4.10 adds padding bits only when the stream
does not end at a codeword boundary, so this tool does not; both symbols decode to the same
text, but their matrices and sometimes the chosen mask differ. The 480-symbol comparison above
emulated segno's padding to compare everything else.

Byte mode always writes UTF-8. ISO's default interpretation for byte mode is ISO-8859-1 and
segno uses it when the text fits; for ASCII the bytes are the same, and common readers detect
UTF-8 (`eci: true` declares it explicitly).

## Limits

No Kanji mode, no mixed-mode segmentation (a long number inside text is sent in byte mode),
no Micro QR, rMQR, structured append or FNC1, no PNG output.
