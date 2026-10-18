# md_convert

Converts Markdown, given inline, to one of three outputs.

**Parsing** follows the CommonMark Spec 0.31.2 for the parts it supports:

- blocks: ATX and setext headings, paragraphs, thematic breaks, indented and fenced code
  (the first word of the info string becomes `class="language-…"`), block quotes and bullet or
  ordered lists with lazy continuation lines, tight and loose lists, tabs as 4-column stops in
  indentation; GFM pipe tables with alignment and `\|` in cells;
- inlines: backslash escapes, entity and numeric references, code spans, emphasis and strong
  emphasis by the spec's delimiter-run algorithm (left/right-flanking runs, the rule of three),
  inline links and images with titles, URI and e-mail autolinks, hard breaks (two spaces or a
  backslash before the newline) and soft breaks. Link destinations are percent-encoded.

Not supported: link reference definitions and reference-style links (`[foo]` with `[foo]: /url`),
HTML blocks and raw inline HTML. Any HTML in the input is **escaped** and shown as text, and,
as cmark's `--safe` option does, link and image URLs with the schemes `javascript:`,
`vbscript:`, `file:` or `data:` (except `data:image/png|gif|jpeg|webp`) are emptied; both are
deliberate departures from the spec, so the HTML can be embedded without passing script through.

Run over every example of the CommonMark 0.31.2 spec, the converter reproduces 504 of 652
byte for byte: all of Tabs, Thematic breaks, ATX and setext headings, indented and fenced code,
Paragraphs, Block quotes, List items, Autolinks, Soft line breaks and Textual content; 129/132
of Emphasis, 23/26 of Lists, 21/22 of Code spans; the misses are the raw-HTML, HTML-block and
reference-link examples. It also passes the eight table examples (198–205) of the GFM spec
0.29-gfm. The tests cite fifteen CommonMark examples by number (12, 25, 62, 80, 107, 119, 228,
301, 306, 350, 402, 505, 572, 594, 633) and GFM examples 198–200.

**Outputs** (`to`):

- `html`: the HTML, one block per line, as in the spec's examples.
- `text`: plain text: headings and paragraphs as lines, `- ` and `1. ` list markers, code
  blocks verbatim, `> ` before quoted lines, tables as tab-separated rows, links as
  `text (url)`, images as `[image: alt]`; blocks separated by a blank line.
- `slides`: an outline for a deck. A heading of level `split_level` (default 2) or higher starts
  a slide (a thematic break starts an untitled one); content before the first heading is an
  untitled first slide. Each slide lists its points: paragraphs, list items (nested items
  indented by two spaces), quotes, and placeholders for code blocks and tables. `outline` is the
  same as numbered text.

Limits: no footnotes, task lists, strikethrough or other GFM extensions besides tables; no
smart punctuation; one input document of at most 1,000,000 characters.
