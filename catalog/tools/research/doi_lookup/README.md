# doi_lookup

Fetches a DOI's metadata from the Crossref REST API (`GET https://api.crossref.org/works/{doi}`)
and returns it normalised to CSL-JSON-like fields plus an APA 7 reference. Tier 3: it needs the
network (one HTTPS request per call, 15 s timeout, User-Agent
`AXLR-catalog-tool/1.0 (+https://axlr.underpassai.com)`, no key). With `mailto` the address is
added to the query string and the User-Agent, which puts the request in Crossref's "polite"
pool. Crossref only knows DOIs registered with Crossref; DataCite DOIs (many datasets) and
others answer "Resource not found." and the tool reports "DOI not found".

Offline or cached use: put the exact body Crossref returned in `response`. The tool then opens
no connection and parses that text with the same code as a live reply, which is also how the
tests run (the test sandbox has no network).

## Fields

`csl` holds `type` (Crossref types mapped to CSL: `journal-article` → `article-journal`,
`proceedings-article` → `paper-conference`, `book-chapter` → `chapter`, …; the original is in
`crossref_type`), `title` (JATS tags stripped, subtitle appended after a colon), `author`
(`family`/`given`, or `literal` for organisations, plus `ORCID`), `container-title`, `issued`
(`date-parts` from Crossref's `issued`, falling back to the published dates), `volume`,
`issue`, `page` (hyphen turned into an en dash), `DOI`, `URL`, `publisher`, `ISSN`.

## APA reference

APA 7 journal-article pattern: `Author, A. A., Author, B. B., & Author, C. C. (Year). Title.
Container, Volume(Issue), pages. https://doi.org/…`. Given names become initials (hyphenated
names keep the hyphen, `Jean-Paul` → `J.-P.`); up to 20 authors are listed with a comma before
the ampersand, and 21 or more give the first 19, an ellipsis and the last. A work with no
authors starts with its title; no date gives `(n.d.)`. `apa_html` italicises the container and
volume. The title is used in Crossref's case: APA wants sentence case for article titles, and
changing case automatically would break proper nouns, so check titles that publishers deposit
in title case.

## Tests

The tests feed real Crossref bodies fetched on 2026-10-18 (named in each test): 10.1037/ppm0000185,
which the APA Style website uses as its own journal-article example, and the expected string is
APA's published reference for it; and 10.1038/nature14539 (LeCun, Bengio & Hinton, *Nature*
521(7553), 436–444), trimmed of its reference, link, licence and assertion arrays. The 404 body
"Resource not found." and usage errors are also covered.

Limits: no content negotiation through doi.org, no reference lists, no other styles; editors,
translators and chapter page ranges inside books are not formatted.
