#!/usr/bin/env python3
"""Write catalog/tools/language/language_detect/profiles.json: the character n-gram profiles
(n = 1..3, the 500 most frequent per language, as counts) that the tool bundles. The file holds
counts only, never the text they were counted from. This generator uses the network; the tool
never does.

Corpus: Unicode CLDR 48.2 (tag release-48-2, Unicode License v3), fetched on 2026-10-18 from
https://raw.githubusercontent.com/unicode-org/cldr/release-48-2/ for each language:
  common/main/<lang>.xml         the locale's own words: names of languages, scripts,
                                 territories, months, weekdays, calendar fields, units, keys
                                 and types, characterLabels... Patterns, symbols, exemplar
                                 characters, number formats, time zone city names and person
                                 name samples are left out.
  common/annotations/<lang>.xml  emoji names and keywords ("cara sonriente | sonrisa"), split
                                 on "|".
  LICENSE                        the Unicode License v3 notice, copied into profiles.json.
Placeholders such as {0} are removed and strings with fewer than two letters are dropped.

Why not the UDHR: the brief named the Unicode "UDHR in Unicode" translations, but
https://unicode.org/udhr/ now says (read on 2026-10-18) "Effective January 2024, the Unicode
Consortium is no longer hosting the UDHR in Unicode project" and points to the OHCHR site. The
UN's terms of use (https://www.un.org/en/about-us/terms-of-use) allow copying "for the User's
personal, non-commercial use, without any right to resell or redistribute them or to compile or
create derivative works therefrom"; the OHCHR page answered with a bot challenge; the successor
project "UDHR in XML" (http://efele.net/udhr, github.com/eric-muller/udhr) has no licence file,
only the sentence "The UDHR and all its translations are obviously part of our commons." None of
these is a clear permissive licence, so no UDHR text was used.

Text normalisation (the tool does the same): NFC, lower case, every character that is not a
letter becomes a space; each word is padded with one space on each side and its 1-, 2- and
3-grams that are not only spaces are counted (Cavnar & Trenkle 1994).

Usage: python3 tools/catalog/data/language_detect.py [--cache DIR]
  --cache DIR  keep the downloaded files in DIR and reuse them on a rerun
"""
import collections
import datetime as dt
import json
import pathlib
import re
import sys
import unicodedata
import urllib.request
import xml.etree.ElementTree as ET

CLDR = "https://raw.githubusercontent.com/unicode-org/cldr/release-48-2/"
TOP = 500
LANGUAGES = {
    "en": "English", "es": "Spanish", "fr": "French", "de": "German", "pt": "Portuguese", "it": "Italian",
    "nl": "Dutch", "ca": "Catalan", "ro": "Romanian", "pl": "Polish", "cs": "Czech", "sv": "Swedish",
    "da": "Danish", "no": "Norwegian (CLDR no, Bokmål)", "fi": "Finnish", "hu": "Hungarian", "tr": "Turkish",
    "id": "Indonesian", "ru": "Russian", "el": "Greek",
}
# Elements whose text is not running language: patterns, symbols, character sets, codes.
SKIP = {
    "alias", "identity", "version", "exemplarCharacters", "exemplarCity", "pattern", "dateFormatItem",
    "intervalFormatItem", "greatestDifference", "symbol", "unitPattern", "compoundUnitPattern",
    "compoundUnitPattern1", "perUnitPattern", "coordinateUnitPattern", "listPatternPart", "gmtFormat",
    "hourFormat", "regionFormat", "fallbackFormat", "gmtZeroFormat", "localePattern", "localeSeparator",
    "localeKeyTypePattern", "codePattern", "quotationStart", "quotationEnd", "alternateQuotationStart",
    "alternateQuotationEnd", "characterLabelPattern", "nameOrderLocales", "foreignSpaceReplacement",
    "nativeSpaceReplacement", "sampleName", "nameField", "timeZoneNames", "numbers", "rbnf", "posix",
    "layout", "contextTransforms", "metadata", "delimiters", "personNames", "parseLenients",
}


def fetch(path, cache):
    name = path.replace("/", "_")
    if cache and (cache / name).exists():
        return (cache / name).read_bytes()
    request = urllib.request.Request(CLDR + path, headers={"User-Agent": "AXLR-catalog data generator"})
    with urllib.request.urlopen(request, timeout=120) as response:
        data = response.read()
    if cache:
        (cache / name).write_bytes(data)
    return data


def strings(lang, cache):
    texts = []

    def walk(element, skipped):
        skipped = skipped or element.tag in SKIP
        if not skipped and element.text and element.text.strip():
            texts.append(element.text.strip())
        for child in element:
            walk(child, skipped)

    walk(ET.fromstring(fetch(f"common/main/{lang}.xml", cache)), False)
    for item in ET.fromstring(fetch(f"common/annotations/{lang}.xml", cache)).iter("annotation"):
        if item.text:
            texts.extend(part.strip() for part in item.text.split("|"))
    cleaned = (re.sub(r"\{[^}]*\}", "", t) for t in texts)
    return [t for t in cleaned if sum(c.isalpha() for c in t) >= 2]


def words(text):
    text = unicodedata.normalize("NFC", text).lower()
    return "".join(c if c.isalpha() else " " for c in text).split()


def ngrams(word_list):
    for word in word_list:
        padded = f" {word} "
        for n in (1, 2, 3):
            for i in range(len(padded) - n + 1):
                gram = padded[i:i + n]
                if gram.strip():
                    yield gram


def main():
    cache = None
    if len(sys.argv) == 3 and sys.argv[1] == "--cache":
        cache = pathlib.Path(sys.argv[2])
        cache.mkdir(parents=True, exist_ok=True)
    today = dt.date.today().isoformat()
    profiles, corpus = {}, {}
    for lang, english in LANGUAGES.items():
        texts = strings(lang, cache)
        counts = collections.Counter(ngrams(words(" ".join(texts))))
        top = sorted(counts.items(), key=lambda item: (-item[1], item[0]))[:TOP]
        profiles[lang] = {"name": english, "corpus_strings": len(texts), "corpus_letters": sum(sum(c.isalpha() for c in t) for t in texts),
                          "ngrams": dict(top)}
        corpus[lang] = len(texts)
    payload = {
        "generated": today,
        "generator": "tools/catalog/data/language_detect.py",
        "method": f"character 1-3-grams of space-padded words, the {TOP} most frequent per language (ties by code point order), as counts; no source text is stored",
        "corpus": "Unicode CLDR 48.2 locale data: common/main/<lang>.xml display names and labels, and common/annotations/<lang>.xml emoji names and keywords",
        "corpus_url": CLDR,
        "corpus_licence": "Unicode License v3",
        "udhr_not_used": (
            "unicode.org/udhr says: \"Effective January 2024, the Unicode Consortium is no longer hosting the UDHR in Unicode project\". "
            "The UN terms of use allow copies \"for the User's personal, non-commercial use, without any right to resell or redistribute them "
            "or to compile or create derivative works therefrom\". The successor project UDHR in XML (efele.net, github.com/eric-muller/udhr) "
            "has no licence, only \"The UDHR and all its translations are obviously part of our commons.\" None is a clear permissive licence, "
            "so no UDHR text was used."),
        "unicode_license_notice": fetch("LICENSE", cache).decode("utf-8").strip().splitlines(),
        "fetched": today,
        "profiles": profiles,
    }
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/language/language_detect/profiles.json"
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n", encoding="utf-8")
    for lang, size in corpus.items():
        print(f"{lang}: {size} strings, {profiles[lang]['corpus_letters']} letters")
    print(out, out.stat().st_size, "bytes")


if __name__ == "__main__":
    main()
