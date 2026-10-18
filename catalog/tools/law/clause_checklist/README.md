# clause_checklist

A review checklist for seven contract types, and an optional first pass over a contract text.

| `contract_type` | Clauses | Notes |
|:--|:--|:--|
| `nda` | 15 | definition, exclusions, purpose, compelled disclosure, duration, return, no licence, remedies |
| `services` | 20 | scope, service levels and acceptance, price, IP, warranties, liability cap, indemnities, insurance |
| `lease` | 16 | property, use, term, rent and updates, deposit, expenses, repairs, works, subletting, return |
| `employment` | 15 | position, contract type, probation, working time, salary, leave, collective agreement, non-compete |
| `sale_of_goods` | 14 | goods, delivery (Incoterms), risk and retention of title, inspection, warranty, CISG (Art. 6 exclusion) |
| `software_license` | 18 | grant, restrictions, support, IP indemnity, open-source components, audit, escrow |
| `dpa` | 19 | GDPR Art. 28(3): subject-matter and duration, nature and purpose, type of data and data subjects, controller's obligations and rights, points (a) to (h), infringing-instruction warning; plus transfers (Chapter V) and breach notice (Art. 33(2)) |

Each clause has an `id`, a title (English or Spanish with `language`), its purpose and the
questions a reviewer should ask. DPA clauses carry `legal_reference` and `mandatory` (true for
the Art. 28(3) contents), and the output lists `mandatory_contents` (a)–(h).

## Text check (heuristic)

With `text`, each clause is searched by English and Spanish keywords and headings on a
lower-cased, accent-free copy of the text (so "Duración" matches "duracion"). The first match
is reported as `found` with the matched words and the surrounding sentence; otherwise
`missing`. The summary counts found and missing clauses, and for a DPA the mandatory ones
missing. This only says whether a word was seen: a clause may be present but inadequate, or
present in other words. The output says so in `heuristic_note`.

## Tests

The known value is the text of GDPR Article 28(3): the DPA checklist must carry the four
chapeau contents and points (a) to (h) in order, each with its reference. Other cases check
a Spanish DPA excerpt and an English NDA excerpt (found and missing counts, a snippet), the
Spanish titles and usage errors.

## Limits

Purposes and questions are in English. Statutory rules that vary by country (lease terms,
employment law) are framed as questions, not stated. Not legal advice; the output carries
that disclaimer.
