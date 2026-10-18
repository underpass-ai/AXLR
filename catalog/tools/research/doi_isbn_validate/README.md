# doi_isbn_validate

Checks and normalises bibliographic identifiers, one (`value`) or up to 1000 (`identifiers`).
`type` is detected (`auto`, the default) or forced to `doi`, `isbn`, `issn` or `ismn`. Hyphens,
spaces and labels such as `ISBN-13:` or `ISSN` are accepted; the output gives the compact form,
the check digit found and expected, the errors, and the conversions.

## Rules

- **ISBN-10** (ISO 2108 before 2007): weights 10 to 2 on the first nine digits; the check digit
  makes the weighted sum a multiple of 11, and 10 is written `X`. Converted to ISBN-13 by
  prefixing `978` and recomputing the check digit.
- **ISBN-13** and every EAN-13: weights 1, 3, 1, 3… on the first twelve digits; the check digit
  makes the sum a multiple of 10. The prefix must be `978` or `979`. A `978` ISBN converts to
  ISBN-10 (drop the prefix, recompute the mod-11 check digit); `979` ISBNs have no ISBN-10.
- **ISSN** (ISO 3297): weights 8 to 2 on seven digits, mod 11, `X` for 10; written `NNNN-NNNC`.
  Its EAN-13 is `977`, the seven digits, the two `issue` digits (default `00`) and an EAN check
  digit.
- **ISMN** (ISO 10957): an EAN-13 beginning `979-0`. The legacy 10-character form `M-…` has the
  same check digit, because `M` stands for `9790`; both forms are returned.
- **DOI** (ISO 26324, DOI Handbook): `10.` + a registrant code of digits (sub-codes after dots)
  + `/` + a non-empty suffix with no whitespace. Accepted bare, as `doi:…` or as a `doi.org` /
  `dx.doi.org` URL (percent-decoded). DOIs are case-insensitive, so `doi_lower` is the form to
  compare; `url` is the `https://doi.org/` resolver link with `%`, `"`, `#`, `?`, spaces, `< > { }
  ^ [ ] \` |` and non-ASCII characters percent-encoded. `crossref_pattern` says whether the DOI
  matches Crossref's recommended pattern `10.\d{4,9}/[-._;()/:A-Z0-9]+`, which covers nearly
  all modern DOIs; one outside it is valid syntax but worth a second look.

Detection: a `/`, `10.`, `doi:` or a doi.org URL means a DOI; an `ISBN`, `ISSN` or `ISMN` label
decides; then `M…` or `9790…` is an ISMN, eight characters an ISSN, anything else an ISBN.

## Tests

The examples of the Wikipedia articles: ISBN 978-0-306-40615-7 ↔ 0-306-40615-2, ISSN 0378-5955
and ISMN 979-0-060-11561-5 and M-2306-7118-7; the DOI Handbook's own DOI 10.1000/182 bare and as
an encoded URL; an ISBN-10 ending in X checked by hand; wrong check digits, a 979 ISBN, an old
SICI-style DOI and the syntax and input errors.

## Limits

Syntax and check digits only: nothing is looked up, so a well-formed DOI or ISBN may still not
exist. ISBN hyphen positions (which depend on the registration-group ranges) are neither checked
nor produced. DOI suffixes are not validated beyond the absence of whitespace, as the standard
allows any printable character.
