# srt_tools

Subtitle editing for SubRip (`.srt`) and WebVTT (`.vtt`) files given as `text`. The format is
detected from the `WEBVTT` header. Every operation returns the cues as JSON (index, identifier,
start, end, milliseconds, duration, settings, text) and, except `parse` and `overlaps`, the
rewritten file in `subtitles` (format `output`, by default the input's).

## Reading

Blocks are separated by blank lines (CRLF, CR and a BOM are accepted). A cue's timing line is
the first or second line of its block, `start --> end` with `HH:MM:SS,mmm` (SRT) or
`[HH:]MM:SS.mmm` (WebVTT; either separator is read in both); in WebVTT a line before it is the
cue identifier and text after it the cue settings. WebVTT `NOTE`, `STYLE` and `REGION` blocks
and the header are skipped. A block without a readable timing is skipped with a warning naming
its line.

## Operations

- `parse`: the cues, first start, last end and the number of overlaps.
- `shift`: add `offset_ms` to every time. A cue that would start before 0 is clamped to 0;
  one that would end before 0 is dropped; both are warned.
- `scale`: multiply every time by `factor`, or by `from_fps / to_fps`, exactly in fractions and
  rounded to the millisecond half up. Subtitles timed for a video at `from_fps` played at
  `to_fps` (frame n is at n/fps): 23.976 → 25 (PAL speed-up) multiplies by 0.959, 25 → 23.976
  by 1.0427.
- `convert`: SRT → WebVTT adds the header, writes `.` before the milliseconds, drops `<font>`
  tags, escapes `&` and stray `<` and replaces `-->` in text. WebVTT → SRT renumbers the cues,
  drops identifiers and cue settings, removes voice (`<v Name>`), class (`<c.x>`), language,
  ruby and timestamp tags (the text inside is kept), keeps `<b>`, `<i>`, `<u>`, and decodes
  `&amp;`, `&lt;`, `&gt;`, `&nbsp;`.
- `merge`: the cues of `text` and `text2` (same format; `offset_ms` shifts `text2`) in order of
  start time, ties keeping file order, renumbered; `sources` says which file each came from.
- `overlaps`: every pair of cues where the later one starts before the earlier one ends, with
  the overlap in milliseconds.

## Tests

A three-cue SRT and a WebVTT file with a header title, NOTE and STYLE blocks, an identifier,
settings, voice and class spans, worked through by hand: parsing, shifting with clamping and
dropping, the 25 → 23.976 fps rescale (factor 3125/2997), both conversions, a merge with an
offset, the overlap report and the error cases.

## Limits

No other formats (ASS/SSA, TTML, SBV); VTT regions, positioning and styling are not carried into
SRT; the speaker name of a voice span is dropped, not written as "Name:"; no splitting or
joining of cues.
