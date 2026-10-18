# holiday_table

Public holidays of a country and year, from a bundled table `holidays.json` that
`tools/catalog/data/holiday_table.py` computes from each holiday's legal rule rather than copying
a published list. The table covers 2024 to 2032 and these countries:

| Code | Holidays | Legal source of the rules |
|:--|:--|:--|
| `ES` | the national holidays common to all Spain (1 Jan, 6 Jan, Good Friday, 1 May, 15 Aug, 12 Oct, 1 Nov, 6 Dec, 8 Dec, 25 Dec); Holy Thursday and Easter Monday marked regional | Estatuto de los Trabajadores art. 37.2, RD 2001/1983 art. 45, yearly BOE resolution |
| `PT` | the 13 mandatory national holidays, Easter Sunday and Corpus Christi included | Código do Trabalho art. 234 (Lei 8/2016) |
| `FR` | the 11 jours fériés; Good Friday and 26 Dec as regional for Alsace-Moselle (`FR-57`, `FR-67`, `FR-68`) | Code du travail L3133-1, L3134-13 |
| `DE` | the nine holidays of every Land | Länder holiday laws; 3 Oct by the Einigungsvertrag |
| `IT` | the national holidays, with 4 October from 2026 | Legge 260/1949 as amended; Legge 151/2025 |
| `GB-ENG` | England and Wales bank holidays with substitute days | Banking and Financial Dealings Act 1971, Schedule 1 |
| `US` | the 11 federal holidays with observed days | 5 U.S.C. 6103, Executive Order 11582 |

## Method

The generator computes each date: fixed dates; Easter Sunday by the anonymous Gregorian algorithm
(Meeus/Jones/Butcher) and the days that hang off it (Holy Thursday −3, Good Friday −2, Easter
Monday +1, Ascension +39, Whit Monday +50, Corpus Christi +60); nth-weekday rules (third Monday of
January, last Monday of May, fourth Thursday of November); and the substitution rules:

- US: a holiday on Saturday is observed the preceding Friday, on Sunday the following Monday.
- England and Wales: a bank holiday on a weekend moves to the next weekday that is not already a
  bank holiday, so Christmas on Saturday gives Monday 27 and Tuesday 28 December, and Boxing Day
  on Saturday gives Monday 28.

Each row has the legal `date`, the `observed` substitute (or null), the `day_off`, local and
English names, the scope (`national` or `regional`), the regions where they are fixed by law, and
the rule's source. A row is listed for a year when its legal or its observed date falls in it,
so US New Year's Day 2028, observed on Friday 31 December 2027, appears in both 2027 and 2028.
`days_off` lists the days off that fall in the year asked; `by_date` maps them to English names.

`region` keeps the regional rows of that region and those whose regions vary from year to year
(Spain's Holy Thursday), which are flagged; `include_regional: false` drops all regional rows.

## Tests

US 2026 and 2027 match the federal holiday list on opm.gov; England and Wales 2026 and 2027
match gov.uk/bank-holidays, substitute days included. Easter 2026 is 5 April.

## Limits

Regional and municipal holidays (Spanish communities and towns, German Länder, Italian patron
saints, Portuguese municipalities, US states) are not in the table, nor any change made after
the table's date: one-off bank holidays by royal proclamation, a Spanish community moving a
Sunday holiday to Monday, a new law. The output carries the table's date and says so. Scotland
and Northern Ireland are not covered; Inauguration Day is not included.
