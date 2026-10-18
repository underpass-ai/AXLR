# license_compat

Answers, for each component of a work, whether code under its licence can be combined into a
work distributed under the target licence (an SPDX id, or `proprietary`). Each component gets
`compatible`, `conditional` (allowed if the listed conditions are met) or `incompatible`, the
reason, the conditions, the sources and its Apache category; `overall` is the worst verdict.

Known licences: MIT, MIT-0, BSD-2-Clause, BSD-3-Clause, ISC, Zlib, BSL-1.0, Unlicense,
CC0-1.0, Apache-2.0, MPL-2.0, CDDL-1.0, EPL-2.0, LGPL-2.1/3.0, GPL-2.0/3.0 and AGPL-3.0 in
`-only` and `-or-later` forms. The deprecated ids (`GPL-3.0`, `LGPL-2.1`, `AGPL-3.0`…) are
read as `-only`, with a note.

## Rules

- **Permissive** licences go into any work if their notice conditions are kept; the FSF lists
  all of them as GPL-compatible, except that Apache-2.0 is compatible with GPL version 3 and
  not version 2 (patent termination and indemnification). Apache-2.0 into a `-or-later`
  GPL/LGPL work forces the combination to version 3.
- **GNU family**: each licence is mapped to the GNU licences its code can be conveyed under
  (LGPL-2.1 section 3 allows conversion to GPL-2.0 or any later GPL; LGPL-3.0 is GPL-3.0 plus
  permissions, so GPL-3.0 only; `-or-later` adds later versions). The verdict is compatible if
  the combination can carry every version the target offers, conditional if only some, and
  incompatible if none. GPL-3.0 and AGPL-3.0 section 13 allow combining the two (AGPL into a
  GPL work is conditional: its network clause applies).
- **Linking**: the FSF treats static and dynamic linking alike, so GPL/AGPL code makes any
  linked work subject to its terms. LGPL code linked into a non-GNU work is conditional
  (dynamic: replaceable shared library; static: provide object files for relinking); copied
  into the work's own files (`linking: source`) it is incompatible.
- **MPL-2.0, CDDL-1.0, EPL-2.0** are file- or module-level copyleft: combining them into a
  larger work under another licence is conditional (their files stay under their licence).
  MPL-2.0 is GPL/LGPL/AGPL-compatible through its Secondary License clause unless the Exhibit B
  notice is present (`incompatible_with_secondary`); EPL-2.0 only when Exhibit A designates the
  GPL (`secondary_license`); CDDL-1.0 is GPL-incompatible. In an Apache-2.0 work they are
  Apache category B (binary only, labelled).
- **Apache categories** (A: may be included; B: binary only; X: not allowed) are reported for
  every component; the ASF does not list CC0 under A but treats it as public-domain-like.

## Tests

Known values from the FSF licence list (Apache-2.0 incompatible with GPL-2.0-only, compatible
with GPL-3.0), the FSF compatibility matrix (LGPL-3.0 not usable in GPL-2.0-only; GPL-2.0-or-
later usable in GPL-3.0), the MPL 2.0 FAQ (Secondary License, Exhibit B), and the ASF policy
(MPL-2.0 category B); plus linking and usage-error cases.

## Limits

Sources read on 18 October 2026. Licence exceptions (Classpath, GCC runtime, OpenSSL linking
exceptions), dual licensing, and the different view some lawyers take on dynamic linking are
not modelled. Not legal advice; the output carries that disclaimer.
