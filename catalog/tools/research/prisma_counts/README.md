# prisma_counts

Works out the numbers of a PRISMA 2020 flow diagram (Page et al., "The PRISMA 2020 statement",
BMJ 2021;372:n71, figure 1: the template for new systematic reviews that searched databases and
registers, and optionally other methods), checks that they add up, and writes the boxes as text
and as a Mermaid flowchart.

## Input

Either the counts at each stage:

- `databases` and `registers`: a total or `{source: n}` per database or register (PRISMA 2020
  asks for the per-source numbers where feasible; a note says so when only a total is given);
- `duplicates_removed`, `automation_removed`, `other_removed`: records removed before screening;
- `records_excluded` (title/abstract screening), `reports_not_retrieved`,
  `reports_excluded` (`{reason: n}` after full-text assessment, or a total);
- `studies_included` (several reports may describe one study; without it one report per study
  is assumed and noted);
- `other_methods`: `sources` (`{"Websites": n, "Citation searching": n}`),
  `reports_not_retrieved`, `reports_excluded`;
- optionally `records_screened`, `reports_sought`, `reports_assessed`, `reports_included` as you
  have them, to be checked against the flow;

or `records`: one object per record with `source`, `via` (`database`, `register`, `other`),
`removed` (`duplicate`, `automation`, `other`), `screening` (`include`, `exclude`), `retrieved`
(default true), `excluded_reason` and `study` (reports with the same `study` count as one
study). Records found by other methods skip removal and screening, as in the template.

## Arithmetic

```
records identified   = databases + registers
records screened     = identified − duplicates − automation − other removed
reports sought       = screened − records excluded
reports assessed     = sought − not retrieved
reports included     = assessed − Σ excluded reasons   (+ the same for other methods:
                       sought = identified, assessed = sought − not retrieved)
```

Every subtraction is checked to be non-negative, the studies may not outnumber the reports, and
each optional total you give is compared with the computed one. Problems are listed in `checks`
and `consistent` is false; the tool still returns the diagram so you can see where it breaks.

## Output

`counts` by stage, `checks`, `notes`, `boxes` (each box's id, stage and lines in the template's
wording: "Records identified from:", "Records removed before screening:", "Duplicate records
removed", "Records marked as ineligible by automation tools", "Records removed for other
reasons", "Records screened", "Records excluded", "Reports sought for retrieval", "Reports not
retrieved", "Reports assessed for eligibility", "Reports excluded:", "Studies included in
review", "Reports of included studies"), `text` (the boxes as plain text) and `mermaid` (a
`flowchart TD` with the template's arrows; quotes and angle brackets in names are escaped as
Mermaid entities).

## Tests

The template's box wording and a worked example whose numbers were checked by hand (220
identified, 45 removed, 175 screened, 120 excluded, 55 sought, 5 not retrieved, 50 assessed, 40
excluded for three reasons, plus 16 reports from other methods: 15 reports of 8 studies), the
same flow derived from per-record decisions, an inconsistent set of counts and the input errors.

## Limits

Only the template for new reviews: the updated-review template (studies from the previous
version) is not drawn. The Mermaid output places boxes by the arrows, not in PRISMA's exact
columns; the tests check its text, not a rendering. The footnote asking reviews to split
automation exclusions between humans and tools is left to the user.

Order: JSON objects carry no order, and the registry and the console pass them on with their keys
sorted, so sources and exclusion reasons given as objects are listed in name order.
