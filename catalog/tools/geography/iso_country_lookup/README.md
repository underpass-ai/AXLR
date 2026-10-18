# iso_country_lookup

Country codes and the facts that hang off them, from a bundled dated table. Give exactly one of:

| Input | Answer |
|:--|:--|
| `code` | the country with that ISO 3166-1 alpha-2 (`ES`), alpha-3 (`ESP`) or numeric (`724`, or `"4"` for `004`) code; case is ignored |
| `name` | the country whose English name matches, ignoring case, accents and the `’`/`'` difference; the CLDR name (`Türkiye`), CLDR's short or variant names (`Turkey`, `Ivory Coast`) and the tz database's name (`Samoa (western)`) all count. An inexact name returns the one country that contains it (`match: "partial"`) or the list of `candidates` (`match: "ambiguous"`) |
| `zone` | the countries that list that IANA time zone in `zone.tab` or `zone1970.tab`, with `in_zone_tab` / `in_zone1970_tab` and the tz comment |
| `currency` | the countries that use that ISO 4217 code as a current tender currency |
| `calling_code` | the countries with that ITU-T E.164 country code (`+7` is Kazakhstan and Russia, `+1` is the 26 NANP members) |
| `list: true` | all 249 countries with their three codes and English name |

A country answer has `alpha2`, `alpha3`, `numeric`, `name`, `other_names`, `tz_name`,
`currencies` (code, English name, ISO 4217 numeric code, CLDR fraction digits), `calling_codes`,
`zones`, `zones_1970` and `zone_comments`. Every answer carries `source`: the tz release, the
fetch date and each dataset's URL and licence.

## Where the table comes from

`countries.json` is written by `tools/catalog/data/iso_country_lookup.py`, which fetched on
2026-10-18:

- **IANA tz database 2026e** (`iso3166.tab`, `zone.tab`, `zone1970.tab`; public domain). The
  249 alpha-2 codes of `iso3166.tab` are the master list, so user-assigned codes such as Kosovo's
  `XK` are not included. `zones` is `zone.tab`, one row per country and zone, which gives each
  country its own zones (Liechtenstein: `Europe/Vaduz`). `zones_1970` is `zone1970.tab`
  inverted: that file lists one row per zone with all the countries that have agreed with it
  since 1970 (`Europe/Zurich`: CH, DE, LI), so Liechtenstein gets `Europe/Zurich`. Reverse
  lookup searches both.
- **Unicode CLDR 48.2** (Unicode License v3): `supplementalData.xml` `codeMappings`
  (`territoryCodes` alpha-3 and numeric per alpha-2), `currencyData` (the currencies of each
  region without an end date and not marked `tender="false"`, in CLDR's order: Panama PAB and
  USD, Bhutan BTN and INR; Antarctica has none), `currencyCodes` (ISO 4217 numeric) and
  `main/en.xml` (English names).
- **Unicode CLDR 33.1 `telephoneCodeData.xml`** (Unicode License). CLDR deprecated this file in
  CLDR 34 (2018) and no current CLDR release has calling codes; its references are the ITU's
  list of assigned E.164 country codes as of 15 September 2007 and wtng.info, and it was last
  edited in 2017. Only countries in the tz list are kept (the world-wide `001` codes such as
  +800 are dropped). Bouvet Island and Heard Island have no calling code or time zone.

Not used: the ISO 4217 lists that SIX (the ISO 4217 maintenance agency) publishes. SIX's terms
of use, read on 2026-10-18, say the site's content is provided "exclusively for personal use"
and "may not be reproduced or reused in any way or used for commercial purposes", which is not
a permissive licence; CLDR's `currencyData` is used instead. No permissively licensed list of
capitals was found, so there are no capitals.

## Tests

The known-value test checks Spain against the published assignments: ISO 3166-1 `ES` / `ESP` /
`724`, ISO 4217 euro `EUR` / `978`, ITU-T E.164 `+34`, and the tz database's three Spanish zones
(Europe/Madrid, Africa/Ceuta, Atlantic/Canary). Others check ISO 3166-1 codes for Germany and
Afghanistan (leading zeros), Turkey/Türkiye (`TUR`, `792`, `TRY` `949`, `+90`), the
`Europe/Zurich` example that `zone1970.tab` itself gives (CH, DE, LI), the shared E.164 code +7,
CHF 756 for Switzerland and Liechtenstein, name matching with accents and partial names, and the
usage errors.

## Limits

The table is a snapshot: a new tz or CLDR release, a currency change or a new calling code needs
the generator rerun (a new version of the tool). Calling codes are CLDR's 2017 data. The
alpha-3 and numeric codes and the currencies are CLDR's reading of ISO 3166-1 and ISO 4217, not
the ISO documents themselves. `cldr_fraction_digits` are CLDR's formatting digits, which can
differ from the ISO 4217 minor unit (CLDR gives IQD 0 where ISO 4217 gives 3). Names are English
only; disputed territories follow the tz database's and CLDR's choices, which take no position
on legal or territorial claims.

## Unicode licence notice

The CLDR 33.1 telephone file says "For terms of use, see http://www.unicode.org/copyright.html", which now applies the Unicode License v3 to Unicode data files, the same licence as CLDR 48.2.

The CLDR data in `countries.json` is used under the Unicode License v3, whose notice the file
carries in `unicode_license_notice`: Copyright © 2004-2026 Unicode, Inc. Permission is hereby
granted, free of charge, to any person obtaining a copy of data files and any associated
documentation (the "Data Files") or software and any associated documentation (the "Software")
to deal in the Data Files or Software without restriction, including without limitation the
rights to use, copy, modify, merge, publish, distribute, and/or sell copies of the Data Files or
Software, and to permit persons to whom the Data Files or Software are furnished to do so,
provided that either (a) this copyright and permission notice appear with all copies of the Data
Files or Software, or (b) this copyright and permission notice appear in associated
Documentation. THE DATA FILES AND SOFTWARE ARE PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.
The full text is in the `unicode_license_notice` field of `countries.json`.
