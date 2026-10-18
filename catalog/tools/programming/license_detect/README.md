# license_detect

Says which SPDX licence a LICENSE, COPYING or README text is, by comparing it with
fingerprints of 28 common licences: MIT, MIT-0, BSD-2-Clause, BSD-3-Clause, 0BSD, ISC,
Apache-2.0, GPL-2.0, GPL-3.0, LGPL-2.1, LGPL-3.0, AGPL-3.0, MPL-2.0, EPL-1.0, EPL-2.0,
EUPL-1.2, Unlicense, CC0-1.0, BSL-1.0, Zlib, CC-BY-4.0, CC-BY-SA-4.0, Artistic-2.0, WTFPL,
OFL-1.1, PostgreSQL, BlueOak-1.0.0 and UPL-1.0, plus the standard notices (headers) of
Apache-2.0, MPL-2.0 and the GPL family.

## Method

1. Normalisation, after the SPDX License List Matching Guidelines: lower case; lines that
   start with `Copyright`, `(c)` or `©` (after comment markers) and "All rights reserved" are
   dropped; `&` is "and"; `http://` and `https://` are ignored; quotes, dashes, punctuation
   and all whitespace differences vanish (only words remain); the equivalent words of SPDX's
   `equivalentwords.txt` are unified (licence/license, organisation/organization,
   sub-license/sublicense, copyright owner/copyright holder…).
2. Fingerprints: every run of 5 consecutive words (a shingle) is hashed with SHA-1 and the
   first 6 hex digits are kept. For long licences only the hashes divisible by a per-licence
   modulus are stored (GPL-3.0 keeps one in a few), and the query is sampled with the same
   rule, so the comparison is unbiased while `fingerprints.json` stays at about 180 KB.
3. Scores, per licence: recall (the share of the licence's shingles found in the text),
   precision (the share of the text's shingles that belong to the licence) and the Dice
   score 2·|both| / (|licence| + |text|), computed exactly with fractions and printed to three
   decimals. The licences are ranked by Dice.
4. Decision: Dice ≥ 0.85 is a `license` match with high confidence, ≥ 0.60 medium (an edited
   or partial text). Otherwise, when a standard header is found (recall ≥ 0.60) and the text
   covers little of any full licence, it is a `notice`: the text says the code is under the
   licence but is not the licence itself (for the GPL family, "or (at your option) any later
   version" picks the `-or-later` id). A licence fully contained in a much longer text is
   `license_within_larger_text`. A text with only an `SPDX-License-Identifier:` line gives
   kind `identifier`. Anything else is `unknown`.

The GPL, LGPL and AGPL texts are the same for the `-only` and `-or-later` identifiers, so a
full text match names the `-only` id and lists both in `possible_ids`: the notice in the
source files decides. Each answer also lists the closest alternatives with their scores and
the data release.

`fingerprints.json` is written by `tools/catalog/data/license_detect.py` from the SPDX License
List data (github.com/spdx/license-list-data, release v3.29.0, `json/details/<id>.json`,
fetched on 18 October 2026; the SPDX project publishes the list under CC0-1.0). Only hashes
are bundled, never the licence texts. A newer list release is a new version of the tool.

## Tests

The texts as published by SPDX (v3.29.0): MIT and Apache-2.0 (with its appendix) score 1.000;
the GPL-3.0-or-later and GPL-3.0-only standard headers are reported as notices with the right
id; BSD-3-Clause ranks BSD-2-Clause second; an MIT text with a real copyright line, rewrapped
lines, "Licence" and "sub-license" still matches (the normalisation rules); an
`SPDX-License-Identifier` line and a home-made licence.

Limits: a similarity score is not a legal opinion (the output carries a disclaimer); only the
28 bundled licences can be found; licences that differ in a few words (MIT and MIT-0, ISC and
0BSD, BSD-2 and BSD-3) are told apart by those words, so heavy edits can swap them; dual
licensing and licence exceptions (such as the GCC or Classpath exceptions) are not parsed.
