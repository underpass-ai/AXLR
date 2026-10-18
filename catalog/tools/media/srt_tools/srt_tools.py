#!/usr/bin/env python3
"""Subtitle tools for SubRip (.srt) and WebVTT (.vtt) given as text: parse, shift by
milliseconds, scale for a frame-rate change, convert SRT <-> VTT, merge two files and find
overlapping cues.

Input (JSON on stdin):
  operation   "parse" (default) | "shift" | "scale" | "convert" | "merge" | "overlaps"
  text        the subtitle file (format detected: WEBVTT header -> vtt, else srt)
  text2       merge: the second file
  offset_ms   shift: milliseconds to add (negative moves earlier); merge: shift of text2 (default 0)
  factor      scale: multiply every time by this (number or decimal string), or
  from_fps, to_fps   scale: factor = from_fps / to_fps (subtitles timed at from_fps for a video
              played at to_fps, e.g. 25 -> 23.976 for a PAL speed-up)
  output      "srt" | "vtt": format of the written file (default: the input's; convert: the other)
"""
import json
import re
import sys
from fractions import Fraction

MAX_TEXT = 10_000_000
TIME = r"(?:(\d+):)?(\d{1,2}):(\d{2})[,.](\d{1,3})"
TIMING = re.compile(rf"^\s*{TIME}\s*-->\s*{TIME}(.*)$")
SRT_TAGS = re.compile(r"</?(?:b|i|u)>|</?font[^>]*>", re.I)


class SubError(Exception):
    pass


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def to_ms(h, m, s, frac):
    if int(m) > 59 or int(s) > 59:
        raise SubError(f"minutes and seconds must be below 60 ({m}:{s})")
    return ((int(h or 0) * 60 + int(m)) * 60 + int(s)) * 1000 + int(frac.ljust(3, "0"))


def stamp(ms, sep):
    h, rest = divmod(ms, 3_600_000)
    m, rest = divmod(rest, 60_000)
    s, rest = divmod(rest, 1000)
    return f"{h:02d}:{m:02d}:{s:02d}{sep}{rest:03d}"


def detect(text):
    return "vtt" if text.lstrip("﻿").startswith("WEBVTT") else "srt"


def blocks(text):
    lines = text.lstrip("﻿").replace("\r\n", "\n").replace("\r", "\n").split("\n")
    block, start = [], 1
    for number, line in enumerate(lines, 1):
        if line.strip() == "":
            if block:
                yield start, block
                block = []
        else:
            if not block:
                start = number
            block.append(line)
    if block:
        yield start, block


def parse(text):
    kind = detect(text)
    cues, warnings = [], []
    first = True
    for line_no, block in blocks(text):
        if kind == "vtt" and first:
            first = False
            if block[0].startswith("WEBVTT"):
                continue
        first = False
        if kind == "vtt" and re.match(r"^(NOTE|STYLE|REGION)(\s|$)", block[0]):
            continue
        timing_at = next((i for i, line in enumerate(block[:2]) if "-->" in line), None)
        if timing_at is None:
            warnings.append(f"line {line_no}: block without a timing line skipped")
            continue
        m = TIMING.match(block[timing_at])
        if not m:
            warnings.append(f"line {line_no + timing_at}: unreadable timing '{block[timing_at].strip()}' skipped")
            continue
        try:
            start = to_ms(*m.group(1, 2, 3, 4))
            end = to_ms(*m.group(5, 6, 7, 8))
        except SubError as error:
            warnings.append(f"line {line_no + timing_at}: {error}")
            continue
        ident = block[0].strip() if timing_at == 1 else None
        if end < start:
            warnings.append(f"line {line_no + timing_at}: cue ends before it starts")
        cues.append({"id": ident, "start_ms": start, "end_ms": end, "settings": m.group(9).strip(),
                     "text": "\n".join(block[timing_at + 1:]), "line": line_no})
    return kind, cues, warnings


def vtt_to_srt_text(text):
    text = re.sub(r"<v(?:\.[^\s>]*)?\s+([^>]*)>", "", text)  # voice spans: keep the words
    text = re.sub(r"<\d{1,2}:\d{2}(?::\d{2})?\.\d{3}>", "", text)  # karaoke timestamps
    text = re.sub(r"</?(?:c|v|lang|ruby|rt)(?:[.\s][^>]*)?>", "", text)
    text = re.sub(r"<(/?)(b|i|u)\.[^>]*>", r"<\1\2>", text)
    return text.replace("&lt;", "<").replace("&gt;", ">").replace("&nbsp;", " ").replace("&amp;", "&")


def srt_to_vtt_text(text):
    text = re.sub(r"</?font[^>]*>", "", text, flags=re.I)
    text = re.sub(r"&(?!(?:amp|lt|gt|nbsp|lrm|rlm);)", "&amp;", text)
    # a '<' that does not open a tag VTT knows must be escaped
    text = re.sub(r"<(?!/?(?:b|i|u|c|v|lang|ruby|rt)\b|\d)", "&lt;", text)
    return text.replace("-->", "->")


def write(cues, kind, source_kind):
    if kind == "srt":
        out = []
        for n, c in enumerate(cues, 1):
            body = c["text"] if source_kind == "srt" else vtt_to_srt_text(c["text"])
            out.append(f"{n}\n{stamp(c['start_ms'], ',')} --> {stamp(c['end_ms'], ',')}\n{body}")
        return "\n\n".join(out) + "\n" if out else ""
    out = ["WEBVTT"]
    for c in cues:
        body = c["text"] if source_kind == "vtt" else srt_to_vtt_text(c["text"])
        head = f"{stamp(c['start_ms'], '.')} --> {stamp(c['end_ms'], '.')}"
        if source_kind == "vtt" and c["settings"]:
            head += " " + c["settings"]
        ident = c["id"] if source_kind == "vtt" and c["id"] else None
        out.append((ident + "\n" if ident else "") + head + "\n" + body)
    return "\n\n".join(out) + "\n"


def overlaps(cues):
    found = []
    ordered = sorted(range(len(cues)), key=lambda i: (cues[i]["start_ms"], cues[i]["end_ms"]))
    for a_pos, i in enumerate(ordered):
        for j in ordered[a_pos + 1:]:
            if cues[j]["start_ms"] >= cues[i]["end_ms"]:
                break
            found.append({"first": i + 1, "second": j + 1, "overlap_ms": min(cues[i]["end_ms"], cues[j]["end_ms"]) - cues[j]["start_ms"],
                          "first_end": stamp(cues[i]["end_ms"], ","), "second_start": stamp(cues[j]["start_ms"], ",")})
    return found


def shifted(cues, offset, warnings):
    out = []
    for n, c in enumerate(cues, 1):
        start, end = c["start_ms"] + offset, c["end_ms"] + offset
        if end <= 0:
            warnings.append(f"cue {n} would end before 0:00 and was dropped")
            continue
        if start < 0:
            warnings.append(f"cue {n} would start before 0:00 and was clamped to 0")
            start = 0
        out.append(dict(c, start_ms=start, end_ms=end))
    return out


def number(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number")
    try:
        f = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number")
    if f <= 0:
        fail(f"{field} must be positive")
    return f


def view(cues):
    return [{"index": n, "id": c["id"], "start": stamp(c["start_ms"], ","), "end": stamp(c["end_ms"], ","),
             "start_ms": c["start_ms"], "end_ms": c["end_ms"], "duration_ms": c["end_ms"] - c["start_ms"],
             **({"settings": c["settings"]} if c["settings"] else {}), "text": c["text"]} for n, c in enumerate(cues, 1)]


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"operation", "text", "text2", "offset_ms", "factor", "from_fps", "to_fps", "output"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    operation = request.get("operation", "parse")
    if operation not in ("parse", "shift", "scale", "convert", "merge", "overlaps"):
        fail("operation must be parse, shift, scale, convert, merge or overlaps")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be the subtitle file as a non-empty string")
    if len(text) > MAX_TEXT:
        fail(f"text may be at most {MAX_TEXT} characters")
    kind, cues, warnings = parse(text)
    if not cues:
        fail("no cues found (expected SRT or WebVTT)")
    output = request.get("output", ("vtt" if kind == "srt" else "srt") if operation == "convert" else kind)
    if output not in ("srt", "vtt"):
        fail("output must be srt or vtt")
    result = {"operation": operation, "input_format": kind}
    if operation == "shift":
        offset = request.get("offset_ms")
        if isinstance(offset, bool) or not isinstance(offset, int):
            fail("offset_ms must be an integer number of milliseconds")
        cues = shifted(cues, offset, warnings)
        result["offset_ms"] = offset
    elif operation == "scale":
        if "factor" in request:
            if "from_fps" in request or "to_fps" in request:
                fail("give factor or from_fps and to_fps, not both")
            factor = number(request["factor"], "factor")
        elif "from_fps" in request and "to_fps" in request:
            factor = number(request["from_fps"], "from_fps") / number(request["to_fps"], "to_fps")
        else:
            fail("scale needs factor, or from_fps and to_fps")
        cues = [dict(c, start_ms=int(c["start_ms"] * factor + Fraction(1, 2)), end_ms=int(c["end_ms"] * factor + Fraction(1, 2))) for c in cues]
        result["factor"] = str(factor)
        result["factor_decimal"] = float(factor)
    elif operation == "merge":
        text2 = request.get("text2")
        if not isinstance(text2, str) or not text2.strip():
            fail("merge needs text2")
        kind2, cues2, warnings2 = parse(text2)
        if kind2 != kind:
            fail("text and text2 must be the same format (convert one first)")
        offset = request.get("offset_ms", 0)
        if isinstance(offset, bool) or not isinstance(offset, int):
            fail("offset_ms must be an integer number of milliseconds")
        warnings += [f"text2: {w}" for w in warnings2]
        cues2 = shifted(cues2, offset, warnings)
        merged = [(c["start_ms"], 0, i, c) for i, c in enumerate(cues)] + [(c["start_ms"], 1, i, c) for i, c in enumerate(cues2)]
        merged.sort(key=lambda t: t[:3])
        cues = [t[3] for t in merged]
        result["sources"] = [t[1] + 1 for t in merged]
    found = overlaps(cues)
    result.update(count=len(cues), first_start=stamp(min(c["start_ms"] for c in cues), ",") if cues else None,
                  last_end=stamp(max(c["end_ms"] for c in cues), ",") if cues else None, overlap_count=len(found))
    if operation == "overlaps":
        result["overlaps"] = found
    else:
        result["cues"] = view(cues)
    if operation != "parse" and operation != "overlaps" or "output" in request:
        result["output_format"] = output
        result["subtitles"] = write(cues, output, kind)
    result["warnings"] = warnings
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
