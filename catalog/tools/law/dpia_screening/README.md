# dpia_screening

First-pass screening of whether a processing operation needs a data protection impact
assessment (DPIA) under GDPR Article 35.

## Method

1. **Article 35(3)** (`art35_3`): (a) systematic and extensive automated evaluation, including
   profiling, with legal or similarly significant effects; (b) large-scale processing of
   special categories (Art. 9(1)) or criminal-offence data (Art. 10); (c) large-scale
   systematic monitoring of a publicly accessible area. Any of them: `required`.
2. **Recital 91** (`recital_91`): patients' or clients' data processed by an individual
   physician, other health care professional or lawyer is not large scale and a DPIA should not
   be mandatory: `not_mandatory_recital_91`.
3. **WP248 rev.01 criteria**: evaluation or scoring; automated decisions with legal or similar
   effect; systematic monitoring; sensitive or highly personal data; large scale; matching or
   combining datasets; vulnerable data subjects; innovative technology; processing that
   prevents exercising a right or using a service. Two or more: `likely_required` ("in most
   cases"); one: `not_likely_required`, with WP248's caveat that one can be enough in some
   cases; none: `not_indicated`.

Criteria come from `criteria` (booleans) or, for those not set, from keyword hints in
`description` (English and Spanish, accent-insensitive); hinted criteria are labelled so the
user can confirm them. When the criteria met point to an Article 35(3) case (e.g. sensitive
data and large scale), `art35_3_possible` says which to check. With `country`, the output names
the supervisory authority whose Article 35(4) list must be checked (for Spain, the AEPD's list
of processing types that require an EIPD).

## Tests

Known values are WP248's own worked examples: the hospital processing patients' genetic and
health data (sensitive data, vulnerable data subjects, large scale: DPIA required); the company
monitoring its employees (systematic monitoring, vulnerable data subjects: required); the
online magazine's daily-digest mailing list (large scale only: not required); and the
individual physician (Recital 91: not required). Also an Article 35(3)(c) case, a Spanish
description, an override of a keyword hint and usage errors.

## Limits

Keyword hints are a heuristic and miss or over-read context. National lists are not bundled
(they change and differ by authority). Not legal advice; the output carries that disclaimer.
