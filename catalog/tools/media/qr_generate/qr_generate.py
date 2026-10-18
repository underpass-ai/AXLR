#!/usr/bin/env python3
"""QR Code encoder (ISO/IEC 18004:2015, model 2, versions 1-40) on the standard library.

Input (JSON on stdin):
  data            text to encode
  error           "L" | "M" (default) | "Q" | "H"
  mode            "auto" (default) | "numeric" | "alphanumeric" | "byte"
  version         1..40 to force a version (default: the smallest that fits)
  mask            0..7 to force a mask (default: the lowest penalty, ISO 7.8.3)
  boost_error     raise the error level while the data still fits the chosen version
  eci             byte mode: add ECI 000026 (UTF-8) before the data
  outputs         any of "svg", "text", "matrix" (default all three)
  scale, border   SVG module size in px (default 4) and quiet zone in modules (default 4)
  dark, light     SVG colours (#rgb or #rrggbb; default #000000 and #ffffff)
  details         also return codewords, blocks, format and version bits, mask penalties
"""
import json
import re
import sys

LEVELS = {"L": 1, "M": 0, "Q": 3, "H": 2}  # format-information bits of each level
ORDER = ["L", "M", "Q", "H"]
ALNUM = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"
MODE_BITS = {"numeric": 0b0001, "alphanumeric": 0b0010, "byte": 0b0100}

# ISO/IEC 18004:2015 Table 9: error correction codewords per block and number of blocks,
# indexed by version (index 0 unused), for L, M, Q, H.
EC_PER_BLOCK = {
    "L": [0, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28,
          28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30],
    "M": [0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26,
          26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28],
    "Q": [0, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30,
          28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30],
    "H": [0, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28,
          30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30],
}
BLOCKS = {
    "L": [0, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8,
          8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25],
    "M": [0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16,
          17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49],
    "Q": [0, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20,
          23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68],
    "H": [0, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25,
          25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81],
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


# ---------------------------------------------------------------- GF(256) and Reed-Solomon
# Field with the primitive polynomial x^8 + x^4 + x^3 + x^2 + 1 (0x11D), generator alpha = 2.
EXP = [0] * 512
LOG = [0] * 256
_value = 1
for _i in range(255):
    EXP[_i] = _value
    LOG[_value] = _i
    _value <<= 1
    if _value & 0x100:
        _value ^= 0x11D
for _i in range(255, 512):
    EXP[_i] = EXP[_i - 255]


def gf_mul(a, b):
    if a == 0 or b == 0:
        return 0
    return EXP[LOG[a] + LOG[b]]


def rs_generator(degree):
    """g(x) = (x - a^0)(x - a^1)...(x - a^(degree-1)); coefficients from x^degree down."""
    poly = [1]
    for i in range(degree):
        nxt = [0] * (len(poly) + 1)
        for j, coef in enumerate(poly):
            nxt[j] ^= coef
            nxt[j + 1] ^= gf_mul(coef, EXP[i])
        poly = nxt
    return poly


def rs_remainder(data, generator):
    """Remainder of data(x) * x^n divided by the generator: the n EC codewords."""
    n = len(generator) - 1
    rem = [0] * n
    for byte in data:
        factor = byte ^ rem[0]
        rem = rem[1:] + [0]
        for i in range(n):
            rem[i] ^= gf_mul(generator[i + 1], factor)
    return rem


# ---------------------------------------------------------------- capacities
def alignment_positions(version):
    if version == 1:
        return []
    count = version // 7 + 2
    size = 17 + 4 * version
    step = (version * 8 + count * 3 + 5) // (count * 4 - 4) * 2
    return [6] + sorted(size - 7 - i * step for i in range(count - 1))


def raw_modules(version):
    """Modules left for data and EC after the function patterns and format/version areas."""
    result = (16 * version + 128) * version + 64
    if version >= 2:
        count = version // 7 + 2
        result -= (25 * count - 10) * count - 55
        if version >= 7:
            result -= 36
    return result


def total_codewords(version):
    return raw_modules(version) // 8


def data_codewords(version, level):
    return total_codewords(version) - EC_PER_BLOCK[level][version] * BLOCKS[level][version]


def count_bits(mode, version):
    group = 0 if version <= 9 else 1 if version <= 26 else 2
    return {"numeric": (10, 12, 14), "alphanumeric": (9, 11, 13), "byte": (8, 16, 16)}[mode][group]


# ---------------------------------------------------------------- data encoding
class Bits:
    def __init__(self):
        self.bits = []

    def put(self, value, length):
        for i in range(length - 1, -1, -1):
            self.bits.append((value >> i) & 1)

    def __len__(self):
        return len(self.bits)


def payload_bits(mode, data):
    bits = Bits()
    if mode == "numeric":
        for i in range(0, len(data), 3):
            chunk = data[i:i + 3]
            bits.put(int(chunk), {3: 10, 2: 7, 1: 4}[len(chunk)])
    elif mode == "alphanumeric":
        for i in range(0, len(data), 2):
            chunk = data[i:i + 2]
            if len(chunk) == 2:
                bits.put(ALNUM.index(chunk[0]) * 45 + ALNUM.index(chunk[1]), 11)
            else:
                bits.put(ALNUM.index(chunk), 6)
    else:
        for byte in data:
            bits.put(byte, 8)
    return bits.bits


def segment_bits(mode, data, version, eci):
    bits = Bits()
    if eci:
        bits.put(0b0111, 4)
        bits.put(26, 8)  # ECI 000026: UTF-8
    bits.put(MODE_BITS[mode], 4)
    count = len(data)
    if count >= 1 << count_bits(mode, version):
        return None
    bits.put(count, count_bits(mode, version))
    bits.bits.extend(payload_bits(mode, data))
    return bits.bits


def make_codewords(bits, version, level):
    capacity = data_codewords(version, level) * 8
    bits = list(bits)
    bits.extend([0] * min(4, capacity - len(bits)))  # terminator
    bits.extend([0] * (-len(bits) % 8))
    words = [int("".join(map(str, bits[i:i + 8])), 2) for i in range(0, len(bits), 8)]
    pad = 0
    while len(words) < capacity // 8:
        words.append((0xEC, 0x11)[pad % 2])
        pad += 1
    return words


def split_blocks(words, version, level):
    blocks = BLOCKS[level][version]
    total = total_codewords(version)
    ec_len = EC_PER_BLOCK[level][version]
    short_count = blocks - total % blocks
    short_len = total // blocks - ec_len  # data codewords in a short block
    generator = rs_generator(ec_len)
    data_blocks, ec_blocks, at = [], [], 0
    for b in range(blocks):
        length = short_len + (0 if b < short_count else 1)
        chunk = words[at:at + length]
        at += length
        data_blocks.append(chunk)
        ec_blocks.append(rs_remainder(chunk, generator))
    final = []
    for i in range(short_len + 1):
        for chunk in data_blocks:
            if i < len(chunk):
                final.append(chunk[i])
    for i in range(ec_len):
        for chunk in ec_blocks:
            final.append(chunk[i])
    return data_blocks, ec_blocks, final, generator


# ---------------------------------------------------------------- matrix
def format_bits(level, mask):
    data = (LEVELS[level] << 3) | mask
    rem = data
    for _ in range(10):
        rem = (rem << 1) ^ ((rem >> 9) * 0x537)
    return ((data << 10) | (rem & 0x3FF)) ^ 0x5412


def version_bits(version):
    rem = version
    for _ in range(12):
        rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
    return (version << 12) | (rem & 0xFFF)


class Symbol:
    def __init__(self, version):
        self.version = version
        self.size = 17 + 4 * version
        self.dark = [[0] * self.size for _ in range(self.size)]
        self.function = [[False] * self.size for _ in range(self.size)]
        self.reserved = set()  # format and version areas and the dark module
        self.draw_function_patterns()

    def put(self, row, col, value, reserved=False):
        self.dark[row][col] = 1 if value else 0
        self.function[row][col] = True
        if reserved:
            self.reserved.add((row, col))

    def draw_function_patterns(self):
        size = self.size
        for i in range(size):  # timing patterns
            self.put(6, i, i % 2 == 0)
            self.put(i, 6, i % 2 == 0)
        for row, col in ((3, 3), (3, size - 4), (size - 4, 3)):  # finders with separators
            for dr in range(-4, 5):
                for dc in range(-4, 5):
                    r, c = row + dr, col + dc
                    if 0 <= r < size and 0 <= c < size:
                        ring = max(abs(dr), abs(dc))
                        self.put(r, c, ring not in (2, 4))
        positions = alignment_positions(self.version)
        last = len(positions) - 1
        for i, row in enumerate(positions):
            for j, col in enumerate(positions):
                if (i, j) in ((0, 0), (0, last), (last, 0)):
                    continue  # these would overlap a finder pattern
                for dr in range(-2, 3):
                    for dc in range(-2, 3):
                        self.put(row + dr, col + dc, max(abs(dr), abs(dc)) != 1)
        self.draw_format(0)  # reserve the format areas (redrawn after masking)
        self.draw_version()

    def draw_format(self, bits):
        size = self.size
        bit = lambda i: (bits >> i) & 1
        for i in range(6):
            self.put(i, 8, bit(i), True)
        self.put(7, 8, bit(6), True)
        self.put(8, 8, bit(7), True)
        self.put(8, 7, bit(8), True)
        for i in range(9, 15):
            self.put(8, 14 - i, bit(i), True)
        for i in range(8):
            self.put(8, size - 1 - i, bit(i), True)
        for i in range(8, 15):
            self.put(size - 15 + i, 8, bit(i), True)
        self.put(size - 8, 8, 1, True)  # the dark module

    def draw_version(self):
        if self.version < 7:
            return
        bits = version_bits(self.version)
        for i in range(18):
            value = (bits >> i) & 1
            a, b = self.size - 11 + i % 3, i // 3
            self.put(a, b, value, True)
            self.put(b, a, value, True)

    def place(self, codewords):
        bits = [(byte >> (7 - k)) & 1 for byte in codewords for k in range(8)]
        size, at = self.size, 0
        right = size - 1
        while right >= 1:
            if right == 6:
                right = 5
            upward = ((right + 1) & 2) == 0
            for step in range(size):
                row = size - 1 - step if upward else step
                for col in (right, right - 1):
                    if not self.function[row][col]:
                        self.dark[row][col] = bits[at] if at < len(bits) else 0  # remainder bits are 0
                        at += 1
            right -= 2


MASKS = [
    lambda r, c: (r + c) % 2 == 0,
    lambda r, c: r % 2 == 0,
    lambda r, c: c % 3 == 0,
    lambda r, c: (r + c) % 3 == 0,
    lambda r, c: (r // 2 + c // 3) % 2 == 0,
    lambda r, c: (r * c) % 2 + (r * c) % 3 == 0,
    lambda r, c: ((r * c) % 2 + (r * c) % 3) % 2 == 0,
    lambda r, c: ((r + c) % 2 + (r * c) % 3) % 2 == 0,
]


def masked(symbol, mask, blank_reserved=False):
    test = MASKS[mask]
    matrix = [[v ^ (1 if (not symbol.function[r][c] and test(r, c)) else 0) for c, v in enumerate(row)]
              for r, row in enumerate(symbol.dark)]
    if blank_reserved:  # evaluated before format and version information are written (ISO 7.1 steps 6-7)
        for r, c in symbol.reserved:
            matrix[r][c] = 0
    return matrix


def finder_like(line):
    """N3: 1:1:3:1:1 dark-light-dark-light-dark with four light modules on either side;
    modules beyond the symbol's edge count as light (they are the quiet zone)."""
    n, count, i = len(line), 0, 0
    while i <= n - 7:
        if line[i:i + 7] == [1, 0, 1, 1, 1, 0, 1] and (not any(line[max(0, i - 4):i]) or not any(line[i + 7:i + 11])):
            count += 1
            i += 7  # a counted pattern is not counted again through an overlapping one
        else:
            i += 1
    return count


def penalty(matrix):
    size = len(matrix)
    columns = [[matrix[r][c] for r in range(size)] for c in range(size)]
    n1 = n2 = n3 = 0
    for line in matrix + columns:
        run, prev = 0, None
        for v in line + [None]:
            if v == prev:
                run += 1
            else:
                if run >= 5:
                    n1 += 3 + run - 5
                run, prev = 1, v
        n3 += 40 * finder_like(line)
    for r in range(size - 1):
        for c in range(size - 1):
            v = matrix[r][c]
            if v == matrix[r][c + 1] == matrix[r + 1][c] == matrix[r + 1][c + 1]:
                n2 += 3
    dark = sum(map(sum, matrix))
    k = abs(dark * 100 - 50 * size * size) // (5 * size * size)  # floor(|percent - 50| / 5)
    n4 = 10 * k
    return n1, n2, n3, n4


# ---------------------------------------------------------------- output
def svg(matrix, scale, border, dark, light):
    size = len(matrix) + 2 * border
    paths = []
    for r, row in enumerate(matrix):
        c = 0
        while c < len(row):
            if row[c]:
                start = c
                while c < len(row) and row[c]:
                    c += 1
                paths.append(f"M{start + border} {r + border}h{c - start}v1h-{c - start}z")
            else:
                c += 1
    px = size * scale
    return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{px}" height="{px}" viewBox="0 0 {size} {size}" '
            f'shape-rendering="crispEdges"><rect width="{size}" height="{size}" fill="{light}"/>'
            f'<path fill="{dark}" d="{"".join(paths)}"/></svg>')


def text_art(matrix, border):
    width = len(matrix) + 2 * border
    blank = "  " * width
    lines = [blank] * border
    for row in matrix:
        lines.append("  " * border + "".join("██" if v else "  " for v in row) + "  " * border)
    lines += [blank] * border
    return "\n".join(lines)


# ---------------------------------------------------------------- main
def choose_mode(text, requested):
    if requested == "auto":
        if text and all(ch in "0123456789" for ch in text):
            return "numeric"
        if text and all(ch in ALNUM for ch in text):
            return "alphanumeric"
        return "byte"
    if requested == "numeric" and not all(ch in "0123456789" for ch in text):
        fail("numeric mode takes only the digits 0-9")
    if requested == "alphanumeric" and not all(ch in ALNUM for ch in text):
        fail("alphanumeric mode takes only 0-9, A-Z (upper case), space and $ % * + - . / :")
    return requested


def option(request, key, default, check, message):
    value = request.get(key, default)
    if not check(value):
        fail(message)
    return value


def is_int(v, lo, hi):
    return isinstance(v, int) and not isinstance(v, bool) and lo <= v <= hi


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    try:
        json.dumps(request, ensure_ascii=False).encode("utf-8")
    except UnicodeEncodeError:
        fail("input contains an unpaired surrogate (not valid Unicode text)")
    known = {"data", "error", "mode", "version", "mask", "boost_error", "eci", "outputs", "scale", "border",
             "dark", "light", "details"}
    unknown = set(request) - known
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    text = request.get("data")
    if not isinstance(text, str) or not text:
        fail("data must be a non-empty string")
    level = option(request, "error", "M", lambda v: v in LEVELS, "error must be L, M, Q or H")
    mode = choose_mode(text, option(request, "mode", "auto", lambda v: v in ("auto", "numeric", "alphanumeric", "byte"),
                                    "mode must be auto, numeric, alphanumeric or byte"))
    forced_version = option(request, "version", None, lambda v: v is None or is_int(v, 1, 40), "version must be an integer from 1 to 40")
    forced_mask = option(request, "mask", None, lambda v: v is None or is_int(v, 0, 7), "mask must be an integer from 0 to 7")
    boost = option(request, "boost_error", False, lambda v: isinstance(v, bool), "boost_error must be true or false")
    eci = option(request, "eci", False, lambda v: isinstance(v, bool), "eci must be true or false")
    if eci and mode != "byte":
        fail("eci applies to byte mode only")
    outputs = option(request, "outputs", ["svg", "text", "matrix"],
                     lambda v: isinstance(v, list) and v and all(o in ("svg", "text", "matrix") for o in v),
                     "outputs must be a non-empty list of svg, text, matrix")
    scale = option(request, "scale", 4, lambda v: is_int(v, 1, 100), "scale must be an integer from 1 to 100")
    border = option(request, "border", 4, lambda v: is_int(v, 0, 40), "border must be an integer from 0 to 40")
    colour = lambda v: isinstance(v, str) and re.match(r"^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$", v)
    dark = option(request, "dark", "#000000", colour, "dark must be a colour #rgb or #rrggbb")
    light = option(request, "light", "#ffffff", colour, "light must be a colour #rgb or #rrggbb")
    details = option(request, "details", False, lambda v: isinstance(v, bool), "details must be true or false")

    content = text.encode("utf-8") if mode == "byte" else text
    versions = [forced_version] if forced_version else range(1, 41)
    chosen = None
    for version in versions:
        bits = segment_bits(mode, content, version, eci)
        if bits is not None and len(bits) <= data_codewords(version, level) * 8:
            chosen = version, bits
            break
    if chosen is None:
        where = f"version {forced_version}" if forced_version else "version 40"
        fail(f"data does not fit in a QR code ({where}, level {level}, {mode} mode, {len(content)} "
             f"{'bytes' if mode == 'byte' else 'characters'})")
    version, bits = chosen
    if boost:
        for better in ORDER[ORDER.index(level) + 1:]:
            if len(bits) <= data_codewords(version, better) * 8:
                level = better
    words = make_codewords(bits, version, level)
    data_blocks, ec_blocks, final, generator = split_blocks(words, version, level)
    symbol = Symbol(version)
    symbol.place(final)
    # ISO 7.8.3: evaluate each mask on the masked symbol before format information is added
    scores = [penalty(masked(symbol, m, blank_reserved=True)) for m in range(8)]
    mask = forced_mask if forced_mask is not None else min(range(8), key=lambda m: (sum(scores[m]), m))
    final_matrix = masked(symbol, mask)
    symbol.dark = final_matrix
    symbol.draw_format(format_bits(level, mask))
    matrix = symbol.dark

    result = {"version": version, "error": level, "mode": mode, "mask": mask, "size": symbol.size,
              "data_bits": len(bits), "capacity_bits": data_codewords(version, level) * 8,
              "encoding": "UTF-8" + (" with ECI 000026" if eci else "") if mode == "byte" else None}
    if result["encoding"] is None:
        del result["encoding"]
    if "matrix" in outputs:
        result["matrix"] = ["".join(map(str, row)) for row in matrix]
    if "text" in outputs:
        result["text"] = text_art(matrix, border)
    if "svg" in outputs:
        result["svg"] = svg(matrix, scale, border, dark, light)
    if details:
        result["details"] = {
            "data_codewords": words,
            "ec_codewords": ec_blocks[0] if len(ec_blocks) == 1 else None,
            "blocks": [{"data": d, "ec": e} for d, e in zip(data_blocks, ec_blocks)],
            "final_codewords": final,
            "generator_exponents": [LOG[c] for c in generator],
            "format_bits": format(format_bits(level, mask), "015b"),
            "version_bits": format(version_bits(version), "018b") if version >= 7 else None,
            "alignment_positions": alignment_positions(version),
            "penalties": [{"mask": m, "n1": s[0], "n2": s[1], "n3": s[2], "n4": s[3], "total": sum(s)}
                          for m, s in enumerate(scores)],
        }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
