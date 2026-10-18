# Pre-forged tools: landscape and catalog

Status: proposed (16 Oct 2026). Part of the [registry design](2026-10-16-tool-registry-design.md). Two reference entries prove the manifest shape and pass their tests through [`tools/catalog/run_tests.py`](../../tools/catalog/run_tests.py) with the `axlr_run_tool` contract (stdin JSON, `PATH` and `HOME` only): [`seat_allocation`](../../catalog/tools/politics_civics/seat_allocation/) and [`business_days`](../../catalog/tools/administration/business_days/). Counts, stars and dates below were read on 16 Oct 2026 with `gh`, `curl` and page fetches; what could not be verified says so.

## Landscape: what exists and what it teaches

| Catalog | Size | What an entry carries | Organisation | Trust |
|:--|:--|:--|:--|:--|
| [Official MCP registry](https://registry.modelcontextprotocol.io) ([schema](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/server-json/generic-server-json.md)) | > 6,000 servers (page cap hit; exact total not read) | `server.json`: reverse-DNS `name`, `description`, `version`, `repository`, `packages[]` with `registryType` (npm, pypi, oci, mcpb…), `environmentVariables{required,secret}` and `fileSha256` for bundles | flat; the namespace is the only grouping | namespace ownership (GitHub OAuth, DNS TXT, `.well-known`); metadata only, no code scan |
| [Glama](https://glama.ai/mcp/servers) | 100,469 | tags, licence/quality/maintenance grades A–D, tool list | facets: language, hosting, capability, subject (Developer Tools 23.7k, Search 14.3k, Finance 6.7k, Databases 5.6k, Security 5.0k…), Official/Claimed | letter grades, method unexplained |
| [Smithery](https://smithery.ai) | 27,172+ MCPs, plus Skills | owner/slug, description, usage count | none visible | a "verified" badge with no stated check |
| [PulseMCP](https://www.pulsemcp.com/servers) | 21,698 | classification, weekly visitors | Reference / Official / Community | badges; submissions paused |
| [mcp.so](https://mcp.so) | not shown | config, tools | five top categories plus tags | manual review of config and tools |
| [Composio](https://docs.composio.dev) | "1000+ toolkits" | `TOOLKIT_ACTION` slug, input and output schema; auth lives outside the tool | by service; a `COMPOSIO_SEARCH_TOOLS` meta-tool for runtime discovery | managed OAuth; every tool wraps a SaaS API |
| [strands-agents/tools](https://github.com/strands-agents/tools) | 50 modules, 1.3k stars | a Python function; docstring and type hints make the schema | flat | repository review |
| [anthropics/skills](https://github.com/anthropics/skills), [Agent Skills spec](https://agentskills.io/specification) | 17 skills, 182k stars; spec repo 26k | `SKILL.md` frontmatter: `name` (≤ 64, lowercase, equals the directory), `description` (≤ 1024), `license`, `compatibility`, `metadata`, `allowed-tools`; `scripts/`, `references/`, `assets/` | plugin bundles in `marketplace.json` | a git marketplace, no checksums; `skills-ref validate` lints |
| LangChain community, LlamaHub, CrewAI tools, smolagents | 81 / 67 / ~80 / ~10 | a class with `name`, `description`, `args_schema` | by vendor | none; PyPI |
| [ToolBench](https://github.com/OpenBMB/ToolBench), [Gorilla](https://github.com/ShishirPatil/gorilla) | 16,464 APIs / 1,600+ | per-tool JSON with `api_list` | 49 RapidAPI categories; G1/G2/G3 instruction groups | pass-rate filtering; BFCL leaderboard |
| npm, PyPI (PEP 691/740), Homebrew `formula.json` | 8,662 formulae in one static file | `dist.integrity`, `hashes.sha256`, `bottle.files.<platform>.{url, sha256}` | flat JSON, per-item pages | per-artifact hashes; registry signatures (npm ECDSA, PyPI attestations) |

Four lessons the design follows:

1. Only the MCP registry and the package indexes carry per-artifact hashes; AXLR's index does, with a signature over the whole index ([D1, D2](2026-10-16-tool-registry-design.md#decisions)).
2. Nobody execute-tests catalog entries. A CI-run `tests.json` per tool is AXLR's verification signal and the thing that makes a catalog tool safer than a freshly forged one.
3. Categories are weak everywhere; Glama's subject facets and RapidAPI's 49 categories are the usable references. AXLR's categories below are by **the person's domain**, not by vendor.
4. Almost every catalog entry wraps a SaaS API. Offline, deterministic, non-programming tools are the gap in the market, and they are exactly what a forged-tool runtime with no secrets can run.

## Entry schema

A catalog tool is a `ForgedTool` plus a `catalog` block ([D3](2026-10-16-tool-registry-design.md#d3-per-tool-manifest)):

| Field | Meaning |
|:--|:--|
| `name`, `description`, `input_schema`, `program`, `args`, `files` | exactly the forged-tool fields; `args` names one of `files` |
| `catalog.version` | semver |
| `catalog.category`, `catalog.tags` | one category from the list below; free tags for search |
| `catalog.license` | SPDX id, MIT or Apache-2.0 compatible |
| `catalog.requires` | `program_min_version` and `packages[]` (empty in tier 1) |
| `catalog.network` | `true` when the tool opens a socket |
| `catalog.example` | one input and the output it produces, shown on the site and in `axlr_catalog describe` |
| `catalog.source` | where the method or idea comes from |
| `tests.json` | at least three cases: `{name, input, expect: {exit, stdout_json | stdout_contains}}`; `stdout_json` is matched as a subset |
| `README.md` | what it does, the method, its limits; for health, law and tax tools, the disclaimer the output also prints |

Tiers: **1** stdlib only (Python 3.10+, bash with coreutils, Node 22 without packages), no network: installed with a single card. **2** needs a package (`requires.packages`): the card warns when the import fails. **3** needs the network, no key: works without the sandbox or with its network allowed. **4** needs an API key: not in the catalog until the `secrets` mechanism exists.

Conventions: every tool prints one JSON object; a usage error exits 2 with `{"error": …}`; numbers that matter are exact (`fractions`, `decimal`); nothing reads files outside the workspace unless the input names them; a tool whose answer depends on a dated table (holidays, CPI, tax brackets) carries the table's date in its output and its `version`.

## Catalog

Tier 1 unless marked. "Source" names the implementation to learn from or "original". 

### Politics and civics

| Tool | Description | Source |
|:--|:--|:--|
| `seat_allocation` **1.0.0** | D'Hondt, Sainte-Laguë, modified Sainte-Laguë, Hare; threshold; award order and ties (**reference entry**) | original |
| `seat_allocation_droop` **1.0.0** | Droop and Imperiali quotas, largest remainder | original |
| `vote_condorcet` **1.0.0** | pairwise matrix, Condorcet winner, Schulze and Copeland | [CIVS](https://github.com/andrewcmyers/civs) ideas |
| `vote_irv` **1.0.0** | instant-runoff rounds with tie handling and exhausted ballots | original |
| `vote_borda` **1.0.0** | Borda and Dowdall scores | original |
| `vote_stv` **1.0.0** | single transferable vote (Gregory method) | original |
| `apportionment` **1.0.0** | Huntington-Hill, Webster, Jefferson apportionment of seats among districts by population | original |
| `gerrymander_metrics` **1.0.0** | efficiency gap, Polsby-Popper and Reock compactness from district vote and geometry data | original |
| `poll_margin_error` **1.0.0** | sample size, margin of error, lead significance | original |
| `legislative_calendar` **1.0.0** | term deadlines from a bill's dates with a session calendar given in the input | original |
| `transparency_request_draft` **1.0.0** | access-to-information request template per jurisdiction (Spain Ley 19/2013, EU 1049/2001, US FOIA, UK FOIA), with "not legal advice" | original |
| `budget_breakdown` **1.0.0** | per-capita, per-programme and year-over-year breakdown of a public budget CSV | original |

### Administration and office

| Tool | Description | Source |
|:--|:--|:--|
| `business_days` **1.0.0** | count, add, list working days with weekends and holidays given in the input (**reference entry**) | original |
| `holiday_table` **1.0.0** | holidays for a country and region and year, from a bundled dated table (tier 2 with `holidays`) | [vacanza/holidays](https://github.com/vacanza/holidays) |
| `invoice_calc` **1.0.0** | lines, discounts, VAT per rate, withholding, totals in `Decimal` | original |
| `iban_validate` **1.0.2** | IBAN checksum, country length, BBAN split | [python-stdnum](https://github.com/arthurdejong/python-stdnum) |
| `es_tax_id_validate` **1.0.0** | Spanish NIF, NIE and CIF check letter | python-stdnum |
| `eu_vat_validate` **1.0.0** | EU VAT number syntax and checksum per country | python-stdnum |
| `ics_generate` **1.0.0** | RFC 5545 event or recurring event, with alarms and attendees | [icalendar](https://github.com/collective/icalendar) |
| `meeting_planner` **1.0.0** | overlap of working hours across time zones (`zoneinfo`) | original |
| `mail_merge` **1.0.0** | CSV plus a text template to many documents | original |
| `doc_template_fill` **1.0.0** | fill `{{placeholders}}` in text or `.docx` (zip and XML, no package) | [docxtpl](https://github.com/elapouya/python-docx-template) ideas |
| `date_parse` **1.0.0** | natural-language dates and durations (tier 2 with `dateparser`; a stdlib subset for ISO and common forms) | [dateparser](https://github.com/scrapinghub/dateparser) |
| `expense_split` **1.0.0** | split shared expenses and settle with the fewest transfers | original |
| `letter_formal` **1.0.0** | formal letter skeleton per language and register with the right salutations and closings | original |

### Mathematics and statistics

| Tool | Description | Source |
|:--|:--|:--|
| `exact_calc` **1.0.0** | expression evaluator over `Fraction` and `Decimal` with an AST whitelist (no `eval`) | original |
| `stats_describe` **1.0.0** | mean, median, SD, quartiles, confidence interval, correlation, linear regression | stdlib `statistics` |
| `prob_dist` **1.0.0** | binomial, Poisson, normal, hypergeometric pmf and cdf; combinatorics | stdlib |
| `hypothesis_test` **1.0.0** | Welch t, paired t, chi-square, Fisher exact, with p-values (exact where stdlib allows; tier 2 with `scipy` otherwise) | scipy |
| `number_theory` **1.0.0** | factorisation, gcd/lcm, primality (Miller-Rabin), modular inverse, totient, CRT | original |
| `matrix_ops` **1.0.0** | determinant, inverse, rank, solve over `Fraction` | original |
| `unit_convert` **1.0.0** | conversion with dimensional check from a bundled table (tier 2 with `pint`) | [pint](https://github.com/hgrecco/pint) |
| `base_convert` **1.0.0** | bases 2–36, Roman numerals, two's complement | original |
| `sequence_tools` **1.0.0** | OEIS-style detection of arithmetic, geometric and polynomial sequences; next terms | original |
| `sympy_solve` | solve, simplify, differentiate, integrate, limit, series (tier 2 with `sympy`) | sympy |
| `sampling_plan` **1.0.0** | sample size for proportions and means, stratified allocation | original |
| `rounding_report` **1.0.0** | significant figures, uncertainty propagation | original |

### Research and science

| Tool | Description | Source |
|:--|:--|:--|
| `citation_format` **1.0.0** | BibTeX or CSL-JSON to APA 7, MLA 9, Chicago, IEEE, Vancouver | [CSL styles](https://github.com/citation-style-language/styles) |
| `bibtex_tools` **1.0.0** | parse, deduplicate, normalise keys, find missing fields | original |
| `doi_isbn_validate` **1.0.0** | DOI syntax, ISBN-10/13 checksum and conversion, ISSN | original |
| `doi_lookup` | metadata from Crossref (tier 3) | Crossref REST |
| `arxiv_lookup` | search and fetch arXiv metadata (tier 3) | arXiv API |
| `chem_formula` **1.0.0** | parse `Ca(OH)2`, molar mass, mass percent, from a bundled element table | original |
| `physics_constants` **1.0.0** | CODATA 2022 constants with units and uncertainty | NIST |
| `dna_tools` **1.0.0** | reverse complement, GC content, translation, ORF search | Biopython ideas |
| `experiment_power` **1.0.0** | statistical power and sample size for two-group designs | original |
| `data_dictionary` **1.0.0** | describe a dataset's columns into a data dictionary skeleton | original |
| `prisma_counts` **1.0.1** | PRISMA flow numbers from screening decisions | original |

### Writing and creativity

| Tool | Description | Source |
|:--|:--|:--|
| `readability` **1.0.0** | Flesch, Flesch-Kincaid, Gunning Fog, SMOG; Spanish via Fernández-Huerta and Szigriszt-Pazos | [textstat](https://github.com/textstat/textstat) |
| `text_report` **1.0.0** | word and sentence counts, frequency, n-grams, reading time, longest sentences | original |
| `style_lint` **1.0.0** | passive voice, filler words, adverbs, repeated words, sentence-length spread (heuristic, English and Spanish) | original |
| `syllables_rhyme` **1.0.0** | syllable count, stress, rhyme scheme check (Spanish rules exact; English heuristic) | original |
| `meter_check` **1.0.0** | scan verse against a meter (endecasílabo, alejandrino, iambic pentameter) | original |
| `name_generator` **1.0.2** | seeded names for characters, places and products, by culture and style | original |
| `story_structure_check` **1.0.0** | outline beats against three-act, Save the Cat, hero's journey | original |
| `md_convert` **1.0.0** | Markdown to HTML, plain text or a deck outline | original |
| `latex_table` **1.0.0** | CSV to a LaTeX table with escaping and alignment | original |
| `slug_title_case` **1.0.0** | slugs, title case per style guide, smart quotes | original |
| `crossword_fill` **1.0.0** | fill a crossword grid from a word list | original |
| `anagram_wordplay` **1.0.0** | anagrams, palindromes, lipograms from a bundled word list | original |

### Programming and devops

| Tool | Description | Source |
|:--|:--|:--|
| `semver_tools` **1.0.0** | parse, compare, bump, match ranges | semver.org |
| `cron_explain` **1.0.0** | explain a cron expression and list the next N runs | original |
| `regex_test` **1.0.0** | run a pattern over text with a timeout, show groups | original |
| `json_schema_check` **1.0.1** | validate against a JSON Schema subset (tier 2 with `jsonschema` for the full spec) | jsonschema |
| `diff_text` **1.0.0** | unified and word diff | stdlib `difflib` |
| `dockerfile_lint` **1.0.0** | latest tag, root user, apt cache, secrets in `ENV` | hadolint ideas |
| `license_detect` **1.0.0** | SPDX licence from a LICENSE text by similarity | SPDX list |
| `gitignore_gen` **1.0.0** | `.gitignore` from named stacks | github/gitignore |
| `env_example` **1.0.0** | `.env.example` from a `.env` with values blanked | original |
| `openapi_outline` **1.0.0** | endpoints, methods and schemas from an OpenAPI file | original |
| `log_summarise` **1.0.0** | group log lines by template, count, first and last time | original |

### Data and files

| Tool | Description | Source |
|:--|:--|:--|
| `csv_stats` **1.0.0** | per-column type, nulls, min, max, mean, distinct | [csvkit](https://github.com/wireservice/csvkit) |
| `csv_query` **1.0.0** | SQL over CSV files through in-memory `sqlite3` | datasette, [miller](https://github.com/johnkerl/miller) |
| `csv_json_convert` **1.0.1** | both ways, nesting by dotted keys | csvkit |
| `xlsx_read` **1.0.0** | sheets to CSV through `zipfile` and XML | original |
| `json_diff` **1.0.0** | structural diff with paths | original |
| `file_hash_tree` **1.0.0** | SHA-256 manifest of a directory and verify | original |
| `encoding_detect` **1.0.0** | detect and convert text encodings (tier 2 with `charset_normalizer`) | charset_normalizer |
| `format_convert` **1.0.0** | JSON, TOML (`tomllib`), INI, YAML subset | stdlib |
| `dedupe_sort` **1.0.0** | deduplicate and natural-sort lines | original |
| `table_pivot` **1.0.0** | pivot and group-by over a CSV | original |

### Finance and economics

| Tool | Description | Source |
|:--|:--|:--|
| `npv_irr` **1.0.0** | NPV, IRR (Newton with bisection fallback), MIRR, payback | [numpy-financial](https://github.com/numpy/numpy-financial) |
| `loan_amortization` **1.0.0** | payment and table for French, German and American systems; early repayment | original |
| `compound_interest` **1.0.0** | future and present value, effective rates, rule of 72 | original |
| `inflation_adjust` **1.0.0** | CPI adjustment from a bundled dated table (Spain INE, US CPI-U, euro area HICP) | original |
| `break_even` **1.0.0** | break-even, contribution margin, sensitivity | original |
| `portfolio_stats` **1.0.0** | return, volatility, Sharpe, drawdown from a price CSV | original |
| `tax_bracket_estimate` **1.0.0** | bracket-based estimate from a yearly table given in the input or bundled with its year (IRPF, federal income tax), with a disclaimer | original |
| `fx_convert` | ECB reference rates (tier 3) | ECB |
| `depreciation` **1.0.0** | straight-line, declining balance, sum-of-years tables | original |
| `cashflow_forecast` **1.0.0** | monthly cash-flow projection from recurring items | original |

### Law and compliance

| Tool | Description | Source |
|:--|:--|:--|
| `deadline_calc` **1.0.0** | calendar versus working-day terms, from a jurisdiction rule given in the input (e.g. "10 hábiles, agosto inhábil"); never guesses the rule | original |
| `clause_checklist` **1.0.0** | checklist per contract type (NDA, services, lease, employment) | original |
| `gdpr_ropa_template` **1.0.0** | Article 30 record of processing skeleton | original |
| `dpia_screening` **1.0.0** | DPIA need screening from the EDPB criteria | original |
| `license_compat` **1.0.0** | compatibility matrix of open-source licences | original |
| `retention_schedule` **1.0.0** | retention periods by record type from a table given in the input | original |
| `legal_citation_format` **1.0.0** | BOE, EUR-Lex, ECLI and US case citation formats | original |
| `consent_text_check` **1.0.0** | consent and privacy notice completeness against GDPR Articles 13 and 14 | original |

### Education

| Tool | Description | Source |
|:--|:--|:--|
| `quiz_cloze` **1.0.0** | cloze questions from a text, with answer key | original |
| `quiz_mcq_scaffold` **1.0.0** | multiple-choice scaffolds with distractors from the same text | original |
| `srs_schedule` **1.0.0** | SM-2 next interval (tier 2 with `fsrs` for FSRS) | [py-fsrs](https://github.com/open-spaced-repetition/py-fsrs) |
| `rubric_grade` **1.0.0** | weighted rubric scoring and feedback report | original |
| `grade_stats` **1.0.0** | class statistics, curves, item difficulty and discrimination | original |
| `flashcards_export` **1.0.0** | CSV to Anki-importable TSV | original |
| `timeline_builder` **1.0.0** | events to a Markdown or ASCII timeline | original |
| `lesson_plan_template` **1.0.0** | lesson plan skeleton by level and duration (Bloom verbs) | original |
| `bloom_classify` **1.0.0** | classify learning objectives by Bloom level (verb table) | original |

### Language

| Tool | Description | Source |
|:--|:--|:--|
| `tokenize_text` **1.0.0** | Unicode-aware words, sentences, counts | original |
| `unicode_tools` **1.0.1** | normalise, strip accents, code points, confusables | stdlib `unicodedata` |
| `transliterate` **1.0.0** | Cyrillic, Greek, Arabic (table-based) | original |
| `es_verb_conjugate` **1.0.0** | regular Spanish conjugation plus bundled irregulars | [mlconjug3](https://github.com/Ars-Linguistica/mlconjug3) ideas |
| `hangul_romanize` **1.0.0** | Revised Romanization from Hangul decomposition | [hangul-romanization](https://github.com/Tyriar/hangul-romanization) |
| `kana_romanize` **1.0.0** | Hepburn for kana (kanji needs `pykakasi`, tier 2) | pykakasi |
| `pinyin_convert` | tier 2 with `pypinyin` | [python-pinyin](https://github.com/mozillazg/python-pinyin) |
| `language_detect` **1.0.0** | character n-gram heuristic (tier 2 with `lingua`) | [lingua-py](https://github.com/pemistahl/lingua-py) |
| `number_to_words` **1.0.2** | numbers, ordinals and currency to words (es, en, fr, de, pt) | original |
| `gender_agreement_es` **1.0.0** | agreement check for Spanish noun phrases (heuristic) | original |

### Geography and time

| Tool | Description | Source |
|:--|:--|:--|
| `haversine` **1.0.0** | great-circle distance and bearing, batch | original |
| `tz_convert` **1.0.0** | conversions, DST transitions, offsets at a date (`zoneinfo`) | stdlib |
| `sun_times` **1.0.1** | sunrise, sunset, twilight by the NOAA algorithm | [astral](https://github.com/sffjunkie/astral) |
| `coord_convert` **1.0.0** | decimal degrees, DMS, UTM, geohash, what3words-style not included | original |
| `calendar_convert` **1.0.0** | Gregorian, ISO week, Julian day, Hebrew, Islamic | original |
| `iso_country_lookup` **1.0.0** | ISO 3166, currency, calling code, time zones | original |
| `moon_phase` **1.0.0** | phase and illumination | original |
| `geojson_tools` **1.0.0** | bounding box, centroid, area of GeoJSON polygons | original |

### Health (deterministic, every output carries a disclaimer)

| Tool | Description | Source |
|:--|:--|:--|
| `bmi_bsa` **1.0.0** | BMI with WHO categories; BSA (Mosteller, DuBois) | original |
| `egfr_calc` **1.0.0** | CKD-EPI 2021 | original |
| `dose_weight_based` **1.0.0** | mg/kg arithmetic with a hard maximum given in the input | original |
| `unit_lab_convert` **1.0.0** | mg/dL to mmol/L for common analytes | original |
| `heart_rate_zones` **1.0.0** | Karvonen zones | original |
| `sleep_cycles` **1.0.0** | bed and wake-time planner | original |
| `nutrition_totals` **1.0.0** | totals from a food table given in the input | original |

### Security (defensive)

| Tool | Description | Source |
|:--|:--|:--|
| `hash_verify` **1.0.0** | MD5, SHA-1, SHA-256, SHA-512, BLAKE2 and verify | stdlib |
| `password_entropy` **1.0.0** | entropy estimate, never echoes the password | original |
| `sbom_check` **1.0.0** | CycloneDX or SPDX JSON required fields and licence summary | original |
| `jwt_decode` **1.0.0** | decode without verifying, with a clear warning | original |
| `cert_inspect` **1.0.0** | PEM certificate fields and expiry (bash with openssl) | original |
| `secrets_scan` **1.0.1** | key-like strings in files | gitleaks ideas |
| `totp_generate` **1.0.0** | RFC 6238 code from a secret in the input | original |
| `csp_lint` **1.0.0** | Content-Security-Policy weaknesses | original |

### Media

| Tool | Description | Source |
|:--|:--|:--|
| `image_info` **1.0.0** | dimensions and format from PNG, JPEG, GIF headers; EXIF tier 2 with Pillow | Pillow |
| `qr_generate` | QR code to SVG (tier 2 with `segno`, pure Python) | [segno](https://github.com/heuer/segno) |
| `wav_info` **1.0.0** | duration and format of WAV | stdlib `wave` |
| `color_tools` **1.0.0** | hex, RGB, HSL, WCAG contrast, palette from a seed | original |
| `srt_tools` **1.0.0** | shift, convert, merge subtitles | original |
| `ascii_banner` **1.0.0** | text banner | original |

### Personal, games, cooking and engineering

| Tool | Description | Source |
|:--|:--|:--|
| `eisenhower_matrix` | tasks to quadrants with a plan | original |
| `habit_streaks` | streaks and rates from a log | original |
| `unit_price_compare` | price per unit across offers | original |
| `sudoku_solve`, `chess_fen_validate`, `dice_roll_seeded`, `wordle_helper` | puzzles and games | original |
| `recipe_scale`, `cooking_unit_convert` | scale servings, convert cups and grams by ingredient density | original |
| `resistor_color`, `ohms_law`, `beam_load` | electronics and statics | original |

## First batch

Fourteen tier-1 tools, one or two per domain, each the kind of computation a model gets wrong and a person checks: `seat_allocation` and `business_days` (done), `vote_condorcet`, `invoice_calc`, `iban_validate`, `exact_calc`, `stats_describe`, `citation_format`, `readability`, `npv_irr`, `deadline_calc`, `srs_schedule`, `tz_convert`, `hash_verify`. Then the Spanish-administration set (`es_tax_id_validate`, `eu_vat_validate`, `tax_bracket_estimate`, `transparency_request_draft`), which no other catalog serves.

## Repositories to learn from

| Repository | Stars | Last push | Why |
|:--|--:|:--|:--|
| [modelcontextprotocol/registry](https://github.com/modelcontextprotocol/registry) | 7.4k | 2026-10-14 | `server.json`, namespace auth, publisher CLI |
| [anthropics/skills](https://github.com/anthropics/skills) | 181.6k | 2026-10-09 | `SKILL.md` conventions, `marketplace.json` |
| [agentskills/agentskills](https://github.com/agentskills/agentskills) | 26.2k | 2026-08-09 | spec and `skills-ref` validator |
| [strands-agents/tools](https://github.com/strands-agents/tools) | 1.3k | 2026-10-13 | 50 tools incl. `calculator`, `current_time`, `rss`, `diagram` |
| [huggingface/smolagents](https://github.com/huggingface/smolagents) | 29.9k | 2026-10-06 | the smallest clean tool interface |
| [punkpeye/awesome-mcp-servers](https://github.com/punkpeye/awesome-mcp-servers) | 96.5k | 2026-10-15 | category taxonomy |
| [arthurdejong/python-stdnum](https://github.com/arthurdejong/python-stdnum) | 602 | 2026-08-15 | IBAN, VAT, NIF/NIE and national ids |
| [vacanza/holidays](https://github.com/vacanza/holidays) | 1.9k | 2026-10-16 | holiday data per country and region |
| [hgrecco/pint](https://github.com/hgrecco/pint) | 2.8k | 2026-10-13 | units with dimensional analysis |
| [textstat/textstat](https://github.com/textstat/textstat) | 1.4k | 2026-10-13 | readability formulas |
| [numpy/numpy-financial](https://github.com/numpy/numpy-financial) | 414 | 2026-10-05 | NPV, IRR, PMT edge cases |
| [collective/icalendar](https://github.com/collective/icalendar) | 1.2k | 2026-10-15 | RFC 5545 reference |
| [wireservice/csvkit](https://github.com/wireservice/csvkit), [johnkerl/miller](https://github.com/johnkerl/miller) | 6.4k / 10.0k | 2026-09 / 2026-10 | CSV verbs |
| [citation-style-language/styles](https://github.com/citation-style-language/styles) | 3.9k | 2026-10-13 | citation styles |
| [open-spaced-repetition/py-fsrs](https://github.com/open-spaced-repetition/py-fsrs) | 513 | 2026-08-09 | spaced repetition |
| [andrewcmyers/civs](https://github.com/andrewcmyers/civs) | 96 | 2026-08-21 | Condorcet and Schulze |
| [heuer/segno](https://github.com/heuer/segno) | 814 | 2026-07-23 | pure-Python QR |

No maintained small repository exists for D'Hondt and Sainte-Laguë (the best have about 14 stars) or for GDPR templates; those are original tools, which is why `seat_allocation` is the first reference entry.

## Status

Implemented tools carry their version in bold in the tables above; each lives under [`catalog/tools/`](../../catalog/tools/) with its manifest, program, `tests.json` and README, passes [`tools/catalog/run_tests.py`](../../tools/catalog/run_tests.py) (which sends inputs and compares outputs exactly as the registry's runner does: keys sorted, numbers typed) and the pre-flight copy of the registry's policy scan, and is approved and served by the registry (`https://registry.underpassai.com/index.json`). Bundled data tables are written by scripts under [`tools/catalog/data/`](../../tools/catalog/data/) that name their sources, licences and dates. The registry's sandbox runs Python 3.12 and Node 22 since axlr-registry v0.3.4, without network and without third-party packages.

| Date | Batch | Tools added | Test cases |
|:--|:--|--:|--:|
| 2026-10-16 | reference entries (`seat_allocation`, `business_days`) | 2 | 10 |
| 2026-10-18 | 1: the first batch and the Spanish administration set (PR #138) | 16 | 117 |
| 2026-10-18 | 2: politics and civics, administration, mathematics, finance, law (PR #139) | 40 | 430 |
| 2026-10-18 | 3: research, writing, education, language, geography, health, security, media, data, programming | 83 | 1040 |

Package-tier tools (tier 2 without a standard-library route, today only `sympy_solve`) cannot be approved until the registry's sandbox installs packages: their tests cannot run there. Where the catalog names a package and a stdlib route exists, the stdlib version is shipped and its README says what the package would add.
