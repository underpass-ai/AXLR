#!/usr/bin/env python3
"""Markdown (a CommonMark subset plus GFM tables) to HTML, to plain text, or to a slide-deck
outline split on headings. Raw HTML in the input is escaped, never passed through.

Supported: ATX and setext headings, paragraphs, thematic breaks, indented and fenced code
blocks, block quotes, bullet and ordered lists (tight and loose), GFM pipe tables; inline:
backslash escapes, entity references, code spans, emphasis and strong emphasis (the
CommonMark delimiter algorithm), inline links and images, autolinks, hard and soft breaks.
Not supported: link reference definitions, HTML blocks and raw inline HTML (escaped instead).

Input (JSON on stdin):
  markdown     the text (required)
  to           "html" (default) | "text" | "slides"
  split_level  slides: headings of this level or higher start a slide (default 2)
"""
import html
import json
import re
import sys
import unicodedata

# ---------------------------------------------------------------- blocks
THEMATIC = re.compile(r"^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$")
ATX = re.compile(r"^ {0,3}(#{1,6})(?:[ \t]+|$)(.*)$")
FENCE = re.compile(r"^( {0,3})(`{3,}|~{3,})(.*)$")
SETEXT = re.compile(r"^ {0,3}(=+|-+)[ \t]*$")
BULLET = re.compile(r"^( {0,3})([-+*])(?=[ \t]|$)")
ORDERED = re.compile(r"^( {0,3})(\d{1,9})([.)])(?=[ \t]|$)")
QUOTE = re.compile(r"^ {0,3}>")
TABLE_DELIM = re.compile(r"^ {0,3}\|?[ \t]*:?-+:?[ \t]*(?:\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$")


def expand_tabs(line):
    """Expand tabs in the leading whitespace (tab stops of 4); tabs inside content stay."""
    out, col, k = [], 0, 0
    while k < len(line) and (line[k] in " \t" or line[k] in ">-+*.)0123456789" and "\t" in line[k:k + 12]):
        if line[k] == "\t":
            n = 4 - col % 4
            out.append(" " * n)
            col += n
        else:
            out.append(line[k])
            col += 1
        k += 1
    return "".join(out) + line[k:]


class Lazy(str):
    """A lazy continuation line: it continues a paragraph and cannot be a setext underline."""


class Collector(list):
    """Lines of a container (quote, list item) that track, as they are added, whether the
    content ends inside a fenced code block and where the last paragraph-like run began, so
    a lazy continuation line can be judged without rescanning."""

    def __init__(self, lines=()):
        super().__init__()
        self.fence, self.run_first, self.closed = None, None, True
        for line in lines:
            self.append(line)

    def append(self, line):
        super().append(line)
        if self.fence is None:
            m = FENCE.match(line)
            if m and not (m.group(2)[0] == "`" and "`" in m.group(3)):
                self.fence = m.group(2)
        elif re.match(r"^ {0,3}" + re.escape(self.fence[0]) + "{" + str(len(self.fence)) + r",}[ \t]*$", line):
            self.fence = None
        if is_blank(line) or ATX.match(line) or THEMATIC.match(line):
            self.run_first, self.closed = None, True
        elif self.run_first is None:
            self.run_first, self.closed = line, False

    def extend(self, lines):
        for line in lines:
            self.append(line)

    def __iadd__(self, lines):
        self.extend(lines)
        return self


def lazy_ok(lines):
    """The collected lines end in a paragraph that a lazy line may continue."""
    if not lines or lines.fence is not None or lines.closed:
        return False
    first = lines.run_first
    return not (indent_of(first) >= 4 or FENCE.match(first))


def indent_of(line):
    return len(line) - len(line.lstrip(" "))


def is_blank(line):
    return not line.strip()


def list_marker(line):
    """(kind, marker, content_offset, rest) of a list item start, or None."""
    m = BULLET.match(line)
    kind = "bullet"
    if not m:
        m = ORDERED.match(line)
        kind = "ordered"
    if not m:
        return None
    end = m.end()
    rest = line[end:]
    spaces = len(rest) - len(rest.lstrip(" "))
    if is_blank(rest):
        offset = end + 1
        content = ""
    elif spaces > 4:
        offset = end + 1
        content = rest[1:]
    else:
        offset = end + spaces
        content = rest[spaces:]
    marker = m.group(2) if kind == "bullet" else m.group(3)
    start = int(m.group(2)) if kind == "ordered" else None
    return {"kind": kind, "marker": marker, "offset": offset, "content": content, "start": start}


def split_row(line):
    s = line.strip()
    if s.startswith("|"):
        s = s[1:]
    if s.endswith("|") and not s.endswith("\\|"):
        s = s[:-1]
    cells, cur, i = [], [], 0
    while i < len(s):
        if s[i] == "\\" and i + 1 < len(s) and s[i + 1] == "|":
            cur.append("|")
            i += 2
            continue
        if s[i] == "|":
            cells.append("".join(cur).strip())
            cur = []
        else:
            cur.append(s[i])
        i += 1
    cells.append("".join(cur).strip())
    return cells


def starts_block(line, in_paragraph=True):
    if is_blank(line):
        return True
    if THEMATIC.match(line) or ATX.match(line) or QUOTE.match(line):
        return True
    m = FENCE.match(line)
    if m and not (m.group(2)[0] == "`" and "`" in m.group(3)):
        return True
    lm = list_marker(line)
    if lm and in_paragraph:
        # a list item can interrupt a paragraph only if it is not empty and an ordered one starts at 1
        return bool(lm["content"].strip()) and (lm["kind"] == "bullet" or lm["start"] == 1)
    return bool(lm)


def parse_blocks(lines):
    blocks, i, n = [], 0, len(lines)
    while i < n:
        line = lines[i]
        if is_blank(line):
            blocks.append({"type": "blank"})
            i += 1
            continue
        if indent_of(line) >= 4:
            body = []
            while i < n and (indent_of(lines[i]) >= 4 or is_blank(lines[i])):
                body.append(lines[i][4:] if not is_blank(lines[i]) else lines[i][4:] if len(lines[i]) > 4 else "")
                i += 1
            while body and is_blank(body[-1]):
                body.pop()
                i -= 1
            blocks.append({"type": "code", "info": "", "text": "".join(b + "\n" for b in body)})
            continue
        m = FENCE.match(line)
        if m and not (m.group(2)[0] == "`" and "`" in m.group(3)):
            pad, fence, info = len(m.group(1)), m.group(2), m.group(3).strip()
            body = []
            i += 1
            while i < n:
                close = re.match(r"^ {0,3}(" + re.escape(fence[0]) + "{" + str(len(fence)) + r",})[ \t]*$", lines[i])
                if close:
                    i += 1
                    break
                text = lines[i]
                strip = min(pad, indent_of(text))
                body.append(text[strip:])
                i += 1
            blocks.append({"type": "code", "info": unescape_text(info), "text": "".join(b + "\n" for b in body)})
            continue
        if THEMATIC.match(line):
            blocks.append({"type": "hr"})
            i += 1
            continue
        m = ATX.match(line)
        if m:
            text = m.group(2)
            text = re.sub(r"(?:^|[ \t]+)#+[ \t]*$", "", text).strip()
            blocks.append({"type": "heading", "level": len(m.group(1)), "text": text})
            i += 1
            continue
        if QUOTE.match(line):
            inner = Collector()
            while i < n:
                cur = lines[i]
                if QUOTE.match(cur):
                    stripped = re.sub(r"^ {0,3}> ?", "", cur, count=1)
                    inner.append(stripped)
                    i += 1
                elif not is_blank(cur) and lazy_ok(inner) and not starts_block(cur) and indent_of(cur) < 4 or \
                        not is_blank(cur) and lazy_ok(inner) and indent_of(cur) >= 4:
                    inner.append(Lazy(cur))  # lazy continuation of a paragraph
                    i += 1
                else:
                    break
            blocks.append({"type": "quote", "children": parse_blocks(inner)})
            continue
        lm = list_marker(line)
        if lm:
            items, kind, marker, start, loose = [], lm["kind"], lm["marker"], lm["start"], False
            while i < n:
                lm = list_marker(lines[i])
                if not lm or lm["kind"] != kind or lm["marker"] != marker or THEMATIC.match(lines[i]):
                    break
                offset = lm["offset"]
                body = Collector([lm["content"]])
                i += 1
                while i < n:
                    cur = lines[i]
                    if is_blank(cur):
                        j = i
                        while j < n and is_blank(lines[j]):
                            j += 1
                        if is_blank(body[0]) and len(body) == 1:
                            break  # an item can begin with at most one blank line
                        if j < n and indent_of(lines[j]) >= offset:
                            body += [""] * (j - i)
                            i = j
                            continue
                        break
                    if indent_of(cur) >= offset:
                        body.append(cur[offset:])
                        i += 1
                        continue
                    if lazy_ok(body) and not list_marker(cur) and (indent_of(cur) >= 4 or not starts_block(cur)):
                        body.append(Lazy(cur))  # lazy continuation
                        i += 1
                        continue
                    break
                children = parse_blocks(body)
                seen, gap = False, False
                for b in children:  # a blank line between two direct children makes the list loose
                    if b["type"] == "blank":
                        gap = seen
                    else:
                        loose = loose or gap
                        seen, gap = True, False
                items.append({"children": [b for b in children if b["type"] != "blank"]})
                j = i
                while j < n and is_blank(lines[j]):
                    j += 1
                if j > i:
                    nxt = list_marker(lines[j]) if j < n else None
                    if nxt and nxt["kind"] == kind and nxt["marker"] == marker and not THEMATIC.match(lines[j]):
                        loose = True
                        i = j
                        continue
                    break
            blocks.append({"type": "list", "kind": kind, "start": start, "items": items, "loose": loose})
            continue
        # table: a header row followed by a delimiter row with the same number of cells
        if "|" in line and i + 1 < n and TABLE_DELIM.match(lines[i + 1]) and "-" in lines[i + 1]:
            head, delim = split_row(line), split_row(lines[i + 1])
            if len(head) == len(delim):
                aligns = []
                for d in delim:
                    d = d.strip()
                    aligns.append("center" if d.startswith(":") and d.endswith(":") else
                                  "right" if d.endswith(":") else "left" if d.startswith(":") else None)
                rows = []
                i += 2
                while i < n and not is_blank(lines[i]) and not starts_block(lines[i], False):
                    cells = split_row(lines[i])
                    cells = (cells + [""] * len(head))[:len(head)]
                    rows.append(cells)
                    i += 1
                blocks.append({"type": "table", "head": head, "aligns": aligns, "rows": rows})
                continue
        # paragraph, possibly a setext heading
        para = [line]
        i += 1
        heading = None
        while i < n:
            cur = lines[i]
            st = SETEXT.match(cur) if not isinstance(cur, Lazy) else None
            if st:
                heading = 1 if st.group(1)[0] == "=" else 2
                i += 1
                break
            if starts_block(cur):
                break
            if "|" in cur and i + 1 < n and TABLE_DELIM.match(lines[i + 1]) and "-" in lines[i + 1] \
                    and len(split_row(cur)) == len(split_row(lines[i + 1])):
                break
            para.append(cur)
            i += 1
        text = "\n".join(p.lstrip(" ") for p in para)
        if heading:
            blocks.append({"type": "heading", "level": heading, "text": text.strip()})
        else:
            blocks.append({"type": "paragraph", "text": text.rstrip(" ")})
    return blocks


# ---------------------------------------------------------------- inlines
PUNCT_ASCII = set("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~")


def is_punct(ch):
    return bool(ch) and (ch in PUNCT_ASCII or unicodedata.category(ch)[0] in "PS")


def is_space(ch):
    return not ch or ch.isspace()


class Node:
    def __init__(self, kind, text="", **attrs):
        self.kind, self.text, self.children = kind, text, []
        self.attrs = attrs
        self.prev = self.next = None


class Delim:
    def __init__(self, node, char, count, can_open, can_close, image=False):
        self.node, self.char, self.count, self.orig = node, char, count, count
        self.can_open, self.can_close, self.image = can_open, can_close, image
        self.active = True
        self.prev = self.next = None


def unescape_text(text):
    text = re.sub(r"\\([!\"#$%&'()*+,\-./:;<=>?@\[\\\]^_`{|}~])", r"\1", text)
    return decode_entities(text)


ENTITY = re.compile(r"&(?:#[xX][0-9a-fA-F]{1,6}|#[0-9]{1,7}|[A-Za-z][A-Za-z0-9]{1,31});")


def decode_entities(text):
    def one(m):
        s = m.group(0)
        if s.startswith("&#"):
            code = int(s[3:-1], 16) if s[2] in "xX" else int(s[2:-1])
            return "\ufffd" if code == 0 or code > 0x10FFFF or 0xD800 <= code <= 0xDFFF else chr(code)
        decoded = html.unescape(s)
        return decoded if decoded != s else s
    return ENTITY.sub(one, text)


def parse_link_tail(s, i):
    """After ']' at s[i-1]: parse '(dest "title")'. Returns (dest, title, end) or None."""
    if i >= len(s) or s[i] != "(":
        return None
    j = i + 1
    while j < len(s) and s[j] in " \t\n":
        j += 1
    dest = ""
    if j < len(s) and s[j] == "<":
        k = j + 1
        buf = []
        while k < len(s) and s[k] not in "<>\n":
            if s[k] == "\\" and k + 1 < len(s) and s[k + 1] in PUNCT_ASCII:
                buf.append(s[k:k + 2])
                k += 2
                continue
            buf.append(s[k])
            k += 1
        if k >= len(s) or s[k] != ">":
            return None
        dest, j = "".join(buf), k + 1
    else:
        depth, k, buf = 0, j, []
        while k < len(s):
            c = s[k]
            if c == "\\" and k + 1 < len(s) and s[k + 1] in PUNCT_ASCII:
                buf.append(s[k:k + 2])
                k += 2
                continue
            if c in " \t\n" or ord(c) < 0x20:
                break
            if c == "(":
                depth += 1
            elif c == ")":
                if depth == 0:
                    break
                depth -= 1
            buf.append(c)
            k += 1
        if depth:
            return None
        dest, j = "".join(buf), k
    title = None
    k = j
    while k < len(s) and s[k] in " \t\n":
        k += 1
    if k < len(s) and s[k] in "\"'(" and k > j:
        close = ")" if s[k] == "(" else s[k]
        m = k + 1
        buf = []
        while m < len(s) and s[m] != close:
            if s[m] == "\\" and m + 1 < len(s) and s[m + 1] in PUNCT_ASCII:
                buf.append(s[m:m + 2])
                m += 2
                continue
            if close == ")" and s[m] == "(":
                return None
            buf.append(s[m])
            m += 1
        if m >= len(s):
            return None
        title = "".join(buf)
        k = m + 1
        while k < len(s) and s[k] in " \t\n":
            k += 1
    if k >= len(s) or s[k] != ")":
        return None
    return unescape_text(dest), (unescape_text(title) if title is not None else None), k + 1


TICKS = re.compile(r"`+")
AUTOLINK = re.compile(r"<([A-Za-z][A-Za-z0-9+.\-]{1,31}:[^\s<>]*)>")
EMAIL = re.compile(r"<([a-zA-Z0-9.!#$%&'*+/=?^_`{|}~\-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?"
                   r"(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*)>")
PLAIN_RUN = re.compile(r"[^\\`<&\n*_!\[\]]+")


def parse_inlines(s):
    head = Node("root")
    tail = [head]
    state = {"delims": None, "brackets": []}

    def append(node):
        node.prev = tail[0]
        tail[0].next = node
        tail[0] = node
        return node

    def text(t):
        if tail[0].kind == "text" and not getattr(tail[0], "frozen", False):
            tail[0].text += t
            return tail[0]
        return append(Node("text", t))

    def push_delim(d):
        d.prev = state["delims"]
        if state["delims"]:
            state["delims"].next = d
        state["delims"] = d

    i, n = 0, len(s)
    while i < n:
        c = s[i]
        if c == "\\":
            if i + 1 < n and s[i + 1] == "\n":
                append(Node("html", "<br />\n", plain="\n"))
                i += 2
                while i < n and s[i] == " ":
                    i += 1
                continue
            if i + 1 < n and s[i + 1] in PUNCT_ASCII:
                text(s[i + 1])
                i += 2
                continue
            text("\\")
            i += 1
            continue
        if c == "`":
            m = TICKS.match(s, i)
            ticks = m.group(0)
            j = i + len(ticks)
            found = None
            for close in TICKS.finditer(s, j):
                if len(close.group(0)) == len(ticks):
                    found = close
                    break
            if found:
                code = s[j:found.start()].replace("\n", " ")
                if len(code) > 2 and code[0] == " " and code[-1] == " " and code.strip(" "):
                    code = code[1:-1]
                append(Node("html", "<code>" + esc(code) + "</code>", plain=code))
                i = found.end()
            else:
                text(ticks)
                i = j
            continue
        if c == "<":
            m = AUTOLINK.match(s, i)
            if m:
                url = m.group(1)
                append(Node("html", f'<a href="{esc(normalize_url(url))}">{esc(url)}</a>', plain=url))
                i = m.end()
                continue
            m = EMAIL.match(s, i)
            if m:
                mail = m.group(1)
                append(Node("html", f'<a href="mailto:{esc(normalize_url(mail))}">{esc(mail)}</a>', plain=mail))
                i = m.end()
                continue
            text("<")
            i += 1
            continue
        if c == "&":
            m = ENTITY.match(s, i)
            if m:
                decoded = decode_entities(m.group(0))
                node = append(Node("text", decoded))
                node.frozen = True
                i = m.end()
                continue
            text("&")
            i += 1
            continue
        if c == "\n":
            node = tail[0]
            if node.kind == "text" and node.text.endswith("  "):
                node.text = node.text.rstrip(" ")
                append(Node("html", "<br />\n", plain="\n"))
            else:
                if node.kind == "text":
                    node.text = node.text.rstrip(" ")
                append(Node("html", "\n", plain="\n"))
            i += 1
            while i < n and s[i] == " ":
                i += 1
            continue
        if c in "*_":
            j = i
            while j < n and s[j] == c:
                j += 1
            before = s[i - 1] if i > 0 else ""
            after = s[j] if j < n else ""
            left = not is_space(after) and (not is_punct(after) or is_space(before) or is_punct(before))
            right = not is_space(before) and (not is_punct(before) or is_space(after) or is_punct(after))
            if c == "*":
                can_open, can_close = left, right
            else:
                can_open = left and (not right or is_punct(before))
                can_close = right and (not left or is_punct(after))
            node = append(Node("text", s[i:j]))
            node.frozen = True
            push_delim(Delim(node, c, j - i, can_open, can_close))
            i = j
            continue
        if c == "!" and i + 1 < n and s[i + 1] == "[":
            node = append(Node("text", "!["))
            node.frozen = True
            state["brackets"].append({"node": node, "image": True, "active": True, "delims": state["delims"], "pos": i + 2})
            i += 2
            continue
        if c == "[":
            node = append(Node("text", "["))
            node.frozen = True
            state["brackets"].append({"node": node, "image": False, "active": True, "delims": state["delims"], "pos": i + 1})
            i += 1
            continue
        if c == "]":
            if not state["brackets"]:
                text("]")
                i += 1
                continue
            opener = state["brackets"][-1]
            if not opener["active"]:
                state["brackets"].pop()
                text("]")
                i += 1
                continue
            tail_info = parse_link_tail(s, i + 1)
            if tail_info is None:
                state["brackets"].pop()
                text("]")
                i += 1
                continue
            dest, title, end = tail_info
            # move the nodes after the opener into the link
            link = Node("image" if opener["image"] else "link", href=dest, title=title)
            first = opener["node"].next
            process_emphasis(state, opener["delims"])
            child = first
            while child is not None:
                nxt = child.next
                link.children.append(child)
                child = nxt
            opener_node = opener["node"]
            link.prev = opener_node.prev
            if opener_node.prev:
                opener_node.prev.next = link
            tail[0] = link
            link.next = None
            state["brackets"].pop()
            if not opener["image"]:
                for b in state["brackets"]:
                    if not b["image"]:
                        b["active"] = False
            i = end
            continue
        m = PLAIN_RUN.match(s, i)
        chunk = m.group(0) if m else c
        text(chunk)
        i += len(chunk)
    process_emphasis(state, None)
    out, node = [], head.next
    while node is not None:
        out.append(node)
        node = node.next
    return out


def remove_delim(state, d):
    if d.prev:
        d.prev.next = d.next
    if d.next:
        d.next.prev = d.prev
    else:
        state["delims"] = d.prev


def process_emphasis(state, bottom):
    # find the first delimiter above bottom
    d = state["delims"]
    first = None
    while d is not None and d is not bottom:
        first = d
        d = d.prev
    openers_bottom = {}
    closer = first
    while closer is not None:
        if not closer.can_close:
            closer = closer.next
            continue
        key = (closer.char, closer.can_open, closer.orig % 3)
        opener = closer.prev
        found = False
        limit = openers_bottom.get(key, bottom)
        while opener is not None and opener is not bottom and opener is not limit:
            odd = (opener.can_close or closer.can_open) and (opener.orig + closer.orig) % 3 == 0 \
                and not (opener.orig % 3 == 0 and closer.orig % 3 == 0)
            if opener.char == closer.char and opener.can_open and not odd:
                found = True
                break
            opener = opener.prev
        if not found:
            openers_bottom[key] = closer.prev
            nxt = closer.next
            if not closer.can_open:
                remove_delim(state, closer)
            closer = nxt
            continue
        use = 2 if opener.count >= 2 and closer.count >= 2 else 1
        opener.count -= use
        closer.count -= use
        opener.node.text = opener.node.text[:len(opener.node.text) - use]
        closer.node.text = closer.node.text[:len(closer.node.text) - use]
        emph = Node("strong" if use == 2 else "em")
        child = opener.node.next
        while child is not None and child is not closer.node:
            nxt = child.next
            emph.children.append(child)
            child = nxt
        opener.node.next = emph
        emph.prev = opener.node
        emph.next = closer.node
        closer.node.prev = emph
        # delimiters between opener and closer are gone
        d = closer.prev
        while d is not None and d is not opener:
            prev = d.prev
            remove_delim(state, d)
            d = prev
        if opener.count == 0:
            unlink(opener.node)
            remove_delim(state, opener)
        if closer.count == 0:
            nxt = closer.next
            unlink(closer.node)
            remove_delim(state, closer)
            closer = nxt
    # remove all delimiters above bottom
    while state["delims"] is not None and state["delims"] is not bottom:
        remove_delim(state, state["delims"])


def unlink(node):
    if node.prev:
        node.prev.next = node.next
    if node.next:
        node.next.prev = node.prev


def esc(text):
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;")


def safe_url(url):
    """Like cmark's --safe: javascript:, vbscript:, file: and non-image data: URLs become empty."""
    scheme = re.sub(r"[\s\x00-\x1f]", "", url).lower()
    if scheme.startswith(("javascript:", "vbscript:", "file:")) or \
            scheme.startswith("data:") and not re.match(r"data:image/(png|gif|jpeg|webp);", scheme):
        return ""
    return url


def normalize_url(url):
    url = safe_url(url)
    safe = "%;/?:@&=+$,-_.!~*'()#"
    out = []
    for ch in url:
        if ch.isascii() and (ch.isalnum() or ch in safe):
            out.append(ch)
        else:
            out.append("".join(f"%{b:02X}" for b in ch.encode("utf-8")))
    return "".join(out)


def render_inline(nodes):
    out = []
    for node in nodes:
        if node.kind == "text":
            out.append(esc(node.text))
        elif node.kind == "html":
            out.append(node.text)
        elif node.kind in ("em", "strong"):
            tag = node.kind
            out.append(f"<{tag}>{render_inline(node.children)}</{tag}>")
        elif node.kind == "link":
            title = f' title="{esc(node.attrs["title"])}"' if node.attrs["title"] is not None else ""
            out.append(f'<a href="{esc(normalize_url(node.attrs["href"]))}"{title}>{render_inline(node.children)}</a>')
        elif node.kind == "image":
            title = f' title="{esc(node.attrs["title"])}"' if node.attrs["title"] is not None else ""
            out.append(f'<img src="{esc(normalize_url(node.attrs["href"]))}" alt="{esc(plain_inline(node.children))}"{title} />')
    return "".join(out)


def plain_inline(nodes, links=False):
    out = []
    for node in nodes:
        if node.kind == "text":
            out.append(node.text)
        elif node.kind == "html":
            out.append(node.attrs.get("plain", ""))
        elif node.kind in ("em", "strong"):
            out.append(plain_inline(node.children, links))
        elif node.kind == "link":
            label = plain_inline(node.children, links)
            out.append(f"{label} ({node.attrs['href']})" if links and node.attrs["href"] and node.attrs["href"] != label else label)
        elif node.kind == "image":
            alt = plain_inline(node.children, links)
            out.append(f"[image: {alt}]" if links else alt)
    return "".join(out)


# ---------------------------------------------------------------- renderers
def to_html(blocks, tight=False):
    out = []
    for b in blocks:
        t = b["type"]
        if t == "blank":
            continue
        if t == "paragraph":
            inner = render_inline(parse_inlines(b["text"]))
            out.append(inner if tight else f"<p>{inner}</p>")
        elif t == "heading":
            out.append(f"<h{b['level']}>{render_inline(parse_inlines(b['text']))}</h{b['level']}>")
        elif t == "hr":
            out.append("<hr />")
        elif t == "code":
            info = b["info"].split()[0] if b["info"].split() else ""
            cls = f' class="language-{esc(info)}"' if info else ""
            out.append(f"<pre><code{cls}>{esc(b['text'])}</code></pre>")
        elif t == "quote":
            inner = to_html(b["children"])
            out.append("<blockquote>\n" + (inner + "\n" if inner else "") + "</blockquote>")
        elif t == "list":
            tag = "ol" if b["kind"] == "ordered" else "ul"
            start = f' start="{b["start"]}"' if b["kind"] == "ordered" and b["start"] != 1 else ""
            items = []
            for item in b["items"]:
                inner = to_html(item["children"], tight=not b["loose"])
                if not item["children"]:
                    items.append("<li></li>")
                elif b["loose"]:
                    items.append(f"<li>\n{inner}\n</li>")
                else:
                    first_par = item["children"][0]["type"] == "paragraph"
                    last_par = item["children"][-1]["type"] == "paragraph"
                    items.append("<li>" + ("" if first_par else "\n") + inner + ("" if last_par else "\n") + "</li>")
            out.append(f"<{tag}{start}>\n" + "\n".join(items) + f"\n</{tag}>")
        elif t == "table":
            def cell(tag, text, align):
                a = f' align="{align}"' if align else ""
                return f"<{tag}{a}>{render_inline(parse_inlines(text))}</{tag}>"
            head = "<tr>\n" + "\n".join(cell("th", h, a) for h, a in zip(b["head"], b["aligns"])) + "\n</tr>"
            rows = ["<tr>\n" + "\n".join(cell("td", c, a) for c, a in zip(r, b["aligns"])) + "\n</tr>" for r in b["rows"]]
            body = ("\n<tbody>\n" + "\n".join(rows) + "\n</tbody>") if rows else ""
            out.append("<table>\n<thead>\n" + head + "\n</thead>" + body + "\n</table>")
    return "\n".join(out)


def to_text(blocks, prefix=""):
    out = []
    for b in blocks:
        t = b["type"]
        if t == "blank":
            continue
        if t in ("paragraph", "heading"):
            out.append(plain_inline(parse_inlines(b["text"]), links=True))
        elif t == "hr":
            out.append("")
        elif t == "code":
            out.append(b["text"].rstrip("\n"))
        elif t == "quote":
            inner = to_text(b["children"])
            out.append("\n".join("> " + x if x else ">" for x in inner.split("\n")))
        elif t == "list":
            lines = []
            for k, item in enumerate(b["items"]):
                mark = f"{(b['start'] or 1) + k}. " if b["kind"] == "ordered" else "- "
                inner = to_text(item["children"]).split("\n")
                lines.append(mark + inner[0])
                lines += [" " * len(mark) + x if x else "" for x in inner[1:]]
            out.append("\n".join(lines))
        elif t == "table":
            rows = [b["head"]] + b["rows"]
            out.append("\n".join("\t".join(plain_inline(parse_inlines(c), links=True) for c in r) for r in rows))
    return "\n\n".join(x for x in out if x is not None)


def to_slides(blocks, level):
    slides, current = [], None

    def points(bs, depth=0):
        found = []
        for b in bs:
            t = b["type"]
            if t == "paragraph":
                found.append("  " * depth + plain_inline(parse_inlines(b["text"])).replace("\n", " "))
            elif t == "list":
                for item in b["items"]:
                    kids = item["children"]
                    first = [k for k in kids if k["type"] == "paragraph"][:1]
                    if first:
                        found.append("  " * depth + plain_inline(parse_inlines(first[0]["text"])).replace("\n", " "))
                    found += points([k for k in kids if k not in first], depth + 1)
            elif t == "quote":
                found += ["  " * depth + "\u201c" + p.strip() + "\u201d" for p in points(b["children"])]
            elif t == "code":
                found.append("  " * depth + f"[code: {b['text'].count(chr(10))} lines]")
            elif t == "table":
                found.append("  " * depth + f"[table: {len(b['rows'])} rows x {len(b['head'])} columns]")
            elif t == "heading":
                found.append("  " * depth + plain_inline(parse_inlines(b["text"])))
        return found

    pending = []
    for b in blocks:
        if b["type"] == "heading" and b["level"] <= level:
            if current is None and pending:
                slides.append({"title": None, "level": None, "blocks": pending})
            current = {"title": plain_inline(parse_inlines(b["text"])), "level": b["level"], "blocks": []}
            slides.append(current)
        elif b["type"] == "hr" and current is not None:
            current = {"title": None, "level": None, "blocks": []}
            slides.append(current)
        elif current is None:
            pending.append(b)
        else:
            current["blocks"].append(b)
    if current is None and pending:
        slides.append({"title": None, "level": None, "blocks": pending})
    result, lines = [], []
    for k, s in enumerate(slides, 1):
        pts = points(s["blocks"])
        result.append({"number": k, "title": s["title"], "level": s["level"], "points": pts})
        lines.append(f"{k}. {s['title'] or '(untitled)'}")
        lines += ["   - " + p for p in pts]
    return result, "\n".join(lines) + ("\n" if lines else "")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    markdown = request.get("markdown")
    if not isinstance(markdown, str):
        fail("markdown must be a string")
    if len(markdown) > 1000000:
        fail("markdown is longer than 1000000 characters")
    target = request.get("to", "html")
    if target not in ("html", "text", "slides"):
        fail("to must be html, text or slides")
    level = request.get("split_level", 2)
    if not isinstance(level, int) or isinstance(level, bool) or not 1 <= level <= 6:
        fail("split_level must be an integer from 1 to 6")
    lines = [expand_tabs(x) for x in markdown.replace("\r\n", "\n").replace("\r", "\n").replace("\0", "\ufffd").split("\n")]
    if lines and lines[-1] == "":
        lines.pop()
    try:
        convert(lines, target, level)
    except RecursionError:
        fail("the document nests block quotes, lists or emphasis too deeply")


def convert(lines, target, level):
    blocks = parse_blocks(lines)
    if target == "html":
        rendered = to_html(blocks)
        print(json.dumps({"to": "html", "html": rendered + "\n" if rendered else ""}, ensure_ascii=False))
    elif target == "text":
        print(json.dumps({"to": "text", "text": to_text(blocks) + "\n"}, ensure_ascii=False))
    else:
        slides, outline = to_slides(blocks, level)
        print(json.dumps({"to": "slides", "split_level": level, "count": len(slides), "slides": slides,
                          "outline": outline}, ensure_ascii=False))


if __name__ == "__main__":
    main()
