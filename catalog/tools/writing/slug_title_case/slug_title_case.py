#!/usr/bin/env python3
"""URL slugs, title case by style guide (Chicago 18th and 17th editions, AP, APA), sentence
case, and smart quotes and dashes.

Input (JSON on stdin):
  operation   "slug" | "title_case" | "sentence_case" | "smart_quotes"
  text        the text (required)
  style       title_case: "chicago" (18th ed., default) | "chicago17" | "ap" | "apa"
  separator   slug: "-" (default), "_" or "."
  max_length  slug: at most this many characters, cut at a separator (default 80)
  keep        sentence_case: words to keep as written (proper nouns)
  language    smart_quotes: "en" (default: “ ” ‘ ’) | "es" (« » outside, “ ” inside)
"""
import json
import re
import sys
import unicodedata

EXTRA = {"ß": "ss", "æ": "ae", "Æ": "AE", "œ": "oe", "Œ": "OE", "ø": "o", "Ø": "O", "đ": "d", "Đ": "D",
         "ł": "l", "Ł": "L", "þ": "th", "Þ": "TH", "ð": "d", "Ð": "D", "ı": "i", "&": " and "}
ARTICLES = {"a", "an", "the"}
COORD = {"and", "but", "for", "or", "nor"}
PREPOSITIONS = set("""
aboard about above across after against along amid among around as at atop before behind below beneath
beside besides between beyond by concerning despite down during except for from in inside into like near
of off on onto opposite out outside over past per regarding round since through throughout till to toward
towards under underneath unlike until unto up upon versus via with within without according
""".split())
SHORT_CONJ = {"and", "as", "but", "for", "if", "nor", "or", "so", "yet"}
PREFIXES = set("""anti co counter de dis extra infra inter intra macro micro mid mini mis multi neo non over
post pre pro pseudo re semi sub super supra trans tri ultra un""".split())


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def minor(word, style, position_in_compound=False):
    """Is this (lowercase) word kept lowercase inside a title?"""
    if word in ARTICLES:
        return True
    if style in ("chicago", "chicago17"):
        if word in COORD or word in ("to", "as"):
            return True
        if word in PREPOSITIONS:
            return style == "chicago17" or len(word) <= 4
        return False
    if style == "ap":
        return len(word) <= 3 and (word in PREPOSITIONS or word in SHORT_CONJ)
    return len(word) <= 3 and (word in PREPOSITIONS or word in SHORT_CONJ)  # apa


def keep_as_is(word):
    """Acronyms (NASA) and words with inner capitals (iPhone, McDonald) are left alone."""
    letters = [c for c in word if c.isalpha()]
    return len(letters) > 1 and (all(c.isupper() for c in letters) or any(c.isupper() for c in letters[1:]))


def cap(word):
    for i, c in enumerate(word):
        if c.isalpha():
            return word[:i] + c.upper() + word[i + 1:]
    return word


def core(word):
    return re.sub(r"^[^\w]+|[^\w']+$", "", word).lower()


def title_case(text, style):
    if not any(c.islower() for c in text):
        text = text.lower()  # a title typed in capitals: nothing to keep as an acronym
    tokens = re.split(r"(\s+)", text.strip())
    words = [i for i, t in enumerate(tokens) if t and not t.isspace()]
    out = list(tokens)
    after_break = True
    for n, i in enumerate(words):
        token = tokens[i]
        first, last = n == 0 or after_break, n == len(words) - 1
        after_break = bool(re.search(r"[:?!—]$", token))
        if keep_as_is(token.replace("-", "")) and "-" not in token:
            continue
        parts = token.split("-")
        new = []
        for k, part in enumerate(parts):
            c = core(part)
            if keep_as_is(part):
                new.append(part)
                continue
            lower = part.lower()
            if k == 0:
                edge = first or (last and len(parts) == 1)
                new.append(cap(lower) if edge or not minor(c, style) else lower)
                continue
            if minor(c, style) and not (last and k == len(parts) - 1 and style != "chicago17"):
                new.append(lower)
            elif style == "chicago17" and k == 1 and core(parts[0]) in PREFIXES:
                new.append(part if part[:1].isupper() else lower)  # 17th ed.: anti-intellectual, pre-Columbian
            else:
                new.append(cap(lower))
        out[i] = "-".join(new)
    return "".join(out)


def sentence_case(text, keep):
    keep_lower = {k.lower(): k for k in keep}
    if not any(c.islower() for c in text):
        text = text.lower()
    tokens = re.split(r"(\s+)", text.strip())
    out, start = [], True
    for token in tokens:
        if not token or token.isspace():
            out.append(token)
            continue
        c = core(token)
        if c in keep_lower:
            new = token.lower().replace(c, keep_lower[c], 1)
        elif keep_as_is(token.replace("-", "")):
            new = token
        elif c == "i" or c.startswith("i'"):
            new = cap(token.lower())
        else:
            new = token.lower()
        if start:
            new = cap(new)
        out.append(new)
        start = bool(re.search(r"[:.?!]$", token))
    return "".join(out)


def slug(text, separator, max_length):
    text = "".join(EXTRA.get(c, c) for c in text)
    text = unicodedata.normalize("NFKD", text)
    text = "".join(c for c in text if not unicodedata.combining(c))
    text = text.encode("ascii", "ignore").decode("ascii").lower()
    words = re.findall(r"[a-z0-9]+", text)
    out = ""
    for w in words:
        candidate = w if not out else out + separator + w
        if len(candidate) > max_length:
            if not out:
                out = w[:max_length]
            break
        out = candidate
    return out, words


def smart_quotes(text, language):
    dq_open, dq_close = ("«", "»") if language == "es" else ("“", "”")
    text = text.replace("---", "—").replace("--", "–").replace("...", "…")
    out = []
    depth = 0
    for i, c in enumerate(text):
        prev = text[i - 1] if i else ""
        nxt = text[i + 1] if i + 1 < len(text) else ""
        opening = not prev or prev.isspace() or prev in "([{—–-/«“"
        if c == '"':
            if language == "es":
                if opening:
                    out.append(dq_open if depth == 0 else "“")
                    depth += 1
                else:
                    depth = max(depth - 1, 0)
                    out.append(dq_close if depth == 0 else "”")
            else:
                out.append(dq_open if opening else dq_close)
        elif c == "'":
            if prev.isalnum() and nxt.isalpha():
                out.append("’")  # don't, l'eau
            elif opening and re.match(r"\d\d(s|\b)", text[i + 1:i + 4]):
                out.append("’")  # '90s
            else:
                out.append("‘" if opening else "’")
        else:
            out.append(c)
    return "".join(out)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be a non-empty string")
    if len(text) > 100000:
        fail("text is longer than 100000 characters")
    operation = request.get("operation")
    if operation == "slug":
        separator = request.get("separator", "-")
        if separator not in ("-", "_", "."):
            fail("separator must be -, _ or .")
        max_length = request.get("max_length", 80)
        if not isinstance(max_length, int) or isinstance(max_length, bool) or not 1 <= max_length <= 2000:
            fail("max_length must be an integer from 1 to 2000")
        result, words = slug(text, separator, max_length)
        out = {"operation": "slug", "slug": result, "length": len(result), "truncated": len(separator.join(words)) > len(result)}
    elif operation == "title_case":
        style = request.get("style", "chicago")
        if style not in ("chicago", "chicago17", "ap", "apa"):
            fail("style must be chicago, chicago17, ap or apa")
        out = {"operation": "title_case", "style": style, "result": title_case(text, style),
               "note": "part of speech is guessed from word lists: a preposition used as an adverb (Look Up) or a word kept lowercase by sense may need a manual fix"}
    elif operation == "sentence_case":
        keep = request.get("keep", [])
        if not isinstance(keep, list) or not all(isinstance(k, str) and k.strip() for k in keep):
            fail("keep must be a list of words")
        out = {"operation": "sentence_case", "result": sentence_case(text, keep),
               "note": "proper nouns are only kept when listed in keep, all in capitals, or with an inner capital"}
    elif operation == "smart_quotes":
        language = request.get("language", "en")
        if language not in ("en", "es"):
            fail("language must be en or es")
        out = {"operation": "smart_quotes", "language": language, "result": smart_quotes(text, language)}
    else:
        fail("operation must be slug, title_case, sentence_case or smart_quotes")
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
