#!/usr/bin/env python3
"""Colour conversions, WCAG 2.x contrast, nearest CSS named colour and palettes from a seed.

Input (JSON on stdin):
  operation   "convert" (default) | "contrast" | "nearest_named" | "palette"
  color       any of: "#rgb", "#rgba", "#rrggbb", "#rrggbbaa", "rgb(255 0 0)", "rgb(255, 0, 0, 0.5)",
              "hsl(120, 50%, 40%)", "hsv(120, 50%, 40%)", "cmyk(0%, 100%, 100%, 0%)", a CSS named colour,
              or {"r": 0-255, "g": 0-255, "b": 0-255}
  foreground, background   contrast (a translucent foreground is composited over the background)
  count       nearest_named: how many names, default 3 (1-20)
  scheme      palette: "complementary" | "triadic" | "analogous" | "split_complementary" |
              "tetradic" | "tints" | "shades" | "all" (default)
  steps       palette tints/shades: number of colours, default 5 (1-20)
"""
import json
import math
import re
import sys

# CSS Color Module Level 4, section 6.1 "Named Colors" (W3C), all 148 keywords
NAMED = {
    "aliceblue": "#f0f8ff", "antiquewhite": "#faebd7", "aqua": "#00ffff", "aquamarine": "#7fffd4",
    "azure": "#f0ffff", "beige": "#f5f5dc", "bisque": "#ffe4c4", "black": "#000000",
    "blanchedalmond": "#ffebcd", "blue": "#0000ff", "blueviolet": "#8a2be2", "brown": "#a52a2a",
    "burlywood": "#deb887", "cadetblue": "#5f9ea0", "chartreuse": "#7fff00", "chocolate": "#d2691e",
    "coral": "#ff7f50", "cornflowerblue": "#6495ed", "cornsilk": "#fff8dc", "crimson": "#dc143c",
    "cyan": "#00ffff", "darkblue": "#00008b", "darkcyan": "#008b8b", "darkgoldenrod": "#b8860b",
    "darkgray": "#a9a9a9", "darkgreen": "#006400", "darkgrey": "#a9a9a9", "darkkhaki": "#bdb76b",
    "darkmagenta": "#8b008b", "darkolivegreen": "#556b2f", "darkorange": "#ff8c00",
    "darkorchid": "#9932cc", "darkred": "#8b0000", "darksalmon": "#e9967a", "darkseagreen": "#8fbc8f",
    "darkslateblue": "#483d8b", "darkslategray": "#2f4f4f", "darkslategrey": "#2f4f4f",
    "darkturquoise": "#00ced1", "darkviolet": "#9400d3", "deeppink": "#ff1493",
    "deepskyblue": "#00bfff", "dimgray": "#696969", "dimgrey": "#696969", "dodgerblue": "#1e90ff",
    "firebrick": "#b22222", "floralwhite": "#fffaf0", "forestgreen": "#228b22", "fuchsia": "#ff00ff",
    "gainsboro": "#dcdcdc", "ghostwhite": "#f8f8ff", "gold": "#ffd700", "goldenrod": "#daa520",
    "gray": "#808080", "green": "#008000", "greenyellow": "#adff2f", "grey": "#808080",
    "honeydew": "#f0fff0", "hotpink": "#ff69b4", "indianred": "#cd5c5c", "indigo": "#4b0082",
    "ivory": "#fffff0", "khaki": "#f0e68c", "lavender": "#e6e6fa", "lavenderblush": "#fff0f5",
    "lawngreen": "#7cfc00", "lemonchiffon": "#fffacd", "lightblue": "#add8e6", "lightcoral": "#f08080",
    "lightcyan": "#e0ffff", "lightgoldenrodyellow": "#fafad2", "lightgray": "#d3d3d3",
    "lightgreen": "#90ee90", "lightgrey": "#d3d3d3", "lightpink": "#ffb6c1", "lightsalmon": "#ffa07a",
    "lightseagreen": "#20b2aa", "lightskyblue": "#87cefa", "lightslategray": "#778899",
    "lightslategrey": "#778899", "lightsteelblue": "#b0c4de", "lightyellow": "#ffffe0",
    "lime": "#00ff00", "limegreen": "#32cd32", "linen": "#faf0e6", "magenta": "#ff00ff",
    "maroon": "#800000", "mediumaquamarine": "#66cdaa", "mediumblue": "#0000cd",
    "mediumorchid": "#ba55d3", "mediumpurple": "#9370db", "mediumseagreen": "#3cb371",
    "mediumslateblue": "#7b68ee", "mediumspringgreen": "#00fa9a", "mediumturquoise": "#48d1cc",
    "mediumvioletred": "#c71585", "midnightblue": "#191970", "mintcream": "#f5fffa",
    "mistyrose": "#ffe4e1", "moccasin": "#ffe4b5", "navajowhite": "#ffdead", "navy": "#000080",
    "oldlace": "#fdf5e6", "olive": "#808000", "olivedrab": "#6b8e23", "orange": "#ffa500",
    "orangered": "#ff4500", "orchid": "#da70d6", "palegoldenrod": "#eee8aa", "palegreen": "#98fb98",
    "paleturquoise": "#afeeee", "palevioletred": "#db7093", "papayawhip": "#ffefd5",
    "peachpuff": "#ffdab9", "peru": "#cd853f", "pink": "#ffc0cb", "plum": "#dda0dd",
    "powderblue": "#b0e0e6", "purple": "#800080", "rebeccapurple": "#663399", "red": "#ff0000",
    "rosybrown": "#bc8f8f", "royalblue": "#4169e1", "saddlebrown": "#8b4513", "salmon": "#fa8072",
    "sandybrown": "#f4a460", "seagreen": "#2e8b57", "seashell": "#fff5ee", "sienna": "#a0522d",
    "silver": "#c0c0c0", "skyblue": "#87ceeb", "slateblue": "#6a5acd", "slategray": "#708090",
    "slategrey": "#708090", "snow": "#fffafa", "springgreen": "#00ff7f", "steelblue": "#4682b4",
    "tan": "#d2b48c", "teal": "#008080", "thistle": "#d8bfd8", "tomato": "#ff6347",
    "turquoise": "#40e0d0", "violet": "#ee82ee", "wheat": "#f5deb3", "white": "#ffffff",
    "whitesmoke": "#f5f5f5", "yellow": "#ffff00", "yellowgreen": "#9acd32",
}
SCHEMES = {"complementary": [0, 180], "triadic": [0, 120, 240], "analogous": [-30, 0, 30],
           "split_complementary": [0, 150, 210], "tetradic": [0, 90, 180, 270]}


class ColorError(Exception):
    pass


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def channel(text, scale):
    """A number or percentage -> 0..scale."""
    text = text.strip()
    if text.endswith("%"):
        value = float(text[:-1]) * scale / 100
    else:
        value = float(text)
    if not math.isfinite(value):
        raise ColorError("not a finite number")
    return value


def hsl_to_rgb(h, s, l):
    h = (h % 360) / 360
    if s == 0:
        return (l * 255,) * 3

    def hue(p, q, t):
        t %= 1
        if t < 1 / 6:
            return p + (q - p) * 6 * t
        if t < 1 / 2:
            return q
        if t < 2 / 3:
            return p + (q - p) * (2 / 3 - t) * 6
        return p
    q = l * (1 + s) if l < 0.5 else l + s - l * s
    p = 2 * l - q
    return tuple(255 * hue(p, q, h + d) for d in (1 / 3, 0, -1 / 3))


def hsv_to_rgb(h, s, v):
    h = (h % 360) / 60
    c = v * s
    x = c * (1 - abs(h % 2 - 1))
    m = v - c
    r, g, b = [(c, x, 0), (x, c, 0), (0, c, x), (0, x, c), (x, 0, c), (c, 0, x)][int(h) % 6]
    return ((r + m) * 255, (g + m) * 255, (b + m) * 255)


def parse(value, field):
    """Return (r, g, b, alpha) with r, g, b floats 0-255 and alpha 0-1."""
    if isinstance(value, dict):
        if set(value) - {"r", "g", "b", "a"} or not {"r", "g", "b"} <= set(value):
            raise ColorError(f"{field}: an object needs r, g, b (and optionally a)")
        parts = []
        for k in ("r", "g", "b"):
            v = value[k]
            if isinstance(v, bool) or not isinstance(v, (int, float)) or not 0 <= v <= 255:
                raise ColorError(f"{field}.{k} must be a number from 0 to 255")
            parts.append(float(v))
        a = value.get("a", 1)
        if isinstance(a, bool) or not isinstance(a, (int, float)) or not 0 <= a <= 1:
            raise ColorError(f"{field}.a must be from 0 to 1")
        return (*parts, float(a))
    if not isinstance(value, str) or not value.strip() or len(value) > 100:
        raise ColorError(f"{field} must be a colour string or an {{r, g, b}} object")
    text = value.strip().lower()
    if text in NAMED:
        text = NAMED[text]
    if text == "transparent":
        return (0.0, 0.0, 0.0, 0.0)
    m = re.fullmatch(r"#?([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})", text)
    if m:
        digits = m.group(1)
        if len(digits) in (3, 4):
            digits = "".join(ch * 2 for ch in digits)
        nums = [int(digits[i:i + 2], 16) for i in range(0, len(digits), 2)]
        return (float(nums[0]), float(nums[1]), float(nums[2]), nums[3] / 255 if len(nums) == 4 else 1.0)
    m = re.fullmatch(r"(rgba?|hsla?|hsv|hsb|cmyk)\((.*)\)", text)
    if not m:
        raise ColorError(f"{field}: '{value}' is not a colour this tool reads (hex, rgb(), hsl(), hsv(), cmyk() or a CSS name)")
    kind, body = m.group(1), m.group(2)
    alpha = 1.0
    if "/" in body:
        body, a = body.split("/", 1)
        alpha = channel(a, 1)
    parts = [p for p in re.split(r"[\s,]+", body.strip()) if p]
    try:
        if kind in ("rgb", "rgba"):
            if len(parts) == 4:
                alpha = channel(parts.pop(), 1)
            if len(parts) != 3:
                raise ColorError(f"{field}: rgb() takes three channels")
            r, g, b = (channel(p, 255) for p in parts)
        elif kind in ("hsl", "hsla", "hsv", "hsb"):
            if len(parts) == 4:
                alpha = channel(parts.pop(), 1)
            if len(parts) != 3:
                raise ColorError(f"{field}: {kind}() takes hue, saturation and lightness/value")
            h = float(parts[0].replace("deg", ""))
            s, l = channel(parts[1], 1), channel(parts[2], 1)
            if not (0 <= s <= 1 and 0 <= l <= 1):
                raise ColorError(f"{field}: saturation and lightness/value must be 0-100%")
            r, g, b = hsl_to_rgb(h, s, l) if kind.startswith("hsl") else hsv_to_rgb(h, s, l)
        else:
            if len(parts) != 4:
                raise ColorError(f"{field}: cmyk() takes four channels")
            c, mm, y, k = (channel(p, 1) for p in parts)
            if not all(0 <= x <= 1 for x in (c, mm, y, k)):
                raise ColorError(f"{field}: cmyk channels must be 0-100%")
            r, g, b = (255 * (1 - x) * (1 - k) for x in (c, mm, y))
    except ValueError:
        raise ColorError(f"{field}: '{value}' has a channel that is not a number")
    if not all(0 <= x <= 255.0000001 for x in (r, g, b)) or not 0 <= alpha <= 1:
        raise ColorError(f"{field}: channels out of range")
    return (min(r, 255.0), min(g, 255.0), min(b, 255.0), alpha)


def rgb8(rgb):
    return tuple(int(math.floor(x + 0.5 + 1e-9)) for x in rgb[:3])


def to_hex(rgb, alpha=1.0):
    r, g, b = rgb8(rgb)
    text = f"#{r:02x}{g:02x}{b:02x}"
    return text + (f"{round(alpha * 255):02x}" if alpha < 1 else "")


def to_hsl(rgb):
    r, g, b = (x / 255 for x in rgb[:3])
    hi, lo = max(r, g, b), min(r, g, b)
    l = (hi + lo) / 2
    d = hi - lo
    if d == 0:
        return 0.0, 0.0, l
    s = d / (1 - abs(2 * l - 1))
    return hue_of(r, g, b, hi, d), s, l


def hue_of(r, g, b, hi, d):
    if hi == r:
        h = ((g - b) / d) % 6
    elif hi == g:
        h = (b - r) / d + 2
    else:
        h = (r - g) / d + 4
    return (h * 60) % 360


def to_hsv(rgb):
    r, g, b = (x / 255 for x in rgb[:3])
    hi, lo = max(r, g, b), min(r, g, b)
    d = hi - lo
    return (hue_of(r, g, b, hi, d) if d else 0.0), (d / hi if hi else 0.0), hi


def to_cmyk(rgb):
    r, g, b = (x / 255 for x in rgb[:3])
    k = 1 - max(r, g, b)
    if k >= 1:
        return 0.0, 0.0, 0.0, 1.0
    return tuple((1 - x - k) / (1 - k) for x in (r, g, b)) + (k,)


def linear(c):
    """sRGB transfer function inverse; WCAG 2.x relative-luminance definition (threshold 0.04045)."""
    c /= 255
    return c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4


def luminance(rgb):
    r, g, b = (linear(x) for x in rgb8(rgb))
    return 0.2126 * r + 0.7152 * g + 0.0722 * b


def lab(rgb):
    r, g, b = (linear(x) for x in rgb[:3])
    x = (0.4124 * r + 0.3576 * g + 0.1805 * b) / 0.95047
    y = 0.2126 * r + 0.7152 * g + 0.0722 * b
    z = (0.0193 * r + 0.1192 * g + 0.9505 * b) / 1.08883
    f = lambda t: t ** (1 / 3) if t > (6 / 29) ** 3 else t / (3 * (6 / 29) ** 2) + 4 / 29
    fx, fy, fz = f(x), f(y), f(z)
    return 116 * fy - 16, 500 * (fx - fy), 200 * (fy - fz)


def nearest(rgb, count):
    target = lab(rgb8(rgb))
    scored = []
    for name, hx in NAMED.items():
        other = parse(hx, "name")
        scored.append((math.dist(target, lab(other)), name, hx))
    scored.sort()
    return [{"name": n, "hex": h, "delta_e76": round(d, 2)} for d, n, h in scored[:count]]


def describe(rgb, alpha=1.0):
    h, s, l = to_hsl(rgb)
    hv, sv, v = to_hsv(rgb)
    c, m, y, k = to_cmyk(rgb)
    r, g, b = rgb8(rgb)
    hx = to_hex(rgb)
    out = {"hex": to_hex(rgb, alpha), "rgb": {"r": r, "g": g, "b": b}, "css_rgb": f"rgb({r} {g} {b}" + (f" / {round(alpha, 3)})" if alpha < 1 else ")"),
           "hsl": {"h": round(h, 1), "s": round(s * 100, 1), "l": round(l * 100, 1)},
           "css_hsl": f"hsl({round(h, 1):g} {round(s * 100, 1):g}% {round(l * 100, 1):g}%)",
           "hsv": {"h": round(hv, 1), "s": round(sv * 100, 1), "v": round(v * 100, 1)},
           "cmyk": {"c": round(c * 100, 1), "m": round(m * 100, 1), "y": round(y * 100, 1), "k": round(k * 100, 1)},
           "relative_luminance": round(luminance(rgb), 6)}
    if alpha < 1:
        out["alpha"] = round(alpha, 4)
    names = [n for n, v in NAMED.items() if v == hx]
    out["css_names"] = names
    return out


def contrast(fg, bg):
    if bg[3] < 1:
        raise ColorError("background must be opaque")
    a = fg[3]
    mixed = tuple(fg[i] * a + bg[i] * (1 - a) for i in range(3))
    l1, l2 = luminance(mixed), luminance(bg)
    ratio = (max(l1, l2) + 0.05) / (min(l1, l2) + 0.05)
    verdict = lambda need: ratio >= need
    out = {"foreground": to_hex(fg, fg[3]), "background": to_hex(bg), "ratio": f"{math.floor(ratio * 100 + 0.5) / 100:.2f}:1",
           "ratio_value": round(ratio, 4), "foreground_luminance": round(l1, 6), "background_luminance": round(l2, 6),
           "wcag": {"AA_normal_text": verdict(4.5), "AA_large_text": verdict(3), "AAA_normal_text": verdict(7),
                    "AAA_large_text": verdict(4.5), "non_text_UI": verdict(3)},
           "rule": "WCAG 2.x: (L1 + 0.05) / (L2 + 0.05); AA 4.5:1 (large text 3:1), AAA 7:1 (large 4.5:1), UI components 3:1; verdicts use the unrounded ratio"}
    if a < 1:
        out["composited_foreground"] = to_hex(mixed)
    return out


def palette(rgb, scheme, steps):
    h, s, l = to_hsl(rgb)
    out = {}
    names = list(SCHEMES) + ["tints", "shades"] if scheme == "all" else [scheme]
    for name in names:
        if name in SCHEMES:
            out[name] = [to_hex(hsl_to_rgb(h + d, s, l)) for d in SCHEMES[name]]
        elif name == "tints":
            out[name] = [to_hex(tuple(x + (255 - x) * i / (steps + 1) for x in rgb[:3])) for i in range(1, steps + 1)]
        else:
            out[name] = [to_hex(tuple(x * (1 - i / (steps + 1)) for x in rgb[:3])) for i in range(1, steps + 1)]
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"operation", "color", "foreground", "background", "count", "scheme", "steps"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    operation = request.get("operation", "convert")
    try:
        if operation == "contrast":
            if "foreground" not in request or "background" not in request:
                fail("contrast needs foreground and background")
            out = contrast(parse(request["foreground"], "foreground"), parse(request["background"], "background"))
        elif operation in ("convert", "nearest_named", "palette"):
            if "color" not in request:
                fail(f"{operation} needs color")
            rgb = parse(request["color"], "color")
            if operation == "convert":
                out = describe(rgb, rgb[3])
                out["nearest_named"] = nearest(rgb, 1)[0]
            elif operation == "nearest_named":
                count = request.get("count", 3)
                if isinstance(count, bool) or not isinstance(count, int) or not 1 <= count <= 20:
                    fail("count must be an integer from 1 to 20")
                out = {"color": to_hex(rgb), "nearest": nearest(rgb, count),
                       "metric": "CIE76 delta E in CIELAB (D65), from sRGB"}
            else:
                scheme = request.get("scheme", "all")
                if scheme not in list(SCHEMES) + ["tints", "shades", "all"]:
                    fail("scheme must be complementary, triadic, analogous, split_complementary, tetradic, tints, shades or all")
                steps = request.get("steps", 5)
                if isinstance(steps, bool) or not isinstance(steps, int) or not 1 <= steps <= 20:
                    fail("steps must be an integer from 1 to 20")
                out = {"seed": to_hex(rgb), "palette": palette(rgb, scheme, steps),
                       "method": "hue rotations in HSL keeping saturation and lightness; tints mix with white and shades with black in equal steps"}
        else:
            fail("operation must be convert, contrast, nearest_named or palette")
    except ColorError as error:
        fail(str(error))
    out = {"operation": operation, **out}
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
