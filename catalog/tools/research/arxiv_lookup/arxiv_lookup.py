#!/usr/bin/env python3
"""Search or fetch arXiv metadata through the arXiv API (Atom 1.0 feed parsed with xml.etree).

Input (JSON on stdin):
  id_list       ["1706.03762", "hep-th/9711200v3", ...] or one id as a string (at most 50)
  search_query  arXiv query syntax: ti:, au:, abs:, cat:, all:, AND/OR/ANDNOT, quotes
  max_results   1..50 (default 10, or the number of ids)
  start         offset of the first result (default 0)
  sort_by       relevance | lastUpdatedDate | submittedDate
  sort_order    ascending | descending
  response      optional: the exact Atom text the API returned (offline or cached use);
                when given, no request is made
One request per call to https://export.arxiv.org/api/query, timeout 15 s. arXiv asks for no
more than one request every three seconds; this tool never pages or retries.
"""
import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET

API = "https://export.arxiv.org/api/query"
UA = "AXLR-catalog-tool/1.0 (+https://axlr.underpassai.com)"
TIMEOUT = 15
MAX_RESULTS = 50
NS = {"a": "http://www.w3.org/2005/Atom", "arxiv": "http://arxiv.org/schemas/atom",
      "os": "http://a9.com/-/spec/opensearch/1.1/"}
NEW_ID = re.compile(r"^(\d{4}\.\d{4,5})(v\d+)?$")
OLD_ID = re.compile(r"^([a-z]+(-[a-z]+)*(\.[A-Z]{2})?/\d{7})(v\d+)?$")
SORT_BY = ("relevance", "lastUpdatedDate", "submittedDate")
SORT_ORDER = ("ascending", "descending")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def clean_id(value):
    if not isinstance(value, str):
        fail("id_list items must be strings")
    text = value.strip()
    text = re.sub(r"^(https?://(export\.)?arxiv\.org/(abs|pdf)/|arxiv:)", "", text, flags=re.I)
    text = re.sub(r"\.pdf$", "", text)
    if not (NEW_ID.match(text) or OLD_ID.match(text)):
        fail(f"not an arXiv identifier: {value!r} (expected 1706.03762, 1706.03762v7 or hep-th/9711200)")
    return text


def split_version(identifier):
    match = re.match(r"^(.*?)(v(\d+))?$", identifier)
    return match.group(1), int(match.group(3)) if match.group(3) else None


def build_query(request):
    ids = request.get("id_list")
    if isinstance(ids, str):
        ids = [ids]
    if ids is not None:
        if not isinstance(ids, list) or not ids:
            fail("id_list must be a non-empty list of arXiv ids")
        if len(ids) > MAX_RESULTS:
            fail(f"id_list has more than {MAX_RESULTS} ids")
        ids = [clean_id(i) for i in ids]
    query = request.get("search_query")
    if query is not None:
        if not isinstance(query, str) or not query.strip():
            fail("search_query must be a non-empty string")
        if len(query) > 1000:
            fail("search_query is longer than 1000 characters")
        query = query.strip()
    if ids is None and query is None:
        fail("give id_list or search_query (or both)")
    default = len(ids) if ids and query is None else 10
    max_results = request.get("max_results", default)
    if not isinstance(max_results, int) or isinstance(max_results, bool) or not 1 <= max_results <= MAX_RESULTS:
        fail(f"max_results must be an integer from 1 to {MAX_RESULTS}")
    start = request.get("start", 0)
    if not isinstance(start, int) or isinstance(start, bool) or not 0 <= start <= 30000:
        fail("start must be an integer from 0 to 30000")
    params = {}
    if query is not None:
        params["search_query"] = query
    if ids is not None:
        params["id_list"] = ",".join(ids)
    params["start"] = start
    params["max_results"] = max_results
    sort_by, sort_order = request.get("sort_by"), request.get("sort_order")
    if sort_by is not None:
        if sort_by not in SORT_BY:
            fail("sort_by must be relevance, lastUpdatedDate or submittedDate")
        params["sortBy"] = sort_by
    if sort_order is not None:
        if sort_order not in SORT_ORDER:
            fail("sort_order must be ascending or descending")
        params["sortOrder"] = sort_order
    return ids, API + "?" + urllib.parse.urlencode(params)


def fetch(url):
    if not url.startswith("https://"):
        fail("only HTTPS is allowed")
    request = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "application/atom+xml"})
    try:
        with urllib.request.urlopen(request, timeout=TIMEOUT) as reply:
            return reply.read().decode("utf-8")
    except urllib.error.HTTPError as error:
        fail(f"arXiv answered HTTP {error.code}: {error.read().decode('utf-8', 'replace').strip()[:200]}")
    except (urllib.error.URLError, OSError, ValueError) as error:
        fail(f"request to arXiv failed: {error}")


def text_of(element, path):
    found = element.find(path, NS)
    if found is None or found.text is None:
        return None
    return re.sub(r"\s+", " ", found.text).strip() or None


def to_int(value):
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def parse_entry(entry):
    raw_id = text_of(entry, "a:id") or ""
    identifier = re.sub(r"^https?://arxiv\.org/abs/", "", raw_id)
    base, version = split_version(identifier)
    links = {"abs": None, "pdf": None, "doi": None}
    for link in entry.findall("a:link", NS):
        href, title, rel = link.get("href"), link.get("title"), link.get("rel")
        if title == "pdf":
            links["pdf"] = href
        elif title == "doi":
            links["doi"] = href
        elif rel == "alternate":
            links["abs"] = href
    authors = []
    for author in entry.findall("a:author", NS):
        item = {"name": text_of(author, "a:name")}
        affiliations = [re.sub(r"\s+", " ", a.text).strip() for a in author.findall("arxiv:affiliation", NS) if a.text]
        if affiliations:
            item["affiliations"] = affiliations
        authors.append(item)
    primary = entry.find("arxiv:primary_category", NS)
    return {
        "id": base,
        "version": version,
        "title": text_of(entry, "a:title"),
        "authors": authors,
        "summary": text_of(entry, "a:summary"),
        "published": text_of(entry, "a:published"),
        "updated": text_of(entry, "a:updated"),
        "primary_category": primary.get("term") if primary is not None else None,
        "categories": [c.get("term") for c in entry.findall("a:category", NS) if c.get("term")],
        "comment": text_of(entry, "arxiv:comment"),
        "journal_ref": text_of(entry, "arxiv:journal_ref"),
        "doi": text_of(entry, "arxiv:doi"),
        "abs_url": links["abs"],
        "pdf_url": links["pdf"],
    }


def parse(text):
    try:
        root = ET.fromstring(text.encode("utf-8") if isinstance(text, str) else text)
    except ET.ParseError as error:
        fail(f"response is not an Atom feed: {error}")
    if root.tag != "{http://www.w3.org/2005/Atom}feed":
        fail("response is not an Atom feed (root element is not <feed>)")
    entries = []
    for entry in root.findall("a:entry", NS):
        entry_id = text_of(entry, "a:id") or ""
        if "/api/errors" in entry_id:  # the API's error report is an entry titled "Error"
            fail("arXiv API error: " + (text_of(entry, "a:summary") or "unknown"))
        entries.append(parse_entry(entry))
    return {
        "total_results": to_int(text_of(root, "os:totalResults")),
        "start_index": to_int(text_of(root, "os:startIndex")),
        "items_per_page": to_int(text_of(root, "os:itemsPerPage")),
        "feed_updated": text_of(root, "a:updated"),
        "entries": entries,
    }


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    try:
        json.dumps(request, ensure_ascii=False).encode("utf-8")
    except UnicodeEncodeError:
        fail("input contains an unpaired surrogate (not valid Unicode text)")
    unknown = set(request) - {"id_list", "search_query", "max_results", "start", "sort_by", "sort_order", "response"}
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    ids, url = build_query(request)
    response = request.get("response")
    if response is not None:
        if not isinstance(response, str) or not response.strip():
            fail("response must be the non-empty Atom text the API returned")
        result = parse(response)
        fetched = False
    else:
        result = parse(fetch(url))
        fetched = True
    if ids:
        found = {e["id"] for e in result["entries"]} | {f"{e['id']}v{e['version']}" for e in result["entries"]}
        result["not_found"] = [i for i in ids if i not in found]
    result["source"] = {"api": "arXiv API", "url": url, "fetched": fetched,
                        "note": "one request per call; arXiv asks for at most one request every 3 seconds"}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
