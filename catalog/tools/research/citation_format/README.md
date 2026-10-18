# citation_format

Formats references from CSL-JSON (the format Zotero, Mendeley and pandoc export) or BibTeX
into five styles:

| Style | Variant | Order |
|:--|:--|:--|
| `apa` | APA 7th edition reference list | by author |
| `mla` | MLA 9th edition works cited | by author |
| `chicago` | Chicago 17th edition, author-date | by author |
| `ieee` | IEEE Reference Guide, numbered | input order |
| `vancouver` | NLM *Citing Medicine* / ICMJE, numbered | input order |

It applies each style's author rules (APA lists up to 20 then ". . . last"; MLA uses "et al."
from three authors; Vancouver lists six then "et al"; BibTeX's `and others` becomes "et al."),
initials, page ranges (en dash; MLA keeps two digits of the second number, NLM drops repeated
digits, 284-287 → 284-7), editions as ordinals and DOIs as `https://doi.org/…` links. Every
entry comes back as `text` and as `markdown` with titles and journals in italics.

The tests are the APA manual's journal example and the ICMJE sample reference, character for
character.

Limits: titles are used as given (APA's sentence case and MLA's title case need knowledge of
proper nouns, so convert them yourself); no in-text citations; journal abbreviations are not
looked up; Chicago notes-bibliography is not offered.
