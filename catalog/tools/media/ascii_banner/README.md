# ascii_banner

Draws text as a large banner in a block font bundled in the program, or puts plain text in a box.

## The font

"AXLR block 5" is an original font made for this tool and licensed MIT with it: five rows high,
letters 5 columns wide (I is 3), digits 5 (1 is 3), and the punctuation `space . , ! ? - + = :
; ' " / \ ( ) [ ] _ * # < > %` from 1 to 5 columns. Each glyph is a list of five strings in which
`#` marks ink, so the drawing in the source is exactly what is printed. Lower case is drawn as
capitals. A character the font lacks becomes `?` with a warning (`unknown: "replace"`, the
default) or is an error (`unknown: "error"`).

## Banner mode

Glyphs are placed side by side with `spacing` blank columns between them (default 1), drawn with
`fill` (default `#`; `"block"` gives `█`), and every row is padded to the full width. A line
wider than `max_width` (default 120) wraps at spaces onto further five-row lines (a single word
wider than that is cut); input lines (`\n`) and wrapped lines are separated by an empty row.
`border` frames the banner.

## Box mode

The text itself, one row per input line, aligned `left`, `center` or `right`, inside a border:
`ascii` (`+ - |`, the default here), `single`, `double`, `rounded` or `heavy` (Unicode box
drawing characters). `padding` sets the spaces between border and content on the left and right,
and half as many blank rows above and below.

The output has `lines`, `text` (the lines joined with newlines), `width`, `height` and
`warnings`.

## Tests

Renderings worked out by hand from the glyph table: `HI` (9 × 5), `hi` with another fill and no
spacing, `Hi!` in an ASCII border, `1+1=2` (21 columns), boxes with centred and right-aligned
text, word wrapping, a replaced character and the input errors.

## Limits

One font, upper case only, no kerning or FIGlet smushing; widths are counted in code points, so
wide (CJK) or combining characters in box mode misalign the right border.
