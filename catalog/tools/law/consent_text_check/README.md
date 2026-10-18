# consent_text_check

Checks a privacy notice or consent text against the information GDPR requires: Article 13 when
the data is collected from the person (`source: data_subject`, default) or Article 14 when it is
obtained elsewhere (`source: third_party`).

| Article 13 | Article 14 | Item |
|:--|:--|:--|
| 13(1)(a) | 14(1)(a) | identity and contact details of the controller (both needed for `found`) |
| 13(1)(b) | 14(1)(b) | DPO contact details, where applicable |
| 13(1)(c) | 14(1)(c) | purposes and legal basis (both needed) |
| 13(1)(d) | 14(2)(b) | legitimate interests, where Art. 6(1)(f) is the basis |
| 13(1)(e) | 14(1)(e) | recipients or categories of recipients, if any |
| 13(1)(f) | 14(1)(f) | transfers to third countries and safeguards, where applicable |
| — | 14(1)(d) | categories of personal data |
| 13(2)(a) | 14(2)(a) | storage period or criteria |
| 13(2)(b) | 14(2)(c) | rights: access, rectification, erasure, restriction, objection, portability (each checked) |
| 13(2)(c) | 14(2)(d) | withdrawing consent, where consent is the basis |
| 13(2)(d) | 14(2)(e) | complaint to a supervisory authority |
| 13(2)(e) | — | statutory or contractual requirement and consequences of not providing the data |
| — | 14(2)(f) | source of the data, and whether publicly accessible |
| 13(2)(f) | 14(2)(g) | automated decision-making, including profiling |

## Method

The text is lower-cased and stripped of accents character by character (so positions still
match the original), and each item is searched with English and Spanish keyword patterns
(e.g. "legal basis", "legitimación", "base jurídica"; an e-mail address counts as contact
details). An item is `found`, `partial` (some of its parts or rights) or `missing`, with the
matched words and the surrounding sentence. Conditional items can be excluded with `applies`
(e.g. `{"transfers": false}`); left unknown, they are checked and a miss carries a note. Score:
found = 1, partial = 1/2, missing = 0, over the applicable items; `completeness_percent` is
exact to one decimal, rounded half up.

## Tests

The known value is the list of items in the Regulation: exactly 13(1)(a)–(f) and 13(2)(a)–(f),
and for Article 14, 14(1)(a)–(f) and 14(2)(a)–(g). Other cases: a Spanish first-layer notice in
the AEPD style (8 of 9 applicable items; the statutory/contractual requirement missing), the
AEPD snippet, an English notice with three of six rights (4.5 of 12 = 37.5 %), and usage
errors.

## Limits

A heuristic: a keyword can appear without the information being given (or be negated), and
information can be given in words the patterns miss. It does not judge clarity, layering or
whether a legal basis is valid. Not legal advice; the output carries that disclaimer.
