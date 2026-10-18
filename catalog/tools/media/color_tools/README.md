# color_tools

Colour arithmetic for design and accessibility work, with no package.

## Input colours

`#rgb`, `#rgba`, `#rrggbb`, `#rrggbbaa`; `rgb()`/`rgba()` with commas or spaces, numbers or
percentages and an optional alpha (`rgb(255 0 0 / 50%)`); `hsl()`, `hsv()` (or `hsb()`) with
hue in degrees; `cmyk()` in percent; any of the 148 CSS named colours (CSS Color Module Level 4,
section 6.1, embedded in the program) and `transparent`; or an object `{r, g, b, a}`.

## Operations

- `convert` (default): hex, RGB, CSS `rgb()` and `hsl()` strings, HSL, HSV and CMYK (the naive
  device-independent formula `k = 1 − max`, no ICC profile), the WCAG relative luminance, the CSS
  names with exactly this value and the nearest named colour. Channels are rounded half up, as
  CSS serialises them.
- `contrast`: WCAG 2.x relative luminance `L = 0.2126 R + 0.7152 G + 0.0722 B` over linearised
  sRGB channels (`c/12.92` up to 0.04045, else `((c + 0.055)/1.055)^2.4`; WCAG 2.0 printed the
  threshold as 0.03928, which gives the same result for 8-bit colours), and the ratio
  `(L1 + 0.05) / (L2 + 0.05)`. Verdicts: AA normal text 4.5:1, AA large text 3:1, AAA normal 7:1,
  AAA large 4.5:1 (success criteria 1.4.3 and 1.4.6) and non-text UI 3:1 (1.4.11). They are
  decided on the unrounded ratio: 4.478 is shown as `4.48:1` and still fails 4.5:1. A translucent
  foreground is first composited over the (opaque) background.
- `nearest_named`: the `count` closest CSS names by CIE76 ΔE in CIELAB (sRGB → XYZ with the
  IEC 61966-2-1 matrix → L*a*b* with the D65 white).
- `palette`: hue rotations in HSL that keep saturation and lightness (complementary 180°,
  triadic 120°/240°, analogous ±30°, split-complementary 150°/210°, tetradic 90° steps), tints
  mixing with white and shades mixing with black in `steps` equal steps (`scheme: all` gives
  every one).

## Tests

WCAG values: `#777777` on white is 4.48:1 and fails AA for normal text; black on white is
21:1; the luminance of pure red is 0.2126. CSS Color 4 table and conversions: rebeccapurple
`#663399` = `hsl(270 50% 40%)`, `hsl(120 100% 25%)` = green `#008000`; plus alpha, compositing,
nearest names, the palettes of pure red worked out by hand and the input errors.

## Limits

sRGB only (no Display P3, Lab or OKLCH input, no colour management); CIE76 rather than
CIEDE2000 for "nearest"; WCAG 2.x contrast, not the APCA method proposed for WCAG 3.
