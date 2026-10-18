# arxiv_lookup

Fetches arXiv metadata through the arXiv API (`https://export.arxiv.org/api/query`), by id
list, by search query, or both (the API then filters the ids by the query). The reply is an
Atom 1.0 feed, parsed with `xml.etree` using the Atom, `arxiv` and `opensearch` namespaces.
Tier 3: one HTTPS request per call, 15 s timeout, User-Agent
`AXLR-catalog-tool/1.0 (+https://axlr.underpassai.com)`, no key.

Rate: arXiv's terms ask for no more than one request every three seconds and a single
connection at a time. The tool makes exactly one request, never pages through results and
never retries; a caller looping over pages must wait between calls. `max_results` is capped at
50 (the API itself allows up to 2000 per page).

Offline or cached use: put the exact feed text in `response`; no connection is opened and the
same parser runs. The tests work that way (the sandbox has no network).

## Input and output

`id_list` takes new-style ids (`1706.03762`, `1706.03762v7`) and old-style ones
(`hep-th/9711200`, `math.GT/0309136`), with an optional `arXiv:` prefix or abs/pdf URL;
malformed ids are rejected before any request. `search_query` is passed as written (fields
`ti`, `au`, `abs`, `co`, `jr`, `cat`, `rn`, `id`, `all`; operators `AND`, `OR`, `ANDNOT`).
`sort_by` and `sort_order` map to the API's `sortBy` and `sortOrder`.

Each entry gives `id` without version and `version` as a number, `title` and `summary` with
line breaks and runs of spaces collapsed, `authors` (name, and `affiliations` when arXiv has
them), `published` (first version) and `updated` (this version), `primary_category`,
`categories`, `comment`, `journal_ref`, `doi`, `abs_url` and `pdf_url`. The feed's
`total_results`, `start_index` and `items_per_page` are returned, and with `id_list`,
`not_found` lists the ids that came back empty (the current API answers an empty feed for an
unknown id; an error entry whose id contains `api/errors`, the form the user manual documents,
is reported as an error).

## Tests

Real feed bodies fetched on 2026-10-18, with the URL in each test name: arXiv:1706.03762
("Attention Is All You Need", Vaswani et al., eight authors, submitted 2017-06-12, version 7,
cs.CL), a title-and-author search that finds it, old-style hep-th/9711200 (Maldacena, with
journal reference and DOI) and the empty feed for 1706.99999. The error entry is the example
from the arXiv API user manual. Usage errors cover bad ids, `max_results` above 50 and missing
query.

Limits: no paging, no full text, no OAI-PMH bulk harvesting; LaTeX in titles and abstracts is
returned as arXiv stores it.
